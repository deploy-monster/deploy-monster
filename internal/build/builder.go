package build

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
func (b *Builder) Build(ctx context.Context, opts BuildOpts, logWriter io.Writer) (*BuildResult, error) {
	start := time.Now()

	// Apply timeout
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Emit build started event
	b.events.Publish(ctx, core.NewEvent(core.EventBuildStarted, "build",
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
			os.WriteFile(dockerfilePath, []byte(template), 0644)
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
	b.events.Publish(ctx, core.NewEvent(core.EventBuildCompleted, "build",
		core.BuildEventData{AppID: opts.AppID, CommitSHA: opts.CommitSHA, Duration: duration}))

	return &BuildResult{
		ImageTag:  imageTag,
		Type:      detected.Type,
		CommitSHA: opts.CommitSHA,
		Duration:  duration,
	}, nil
}

func (b *Builder) emitFailed(ctx context.Context, appID string, err error) {
	b.events.Publish(ctx, core.NewEvent(core.EventBuildFailed, "build",
		core.BuildEventData{AppID: appID, Error: err.Error()}))
}

// gitClone clones a git repository with depth=1.
func gitClone(ctx context.Context, url, branch, token, dir string, logWriter io.Writer) (string, error) {
	// Inject token into HTTPS URL if provided
	if token != "" {
		url = injectToken(url, token)
	}

	args := []string{"clone", "--depth=1"}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, url, dir)

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
func injectToken(url, token string) string {
	if len(url) > 8 && url[:8] == "https://" {
		return "https://" + token + "@" + url[8:]
	}
	return url
}

// dockerBuild runs `docker build` as a subprocess.
func dockerBuild(ctx context.Context, contextDir, dockerfile, tag string, buildArgs map[string]string, logWriter io.Writer) error {
	args := []string{"build", "-t", tag, "-f", dockerfile}

	for k, v := range buildArgs {
		args = append(args, "--build-arg", k+"="+v)
	}

	args = append(args, contextDir)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter

	return cmd.Run()
}
