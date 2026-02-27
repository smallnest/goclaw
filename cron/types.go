package cron

import (
	"encoding/json"
	"time"
)

// ScheduleType represents the type of schedule
type ScheduleType string

const (
	ScheduleTypeAt    ScheduleType = "at"    // One-shot at specific time
	ScheduleTypeEvery ScheduleType = "every" // Fixed interval
	ScheduleTypeCron  ScheduleType = "cron"  // Cron expression
)

// Schedule defines when a job should run
type Schedule struct {
	Type ScheduleType `json:"type"`

	// For "at" type
	At time.Time `json:"at,omitempty"`

	// For "every" type
	EveryDuration time.Duration `json:"every_duration,omitempty"`

	// For "cron" type
	CronExpression string `json:"cron_expression,omitempty"`
	Timezone       string `json:"timezone,omitempty"`

	// Stagger support (for load distribution)
	StaggerDuration time.Duration `json:"stagger_duration,omitempty"`
}

// DeliveryMode defines how job results are delivered
type DeliveryMode string

const (
	DeliveryModeAnnounce DeliveryMode = "announce" // Send to chat channels
	DeliveryModeWebhook  DeliveryMode = "webhook"  // HTTP POST to URL
	DeliveryModeNone     DeliveryMode = "none"     // No delivery
)

// Delivery configuration
type Delivery struct {
	Mode       DeliveryMode `json:"mode"`
	WebhookURL string       `json:"webhook_url,omitempty"`
	WebhookToken string     `json:"webhook_token,omitempty"`
	BestEffort bool         `json:"best_effort,omitempty"` // Don't fail job on delivery error
}

// SessionTarget defines where the job runs
type SessionTarget string

const (
	SessionTargetMain      SessionTarget = "main"      // Run in main session
	SessionTargetIsolated  SessionTarget = "isolated"  // Run in dedicated isolated session
)

// WakeMode defines when to wake up for scheduled jobs
type WakeMode string

const (
	WakeModeNow         WakeMode = "now"           // Wake immediately
	WakeModeNextHeartbeat WakeMode = "next-heartbeat" // Wake on next heartbeat
)

// PayloadType defines the job payload type
type PayloadType string

const (
	PayloadTypeSystemEvent PayloadType = "system-event"
	PayloadTypeAgentTurn   PayloadType = "agent-turn"
)

// Payload defines what the job executes
type Payload struct {
	Type PayloadType `json:"type"`

	// For system-event type
	SystemEventType string `json:"system_event_type,omitempty"`

	// For agent-turn type
	Message string `json:"message,omitempty"`
}

// JobState represents the current state of a job
type JobState struct {
	Enabled          bool       `json:"enabled"`
	RunningAt        *time.Time `json:"running_at,omitempty"`         // Set when job is running
	LastRunAt        *time.Time `json:"last_run_at,omitempty"`         // Last successful run
	NextRunAt        *time.Time `json:"next_run_at,omitempty"`         // Next scheduled run
	ConsecutiveErrors int       `json:"consecutive_errors"`            // Count of consecutive errors
	RunCount         int        `json:"run_count"`                     // Total successful runs
	ErrorBackoffUntil *time.Time `json:"error_backoff_until,omitempty"` // Backoff for failed jobs
	LastStatus       string     `json:"last_status"`                   // Last run status: "ok", "error", "skipped"
	LastError        string     `json:"last_error,omitempty"`          // Last error message
}

// Job represents a scheduled cron job
type Job struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Schedule      Schedule    `json:"schedule"`
	SessionTarget SessionTarget `json:"session_target"`
	WakeMode      WakeMode    `json:"wake_mode"`
	Payload       Payload     `json:"payload"`
	Delivery      *Delivery   `json:"delivery,omitempty"`
	State         JobState    `json:"state"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
}

// IsRunning checks if the job is currently running
func (j *Job) IsRunning() bool {
	return j.State.RunningAt != nil
}

// ShouldRun checks if the job should run at the given time
func (j *Job) ShouldRun(now time.Time) bool {
	if !j.State.Enabled {
		return false
	}

	if j.IsRunning() {
		return false
	}

	// Check error backoff
	if j.State.ErrorBackoffUntil != nil && now.Before(*j.State.ErrorBackoffUntil) {
		return false
	}

	// Check if next run time has arrived
	if j.State.NextRunAt != nil && now.After(*j.State.NextRunAt) {
		return true
	}

	return false
}

// MarkRunning marks the job as running
func (j *Job) MarkRunning(now time.Time) {
	j.State.RunningAt = &now
	j.State.LastRunAt = &now
}

// MarkCompleted marks the job as completed with a status
func (j *Job) MarkCompleted(now time.Time, status string, errMsg string) {
	j.State.RunningAt = nil
	j.State.LastStatus = status
	j.State.LastError = errMsg
	j.State.RunCount++

	if status == "ok" {
		j.State.ConsecutiveErrors = 0
	} else if status == "error" {
		j.State.ConsecutiveErrors++
	}

	j.UpdatedAt = now
}

// CalculateNextRun calculates the next run time based on schedule
func (j *Job) CalculateNextRun(from time.Time) (time.Time, error) {
	var next time.Time
	var err error

	switch j.Schedule.Type {
	case ScheduleTypeAt:
		// One-shot jobs don't have next runs
		next = time.Time{}
	case ScheduleTypeEvery:
		next = from.Add(j.Schedule.EveryDuration)
	case ScheduleTypeCron:
		next, err = parseCronExpression(j.Schedule.CronExpression, from)
		if err != nil {
			return time.Time{}, err
		}
	}

	// Apply stagger if configured
	if j.Schedule.StaggerDuration > 0 && !next.IsZero() {
		next = next.Add(j.Schedule.StaggerDuration)
	}

	j.State.NextRunAt = &next
	return next, nil
}

// IsOneShot returns true if this is a one-time job
func (j *Job) IsOneShot() bool {
	return j.Schedule.Type == ScheduleTypeAt
}

// ShouldDisableOnComplete returns true if job should be disabled after terminal status
func (j *Job) ShouldDisableOnComplete() bool {
	// One-shot jobs disable after any terminal status
	return j.IsOneShot()
}

// GetBackoffDelay calculates exponential backoff delay for consecutive errors
func GetBackoffDelay(consecutiveErrors int) time.Duration {
	// Backoff sequence: 30s, 1m, 5m, 15m, 60m (max)
	backoffs := []time.Duration{
		30 * time.Second,
		1 * time.Minute,
		5 * time.Minute,
		15 * time.Minute,
		60 * time.Minute,
	}

	if consecutiveErrors <= 0 {
		return 0
	}

	idx := consecutiveErrors - 1
	if idx >= len(backoffs) {
		return backoffs[len(backoffs)-1]
	}

	return backoffs[idx]
}

// RunLog represents a single run of a job
type RunLog struct {
	RunID       string                 `json:"run_id"`
	JobID       string                 `json:"job_id"`
	JobName     string                 `json:"job_name"`
	StartedAt   time.Time              `json:"started_at"`
	FinishedAt  time.Time              `json:"finished_at"`
	Status      string                 `json:"status"` // "ok", "error", "skipped"
	Error       string                 `json:"error,omitempty"`
	Duration    time.Duration          `json:"duration"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	// Additional telemetry
	Timestamp   time.Time              `json:"timestamp"`
}

// RunLogFilter defines filters for querying run logs
type RunLogFilter struct {
	JobID   string    `json:"job_id,omitempty"`
	After   time.Time `json:"after,omitempty"`
	Before  time.Time `json:"before,omitempty"`
	Status  string    `json:"status,omitempty"`
	Limit   int       `json:"limit,omitempty"`
	Offset  int       `json:"offset,omitempty"`
}

// CronConfig represents cron configuration
type CronConfig struct {
	Enabled           bool            `json:"enabled"`
	StorePath         string          `json:"store_path,omitempty"`
	MaxConcurrentRuns int             `json:"max_concurrent_runs,omitempty"`
	SessionRetention  time.Duration   `json:"session_retention,omitempty"` // Session cleanup retention
	RunLogConfig      RunLogConfig    `json:"run_log_config"`
	DefaultTimeout    time.Duration   `json:"default_timeout"` // Default job timeout
}

// RunLogConfig represents run log configuration
type RunLogConfig struct {
	MaxBytes int64 `json:"max_bytes,omitempty"` // Max file size before rotation
	KeepLines int  `json:"keep_lines,omitempty"` // Max lines to keep per job
}

// DefaultCronConfig returns default cron configuration
func DefaultCronConfig() CronConfig {
	return CronConfig{
		Enabled:           true,
		MaxConcurrentRuns: 1,
		SessionRetention:  24 * time.Hour,
		RunLogConfig: RunLogConfig{
			MaxBytes: 2 * 1024 * 1024, // 2MB
			KeepLines: 2000,
		},
		DefaultTimeout: 10 * time.Minute,
	}
}

// MarshalJSON implements custom JSON marshaling for Schedule
func (s Schedule) MarshalJSON() ([]byte, error) {
	type Alias Schedule
	aux := &struct {
		EveryDurationMs int64  `json:"every_duration_ms,omitempty"`
		StaggerDurationMs int64 `json:"stagger_duration_ms,omitempty"`
		AtISO           string `json:"at_iso,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(&s),
	}

	if !s.At.IsZero() {
		aux.AtISO = s.At.Format(time.RFC3339)
	}

	if s.EveryDuration > 0 {
		aux.EveryDurationMs = int64(s.EveryDuration / time.Millisecond)
	}

	if s.StaggerDuration > 0 {
		aux.StaggerDurationMs = int64(s.StaggerDuration / time.Millisecond)
	}

	return json.Marshal(aux)
}

// UnmarshalJSON implements custom JSON unmarshaling for Schedule
func (s *Schedule) UnmarshalJSON(data []byte) error {
	type Alias Schedule
	aux := &struct {
		EveryDurationMs int64  `json:"every_duration_ms,omitempty"`
		StaggerDurationMs int64 `json:"stagger_duration_ms,omitempty"`
		AtISO           string `json:"at_iso,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(s),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if aux.EveryDurationMs > 0 {
		s.EveryDuration = time.Duration(aux.EveryDurationMs) * time.Millisecond
	}

	if aux.StaggerDurationMs > 0 {
		s.StaggerDuration = time.Duration(aux.StaggerDurationMs) * time.Millisecond
	}

	if aux.AtISO != "" {
		if t, err := time.Parse(time.RFC3339, aux.AtISO); err == nil {
			s.At = t
		}
	}

	return nil
}
