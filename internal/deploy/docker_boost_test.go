package deploy

import "testing"

func TestDockerManager_SetResourceDefaults(t *testing.T) {
	dm := &DockerManager{}
	dm.SetResourceDefaults(1024, 512)
	if dm.defaultCPU != 1024 {
		t.Errorf("defaultCPU = %d, want 1024", dm.defaultCPU)
	}
	if dm.defaultMemMB != 512 {
		t.Errorf("defaultMemMB = %d, want 512", dm.defaultMemMB)
	}
}

func TestClampToInt64_Normal(t *testing.T) {
	if got := clampToInt64(42); got != 42 {
		t.Errorf("clampToInt64(42) = %d, want 42", got)
	}
}

func TestClampToInt64_Overflow(t *testing.T) {
	if got := clampToInt64(1<<64 - 1); got != (1<<63 - 1) {
		t.Errorf("clampToInt64(max uint64) = %d, want MaxInt64", got)
	}
}
