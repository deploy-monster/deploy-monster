package build

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// BuildOpts holds options for a build.
type BuildOpts struct {
	AppID      string
	AppName    string
	SourceURL  string
	Branch     string
	CommitSHA  string
	Token      string // Git auth token
	Dockerfile string // Custom Dockerfile path (empty = auto-detect)
	EnvVars    map[string]string
	ImageTag   string // Target image tag
	Timeout    time.Duration
}

// BuildResult holds the result of a build.
type BuildResult struct {
	ImageTag  string        `json:"image_tag"`
	Type      ProjectType   `json:"type"`
	CommitSHA string        `json:"commit_sha"`
	Duration  time.Duration `json:"duration"`
	LogOutput string        `json:"log_output,omitempty"`
}

// Builder executes the full build pipeline: clone → detect → generate Dockerfile → docker build.
type Builder struct {
	runtime core.ContainerRuntime
	events  *core.EventBus
	workDir string
}

type redactingWriter struct {
	dst     io.Writer
	secrets []string
}

func (w redactingWriter) Write(p []byte) (int, error) {
	if len(w.secrets) == 0 {
		return w.dst.Write(p)
	}
	out := string(p)
	for _, secret := range w.secrets {
		if secret != "" {
			out = strings.ReplaceAll(out, secret, "[redacted]")
		}
	}
	_, err := w.dst.Write([]byte(out))
	return len(p), err
}

// NewBuilder creates a new builder.
func NewBuilder(runtime core.ContainerRuntime, events *core.EventBus) *Builder {
	return &Builder{
		runtime: runtime,
		events:  events,
		workDir: os.TempDir(),
	}
}

// Build runs the full build pipeline.
func (b *Builder) Build(ctx context.Context, opts BuildOpts, logWriter io.Writer) (result *BuildResult, err error) {
	// Turn any panic inside the pipeline into a structured error so a
	// bad git-clone argv, a misbehaving detector, or a surprise docker
	// output format can't crash the module. The recover is the last
	// defer and wraps `err` in the named return so the caller sees a
	// real error instead of a zero-value result.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("build: panic: %v", r)
			if b.events != nil {
				b.emitFailed(context.Background(), opts.AppID, err)
			}
		}
	}()

	start := time.Now()

	// Apply timeout
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Emit build started event
	_ = b.events.Publish(ctx, core.NewEvent(core.EventBuildStarted, "build",
		core.BuildEventData{AppID: opts.AppID, CommitSHA: opts.CommitSHA}))

	// 1. Create work directory
	buildDir := filepath.Join(b.workDir, "monster-build-"+core.GenerateID())
	if err := os.MkdirAll(buildDir, 0750); err != nil {
		return nil, fmt.Errorf("create build dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(buildDir) }()

	_, _ = fmt.Fprintf(logWriter, "==> Build started for %s\n", opts.AppName)

	// 2. Clone repository
	_, _ = fmt.Fprintf(logWriter, "==> Cloning %s (branch: %s)\n", redactURL(opts.SourceURL), opts.Branch)
	commitSHA, err := gitClone(ctx, opts.SourceURL, opts.Branch, opts.Token, buildDir, logWriter)
	if err != nil {
		b.emitFailed(ctx, opts.AppID, err)
		return nil, fmt.Errorf("git clone: %w", err)
	}
	if opts.CommitSHA == "" {
		opts.CommitSHA = commitSHA
	}

	// 3. Detect project type
	detected := Detect(buildDir)
	_, _ = fmt.Fprintf(logWriter, "==> Detected project type: %s (indicators: %v)\n", detected.Type, detected.Indicators)

	// 4. Generate Dockerfile if needed
	dockerfilePath := filepath.Join(buildDir, "Dockerfile")
	if opts.Dockerfile != "" {
		dockerfilePath = filepath.Join(buildDir, opts.Dockerfile)
	} else if detected.Type != TypeDockerfile && detected.Type != TypeUnknown {
		template := GetDockerfileTemplate(detected.Type)
		if template != "" {
			_, _ = fmt.Fprintf(logWriter, "==> Generating Dockerfile for %s\n", detected.Type)
			if err := os.WriteFile(dockerfilePath, []byte(template), 0644); err != nil {
				return nil, fmt.Errorf("write generated Dockerfile: %w", err)
			}
		}
	}

	if !exists(dockerfilePath) {
		b.emitFailed(ctx, opts.AppID, fmt.Errorf("no Dockerfile found"))
		return nil, fmt.Errorf("no Dockerfile found or generated for project type %s", detected.Type)
	}

	// 5. Docker build
	imageTag := opts.ImageTag
	if imageTag == "" {
		imageTag = fmt.Sprintf("monster/%s:%s", opts.AppName, opts.CommitSHA[:8])
	}

	_, _ = fmt.Fprintf(logWriter, "==> Building image %s\n", imageTag)
	if err := dockerBuild(ctx, buildDir, dockerfilePath, imageTag, opts.EnvVars, logWriter); err != nil {
		b.emitFailed(ctx, opts.AppID, err)
		return nil, fmt.Errorf("docker build: %w", err)
	}

	duration := time.Since(start)
	_, _ = fmt.Fprintf(logWriter, "==> Build completed in %s\n", duration.Round(time.Millisecond))

	// Emit build completed event
	_ = b.events.Publish(ctx, core.NewEvent(core.EventBuildCompleted, "build",
		core.BuildEventData{AppID: opts.AppID, CommitSHA: opts.CommitSHA, Duration: duration}))

	return &BuildResult{
		ImageTag:  imageTag,
		Type:      detected.Type,
		CommitSHA: opts.CommitSHA,
		Duration:  duration,
	}, nil
}

func (b *Builder) emitFailed(ctx context.Context, appID string, err error) {
	_ = b.events.Publish(ctx, core.NewEvent(core.EventBuildFailed, "build",
		core.BuildEventData{AppID: appID, Error: err.Error()}))
}

// shellMetaChars matches characters that could enable shell injection if a URL
// were ever interpolated into a shell command. exec.Command doesn't use a shell
// but we defend in depth. Backslash is excluded because Windows paths use it.
var shellMetaChars = regexp.MustCompile("[;|&$`!><(){}\\[\\]\n\r]")

// sshLikeURL matches git@host:org/repo patterns (valid SSH URLs).
var sshLikeURL = regexp.MustCompile(`^[\w.-]+@[\w.-]+:[\w./-]+$`)

// isPrivateOrBlockedIP returns true if the host is a private IP, loopback,
// link-local (cloud metadata), or multicast range.
func isPrivateOrBlockedIP(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	// Private ranges: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
	if ip.IsPrivate() {
		return true
	}
	// Loopback: 127.0.0.0/8
	if ip.IsLoopback() {
		return true
	}
	// Link-local: 169.254.0.0/16 (includes AWS/GCP/Azure cloud metadata 169.254.169.254)
	if ip.IsLinkLocalUnicast() {
		return true
	}
	// Unspecified: 0.0.0.0 (used in some internal network configs)
	if ip.IsUnspecified() {
		return true
	}
	return false
}

// dockerImageRef matches Docker-style image references (e.g., nginx:latest,
// registry.example.com/app:v1). These are not git URLs and should not be
// validated as such — they are accepted as-is for source_type=image deployments.
var dockerImageRef = regexp.MustCompile(`^[\w.\-/:]+$`)

// ValidateGitURL checks that a git repository URL is safe to pass to git clone.
// It rejects shell metacharacters, non-standard schemes, and private/internal IPs.
// Local absolute paths are rejected by default and only allowed when
// MONSTER_ALLOW_LOCAL_GIT_PATHS=true for explicit development use.
// Docker image references (e.g., nginx:latest) are accepted without git validation.
func ValidateGitURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("git URL is empty")
	}
	if shellMetaChars.MatchString(raw) {
		return fmt.Errorf("git URL contains disallowed characters")
	}
	if strings.HasPrefix(raw, "-") {
		return fmt.Errorf("git URL must not start with a dash")
	}

	// Local absolute path (Unix: /path, Windows: C:\path or C:/path)
	// git clone supports bare paths as local repos.
	if isAbsPath(raw) {
		if os.Getenv("MONSTER_ALLOW_LOCAL_GIT_PATHS") != "true" {
			return fmt.Errorf("local git paths are disabled (set MONSTER_ALLOW_LOCAL_GIT_PATHS=true for development)")
		}
		return nil
	}

	// Docker image references (source_type=image) — not git URLs, skip validation
	if dockerImageRef.MatchString(raw) && !strings.Contains(raw, "://") {
		return nil
	}

	// SSH shorthand: git@github.com:org/repo.git
	if sshLikeURL.MatchString(raw) {
		return nil
	}

	// Standard URL: https://, ssh://, git:// (file:// NOT allowed — SSRF risk)
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("git URL is malformed: %w", err)
	}
	switch parsed.Scheme {
	case "https", "ssh":
		// allowed
	case "git":
		return fmt.Errorf("git URL scheme %q is not allowed (unencrypted and can redirect to local protocols)", parsed.Scheme)
	case "http":
		return fmt.Errorf("git URL scheme %q is not allowed (use HTTPS)", parsed.Scheme)
	case "file":
		return fmt.Errorf("git URL scheme %q is not allowed (local file access is a security risk)", parsed.Scheme)
	default:
		return fmt.Errorf("git URL scheme %q is not allowed", parsed.Scheme)
	}
	if parsed.Host == "" {
		return fmt.Errorf("git URL has no host")
	}

	// Block private/internal IPs and cloud metadata endpoints
	if parsed.Host != "" && isPrivateOrBlockedIP(parsed.Host) {
		return fmt.Errorf("git URL host %q resolves to a private or blocked IP range", parsed.Host)
	}

	return nil
}

// validateResolvedHost performs a real-time DNS lookup and validates the
// resolved IP against the private/blocked ranges. This closes the DNS
// rebinding window where a URL validated at store time (clean DNS) could
// resolve to a private IP at clone time (TTL expiry or attack).
func validateResolvedHost(repoURL string) error {
	parsed, err := url.Parse(repoURL)
	if err != nil {
		return err
	}

	// Only check schemes that involve network access
	switch parsed.Scheme {
	case "https", "http", "ssh":
		// SSH shorthand (git@host:path) — can't resolve without DNS
		if parsed.Scheme == "ssh" && parsed.Host == "" {
			return nil
		}
		if parsed.Host == "" {
			return nil
		}

		// Resolve the hostname to IP
		addrs, err := net.LookupHost(parsed.Host)
		if err != nil {
			// DNS lookup failed — DNS rebinding attack in progress or legit DNS issue.
			// Fail closed: block the clone rather than allow potentially unsafe URL.
			return fmt.Errorf("git URL host %q DNS lookup failed (possible DNS rebinding attack)", parsed.Host)
		}

		// Check all resolved IPs against private/blocked ranges
		for _, addr := range addrs {
			ip := net.ParseIP(addr)
			if ip == nil {
				continue
			}
			if isPrivateOrBlockedIP(addr) {
				return fmt.Errorf("git URL host %q resolved to private/blocked IP %q", parsed.Host, addr)
			}
		}
		return nil
	}

	// file:// and local paths don't involve network — skip
	return nil
}

// isAbsPath checks if a string looks like an absolute filesystem path.
func isAbsPath(s string) bool {
	if strings.HasPrefix(s, "/") {
		return true
	}
	// Windows: C:\ or C:/
	if len(s) >= 3 && s[1] == ':' && (s[2] == '/' || s[2] == '\\') {
		return (s[0] >= 'A' && s[0] <= 'Z') || (s[0] >= 'a' && s[0] <= 'z')
	}
	return false
}

// gitClone clones a git repository with depth=1.
func gitClone(ctx context.Context, repoURL, branch, token, dir string, logWriter io.Writer) (string, error) {
	if err := ValidateGitURL(repoURL); err != nil {
		return "", fmt.Errorf("invalid git URL: %w", err)
	}

	// Re-validate at clone time: resolve DNS and check for private/blocked IPs.
	// This closes the DNS rebinding window where a URL validated at store time
	// (when DNS was clean) could resolve to a private IP at clone time.
	if err := validateResolvedHost(repoURL); err != nil {
		return "", fmt.Errorf("git URL resolved to blocked range: %w", err)
	}

	var env []string
	if token != "" {
		authEnv, cleanup, err := setupGitAskpass(dir, repoURL, token)
		if err != nil {
			return "", err
		}
		defer cleanup()
		env = authEnv
	}

	args := []string{"clone", "--depth=1", "-q"} // -q suppresses URL output that could leak token
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, repoURL, dir)

	cmd := exec.CommandContext(ctx, "git", args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	safeLogWriter := redactingWriter{dst: logWriter, secrets: []string{token}}
	cmd.Stdout = safeLogWriter
	cmd.Stderr = safeLogWriter

	if err := cmd.Run(); err != nil {
		return "", err
	}

	// Get commit SHA
	shaCmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD")
	out, err := shaCmd.Output()
	if err != nil {
		return "", nil // Non-fatal
	}

	sha := string(out)
	if len(sha) > 0 && sha[len(sha)-1] == '\n' {
		sha = sha[:len(sha)-1]
	}
	return sha, nil
}

func setupGitAskpass(dir, repoURL, token string) ([]string, func(), error) {
	parsed, err := url.Parse(repoURL)
	if err != nil {
		return nil, nil, fmt.Errorf("parse git URL for auth: %w", err)
	}
	if parsed.Scheme != "https" {
		return nil, nil, fmt.Errorf("git token authentication requires an HTTPS repository URL")
	}
	if parsed.Host == "" {
		return nil, nil, fmt.Errorf("git token authentication requires a repository URL host")
	}

	authDir, err := os.MkdirTemp(filepath.Dir(dir), ".monster-git-auth-")
	if err != nil {
		return nil, nil, fmt.Errorf("create git auth dir: %w", err)
	}

	tokenPath := filepath.Join(authDir, "token")
	if err := os.WriteFile(tokenPath, []byte(token+"\n"), 0600); err != nil {
		return nil, nil, fmt.Errorf("write git token file: %w", err)
	}

	askpassPath := filepath.Join(authDir, "askpass.sh")
	askpass := `#!/bin/sh
case "$1" in
*Username*|*username*) printf '%s\n' 'x-access-token' ;;
*Password*|*password*) cat "$MONSTER_GIT_TOKEN_FILE" ;;
*) exit 1 ;;
esac
`
	if err := os.WriteFile(askpassPath, []byte(askpass), 0700); err != nil {
		_ = os.RemoveAll(authDir)
		return nil, nil, fmt.Errorf("write git askpass helper: %w", err)
	}

	env := []string{
		"GIT_ASKPASS=" + askpassPath,
		"GIT_TERMINAL_PROMPT=0",
		"MONSTER_GIT_TOKEN_FILE=" + tokenPath,
	}
	return env, func() { _ = os.RemoveAll(authDir) }, nil
}

func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	u.User = url.UserPassword("redacted", "redacted")
	return u.String()
}

// dockerBuild runs `docker build` as a subprocess. --force-rm ensures
// intermediate layers are removed even if the build is aborted mid-run,
// so a canceled build (via Builder.Stop) doesn't leak dangling
// containers from failed stages.
// SECURITY: Validates build arg keys and values to prevent command injection.
// SECURITY: For stronger production isolation, consider using gVisor or
// Firecracker microVMs to sandbox the build container. Docker's default
// security profile can be hardened by dropping all capabilities and
// running with a read-only root filesystem:
//
//	docker build --cap-drop=ALL --read-only
func dockerBuild(ctx context.Context, contextDir, dockerfile, tag string, buildArgs map[string]string, logWriter io.Writer) error {
	// Validate image tag format
	if err := validateDockerImageTag(tag); err != nil {
		return fmt.Errorf("invalid image tag: %w", err)
	}

	args := []string{"build", "--force-rm", "-t", tag, "-f", dockerfile}

	for k, v := range buildArgs {
		// SECURITY FIX: Validate build arg key and value
		if err := validateBuildArg(k, v); err != nil {
			return fmt.Errorf("invalid build arg %q: %w", k, err)
		}
		args = append(args, "--build-arg", k+"="+v)
	}

	args = append(args, contextDir)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter

	return cmd.Run()
}

// validateDockerImageTag checks if an image tag is safe.
// Docker image tags must match: [registry/]name[:tag] with alphanumeric, dots, hyphens, underscores.
var validImageTagPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*(/[a-zA-Z0-9][a-zA-Z0-9._-]*)*(:[a-zA-Z0-9._-]+)?(@[a-zA-Z0-9:]+)?$`)

func validateDockerImageTag(tag string) error {
	if tag == "" {
		return fmt.Errorf("image tag is empty")
	}
	if !validImageTagPattern.MatchString(tag) {
		return fmt.Errorf("image tag contains invalid characters: %q", tag)
	}
	return nil
}

// validateBuildArg checks if a build arg key/value is safe.
// Prevents shell injection and flag injection attacks.
func validateBuildArg(key, value string) error {
	// Key validation: must be valid shell variable name format
	validKeyPattern := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	if !validKeyPattern.MatchString(key) {
		return fmt.Errorf("invalid key format (must match [a-zA-Z_][a-zA-Z0-9_]*)")
	}

	// Value validation: prevent control characters and injection attempts
	if strings.ContainsAny(value, "\x00\n\r") {
		return fmt.Errorf("value contains control characters")
	}
	// Prevent flag injection (values starting with -)
	if strings.HasPrefix(value, "-") {
		return fmt.Errorf("value cannot start with '-'")
	}
	return nil
}
