package cron

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Store handles persistence of cron jobs
type Store struct {
	filePath string
	mu       sync.RWMutex
}

// NewStore creates a new job store
func NewStore(filePath string) (*Store, error) {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create store directory: %w", err)
	}

	return &Store{
		filePath: filePath,
	}, nil
}

// SaveJobs saves all jobs to the store (atomic write)
func (s *Store) SaveJobs(jobs []*Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create temporary file
	tempPath := s.filePath + ".tmp"
	backupPath := s.filePath + ".bak"

	data, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal jobs: %w", err)
	}

	// Write to temporary file
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Create backup if original exists
	if _, err := os.Stat(s.filePath); err == nil {
		_ = os.Remove(backupPath) // Remove old backup if exists
		_ = os.Rename(s.filePath, backupPath)
	}

	// Atomic rename
	if err := os.Rename(tempPath, s.filePath); err != nil {
		// Restore backup on failure
		_ = os.Rename(backupPath, s.filePath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	// Remove backup on success
	_ = os.Remove(backupPath)

	return nil
}

// LoadJobs loads all jobs from the store
func (s *Store) LoadJobs() ([]*Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Job{}, nil // No jobs yet
		}
		return nil, fmt.Errorf("failed to read jobs file: %w", err)
	}

	var jobs []*Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal jobs: %w", err)
	}

	// Migrate legacy jobs if needed
	for _, job := range jobs {
		if err := migrateJob(job); err != nil {
			return nil, fmt.Errorf("failed to migrate job %s: %w", job.ID, err)
		}
	}

	return jobs, nil
}

// migrateJob handles migration from legacy job formats
func migrateJob(job *Job) error {
	// If job has legacy fields, migrate them
	if job.State.RunCount == 0 && job.State.LastRunAt == nil {
		// This is a legacy job - perform migration
		if job.Schedule.Type == "" {
			// Try to infer schedule type from expression
			if job.Schedule.CronExpression != "" {
				job.Schedule.Type = ScheduleTypeCron
			} else if !job.Schedule.At.IsZero() {
				job.Schedule.Type = ScheduleTypeAt
			} else if job.Schedule.EveryDuration > 0 {
				job.Schedule.Type = ScheduleTypeEvery
			}
		}
	}
	return nil
}
