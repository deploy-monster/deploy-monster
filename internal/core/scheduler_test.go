package core

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func TestScheduler_AddAndList(t *testing.T) {
	s := NewScheduler(slog.Default())

	s.Add(&CronJob{
		Name:     "test-job",
		Schedule: "@every 1h",
		Handler:  func(_ context.Context) error { return nil },
	})

	jobs := s.Jobs()
	if len(jobs) != 1 {
		t.Errorf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].Name != "test-job" {
		t.Errorf("expected test-job, got %s", jobs[0].Name)
	}
	if !jobs[0].Enabled {
		t.Error("job should be enabled by default")
	}
}

func TestScheduler_Remove(t *testing.T) {
	s := NewScheduler(slog.Default())

	s.Add(&CronJob{ID: "job-1", Name: "j1", Schedule: "@every 1h",
		Handler: func(_ context.Context) error { return nil }})
	s.Add(&CronJob{ID: "job-2", Name: "j2", Schedule: "@every 1h",
		Handler: func(_ context.Context) error { return nil }})

	s.Remove("job-1")

	if len(s.Jobs()) != 1 {
		t.Error("expected 1 job after remove")
	}
}

func TestScheduler_ExecutesJob(t *testing.T) {
	s := NewScheduler(slog.Default())

	var called atomic.Bool
	s.Add(&CronJob{
		Name:     "instant",
		Schedule: "@every 1s",
		Handler: func(_ context.Context) error {
			called.Store(true)
			return nil
		},
	})

	// Manually override next run to now
	s.mu.Lock()
	for _, j := range s.jobs {
		j.NextRun = time.Now().Add(-time.Second)
	}
	s.mu.Unlock()

	s.tick()

	time.Sleep(100 * time.Millisecond)

	if !called.Load() {
		t.Error("expected job to be executed")
	}
}

func TestParseHHMM(t *testing.T) {
	tests := []struct {
		input    string
		wantMins int
	}{
		{"02:00", 120},
		{"14:30", 870},
		{"00:00", 0},
		{"23:59", 1439},
	}

	for _, tt := range tests {
		got, _ := parseHHMM(tt.input)
		if got != tt.wantMins {
			t.Errorf("parseHHMM(%q) = %d, want %d", tt.input, got, tt.wantMins)
		}
	}
}
