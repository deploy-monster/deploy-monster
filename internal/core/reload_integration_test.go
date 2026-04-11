package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// writeYAML is a tiny helper so test bodies stay short — each case
// only needs to vary a handful of fields.
func writeYAML(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// simulatedDeploy mimics the Config-read pattern of a deploy pipeline
// in flight: every iteration snapshots a handful of fields that a
// real Build/Deploy step would inspect (worker-pool caps, CORS origins,
// registration gating, log level, etc.). The test spawns several of
// these alongside ReloadConfig callers to drive the concurrent path.
//
// Returns the number of iterations the goroutine completed before
// ctx was cancelled. Any visible corruption (nil-pointer panic, a
// reload mid-field that leaves a struct half-mutated) would surface
// as a recover() catching a panic; the test asserts the recovered
// count stays at zero.
func simulatedDeploy(ctx context.Context, c *Core, panics *atomic.Int64) int {
	iterations := 0
	defer func() {
		if r := recover(); r != nil {
			panics.Add(1)
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return iterations
		default:
		}
		// Read every field ReloadConfig is allowed to mutate. We
		// intentionally consume each value so the compiler cannot
		// hoist the read out of the loop.
		_ = c.Config.Server.LogLevel
		_ = c.Config.Server.LogFormat
		_ = c.Config.Server.CORSOrigins
		_ = c.Config.Registration.Mode
		_ = c.Config.Backup.Schedule
		_ = c.Config.Limits.MaxAppsPerTenant
		_ = c.Config.Limits.MaxConcurrentBuilds
		iterations++
		// A brief yield so the scheduler actually interleaves this
		// loop with the reloader goroutine; without a yield the Go
		// runtime tends to let one goroutine run to completion on a
		// lightly loaded machine.
		if iterations%100 == 0 {
			time.Sleep(time.Microsecond)
		}
	}
}

// TestReloadConfig_ConcurrentWithInFlightDeploy exercises the
// Roadmap 3.2.4 scenario: SIGHUP / ReloadConfig is fired while a
// deploy is already reading Config fields, and the deploy has to
// keep running without corruption.
//
// The test cannot assert "zero races" without the race detector
// (which requires cgo on Windows) — what it CAN assert is:
//
//  1. No reader panics. A torn read that happened to land on a nil
//     pointer or out-of-range index would recover() here.
//  2. After every reload completes, Core.Config matches the file
//     that was last written. No stale snapshot survives.
//  3. The EventConfigReloaded event fires once per applied reload.
//  4. The concurrent readers together complete far more iterations
//     than the number of reloads, proving the reload path did not
//     block the deploy pipeline.
func TestReloadConfig_ConcurrentWithInFlightDeploy(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "monster.yaml")

	// Initial config: low traffic settings.
	writeYAML(t, yamlPath, `server:
  port: 8443
  host: 0.0.0.0
  log_level: info
  log_format: text
  cors_origins: "https://app.example.com"
registration:
  mode: open
backup:
  schedule: "02:00"
limits:
  max_apps_per_tenant: 50
  max_concurrent_builds: 3
`)

	c := &Core{
		Config: &Config{},
		Logger: discardLogger(),
		Events: NewEventBus(discardLogger()),
	}
	applyDefaults(c.Config)
	// Seed the in-memory config with the values from the YAML so
	// the first ReloadConfig sees a non-trivial delta.
	c.Config.Server.LogLevel = "info"
	c.Config.Server.LogFormat = "text"
	c.Config.Server.CORSOrigins = "https://app.example.com"
	c.Config.Registration.Mode = "open"
	c.Config.Backup.Schedule = "02:00"
	c.Config.Limits.MaxAppsPerTenant = 50
	c.Config.Limits.MaxConcurrentBuilds = 3
	c.ConfigPath = yamlPath

	// Count EventConfigReloaded so the test can assert the event
	// fired exactly once per applied reload. Sync subscriber — the
	// ReloadConfig caller uses PublishAsync which spawns a goroutine,
	// so we wait for Events.Drain at the end.
	var reloadEvents atomic.Int64
	c.Events.Subscribe(EventConfigReloaded, func(_ context.Context, _ Event) error {
		reloadEvents.Add(1)
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	var panics atomic.Int64
	var readerWG sync.WaitGroup
	var totalIters atomic.Int64

	// Launch 5 concurrent "in-flight deploys". Five is enough to get
	// real interleaving without being flaky on slow CI.
	for i := 0; i < 5; i++ {
		readerWG.Add(1)
		go func() {
			defer readerWG.Done()
			totalIters.Add(int64(simulatedDeploy(ctx, c, &panics)))
		}()
	}

	// Run a series of reloads while the deploys are churning. Each
	// iteration rewrites the YAML with a different value so
	// ReloadConfig is guaranteed to see a delta.
	const reloadRounds = 10
	for i := 0; i < reloadRounds; i++ {
		maxBuilds := 4 + i
		writeYAML(t, yamlPath, fmt.Sprintf(`server:
  port: 8443
  host: 0.0.0.0
  log_level: debug
  log_format: json
  cors_origins: "https://app-%d.example.com"
registration:
  mode: invite_only
backup:
  schedule: "03:%02d"
limits:
  max_apps_per_tenant: %d
  max_concurrent_builds: %d
`, i, i, 100+i, maxBuilds))
		if err := c.ReloadConfig(); err != nil {
			t.Fatalf("reload %d: %v", i, err)
		}
		// Yield so the readers observe the new state before the next
		// rewrite.
		time.Sleep(2 * time.Millisecond)
	}

	cancel()
	readerWG.Wait()
	c.Events.Drain()

	if panics.Load() != 0 {
		t.Errorf("simulatedDeploy panicked %d times — torn read from ReloadConfig",
			panics.Load())
	}

	// Sanity: readers did real work. If the reload path had somehow
	// deadlocked the deploy goroutines, totalIters would be close to
	// zero.
	if totalIters.Load() < int64(reloadRounds*100) {
		t.Errorf("simulatedDeploy iterations = %d, want >> %d (reload should not block readers)",
			totalIters.Load(), reloadRounds*100)
	}

	// Final state: the newest YAML should be the active config.
	wantMaxBuilds := 4 + (reloadRounds - 1)
	if c.Config.Limits.MaxConcurrentBuilds != wantMaxBuilds {
		t.Errorf("final MaxConcurrentBuilds = %d, want %d (last reload not applied)",
			c.Config.Limits.MaxConcurrentBuilds, wantMaxBuilds)
	}
	if c.Config.Server.LogLevel != "debug" {
		t.Errorf("final LogLevel = %q, want %q", c.Config.Server.LogLevel, "debug")
	}
	if c.Config.Registration.Mode != "invite_only" {
		t.Errorf("final Registration.Mode = %q, want %q",
			c.Config.Registration.Mode, "invite_only")
	}

	// Event count: one ConfigReloaded event per reload round since
	// every round mutates at least one field.
	if got := reloadEvents.Load(); got != int64(reloadRounds) {
		t.Errorf("EventConfigReloaded fired %d times, want %d", got, reloadRounds)
	}
}

// TestReloadConfig_NoChangesSkipsEvent verifies the "no changes"
// fast path does not publish a reload event. This is the correctness
// check the SIGHUP handler in main.go relies on to avoid spamming
// EventConfigReloaded when an operator SIGHUPs a config file they
// haven't actually edited.
func TestReloadConfig_NoChangesSkipsEvent(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "monster.yaml")
	writeYAML(t, yamlPath, `server:
  port: 8443
  host: 0.0.0.0
  log_level: info
  log_format: text
registration:
  mode: open
`)

	c := &Core{
		Config: &Config{},
		Logger: discardLogger(),
		Events: NewEventBus(discardLogger()),
	}
	applyDefaults(c.Config)
	c.Config.Server.LogLevel = "info"
	c.Config.Server.LogFormat = "text"
	c.Config.Registration.Mode = "open"
	c.ConfigPath = yamlPath

	var reloadEvents atomic.Int64
	c.Events.Subscribe(EventConfigReloaded, func(_ context.Context, _ Event) error {
		reloadEvents.Add(1)
		return nil
	})

	// Reload twice with no file changes between calls.
	if err := c.ReloadConfig(); err != nil {
		t.Fatalf("first reload: %v", err)
	}
	if err := c.ReloadConfig(); err != nil {
		t.Fatalf("second reload: %v", err)
	}
	c.Events.Drain()

	if got := reloadEvents.Load(); got != 0 {
		t.Errorf("no-change reload fired %d EventConfigReloaded events, want 0", got)
	}
}

// TestReloadConfig_InFlightDeployOnlyReadsAtomicStructs is a
// regression guard: ReloadConfig MUST mutate Core.Config in-place
// (not swap a pointer) because modules that grabbed a reference to
// *Config at Init time still hold that pointer. Swapping the pointer
// would orphan all live references and break hot-reload silently.
//
// The test grabs *Config before any reload, then compares it to
// *Core.Config after reload — they must still be the same pointer.
func TestReloadConfig_InFlightDeployOnlyReadsAtomicStructs(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "monster.yaml")
	writeYAML(t, yamlPath, `server:
  port: 8443
  host: 0.0.0.0
  log_level: info
`)

	c := &Core{
		Config: &Config{},
		Logger: discardLogger(),
		Events: NewEventBus(discardLogger()),
	}
	applyDefaults(c.Config)
	c.ConfigPath = yamlPath

	// Snapshot the pointer a freshly-initialized module would keep.
	modulePtr := c.Config

	writeYAML(t, yamlPath, `server:
  port: 8443
  host: 0.0.0.0
  log_level: warn
`)
	if err := c.ReloadConfig(); err != nil {
		t.Fatalf("ReloadConfig: %v", err)
	}

	if c.Config != modulePtr {
		t.Fatal("ReloadConfig swapped Core.Config pointer — existing module references would observe stale fields")
	}
	if modulePtr.Server.LogLevel != "warn" {
		t.Errorf("module's config pointer LogLevel = %q, want %q (reload did not mutate in place)",
			modulePtr.Server.LogLevel, "warn")
	}
}
