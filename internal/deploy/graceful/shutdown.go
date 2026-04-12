package graceful

import (
	"context"
	"fmt"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// DefaultStopGracePeriodSeconds is the SIGTERM grace window applied
// when neither the app nor the strategy overrides it. Matches the
// Docker default of 10 seconds.
const DefaultStopGracePeriodSeconds = 10

// Shutdown stops and removes a container using a configurable
// SIGTERM grace period. The runtime's Stop call is expected to issue
// SIGTERM, wait up to graceSeconds for a clean exit, then SIGKILL.
//
// Pre-Tier-Phase-2 the deploy strategies called runtime.Stop with
// either a hardcoded 5-second timeout or whatever DrainTimeout was
// configured globally, with no per-app override. This helper lets the
// caller express "honor this app's configured grace period" without
// duplicating the boilerplate in every strategy.
//
// Errors from Stop are logged to the returned error but do not block
// the Remove call — a container that's already gone is still gone.
func Shutdown(ctx context.Context, rt core.ContainerRuntime, containerID string, graceSeconds int) error {
	if rt == nil || containerID == "" {
		return nil
	}
	if graceSeconds <= 0 {
		graceSeconds = DefaultStopGracePeriodSeconds
	}

	stopErr := rt.Stop(ctx, containerID, graceSeconds)
	removeErr := rt.Remove(ctx, containerID, true)

	switch {
	case stopErr != nil && removeErr != nil:
		return fmt.Errorf("stop: %w (remove also failed: %v)", stopErr, removeErr)
	case removeErr != nil:
		return fmt.Errorf("remove after stop: %w", removeErr)
	default:
		// Stop errors are intentionally non-fatal: a container may
		// already be stopped, exited, or removed by a racing watchdog.
		return nil
	}
}
