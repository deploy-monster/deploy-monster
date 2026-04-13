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
	defer os.RemoveAll(buildDir)

	fmt.Fprintf(logWriter, "==> Build started for %s\n", opts.AppName)

	// 2. Clone repository
	fmt.Fprintf(logWriter, "==> Cloning %s (branch: %s)\n", opts.SourceURL, opts.Branch)
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
	fmt.Fprintf(logWriter, "==> Detected project type: %s (indicators: %v)\n", detected.Type, detected.Indicators)

	// 4. Generate Dockerfile if needed
	dockerfilePath := filepath.Join(buildDir, "Dockerfile")
	if opts.Dockerfile != "" {
		dockerfilePath = filepath.Join(buildDir, opts.Dockerfile)
	} else if detected.Type != TypeDockerfile && detected.Type != TypeUnknown {
		template := GetDockerfileTemplate(detected.Type)
		if template != "" {
			fmt.Fprintf(logWriter, "==> Generating Dockerfile for %s\n", detected.Type)
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

	fmt.Fprintf(logWriter, "==> Building image %s\n", imageTag)
	if err := dockerBuild(ctx, buildDir, dockerfilePath, imageTag, opts.EnvVars, logWriter); err != nil {
		b.emitFailed(ctx, opts.AppID, err)
		return nil, fmt.Errorf("docker build: %w", err)
	}

	duration := time.Since(start)
	fmt.Fprintf(logWriter, "==> Build completed in %s\n", duration.Round(time.Millisecond))

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
// Local absolute paths (e.g. /home/user/repo) are allowed for development use.
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

	// Docker image references (source_type=image) — not git URLs, skip validation
	if dockerImageRef.MatchString(raw) && !strings.Contains(raw, "://") {
		return nil
	}

	// Local absolute path (Unix: /path, Windows: C:\path or C:/path)
	// git clone supports bare paths as local repos.
	if isAbsPath(raw) {
		return nil
	}

	// SSH shorthand: git@github.com:org/repo.git
	if sshLikeURL.MatchString(raw) {
		return nil
	}

	// Standard URL: https://, ssh://, git://, file:// (http:// is NOT allowed — SSRF risk)
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("git URL is malformed: %w", err)
	}
	switch parsed.Scheme {
	case "https", "ssh", "git", "file":
		// allowed
	case "http":
		return fmt.Errorf("git URL scheme %q is not allowed (use HTTPS)", parsed.Scheme)
	default:
		return fmt.Errorf("git URL scheme %q is not allowed", parsed.Scheme)
	}
	if parsed.Scheme != "file" && parsed.Host == "" {
		return fmt.Errorf("git URL has no host")
	}

	// Block private/internal IPs and cloud metadata endpoints
	if parsed.Host != "" && isPrivateOrBlockedIP(parsed.Host) {
		return fmt.Errorf("git URL host %q resolves to a private or blocked IP range", parsed.Host)
	}

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

	// Inject token into HTTPS URL if provided
	if token != "" {
		repoURL = injectToken(repoURL, token)
	}

	args := []string{"clone", "--depth=1", "-q"} // -q suppresses URL output that could leak token
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, repoURL, dir)

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter

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

// injectToken adds an auth token to an HTTPS git URL.
func injectToken(gitURL, token string) string {
	if len(gitURL) > 8 && gitURL[:8] == "https://" {
		return "https://" + token + "@" + gitURL[8:]
	}
	return gitURL
}

// dockerBuild runs `docker build` as a subprocess. --force-rm ensures
// intermediate layers are removed even if the build is aborted mid-run,
// so a canceled build (via Builder.Stop) doesn't leak dangling
// containers from failed stages.
func dockerBuild(ctx context.Context, contextDir, dockerfile, tag string, buildArgs map[string]string, logWriter io.Writer) error {
	args := []string{"build", "--force-rm", "-t", tag, "-f", dockerfile}

	for k, v := range buildArgs {
		args = append(args, "--build-arg", k+"="+v)
	}

	args = append(args, contextDir)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter

	return cmd.Run()
}
