package swarm

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// RemoteExecutor implements core.NodeExecutor for remote agent nodes.
// Each method sends an AgentMessage to the agent and waits for the result.
type RemoteExecutor struct {
	conn   *AgentConn
	server *AgentServer
}

// Ensure RemoteExecutor implements NodeExecutor.
var _ core.NodeExecutor = (*RemoteExecutor)(nil)

func (r *RemoteExecutor) ServerID() string { return r.conn.ServerID }
func (r *RemoteExecutor) IsLocal() bool    { return false }

func (r *RemoteExecutor) CreateAndStart(ctx context.Context, opts core.ContainerOpts) (string, error) {
	resp, err := r.sendCommand(ctx, core.AgentMsgContainerCreate, opts)
	if err != nil {
		return "", err
	}
	containerID, err := decodePayload[string](resp.Payload)
	if err != nil {
		return "", fmt.Errorf("decode container ID: %w", err)
	}
	return containerID, nil
}

func (r *RemoteExecutor) Stop(ctx context.Context, containerID string, timeoutSec int) error {
	_, err := r.sendCommand(ctx, core.AgentMsgContainerStop, map[string]any{
		"container_id": containerID,
		"timeout_sec":  timeoutSec,
	})
	return err
}

func (r *RemoteExecutor) Remove(ctx context.Context, containerID string, force bool) error {
	_, err := r.sendCommand(ctx, core.AgentMsgContainerRemove, map[string]any{
		"container_id": containerID,
		"force":        force,
	})
	return err
}

func (r *RemoteExecutor) Restart(ctx context.Context, containerID string) error {
	_, err := r.sendCommand(ctx, core.AgentMsgContainerRestart, map[string]any{
		"container_id": containerID,
	})
	return err
}

func (r *RemoteExecutor) Logs(ctx context.Context, containerID string, tail string, follow bool) (io.ReadCloser, error) {
	if follow {
		return nil, fmt.Errorf("follow mode not supported over agent protocol")
	}

	resp, err := r.sendCommand(ctx, core.AgentMsgContainerLogs, map[string]any{
		"container_id": containerID,
		"tail":         tail,
	})
	if err != nil {
		return nil, err
	}

	logData, err := decodePayload[string](resp.Payload)
	if err != nil {
		return nil, fmt.Errorf("decode logs: %w", err)
	}

	return io.NopCloser(strings.NewReader(logData)), nil
}

func (r *RemoteExecutor) ListByLabels(ctx context.Context, labels map[string]string) ([]core.ContainerInfo, error) {
	resp, err := r.sendCommand(ctx, core.AgentMsgContainerList, map[string]any{
		"labels": labels,
	})
	if err != nil {
		return nil, err
	}

	containers, err := decodePayload[[]core.ContainerInfo](resp.Payload)
	if err != nil {
		return nil, fmt.Errorf("decode container list: %w", err)
	}
	return containers, nil
}

func (r *RemoteExecutor) Exec(ctx context.Context, command string) (string, error) {
	resp, err := r.sendCommand(ctx, core.AgentMsgContainerExec, map[string]any{
		"container_id": "",
		"cmd":          []string{"sh", "-c", command},
	})
	if err != nil {
		return "", err
	}

	output, err := decodePayload[string](resp.Payload)
	if err != nil {
		return "", fmt.Errorf("decode exec output: %w", err)
	}
	return output, nil
}

func (r *RemoteExecutor) Metrics(ctx context.Context) (*core.ServerMetrics, error) {
	resp, err := r.sendCommand(ctx, core.AgentMsgMetricsCollect, nil)
	if err != nil {
		return nil, err
	}

	metrics, err := decodePayload[core.ServerMetrics](resp.Payload)
	if err != nil {
		return nil, fmt.Errorf("decode metrics: %w", err)
	}
	return &metrics, nil
}

func (r *RemoteExecutor) Ping(ctx context.Context) error {
	_, err := r.sendCommand(ctx, core.AgentMsgPing, nil)
	return err
}

// sendCommand builds and sends an AgentMessage, then waits for the response.
func (r *RemoteExecutor) sendCommand(ctx context.Context, msgType string, payload any) (core.AgentMessage, error) {
	msg := core.AgentMessage{
		ID:        core.GenerateID(),
		Type:      msgType,
		ServerID:  r.conn.ServerID,
		Timestamp: time.Now(),
		Payload:   payload,
	}

	// Apply a default timeout if the context has none
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	return r.server.Send(ctx, r.conn, msg)
}
