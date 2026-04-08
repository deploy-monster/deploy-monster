package topology

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/build"
)

// Deployer handles the actual deployment of a topology
type Deployer struct {
	workDir     string
	composePath string
}

// NewDeployer creates a new deployer
func NewDeployer(workDir string) *Deployer {
	return &Deployer{
		workDir:     workDir,
		composePath: filepath.Join(workDir, "docker-compose.yaml"),
	}
}

// Deploy executes the deployment
func (d *Deployer) Deploy(ctx context.Context, compose *ComposeConfig, caddyfile, envFile string, dryRun bool) (*DeployResult, error) {
	start := time.Now()
	result := &DeployResult{
		Success: true,
	}

	// Create work directory
	if err := os.MkdirAll(d.workDir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create work directory: %w", err)
	}

	// Write compose file
	composeYAML := compose.ToYAML()
	result.ComposeYAML = composeYAML
	if err := os.WriteFile(d.composePath, []byte(composeYAML), 0640); err != nil {
		return nil, fmt.Errorf("failed to write compose file: %w", err)
	}

	// Write Caddyfile if we have domains
	if caddyfile != "" {
		result.Caddyfile = caddyfile
		caddyPath := filepath.Join(d.workDir, "Caddyfile")
		if err := os.WriteFile(caddyPath, []byte(caddyfile), 0640); err != nil {
			return nil, fmt.Errorf("failed to write Caddyfile: %w", err)
		}
	}

	// Write .env file
	if envFile != "" {
		result.EnvFile = envFile
		envPath := filepath.Join(d.workDir, ".env")
		if err := os.WriteFile(envPath, []byte(envFile), 0600); err != nil {
			return nil, fmt.Errorf("failed to write env file: %w", err)
		}
	}

	// If dry run, return here
	if dryRun {
		result.Message = "Dry run completed - files generated but not deployed"
		result.Duration = time.Since(start).String()
		return result, nil
	}

	// Verify compose file
	if err := d.verifyCompose(ctx); err != nil {
		result.Success = false
		result.Errors = append(result.Errors, DeployError{
			Stage:   "verify",
			Message: err.Error(),
		})
		return result, err
	}

	// Pull images first
	if err := d.pullImages(ctx); err != nil {
		// Non-fatal - build will happen if needed
		fmt.Printf("Warning: failed to pull images: %v\n", err)
	}

	// Deploy with compose
	if _, err := d.composeUp(ctx); err != nil {
		result.Success = false
		result.Errors = append(result.Errors, DeployError{
			Stage:   "deploy",
			Message: fmt.Sprintf("docker compose up failed: %v", err),
		})
		return result, err
	}

	// Collect deployed resources
	result.Containers = d.extractContainerNames(compose)
	result.Networks = d.extractNetworkNames(compose)
	result.Volumes = d.extractVolumeNames(compose)

	result.Message = "Deployment completed successfully"
	result.Duration = time.Since(start).String()

	return result, nil
}

// verifyCompose validates the compose file
func (d *Deployer) verifyCompose(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", d.composePath, "config", "--quiet")
	cmd.Dir = d.workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("compose validation failed: %s", string(output))
	}
	return nil
}

// pullImages pulls all required images
func (d *Deployer) pullImages(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", d.composePath, "pull", "--ignore-pull-failures")
	cmd.Dir = d.workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pull failed: %s", string(output))
	}
	return nil
}

// composeUp runs docker compose up
func (d *Deployer) composeUp(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", d.composePath, "up", "-d", "--remove-orphans")
	cmd.Dir = d.workDir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// ComposeDown stops and removes containers
func (d *Deployer) ComposeDown(ctx context.Context, removeVolumes bool) error {
	args := []string{"compose", "-f", d.composePath, "down"}
	if removeVolumes {
		args = append(args, "-v")
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = d.workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("compose down failed: %s", string(output))
	}
	return nil
}

// Logs returns logs from services
func (d *Deployer) Logs(ctx context.Context, service string, tail int) (string, error) {
	args := []string{"compose", "-f", d.composePath, "logs"}
	if service != "" {
		args = append(args, service)
	}
	if tail > 0 {
		args = append(args, "--tail", fmt.Sprintf("%d", tail))
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = d.workDir
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// PS returns status of running services
func (d *Deployer) PS(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", d.composePath, "ps")
	cmd.Dir = d.workDir
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// Helper functions

func (d *Deployer) extractContainerNames(compose *ComposeConfig) []string {
	var names []string
	for name, svc := range compose.Services {
		if svc.ContainerName != "" {
			names = append(names, svc.ContainerName)
		} else {
			names = append(names, name)
		}
	}
	return names
}

func (d *Deployer) extractNetworkNames(compose *ComposeConfig) []string {
	var names []string
	for name := range compose.Networks {
		names = append(names, name)
	}
	return names
}

func (d *Deployer) extractVolumeNames(compose *ComposeConfig) []string {
	var names []string
	for name := range compose.Volumes {
		names = append(names, name)
	}
	return names
}

// Build builds images for services with build configurations
func (d *Deployer) Build(ctx context.Context, compose *ComposeConfig, noCache bool) error {
	// Find services that need building
	var buildServices []string
	for name, svc := range compose.Services {
		if svc.Build != nil {
			buildServices = append(buildServices, name)
		}
	}

	if len(buildServices) == 0 {
		return nil
	}

	// Build each service
	for _, name := range buildServices {
		args := []string{"compose", "-f", d.composePath, "build"}
		if noCache {
			args = append(args, "--no-cache")
		}
		args = append(args, name)

		cmd := exec.CommandContext(ctx, "docker", args...)
		cmd.Dir = d.workDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("build %s failed: %s", name, string(output))
		}
	}

	return nil
}

// CloneGitRepo clones a git repository for building.
// The URL is validated to prevent command injection before being passed to git.
func CloneGitRepo(ctx context.Context, gitURL, branch, destDir string) error {
	if err := build.ValidateGitURL(gitURL); err != nil {
		return fmt.Errorf("invalid git URL: %w", err)
	}

	// Clone the repository
	args := []string{"clone", "--depth", "1", "--branch", branch, gitURL, destDir}
	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %s", string(output))
	}
	return nil
}

// DetectBuildPack auto-detects the build pack from repository contents
func DetectBuildPack(repoDir string) string {
	// Check for Dockerfile first
	if _, err := os.Stat(filepath.Join(repoDir, "Dockerfile")); err == nil {
		return "dockerfile"
	}

	// Check for package.json (Node.js)
	if _, err := os.Stat(filepath.Join(repoDir, "package.json")); err == nil {
		// Check for Next.js
		pkgJSON, _ := os.ReadFile(filepath.Join(repoDir, "package.json"))
		if strings.Contains(string(pkgJSON), `"next"`) {
			return "nextjs"
		}
		return "nodejs"
	}

	// Check for go.mod
	if _, err := os.Stat(filepath.Join(repoDir, "go.mod")); err == nil {
		return "go"
	}

	// Check for requirements.txt or pyproject.toml
	if _, err := os.Stat(filepath.Join(repoDir, "requirements.txt")); err == nil {
		return "python"
	}
	if _, err := os.Stat(filepath.Join(repoDir, "pyproject.toml")); err == nil {
		return "python"
	}

	// Check for Cargo.toml
	if _, err := os.Stat(filepath.Join(repoDir, "Cargo.toml")); err == nil {
		return "rust"
	}

	// Default to dockerfile
	return "dockerfile"
}
