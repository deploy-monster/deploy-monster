//go:build !windows

package core

import (
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// TestReloadConfig_SIGHUPTriggersReload exercises the exact signal
// plumbing cmd/deploymonster/main.go:122-131 uses: signal.Notify on
// a SIGHUP channel, a background goroutine that calls ReloadConfig
// on every signal, and a real syscall.Kill to the current process.
//
// Windows does not support SIGHUP at the OS level, so this file is
// gated with a build tag. The portable
// TestReloadConfig_ConcurrentWithInFlightDeploy test covers the
// same ReloadConfig logic on every platform; this file specifically
// guards the signal-to-reload wiring that cmd/deploymonster owns.
func TestReloadConfig_SIGHUPTriggersReload(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "monster.yaml")
	writeYAML(t, yamlPath, `server:
  port: 8443
  host: 0.0.0.0
  log_level: info
  log_format: text
`)

	c := &Core{
		Config: &Config{},
		Logger: discardLogger(),
		Events: NewEventBus(discardLogger()),
	}
	applyDefaults(c.Config)
	c.Config.Server.LogLevel = "info"
	c.Config.Server.LogFormat = "text"
	c.ConfigPath = yamlPath

	sighup := make(chan os.Signal, 1)
	signal.Notify(sighup, syscall.SIGHUP)
	defer signal.Stop(sighup)

	stop := make(chan struct{})
	var reloaded atomic.Int32
	var reloadErrs atomic.Int32
	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			select {
			case <-sighup:
				if err := c.ReloadConfig(); err != nil {
					reloadErrs.Add(1)
					continue
				}
				reloaded.Add(1)
			case <-stop:
				return
			}
		}
	}()

	// Rewrite the YAML, then send SIGHUP. syscall.Kill to self is
	// the standard trick for self-signaling in Go tests on Unix.
	writeYAML(t, yamlPath, `server:
  port: 8443
  host: 0.0.0.0
  log_level: debug
  log_format: json
`)
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGHUP); err != nil {
		t.Fatalf("Kill self: %v", err)
	}
	// Give the signal goroutine time to observe the signal and run
	// ReloadConfig. A real deployment would not poll for a reload to
	// finish, but the test needs a bounded wait so a stuck handler
	// fails loudly instead of hanging.
	deadline := time.Now().Add(time.Second)
	for reloaded.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if reloaded.Load() < 1 {
		t.Fatalf("ReloadConfig not observed within 1s after SIGHUP (got %d reloads)",
			reloaded.Load())
	}

	close(stop)
	<-done

	if reloadErrs.Load() != 0 {
		t.Errorf("ReloadConfig failed %d times from SIGHUP path", reloadErrs.Load())
	}
	if c.Config.Server.LogLevel != "debug" {
		t.Errorf("LogLevel after SIGHUP reload = %q, want %q",
			c.Config.Server.LogLevel, "debug")
	}
}
