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
