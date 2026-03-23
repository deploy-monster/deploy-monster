package core

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// CronJob represents a scheduled recurring task.
type CronJob struct {
	ID       string
	Name     string
	Schedule string // "HH:MM" daily or "@every 5m" interval
	Handler  func(ctx context.Context) error
	LastRun  time.Time
	NextRun  time.Time
	Running  bool
	Enabled  bool
}

// Scheduler manages cron-like recurring jobs.
type Scheduler struct {
	mu     sync.RWMutex
	jobs   map[string]*CronJob
	logger *slog.Logger
	stopCh chan struct{}
}

// NewScheduler creates a task scheduler.
func NewScheduler(logger *slog.Logger) *Scheduler {
	return &Scheduler{
		jobs:   make(map[string]*CronJob),
		logger: logger,
		stopCh: make(chan struct{}),
	}
}

// Add registers a new cron job.
func (s *Scheduler) Add(job *CronJob) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if job.ID == "" {
		job.ID = GenerateID()
	}
	job.Enabled = true
	job.NextRun = s.calcNextRun(job.Schedule)
	s.jobs[job.ID] = job

	s.logger.Info("cron job registered", "id", job.ID, "name", job.Name, "schedule", job.Schedule)
}

// Remove unregisters a cron job.
func (s *Scheduler) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, id)
}

// Start begins the scheduler loop.
func (s *Scheduler) Start() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.tick()
			case <-s.stopCh:
				return
			}
		}
	}()
	s.logger.Info("scheduler started", "jobs", len(s.jobs))
}

// Stop halts the scheduler.
func (s *Scheduler) Stop() {
	close(s.stopCh)
}

// Jobs returns all registered jobs.
func (s *Scheduler) Jobs() []*CronJob {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*CronJob, 0, len(s.jobs))
	for _, j := range s.jobs {
		result = append(result, j)
	}
	return result
}

func (s *Scheduler) tick() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for _, job := range s.jobs {
		if !job.Enabled || job.Running || now.Before(job.NextRun) {
			continue
		}

		job.Running = true
		go func(j *CronJob) {
			defer func() {
				s.mu.Lock()
				j.Running = false
				j.LastRun = time.Now()
				j.NextRun = s.calcNextRun(j.Schedule)
				s.mu.Unlock()
			}()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()

			if err := j.Handler(ctx); err != nil {
				s.logger.Error("cron job failed", "job", j.Name, "error", err)
			} else {
				s.logger.Debug("cron job completed", "job", j.Name)
			}
		}(job)
	}
}

func (s *Scheduler) calcNextRun(schedule string) time.Time {
	now := time.Now()

	// Parse "@every Xm" format
	if len(schedule) > 7 && schedule[:6] == "@every" {
		dur, err := time.ParseDuration(schedule[7:])
		if err == nil {
			return now.Add(dur)
		}
	}

	// Parse "HH:MM" daily format
	if len(schedule) >= 4 {
		var h, m int
		n, _ := parseHHMM(schedule)
		h = n / 60
		m = n % 60

		next := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, now.Location())
		if next.Before(now) {
			next = next.Add(24 * time.Hour)
		}
		return next
	}

	return now.Add(time.Hour)
}

func parseHHMM(s string) (int, error) {
	var h, m int
	for i, c := range s {
		if c == ':' {
			h = atoi(s[:i])
			m = atoi(s[i+1:])
			break
		}
	}
	return h*60 + m, nil
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}
