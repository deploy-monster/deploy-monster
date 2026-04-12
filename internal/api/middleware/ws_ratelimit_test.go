package middleware

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeClock returns a controllable time source so tests don't have
// to sleep. Calling advance(d) moves the clock forward.
type fakeClock struct {
	t atomic.Int64 // unix nanos
}

func newFakeClock(t time.Time) *fakeClock {
	c := &fakeClock{}
	c.t.Store(t.UnixNano())
	return c
}

func (c *fakeClock) now() time.Time { return time.Unix(0, c.t.Load()) }
func (c *fakeClock) advance(d time.Duration) {
	c.t.Add(int64(d))
}

func TestWSFrameLimiter_DefaultsApplied(t *testing.T) {
	// Zero values should roll over to the package defaults.
	lim := NewWSFrameLimiter(0, 0)
	if lim.rate != float64(WSFrameRatePerSec) {
		t.Errorf("rate = %v, want %d", lim.rate, WSFrameRatePerSec)
	}
	if lim.capacity != float64(WSFrameBurst) {
		t.Errorf("capacity = %v, want %d", lim.capacity, WSFrameBurst)
	}
	if lim.tokens != float64(WSFrameBurst) {
		t.Errorf("initial tokens = %v, want %d (seeded full)", lim.tokens, WSFrameBurst)
	}
}

func TestWSFrameLimiter_BurstThenDeny(t *testing.T) {
	// 10-token bucket, no refill because clock is frozen.
	lim := NewWSFrameLimiter(10, 10)
	clock := newFakeClock(time.Unix(0, 0))
	lim.now = clock.now

	// First 10 must succeed.
	for i := 0; i < 10; i++ {
		if !lim.Allow() {
			t.Fatalf("Allow #%d denied, want allowed", i)
		}
	}
	// 11th must be denied — bucket is empty, clock hasn't moved.
	if lim.Allow() {
		t.Error("Allow after burst returned true, want false (bucket drained)")
	}
	// 12th still denied.
	if lim.Allow() {
		t.Error("second Allow after burst returned true, want false")
	}
}

func TestWSFrameLimiter_RefillOverTime(t *testing.T) {
	// 10 tokens/sec, burst 10.
	lim := NewWSFrameLimiter(10, 10)
	clock := newFakeClock(time.Unix(0, 0))
	lim.now = clock.now

	// Drain the bucket.
	for i := 0; i < 10; i++ {
		lim.Allow()
	}
	if lim.Allow() {
		t.Fatal("bucket should be empty after drain")
	}

	// Advance 500ms → 5 tokens refilled.
	clock.advance(500 * time.Millisecond)
	for i := 0; i < 5; i++ {
		if !lim.Allow() {
			t.Errorf("Allow #%d after refill denied", i)
		}
	}
	if lim.Allow() {
		t.Error("Allow after 5 refilled tokens spent returned true, want false")
	}
}

func TestWSFrameLimiter_RefillCappedAtCapacity(t *testing.T) {
	lim := NewWSFrameLimiter(10, 10)
	clock := newFakeClock(time.Unix(0, 0))
	lim.now = clock.now

	// Take 3 then sit idle for an hour — the bucket should cap at
	// capacity, not balloon up to thousands of tokens.
	for i := 0; i < 3; i++ {
		lim.Allow()
	}
	clock.advance(time.Hour)

	// Should be able to allow the full capacity again.
	for i := 0; i < 10; i++ {
		if !lim.Allow() {
			t.Errorf("Allow #%d after idle-hour denied, want 10 (capped)", i)
		}
	}
	if lim.Allow() {
		t.Error("11th Allow after cap returned true, want false")
	}
}

func TestWSFrameLimiter_SustainedRate(t *testing.T) {
	// 100 tokens/sec, burst 100. Over a simulated second we should
	// be able to spend ~200 tokens total (100 initial + 100 refill).
	lim := NewWSFrameLimiter(100, 100)
	clock := newFakeClock(time.Unix(0, 0))
	lim.now = clock.now

	allowed := 0
	for i := 0; i < 100; i++ {
		if lim.Allow() {
			allowed++
		}
	}
	// After initial drain, advance 1 second → 100 more tokens.
	clock.advance(time.Second)
	for i := 0; i < 100; i++ {
		if lim.Allow() {
			allowed++
		}
	}
	// And one more should be denied now.
	if lim.Allow() {
		t.Error("201st Allow returned true, want false (rate exceeded)")
	}
	if allowed != 200 {
		t.Errorf("allowed = %d, want 200", allowed)
	}
}

func TestWSFrameLimiter_Concurrent(t *testing.T) {
	// Hammer Allow from many goroutines and verify no race and the
	// total allowed count never exceeds the initial burst when the
	// clock is frozen. Uses real clock but measurements complete
	// well under a millisecond so refill is negligible.
	lim := NewWSFrameLimiter(1, 50) // 1/s refill, 50 burst
	// Freeze the clock at zero so refill contributes nothing during
	// the ~sub-millisecond race window.
	var mu sync.Mutex
	frozen := time.Unix(0, 0)
	lim.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return frozen
	}

	var wg sync.WaitGroup
	var allowed atomic.Int32
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if lim.Allow() {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := allowed.Load(); got != 50 {
		t.Errorf("concurrent allowed = %d, want 50 (burst ceiling)", got)
	}
}
