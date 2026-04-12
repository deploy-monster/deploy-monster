package graceful

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// fakeRuntime records Stop/Remove calls for assertion.
type fakeRuntime struct {
	stopCalled      bool
	stopContainerID string
	stopTimeout     int
	stopErr         error

	removeCalled      bool
	removeContainerID string
	removeForce       bool
	removeErr         error
}

func (f *fakeRuntime) Stop(_ context.Context, id string, timeoutSec int) error {
	f.stopCalled = true
	f.stopContainerID = id
	f.stopTimeout = timeoutSec
	return f.stopErr
}

func (f *fakeRuntime) Remove(_ context.Context, id string, force bool) error {
	f.removeCalled = true
	f.removeContainerID = id
	f.removeForce = force
	return f.removeErr
}

// All remaining core.ContainerRuntime methods are no-ops / zero values.
func (f *fakeRuntime) Ping() error { return nil }
func (f *fakeRuntime) CreateAndStart(context.Context, core.ContainerOpts) (string, error) {
	return "", nil
}
func (f *fakeRuntime) Restart(context.Context, string) error { return nil }
func (f *fakeRuntime) Logs(context.Context, string, string, bool) (io.ReadCloser, error) {
	return nil, nil
}
func (f *fakeRuntime) ListByLabels(context.Context, map[string]string) ([]core.ContainerInfo, error) {
	return nil, nil
}
func (f *fakeRuntime) Exec(context.Context, string, []string) (string, error) { return "", nil }
func (f *fakeRuntime) Stats(context.Context, string) (*core.ContainerStats, error) {
	return nil, nil
}
func (f *fakeRuntime) ImagePull(context.Context, string) error             { return nil }
func (f *fakeRuntime) ImageList(context.Context) ([]core.ImageInfo, error) { return nil, nil }
func (f *fakeRuntime) ImageRemove(context.Context, string) error           { return nil }
func (f *fakeRuntime) NetworkList(context.Context) ([]core.NetworkInfo, error) {
	return nil, nil
}
func (f *fakeRuntime) VolumeList(context.Context) ([]core.VolumeInfo, error) { return nil, nil }

func TestShutdown_NilRuntime(t *testing.T) {
	// Defensive: nil runtime must be a no-op, not a panic.
	if err := Shutdown(context.Background(), nil, "abc", 5); err != nil {
		t.Errorf("Shutdown(nil rt) = %v, want nil", err)
	}
}

func TestShutdown_EmptyID(t *testing.T) {
	rt := &fakeRuntime{}
	if err := Shutdown(context.Background(), rt, "", 5); err != nil {
		t.Errorf("Shutdown(empty id) = %v, want nil", err)
	}
	if rt.stopCalled || rt.removeCalled {
		t.Error("Shutdown with empty id should not touch the runtime")
	}
}

func TestShutdown_UsesProvidedGracePeriod(t *testing.T) {
	rt := &fakeRuntime{}
	if err := Shutdown(context.Background(), rt, "abc", 42); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if rt.stopTimeout != 42 {
		t.Errorf("stopTimeout = %d, want 42", rt.stopTimeout)
	}
	if !rt.removeCalled || !rt.removeForce {
		t.Error("Remove must be called with force=true after Stop")
	}
}

func TestShutdown_DefaultsWhenNonPositive(t *testing.T) {
	rt := &fakeRuntime{}
	_ = Shutdown(context.Background(), rt, "abc", 0)
	if rt.stopTimeout != DefaultStopGracePeriodSeconds {
		t.Errorf("stopTimeout = %d, want default %d",
			rt.stopTimeout, DefaultStopGracePeriodSeconds)
	}
}

func TestShutdown_StopErrorNonFatal(t *testing.T) {
	// Docker Stop often errors for already-stopped containers. That
	// must not block the Remove call.
	rt := &fakeRuntime{stopErr: errors.New("already stopped")}
	if err := Shutdown(context.Background(), rt, "abc", 5); err != nil {
		t.Errorf("Shutdown with non-fatal stop error = %v, want nil", err)
	}
	if !rt.removeCalled {
		t.Error("Remove must still be called when Stop errors")
	}
}

func TestShutdown_BothErrorsReported(t *testing.T) {
	rt := &fakeRuntime{
		stopErr:   errors.New("stop failed"),
		removeErr: errors.New("remove failed"),
	}
	err := Shutdown(context.Background(), rt, "abc", 5)
	if err == nil {
		t.Fatal("Shutdown with both errors = nil, want error")
	}
}

