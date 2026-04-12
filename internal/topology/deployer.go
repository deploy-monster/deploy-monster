package topology

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"
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
		slog.Warn("failed to pull images, will attempt build", "error", err)
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
