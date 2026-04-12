package build

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ErrBuilderClosed is returned by Build when the Builder has already
// been shut down via StopAll. Callers (pipelines, HTTP handlers) should
// treat this as a transient condition: the process is draining and the
// caller's parent module is about to return from its own Stop.
var ErrBuilderClosed = errors.New("build: builder is closed")

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

// inflightBuild is a live build registered in Builder.inflight. The
// token is a unique pointer address used to match the cleanup deferred
// by Build against the current map entry — so a concurrent second
// build for the same AppID cannot have its entry stomped when the
// first build finishes.
type inflightBuild struct {
	cancel context.CancelFunc
	token  *int
}

// Builder executes the full build pipeline: clone → detect → generate Dockerfile → docker build.
//
// Lifecycle notes for the Tier 73-77 hardening pass:
//
//   - Build used to be fire-and-go with a defer-less body. A panic
//     inside git-clone argv construction, detector filesystem walks,
//     or docker build output parsing would crash the whole process.
//     Build now runs inside a defer/recover that turns the panic into
//     an error and emits build.failed so the caller can react the same
//     way it does to any other build error.
//   - Stop(appID) only cancels a single build; Module.Stop needs to
//     cancel every in-flight build so the pool drain does not sit on
//     a docker build that could run for minutes. StopAll adds that,
//     plus a closed flag that rejects new Build calls once shutdown
//     has started (matching the WorkerPool contract in module.go).
//   - wg tracks every concurrent Build so Wait can drain them with a
//     ctx deadline, mirroring WorkerPool.Shutdown / ingress gateway
//     Shutdown / deploy manager Shutdown from the earlier hardening
//     tiers.
type Builder struct {
	runtime core.ContainerRuntime
	events  *core.EventBus
	workDir string

	// inflight tracks cancel funcs for running Builds by AppID so Stop
	// can abort a build started by a different caller (e.g. the user
	// clicking "cancel deploy" in the UI while a background worker owns
	// the original context).
	mu       sync.Mutex
	inflight map[string]inflightBuild

	// closed is set by StopAll and checked at Build entry so a builder
	// that is draining refuses new work cleanly instead of starting a
	// docker build that will be canceled a moment later.
	closed bool

	// wg is incremented for every accepted Build call so Wait can block
	// on a clean drain with a deadline from the module's shutdown ctx.
	wg sync.WaitGroup
}

// NewBuilder creates a new builder.
func NewBuilder(runtime core.ContainerRuntime, events *core.EventBus) *Builder {
	return &Builder{
		runtime:  runtime,
		events:   events,
		workDir:  os.TempDir(),
		inflight: make(map[string]inflightBuild),
	}
}

// registerInflight installs a cancel func for the given AppID and
// returns a cleanup closure the caller must defer. The cleanup uses a
// unique token to avoid clobbering a later concurrent Build's entry
// when an earlier Build's defer fires.
func (b *Builder) registerInflight(appID string, cancel context.CancelFunc) func() {
	if appID == "" {
		return func() {}
	}
	token := new(int) // address-unique per call
	b.mu.Lock()
	if b.inflight == nil {
		b.inflight = make(map[string]inflightBuild)
	}
	b.inflight[appID] = inflightBuild{cancel: cancel, token: token}
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if cur, ok := b.inflight[appID]; ok && cur.token == token {
			delete(b.inflight, appID)
		}
	}
}

// StopAll cancels every in-flight Build and marks the Builder as
func (b *Builder) Build(ctx context.Context, opts BuildOpts, logWriter io.Writer) (result *BuildResult, err error) {
	// Refuse new work once StopAll has run. Atomic with wg.Add so a
	// concurrent StopAll either observes the Builder is still open
	// (and the wg.Add happens-before Wait's drain) or rejects this
	// call outright — the same contract WorkerPool.SubmitCtx uses to
	// avoid racing wg.Add against wg.Wait.
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, ErrBuilderClosed
	}
	b.wg.Add(1)
	b.mu.Unlock()
	defer b.wg.Done()

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

	// Register the cancel func so Stop(appID) can abort this build
	// from another goroutine. Cleanup runs in LIFO order with the
	// cancel() defer above, so the map entry is removed before the
	// context is canceled on normal return — Stop will then fail
	// when the entry is missing.
	unregister := b.registerInflight(opts.AppID, cancel)
	defer unregister()

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

// ValidateGitURL checks that a git repository URL is safe to pass to git clone.
// It rejects shell metacharacters, non-standard schemes, and suspicious patterns.
// Local absolute paths (e.g. /home/user/repo) are allowed for development use.
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
		return nil
	}

	// SSH shorthand: git@github.com:org/repo.git
	if sshLikeURL.MatchString(raw) {
		return nil
	}

	// Standard URL: https://, http://, ssh://, git://, file://
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("git URL is malformed: %w", err)
	}
	switch parsed.Scheme {
	case "https", "http", "ssh", "git", "file":
		// allowed
	default:
		return fmt.Errorf("git URL scheme %q is not allowed", parsed.Scheme)
	}
	if parsed.Scheme != "file" && parsed.Host == "" {
		return fmt.Errorf("git URL has no host")
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

	args := []string{"clone", "--depth=1"}
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
