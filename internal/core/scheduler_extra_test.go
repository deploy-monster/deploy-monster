package core

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func TestScheduler_Stop(t *testing.T) {
	s := NewScheduler(slog.Default())

	var tickCount atomic.Int32
	s.Add(&CronJob{
		Name:     "counter",
		Schedule: "@every 1s",
		Handler: func(_ context.Context) error {
			tickCount.Add(1)
			return nil
		},
	})

	s.Start()

	// Let the scheduler run briefly
	time.Sleep(100 * time.Millisecond)

	// Stop should not panic and should halt the scheduler loop
	s.Stop()

	// Give the goroutine time to exit
	time.Sleep(100 * time.Millisecond)

	// Record the count after stop
	countAfterStop := tickCount.Load()

	// Wait again to verify no more ticks happen
	time.Sleep(200 * time.Millisecond)
	countLater := tickCount.Load()

	if countLater != countAfterStop {
		t.Errorf("scheduler continued ticking after Stop: count went from %d to %d",
			countAfterStop, countLater)
	}
}

func TestScheduler_StartAndStop_NoJobs(t *testing.T) {
	s := NewScheduler(slog.Default())

	// Start with no jobs should not panic
	s.Start()

	time.Sleep(50 * time.Millisecond)

	// Stop with no jobs should not panic
	s.Stop()
}

func TestScheduler_Stop_Idempotent(t *testing.T) {
	s := NewScheduler(slog.Default())
	s.Start()

	time.Sleep(50 * time.Millisecond)

	// First stop should work
	s.Stop()

	// Second stop on a closed channel would panic if not handled,
	// but the current implementation uses close() which will panic.
	// This test documents the behavior: calling Stop() twice panics.
	// If the implementation is changed to be idempotent, update this test.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Stop() panicked on second call: %v", r)
		}
	}()

	// This should be safe if the implementation is idempotent
	s.Stop()
}

func TestScheduler_CalcNextRun_EveryInterval(t *testing.T) {
	s := NewScheduler(slog.Default())

	tests := []struct {
		schedule string
		minDur   time.Duration
		maxDur   time.Duration
	}{
		{"@every 5m", 4*time.Minute + 59*time.Second, 5*time.Minute + 1*time.Second},
		{"@every 1h", 59*time.Minute + 59*time.Second, 1*time.Hour + 1*time.Second},
		{"@every 30s", 29 * time.Second, 31 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.schedule, func(t *testing.T) {
			before := time.Now()
			next := s.calcNextRun(tt.schedule)
			diff := next.Sub(before)

			if diff < tt.minDur || diff > tt.maxDur {
				t.Errorf("calcNextRun(%q) = %v from now, expected between %v and %v",
					tt.schedule, diff, tt.minDur, tt.maxDur)
			}
		})
	}
}

func TestScheduler_CalcNextRun_HHMMFormat(t *testing.T) {
	s := NewScheduler(slog.Default())

	// Use 05:00 to avoid DST transition issues (DST in Turkey is at 03:00 on March 30)
	next := s.calcNextRun("05:00")

	// The result should be a valid time in the future or at most 24h from now
	now := time.Now()
	if next.Before(now.Add(-time.Second)) {
		t.Error("calcNextRun('05:00') returned a time in the past")
	}
	diff := next.Sub(now)
	if diff > 25*time.Hour {
		t.Errorf("calcNextRun('05:00') returned time %v in the future, expected <= 24h", diff)
	}

	// Verify it targets 05:00
	if next.Hour() != 5 || next.Minute() != 0 {
		t.Errorf("calcNextRun('05:00') = %02d:%02d, want 05:00", next.Hour(), next.Minute())
	}
}
