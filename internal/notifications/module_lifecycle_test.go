package notifications

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// stubProvider is a Send target whose behavior tests can script per
// call: it can block until released, return an error, or panic. Used
// to drive the module's drain and recover paths without a real SMTP
// or HTTP transport.
type stubProvider struct {
	name     string
	sendFn   func(ctx context.Context) error
	sendWG   sync.WaitGroup
	released chan struct{}
}

func newStubProvider(name string) *stubProvider {
	return &stubProvider{
		name:     name,
		released: make(chan struct{}),
	}
}

func (p *stubProvider) Name() string    { return p.name }
func (p *stubProvider) Validate() error { return nil }

func (p *stubProvider) Send(ctx context.Context, _, _, _, _ string) error {
	p.sendWG.Add(1)
	defer p.sendWG.Done()
	if p.sendFn != nil {
		return p.sendFn(ctx)
	}
	return nil
}

func newTestModule(t *testing.T) *Module {
	t.Helper()
	// Build just enough of core.Core for Send to run. We don't use
	// the full New() module because Start wants a real Core; instead
	// we poke dispatcher + core in directly so we can script the
	// provider.
	m := New()
	m.dispatcher = NewDispatcher()
	m.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	// The EventBus is only used by PublishAsync paths. Use a real
	// bus with a silent logger so the async publishes don't NPE.
	bus := core.NewEventBus(m.logger)
	m.core = &core.Core{Events: bus}
	// Initialize the lifecycle context the same way Start does so
	// Stop has something to cancel even without a full Start.
	m.stopCtx, m.stopCancel = context.WithCancel(context.Background())
	return m
}

func TestNotificationModule_Send_RejectedAfterStop(t *testing.T) {
	m := newTestModule(t)
	m.dispatcher.RegisterProvider(newStubProvider("stub"))

	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !m.Closed() {
		t.Error("Closed() = false after Stop, want true")
	}

	err := m.Send(context.Background(), core.Notification{Channel: "stub", Body: "x"})
	if !errors.Is(err, ErrNotificationsClosed) {
		t.Errorf("Send after Stop = %v, want ErrNotificationsClosed", err)
	}
}

func TestNotificationModule_Stop_WaitsForInflight(t *testing.T) {
	m := newTestModule(t)
	release := make(chan struct{})
	stub := newStubProvider("slow")
	stub.sendFn = func(ctx context.Context) error {
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	m.dispatcher.RegisterProvider(stub)

	// Kick off a Send in the background; it blocks until release.
	sendErrCh := make(chan error, 1)
	go func() {
		sendErrCh <- m.Send(context.Background(), core.Notification{Channel: "slow", Body: "x"})
	}()

	// Give the goroutine a moment to enter the provider's Send.
	time.Sleep(20 * time.Millisecond)

	// Stop with a short deadline should time out because the Send is
	// still parked on release. Stop must NOT return an error even on
	// drain timeout — the contract is "shut down anyway".
	stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := m.Stop(stopCtx); err != nil {
		t.Errorf("Stop with drain timeout = %v, want nil (log-and-press-on)", err)
	}
	if !m.Closed() {
		t.Error("Closed() = false after drain timeout, want true")
	}

	// Release the blocked Send so the test doesn't leak goroutines.
	close(release)
	select {
	case <-sendErrCh:
	case <-time.After(time.Second):
		t.Fatal("background Send did not unblock after release")
	}
}

func TestNotificationModule_Stop_IdempotentAndDrains(t *testing.T) {
	m := newTestModule(t)
	stub := newStubProvider("fast")
	m.dispatcher.RegisterProvider(stub)

	// Run a Send to completion so wg goes up and comes back down.
	if err := m.Send(context.Background(), core.Notification{Channel: "fast", Body: "x"}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// First Stop drains the (already-empty) wg and marks closed.
	if err := m.Stop(context.Background()); err != nil {
		t.Errorf("first Stop = %v, want nil", err)
	}
	// Second Stop must be a no-op.
	if err := m.Stop(context.Background()); err != nil {
		t.Errorf("second Stop = %v, want nil (idempotent)", err)
	}
}

func TestNotificationModule_Send_PanicRecovered(t *testing.T) {
	m := newTestModule(t)
	stub := newStubProvider("panicker")
	stub.sendFn = func(context.Context) error {
		panic("boom from provider")
	}
	m.dispatcher.RegisterProvider(stub)

	err := m.Send(context.Background(), core.Notification{Channel: "panicker", Body: "x"})
	if err == nil {
		t.Fatal("Send returned nil after provider panic, want recovered error")
	}
	// The recover wraps the panic message in a structured error.
	if !contains(err.Error(), "panic") {
		t.Errorf("err = %v, want message containing 'panic'", err)
	}

	// After a recovered panic, Stop must still drain cleanly — the
	// wg.Done in the defer chain runs before the recover so the wg
	// counter returns to zero.
	if err := m.Stop(context.Background()); err != nil {
		t.Errorf("Stop after recovered panic = %v, want nil", err)
	}
}

func TestNotificationModule_Send_UnknownChannel(t *testing.T) {
	m := newTestModule(t)
	err := m.Send(context.Background(), core.Notification{Channel: "missing", Body: "x"})
	if err == nil {
		t.Fatal("Send with unknown channel returned nil, want error")
	}
}

func TestNotificationModule_Send_ConcurrentRespectClosed(t *testing.T) {
	// 50 concurrent Sends race against a Stop. Every Send must either
	// succeed (wg properly balanced) or return ErrNotificationsClosed.
	// No Send must observe a partially-torn-down module.
	m := newTestModule(t)
	m.dispatcher.RegisterProvider(newStubProvider("race"))

	var ok atomic.Int32
	var closed atomic.Int32
	var other atomic.Int32

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := m.Send(context.Background(), core.Notification{Channel: "race", Body: "x"})
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, ErrNotificationsClosed):
				closed.Add(1)
			default:
				other.Add(1)
			}
		}()
	}

	// Stop in the middle of the storm.
	time.Sleep(2 * time.Millisecond)
	_ = m.Stop(context.Background())

	wg.Wait()

	if other.Load() != 0 {
		t.Errorf("got %d Send calls with unexpected error class", other.Load())
	}
	// We don't assert a specific split between ok and closed — that
	// depends on scheduling. We only require both classes remain
	// accounted for and wg is drained (Stop returned without a leak).
	t.Logf("ok=%d closed=%d", ok.Load(), closed.Load())
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
