package cron

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

// JobExecutor handles execution of cron jobs
type JobExecutor struct {
	bus        *bus.MessageBus
	runLogger  *RunLogger
	timeout    time.Duration
}

// NewJobExecutor creates a new job executor
func NewJobExecutor(bus *bus.MessageBus, runLogger *RunLogger, timeout time.Duration) *JobExecutor {
	if timeout == 0 {
		timeout = 10 * time.Minute
	}

	return &JobExecutor{
		bus:       bus,
		runLogger: runLogger,
		timeout:   timeout,
	}
}

// Execute executes a cron job
func (e *JobExecutor) Execute(ctx context.Context, job *Job) error {
	startTime := time.Now()
	runID := uuid.New().String()

	logger.Info("Executing cron job",
		zap.String("job_id", job.ID),
		zap.String("run_id", runID),
		zap.String("name", job.Name),
	)

	// Check if bus is available
	if e.bus == nil {
		return fmt.Errorf("message bus is not available - cron service must be initialized with a valid message bus")
	}

	// Create run log entry
	runLog := &RunLog{
		RunID:     runID,
		JobID:     job.ID,
		JobName:   job.Name,
		StartedAt: startTime,
		Status:    "running",
		Metadata:  make(map[string]interface{}),
		Timestamp: startTime,
	}

	var status string
	var errMsg string

	// Execute based on payload type
	switch job.Payload.Type {
	case PayloadTypeSystemEvent:
		err := e.executeSystemEvent(ctx, job)
		if err != nil {
			status = "error"
			errMsg = err.Error()
		} else {
			status = "ok"
		}

	case PayloadTypeAgentTurn:
		err := e.executeAgentTurn(ctx, job)
		if err != nil {
			status = "error"
			errMsg = err.Error()
		} else {
			status = "ok"
		}

	default:
		status = "error"
		errMsg = fmt.Sprintf("unknown payload type: %s", job.Payload.Type)
	}

	// Update run log
	finishTime := time.Now()
	runLog.FinishedAt = finishTime
	runLog.Status = status
	runLog.Error = errMsg
	runLog.Duration = finishTime.Sub(startTime)

	// Log the run
	if e.runLogger != nil {
		if err := e.runLogger.LogRun(runLog); err != nil {
			logger.Error("Failed to log run",
				zap.String("job_id", job.ID),
				zap.Error(err),
			)
		}
	}

	// Handle delivery
	if job.Delivery != nil && job.Delivery.Mode != DeliveryModeNone {
		if err := e.deliverResult(ctx, job, runLog); err != nil {
			if !job.Delivery.BestEffort {
				return fmt.Errorf("delivery failed: %w", err)
			}
			logger.Warn("Job delivery failed (best effort)",
				zap.String("job_id", job.ID),
				zap.Error(err),
			)
		}
	}

	logger.Info("Cron job execution completed",
		zap.String("job_id", job.ID),
		zap.String("run_id", runID),
		zap.String("status", status),
		zap.Duration("duration", runLog.Duration),
	)

	if errMsg != "" {
		return fmt.Errorf("job execution failed: %s", errMsg)
	}

	return nil
}

// executeSystemEvent executes a system event job
func (e *JobExecutor) executeSystemEvent(ctx context.Context, job *Job) error {
	// Publish system event to bus
	msg := &bus.InboundMessage{
		Channel:  "cron",
		SenderID: job.ID,
		Content:  job.Payload.SystemEventType,
		Metadata: map[string]interface{}{
			"job_id":         job.ID,
			"job_name":       job.Name,
			"scheduled":      true,
			"event_type":     job.Payload.SystemEventType,
			"is_system_event": true,
		},
		Timestamp: time.Now(),
	}

	return e.bus.PublishInbound(ctx, msg)
}

// executeAgentTurn executes an agent turn job
func (e *JobExecutor) executeAgentTurn(ctx context.Context, job *Job) error {
	// Publish message to bus for agent processing
	msg := &bus.InboundMessage{
		Channel:  "cron",
		SenderID: job.ID,
		Content:  job.Payload.Message,
		Metadata: map[string]interface{}{
			"job_id":    job.ID,
			"job_name":  job.Name,
			"scheduled": true,
			"is_cron":   true,
		},
		Timestamp: time.Now(),
	}

	return e.bus.PublishInbound(ctx, msg)
}

// deliverResult delivers the job result
func (e *JobExecutor) deliverResult(ctx context.Context, job *Job, runLog *RunLog) error {
	if job.Delivery == nil {
		return nil
	}

	switch job.Delivery.Mode {
	case DeliveryModeAnnounce:
		return e.deliverAnnounce(ctx, job, runLog)
	case DeliveryModeWebhook:
		return e.deliverWebhook(ctx, job, runLog)
	default:
		return nil
	}
}

// deliverAnnounce delivers result to chat channels
func (e *JobExecutor) deliverAnnounce(ctx context.Context, job *Job, runLog *RunLog) error {
	// Create outbound message
	content := job.Payload.Message
	if content == "" {
		content = fmt.Sprintf("Job '%s' completed", job.Name)
	}

	msg := &bus.OutboundMessage{
		ChatID:   job.ID, // Use job ID as chat ID for announce
		Content:  content,
		Metadata: map[string]interface{}{
			"job_id":    job.ID,
			"run_id":    runLog.RunID,
			"status":    runLog.Status,
			"duration":  runLog.Duration.String(),
		},
	}

	// Publish to outbound bus
	return e.bus.PublishOutbound(ctx, msg)
}

// deliverWebhook delivers result via HTTP webhook
func (e *JobExecutor) deliverWebhook(ctx context.Context, job *Job, runLog *RunLog) error {
	// TODO: Implement HTTP webhook delivery
	// For now, just log
	logger.Info("Webhook delivery (not yet implemented)",
		zap.String("job_id", job.ID),
		zap.String("url", job.Delivery.WebhookURL),
	)
	return nil
}
