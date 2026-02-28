package cron

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// RunLogger handles logging of job runs
type RunLogger struct {
	logDir string
	config RunLogConfig
	mu     sync.Mutex
}

// NewRunLogger creates a new run logger
func NewRunLogger(logDir string, config RunLogConfig) (*RunLogger, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	return &RunLogger{
		logDir: logDir,
		config: config,
	}, nil
}

// LogRun records a job run
func (l *RunLogger) LogRun(log *RunLog) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	logFile := l.getJobLogPath(log.JobID)

	// Check if file needs rotation
	if l.config.MaxBytes > 0 {
		if info, err := os.Stat(logFile); err == nil {
			if info.Size() >= l.config.MaxBytes {
				// Rotate by pruning old entries
				_ = l.pruneLog(logFile)
			}
		}
	}

	// Open file in append mode
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	// Write as JSONL
	data, err := json.Marshal(log)
	if err != nil {
		return fmt.Errorf("failed to marshal log: %w", err)
	}

	if _, err := file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write log: %w", err)
	}

	return nil
}

// ReadLogs reads run logs for a job
func (l *RunLogger) ReadLogs(jobID string, filter RunLogFilter) ([]*RunLog, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	logFile := l.getJobLogPath(jobID)

	file, err := os.Open(logFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []*RunLog{}, nil
		}
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	var logs []*RunLog
	scanner := bufio.NewScanner(file)

	// Read all logs
	for scanner.Scan() {
		var log RunLog
		if err := json.Unmarshal(scanner.Bytes(), &log); err != nil {
			continue // Skip malformed entries
		}

		// Apply filters
		if filter.JobID != "" && log.JobID != filter.JobID {
			continue
		}
		if !filter.After.IsZero() && log.StartedAt.Before(filter.After) {
			continue
		}
		if !filter.Before.IsZero() && log.StartedAt.After(filter.Before) {
			continue
		}
		if filter.Status != "" && log.Status != filter.Status {
			continue
		}

		logs = append(logs, &log)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading log file: %w", err)
	}

	// Reverse to get most recent first
	reverseLogs(logs)

	// Apply offset and limit
	if filter.Offset > 0 {
		if filter.Offset >= len(logs) {
			return []*RunLog{}, nil
		}
		logs = logs[filter.Offset:]
	}

	if filter.Limit > 0 && filter.Limit < len(logs) {
		logs = logs[:filter.Limit]
	}

	return logs, nil
}

// DeleteJobLogs removes all logs for a job
func (l *RunLogger) DeleteJobLogs(jobID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	logFile := l.getJobLogPath(jobID)
	if err := os.Remove(logFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete log file: %w", err)
	}

	return nil
}

// getJobLogPath returns the log file path for a job
func (l *RunLogger) getJobLogPath(jobID string) string {
	return filepath.Join(l.logDir, fmt.Sprintf("%s.jsonl", jobID))
}

// pruneLog removes old log entries to keep within line limit
func (l *RunLogger) pruneLog(logFile string) error {
	if l.config.KeepLines <= 0 {
		return nil
	}

	// Read all logs
	var logs []*RunLog
	file, err := os.Open(logFile)
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var log RunLog
		if err := json.Unmarshal(scanner.Bytes(), &log); err == nil {
			logs = append(logs, &log)
		}
	}
	file.Close()

	// Keep only last N entries
	if len(logs) > l.config.KeepLines {
		logs = logs[len(logs)-l.config.KeepLines:]
	}

	// Write back
	file, err = os.Create(logFile)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, log := range logs {
		data, _ := json.Marshal(log)
		if _, err := file.Write(append(data, '\n')); err != nil {
			return err
		}
	}

	return nil
}

func reverseLogs(logs []*RunLog) {
	for i, j := 0, len(logs)-1; i < j; i, j = i+1, j-1 {
		logs[i], logs[j] = logs[j], logs[i]
	}
}
