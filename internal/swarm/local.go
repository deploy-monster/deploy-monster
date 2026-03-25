package swarm

import (
	"context"
	"io"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// LocalExecutor implements core.NodeExecutor for the master's local Docker.
// It delegates directly to the ContainerRuntime without any network hop.
type LocalExecutor struct {
	runtime  core.ContainerRuntime
	serverID string
}

// Ensure LocalExecutor implements NodeExecutor.
var _ core.NodeExecutor = (*LocalExecutor)(nil)

// NewLocalExecutor creates a local executor wrapping the master's container runtime.
func NewLocalExecutor(runtime core.ContainerRuntime, serverID string) *LocalExecutor {
	return &LocalExecutor{runtime: runtime, serverID: serverID}
}

func (l *LocalExecutor) ServerID() string { return l.serverID }
func (l *LocalExecutor) IsLocal() bool    { return true }

func (l *LocalExecutor) CreateAndStart(ctx context.Context, opts core.ContainerOpts) (string, error) {
	return l.runtime.CreateAndStart(ctx, opts)
}

func (l *LocalExecutor) Stop(ctx context.Context, containerID string, timeoutSec int) error {
	return l.runtime.Stop(ctx, containerID, timeoutSec)
}

func (l *LocalExecutor) Remove(ctx context.Context, containerID string, force bool) error {
	return l.runtime.Remove(ctx, containerID, force)
}

func (l *LocalExecutor) Restart(ctx context.Context, containerID string) error {
	return l.runtime.Restart(ctx, containerID)
}

func (l *LocalExecutor) Logs(ctx context.Context, containerID string, tail string, follow bool) (io.ReadCloser, error) {
	return l.runtime.Logs(ctx, containerID, tail, follow)
}

func (l *LocalExecutor) ListByLabels(ctx context.Context, labels map[string]string) ([]core.ContainerInfo, error) {
	return l.runtime.ListByLabels(ctx, labels)
}

func (l *LocalExecutor) Exec(ctx context.Context, command string) (string, error) {
	// For local execution, run through the container runtime exec
	// using a utility container or directly via the host shell.
	// For now, delegate to the runtime's exec with a shell wrapper.
	return l.runtime.Exec(ctx, "", []string{"sh", "-c", command})
}

func (l *LocalExecutor) Metrics(_ context.Context) (*core.ServerMetrics, error) {
	// Collect local server metrics
	return &core.ServerMetrics{
		ServerID: l.serverID,
	}, nil
}

func (l *LocalExecutor) Ping(_ context.Context) error {
	return l.runtime.Ping()
}
