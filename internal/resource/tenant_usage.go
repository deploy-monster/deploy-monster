package resource

import (
	"context"
	"errors"
	"fmt"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// TenantUsage is the live resource footprint of a single tenant as
// observed from the container runtime. It is populated by
// AggregateTenantUsage, which walks every running container carrying
// a monster.tenant label and groups their Docker Stats by tenant ID.
//
// This is the quota-enforcement side of the resource module: the
// existing collector persists per-container rings for the dashboard,
// but pre-3.3.6 nothing in the codebase would tell you "how much RAM
// is tenant X currently consuming?" — so the deploy pipeline had no
// way to refuse an N+1st container that would blow through a plan
// limit. AggregateTenantUsage gives the deploy gate a single call it
// can make at pre-flight time.
type TenantUsage struct {
	TenantID   string
	Apps       int
	Containers int
	CPUPercent float64
	MemoryMB   int64
}

// TenantQuota is the per-tenant ceiling used by CheckTenantQuota.
// A non-positive value in any field means "unlimited" for that
// dimension — matching the convention the billing Plan struct already
// uses for enterprise plans.
type TenantQuota struct {
	MaxApps       int
	MaxContainers int
	MaxMemoryMB   int64
	MaxCPUPercent float64
}

// ErrQuotaExceeded is the sentinel returned by CheckTenantQuota when
// any dimension of a TenantUsage exceeds the matching TenantQuota
// ceiling. Callers in the deploy pipeline use errors.Is to
// distinguish a quota refusal (4xx-worthy) from an infrastructure
// failure (5xx-worthy).
var ErrQuotaExceeded = errors.New("tenant resource quota exceeded")

// AggregateTenantUsage walks every running container tagged with the
// monster.enable label, queries live Docker Stats, and groups the
// result by the monster.tenant label. Containers missing a tenant
// label are ignored — they are either system containers or a
// misconfigured deploy that should be caught elsewhere.
//
// A nil runtime is treated as "no containers running", which is the
// correct answer for a master running in dev mode with no Docker
// socket. Callers can still feed the returned (empty) map into
// CheckTenantQuota for a consistent gate.
func AggregateTenantUsage(ctx context.Context, runtime core.ContainerRuntime) (map[string]*TenantUsage, error) {
	if runtime == nil {
		return map[string]*TenantUsage{}, nil
	}

	containers, err := runtime.ListByLabels(ctx, map[string]string{
		"monster.enable": "true",
	})
	if err != nil {
		return nil, fmt.Errorf("list tenant containers: %w", err)
	}

	usage := make(map[string]*TenantUsage)
	seenApps := make(map[string]map[string]struct{}) // tenant → appID set

	for _, ctr := range containers {
		if ctr.State != "running" {
			continue
		}
		tenantID := ctr.Labels["monster.tenant"]
		if tenantID == "" {
			continue
		}

		tu, ok := usage[tenantID]
		if !ok {
			tu = &TenantUsage{TenantID: tenantID}
			usage[tenantID] = tu
			seenApps[tenantID] = make(map[string]struct{})
		}

		tu.Containers++
		if appID := ctr.Labels["monster.app.id"]; appID != "" {
			if _, dup := seenApps[tenantID][appID]; !dup {
				seenApps[tenantID][appID] = struct{}{}
				tu.Apps++
			}
		}

		stats, err := runtime.Stats(ctx, ctr.ID)
		if err != nil || stats == nil {
			// A single bad container should not void the whole tenant's
			// snapshot. The most defensive answer is "count it but
			// attribute zero live usage" — missing stats will under-
			// report, not over-report, which is the fail-safe direction
			// for a quota gate (tenant may sneak past but never gets
			// refused incorrectly).
			continue
		}
		tu.CPUPercent += stats.CPUPercent
		tu.MemoryMB += stats.MemoryUsage / (1024 * 1024)
	}

	return usage, nil
}

// CheckTenantQuota returns ErrQuotaExceeded wrapped with the first
// dimension that is over the limit. Dimensions with non-positive
// limits are skipped (treated as unlimited). A nil usage is treated
// as "zero across the board" and always passes.
func CheckTenantQuota(usage *TenantUsage, quota TenantQuota) error {
	if usage == nil {
		return nil
	}
	if quota.MaxApps > 0 && usage.Apps > quota.MaxApps {
		return fmt.Errorf("%w: apps %d > %d", ErrQuotaExceeded, usage.Apps, quota.MaxApps)
	}
	if quota.MaxContainers > 0 && usage.Containers > quota.MaxContainers {
		return fmt.Errorf("%w: containers %d > %d", ErrQuotaExceeded, usage.Containers, quota.MaxContainers)
	}
	if quota.MaxMemoryMB > 0 && usage.MemoryMB > quota.MaxMemoryMB {
		return fmt.Errorf("%w: memory %dMB > %dMB", ErrQuotaExceeded, usage.MemoryMB, quota.MaxMemoryMB)
	}
	if quota.MaxCPUPercent > 0 && usage.CPUPercent > quota.MaxCPUPercent {
		return fmt.Errorf("%w: cpu %.1f%% > %.1f%%", ErrQuotaExceeded, usage.CPUPercent, quota.MaxCPUPercent)
	}
	return nil
}
