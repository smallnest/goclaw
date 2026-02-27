package cron

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

// Service manages cron jobs
type Service struct {
	config    CronConfig
	jobs      map[string]*Job
	jobsMutex sync.RWMutex
	bus       *bus.MessageBus
	store     *Store
	runLogger *RunLogger

	// Execution control
	executor   *JobExecutor
	running    bool
	stopChan   chan struct{}
	wg         sync.WaitGroup

	// Timer
	timerMu      sync.Mutex
	timerArmed   bool
	timerStop    chan struct{}
}

// NewService creates a new cron service
func NewService(config CronConfig, bus *bus.MessageBus) (*Service, error) {
	// Ensure store directory exists
	storePath := config.StorePath
	if storePath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		storePath = filepath.Join(homeDir, ".goclaw", "cron", "jobs.json")
	}

	store, err := NewStore(storePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create store: %w", err)
	}

	// Create run logger
	runLogDir := filepath.Join(filepath.Dir(storePath), "runs")
	runLogger, err := NewRunLogger(runLogDir, config.RunLogConfig)
	if err != nil {
		logger.Warn("Failed to create run logger", zap.Error(err))
		runLogger = nil
	}

	service := &Service{
		config:    config,
		jobs:      make(map[string]*Job),
		bus:       bus,
		store:     store,
		runLogger: runLogger,
		executor:  NewJobExecutor(bus, runLogger, config.DefaultTimeout),
		stopChan:  make(chan struct{}),
	}

	// Load jobs from store
	if err := service.loadJobs(); err != nil {
		return nil, fmt.Errorf("failed to load jobs: %w", err)
	}

	return service, nil
}

// Start starts the cron service
func (s *Service) Start(ctx context.Context) error {
	s.jobsMutex.Lock()
	defer s.jobsMutex.Unlock()

	if s.running {
		return nil
	}

	s.running = true
	s.timerStop = make(chan struct{})

	// Start the timer
	s.wg.Add(1)
	go s.runTimer(ctx)

	logger.Info("Cron service started",
		zap.Int("total_jobs", len(s.jobs)),
		zap.Int("enabled_jobs", s.countEnabledJobs()),
	)

	return nil
}

// Stop stops the cron service
func (s *Service) Stop() error {
	s.jobsMutex.Lock()
	defer s.jobsMutex.Unlock()

	if !s.running {
		return nil
	}

	// Stop timer
	close(s.timerStop)

	// Wait for goroutines
	s.wg.Wait()

	s.running = false

	// Persist jobs
	if err := s.persistJobs(); err != nil {
		logger.Error("Failed to persist jobs on stop", zap.Error(err))
	}

	logger.Info("Cron service stopped")

	return nil
}

// AddJob adds a new job
func (s *Service) AddJob(job *Job) error {
	s.jobsMutex.Lock()
	defer s.jobsMutex.Unlock()

	if job.ID == "" {
		job.ID = generateJobID()
	}

	if _, exists := s.jobs[job.ID]; exists {
		return fmt.Errorf("job with ID %s already exists", job.ID)
	}

	// Set timestamps
	now := time.Now()
	job.CreatedAt = now
	job.UpdatedAt = now

	// Calculate initial next run time
	if job.State.Enabled {
		if _, err := job.CalculateNextRun(now); err != nil {
			return fmt.Errorf("failed to calculate next run: %w", err)
		}
	}

	s.jobs[job.ID] = job

	// Persist
	if err := s.persistJobs(); err != nil {
		delete(s.jobs, job.ID)
		return fmt.Errorf("failed to persist job: %w", err)
	}

	logger.Info("Cron job added",
		zap.String("job_id", job.ID),
		zap.String("name", job.Name),
		zap.String("schedule_type", string(job.Schedule.Type)),
	)

	return nil
}

// UpdateJob updates an existing job
func (s *Service) UpdateJob(id string, update func(*Job) error) error {
	s.jobsMutex.Lock()
	defer s.jobsMutex.Unlock()

	job, exists := s.jobs[id]
	if !exists {
		return fmt.Errorf("job not found: %s", id)
	}

	// Apply update
	if err := update(job); err != nil {
		return err
	}

	job.UpdatedAt = time.Now()

	// Recalculate next run time
	if job.State.Enabled {
		if _, err := job.CalculateNextRun(time.Now()); err != nil {
			return fmt.Errorf("failed to calculate next run: %w", err)
		}
	}

	// Persist
	if err := s.persistJobs(); err != nil {
		return fmt.Errorf("failed to persist job: %w", err)
	}

	logger.Info("Cron job updated", zap.String("job_id", id))

	return nil
}

// RemoveJob removes a job
func (s *Service) RemoveJob(id string) error {
	s.jobsMutex.Lock()
	defer s.jobsMutex.Unlock()

	if _, exists := s.jobs[id]; !exists {
		return fmt.Errorf("job not found: %s", id)
	}

	delete(s.jobs, id)

	// Persist
	if err := s.persistJobs(); err != nil {
		return fmt.Errorf("failed to persist jobs: %w", err)
	}

	// Clean up run logs
	if s.runLogger != nil {
		_ = s.runLogger.DeleteJobLogs(id)
	}

	logger.Info("Cron job removed", zap.String("job_id", id))

	return nil
}

// GetJob retrieves a job by ID
func (s *Service) GetJob(id string) (*Job, error) {
	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()

	job, exists := s.jobs[id]
	if !exists {
		return nil, fmt.Errorf("job not found: %s", id)
	}

	return job, nil
}

// ListJobs returns all jobs
func (s *Service) ListJobs() []*Job {
	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()

	jobs := make([]*Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, job)
	}

	return jobs
}

// EnableJob enables a job
func (s *Service) EnableJob(id string) error {
	return s.UpdateJob(id, func(job *Job) error {
		job.State.Enabled = true
		now := time.Now()
		if _, err := job.CalculateNextRun(now); err != nil {
			return err
		}
		return nil
	})
}

// DisableJob disables a job
func (s *Service) DisableJob(id string) error {
	return s.UpdateJob(id, func(job *Job) error {
		job.State.Enabled = false
		job.State.NextRunAt = nil
		return nil
	})
}

// RunJob executes a job immediately
func (s *Service) RunJob(ctx context.Context, id string, force bool) error {
	s.jobsMutex.RLock()
	job, exists := s.jobs[id]
	s.jobsMutex.RUnlock()

	if !exists {
		return fmt.Errorf("job not found: %s", id)
	}

	if !job.State.Enabled && !force {
		return fmt.Errorf("job is disabled (use force to run anyway)")
	}

	// Execute the job
	return s.executeJob(ctx, job)
}

// GetRunLogs retrieves run logs for a job
func (s *Service) GetRunLogs(jobID string, filter RunLogFilter) ([]*RunLog, error) {
	if s.runLogger == nil {
		return []*RunLog{}, nil
	}

	return s.runLogger.ReadLogs(jobID, filter)
}

// runTimer is the main timer loop
func (s *Service) runTimer(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		case <-s.timerStop:
			return
		case now := <-ticker.C:
			s.checkAndRunJobs(ctx, now)
		}
	}
}

// checkAndRunJobs checks for due jobs and executes them
func (s *Service) checkAndRunJobs(ctx context.Context, now time.Time) {
	s.jobsMutex.RLock()
	jobs := make([]*Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, job)
	}
	s.jobsMutex.RUnlock()

	// Find due jobs
	var dueJobs []*Job
	for _, job := range jobs {
		if job.ShouldRun(now) {
			dueJobs = append(dueJobs, job)
		}
	}

	// Execute due jobs
	for _, job := range dueJobs {
		if err := s.executeJob(ctx, job); err != nil {
			logger.Error("Failed to execute cron job",
				zap.String("job_id", job.ID),
				zap.Error(err),
			)
		}
	}
}

// executeJob executes a single job
func (s *Service) executeJob(ctx context.Context, job *Job) error {
	// Mark as running
	now := time.Now()
	job.MarkRunning(now)

	// Update state
	s.jobsMutex.Lock()
	s.jobs[job.ID] = job
	s.jobsMutex.Unlock()

	// Create run context with timeout
	timeout := s.config.DefaultTimeout
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute the job (don't block - runs in executor)
	if err := s.executor.Execute(runCtx, job); err != nil {
		logger.Error("Job execution failed",
			zap.String("job_id", job.ID),
			zap.Error(err),
		)
		// Mark as error
		job.MarkCompleted(now, "error", err.Error())
	} else {
		job.MarkCompleted(now, "ok", "")
	}

	// Calculate next run time
	nextRun, err := job.CalculateNextRun(now)
	if err != nil {
		logger.Error("Failed to calculate next run",
			zap.String("job_id", job.ID),
			zap.Error(err),
		)
	}

	// Disable one-shot jobs after completion
	if job.ShouldDisableOnComplete() {
		job.State.Enabled = false
		job.State.NextRunAt = nil
	} else if job.State.LastStatus == "error" && job.State.ConsecutiveErrors > 0 {
		// Apply exponential backoff
		backoffDelay := GetBackoffDelay(job.State.ConsecutiveErrors)
		backoffUntil := now.Add(backoffDelay)
		job.State.ErrorBackoffUntil = &backoffUntil
		if nextRun.Before(backoffUntil) {
			job.State.NextRunAt = &backoffUntil
		}
	}

	// Update state
	s.jobsMutex.Lock()
	s.jobs[job.ID] = job
	s.jobsMutex.Unlock()

	// Persist changes
	if err := s.persistJobs(); err != nil {
		logger.Error("Failed to persist job state", zap.Error(err))
	}

	logger.Debug("Job execution completed",
		zap.String("job_id", job.ID),
		zap.String("status", job.State.LastStatus),
		zap.String("next_run", formatTimePtr(job.State.NextRunAt)),
	)

	return nil
}

// persistJobs saves all jobs to the store
func (s *Service) persistJobs() error {
	jobs := make([]*Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, job)
	}

	return s.store.SaveJobs(jobs)
}

// loadJobs loads jobs from the store
func (s *Service) loadJobs() error {
	jobs, err := s.store.LoadJobs()
	if err != nil {
		return err
	}

	for _, job := range jobs {
		s.jobs[job.ID] = job
	}

	return nil
}

// countEnabledJobs returns the number of enabled jobs
func (s *Service) countEnabledJobs() int {
	count := 0
	for _, job := range s.jobs {
		if job.State.Enabled {
			count++
		}
	}
	return count
}

// GetStatus returns the current status of the cron service
func (s *Service) GetStatus() map[string]interface{} {
	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()

	enabledCount := 0
	runningCount := 0
	for _, job := range s.jobs {
		if job.State.Enabled {
			enabledCount++
		}
		if job.IsRunning() {
			runningCount++
		}
	}

	return map[string]interface{}{
		"running":        s.running,
		"total_jobs":     len(s.jobs),
		"enabled_jobs":   enabledCount,
		"running_jobs":   runningCount,
		"disabled_jobs":  len(s.jobs) - enabledCount,
		"config":         s.config,
	}
}

// Helper functions

func generateJobID() string {
	return fmt.Sprintf("job-%s", uuid.New().String()[:8])
}

func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}
