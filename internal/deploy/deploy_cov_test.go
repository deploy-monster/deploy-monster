package deploy

import (
	"testing"
)

// =============================================================================
// autorestart.go:32 — defaultAutoRestartRetryDelay (0%)
// =============================================================================

func TestDefaultAutoRestartRetryDelay(t *testing.T) {
	d := defaultAutoRestartRetryDelay(1)
	if d <= 0 {
		t.Errorf("expected positive duration, got %v", d)
	}
	d2 := defaultAutoRestartRetryDelay(3)
	if d2 <= d {
		t.Errorf("expected retry delay to increase with attempt, got %v vs %v", d2, d)
	}
}

// =============================================================================
// module.go:254 — shortContainerID (66.7%)
// =============================================================================

func TestShortContainerID_ShortID(t *testing.T) {
	id := shortContainerID("abc123")
	if id != "abc123" {
		t.Errorf("shortContainerID('abc123') = %q, want 'abc123'", id)
	}
}

func TestShortContainerID_LongID(t *testing.T) {
	id := shortContainerID("abcdef1234567890")
	if id != "abcdef123456" {
		t.Errorf("shortContainerID(long) = %q, want 'abcdef123456'", id)
	}
}

// =============================================================================
// docker.go:62 — SetRegistryAuth uncovered branches
// =============================================================================

func TestSetRegistryAuth_OneEmpty(t *testing.T) {
	d := &DockerManager{}
	err := d.SetRegistryAuth("user", "")
	if err == nil {
		t.Fatal("expected error when password is empty")
	}
}

func TestSetRegistryAuth_OtherEmpty(t *testing.T) {
	d := &DockerManager{}
	err := d.SetRegistryAuth("", "pass")
	if err == nil {
		t.Fatal("expected error when username is empty")
	}
}

func TestSetRegistryAuth_AllEmpty(t *testing.T) {
	d := &DockerManager{}
	err := d.SetRegistryAuth("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.registryAuth != "" {
		t.Error("expected registryAuth to be empty")
	}
}

func TestSetRegistryAuth_BothSet(t *testing.T) {
	d := &DockerManager{}
	err := d.SetRegistryAuth("myuser", "mypass")
	if err != nil {
		t.Fatalf("SetRegistryAuth: %v", err)
	}
	if d.registryAuth == "" {
		t.Error("expected registryAuth to be set")
	}
}
