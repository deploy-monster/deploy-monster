package topology

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// fakeDockerInPath creates a no-op docker executable in a temp directory
// and prepends that directory to PATH so verifyCompose/pullImages/composeUp
// can succeed without a real Docker installation.
func fakeDockerInPath(t *testing.T) func() {
	t.Helper()
	tmpDir := t.TempDir()

	var dockerPath string
	if runtime.GOOS == "windows" {
		dockerPath = filepath.Join(tmpDir, "docker.bat")
		if err := os.WriteFile(dockerPath, []byte("@echo off\r\nexit /b 0\r\n"), 0755); err != nil {
			t.Fatalf("write fake docker.bat: %v", err)
		}
	} else {
		dockerPath = filepath.Join(tmpDir, "docker")
		if err := os.WriteFile(dockerPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
			t.Fatalf("write fake docker: %v", err)
		}
	}

	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+string(filepath.ListSeparator)+oldPath)
	return func() { os.Setenv("PATH", oldPath) }
}

func TestDeployer_verifyCompose(t *testing.T) {
	cleanup := fakeDockerInPath(t)
	defer cleanup()

	d := NewDeployer(t.TempDir())
	// Write a minimal compose file so the path exists
	compose := minimalCompose()
	_ = os.WriteFile(d.composePath, []byte(compose.ToYAML()), 0640)

	if err := d.verifyCompose(context.Background()); err != nil {
		t.Fatalf("verifyCompose: %v", err)
	}
}

func TestDeployer_pullImages(t *testing.T) {
	cleanup := fakeDockerInPath(t)
	defer cleanup()

	d := NewDeployer(t.TempDir())
	compose := minimalCompose()
	_ = os.WriteFile(d.composePath, []byte(compose.ToYAML()), 0640)

	if err := d.pullImages(context.Background()); err != nil {
		t.Fatalf("pullImages: %v", err)
	}
}

func TestDeployer_composeUp(t *testing.T) {
	cleanup := fakeDockerInPath(t)
	defer cleanup()

	d := NewDeployer(t.TempDir())
	compose := minimalCompose()
	_ = os.WriteFile(d.composePath, []byte(compose.ToYAML()), 0640)

	out, err := d.composeUp(context.Background())
	if err != nil {
		t.Fatalf("composeUp: %v (output: %s)", err, out)
	}
}

func TestDeployer_verifyCompose_Error(t *testing.T) {
	// Remove docker from PATH so it cannot be found
	t.Setenv("PATH", t.TempDir())
	d := NewDeployer(t.TempDir())
	compose := minimalCompose()
	_ = os.WriteFile(d.composePath, []byte(compose.ToYAML()), 0640)

	err := d.verifyCompose(context.Background())
	if err == nil {
		t.Error("expected error when docker is not available")
	}
}

func TestDeployer_pullImages_Error(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	d := NewDeployer(t.TempDir())
	compose := minimalCompose()
	_ = os.WriteFile(d.composePath, []byte(compose.ToYAML()), 0640)

	err := d.pullImages(context.Background())
	if err == nil {
		t.Error("expected error when docker is not available")
	}
}

func TestDeployer_composeUp_Error(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	d := NewDeployer(t.TempDir())
	compose := minimalCompose()
	_ = os.WriteFile(d.composePath, []byte(compose.ToYAML()), 0640)

	_, err := d.composeUp(context.Background())
	if err == nil {
		t.Error("expected error when docker is not available")
	}
}
