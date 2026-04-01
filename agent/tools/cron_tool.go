package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/cron"
	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

// CronTool provides cron job management functionality
type CronTool struct {
	enabled bool
	service *cron.Service
}

// NewCronTool creates a new cron tool with an existing cron service
func NewCronTool(service *cron.Service) *CronTool {
	if service == nil {
		return &CronTool{enabled: false}
	}

	return &CronTool{
		enabled: true,
		service: service,
	}
}

// NewCronToolWithConfig creates a new cron tool and creates its own cron service
// Use this when you want the tool to manage its own cron service
func NewCronToolWithConfig(enabled bool, storePath string, messageBus *bus.MessageBus) *CronTool {
	if !enabled {
		return &CronTool{enabled: false}
	}

	cfg := cron.DefaultCronConfig()
	if storePath != "" {
		cfg.StorePath = storePath
	}

	service, err := cron.NewService(cfg, messageBus)
	if err != nil {
		return &CronTool{enabled: false}
	}

	return &CronTool{
		enabled: true,
		service: service,
	}
}

// GetTools returns both the legacy `cron` tool and new explicit `cron_*` tools
// The legacy tool provides backward compatibility and advanced features
// The new tools provide better LLM compatibility with explicit parameters
func (t *CronTool) GetTools() []Tool {
	if !t.enabled {
		return []Tool{}
	}

	tools := []Tool{
		// Legacy cron tool -保持了所有原有功能（向后兼容）
		// Supports: --every "1d", --at "2024-01-01T09:00:00Z", --system-event
		NewBaseTool(
			"cron",
			"Manage goclaw's built-in cron/scheduler service. This is the ONLY WAY to manage scheduled tasks in goclaw. DO NOT use system 'crontab' commands or any other scheduling methods.\n\nLegacy command format (supports all features):\n  add: Create job --name <name> --every <duration|\"1d\"|\"2h\"> | --at <RFC3339 time> | --cron <expr> --message <text> | --system-event <type>\n  list/ls: List all jobs\n  rm/remove: Delete job (requires job ID)\n  enable: Enable job (requires job ID)\n  disable: Disable job (requires job ID)\n  run: Run job immediately (requires job ID, optional --force)\n  status: Show service status\n  runs: Show run history (requires job ID)\n\nExamples:\n  cron command=\"add --name \\\"daily\\\" --every \\\"1d\\\" --message \\\"Run backup\\\"\"\n  cron command=\"add --name \\\"meeting\\\" --at \\\"2024-12-25T09:00:00Z\\\" --message \\\"Christmas meeting\\\"\"\n  cron command=\"list\"",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{
						"type":        "string",
						"description": "Cron command to execute. Examples: 'add --name \"daily backup\" --every \"1d\" --message \"run backup\"', 'add --name \"daily check\" --cron \"0 8,20 * * *\" --message \"check issues\"', 'list', 'rm job-abc123', 'enable job-abc123', 'disable job-abc123', 'run job-abc123 --force', 'status', 'runs job-abc123'",
					},
				},
				"required": []string{"command"},
			},
			t.Exec,
		),

		// New explicit tools for better LLM compatibility
		// cron_add - Add a new scheduled job
		NewBaseTool(
			"cron_add",
			"Add a new scheduled job. Use this when the user wants to schedule a reminder or task. For one-time reminders (e.g., 'remind me in 10 minutes'), use at_seconds. For recurring tasks (e.g., 'every day at 9am'), use every_seconds or cron_expr. IMPORTANT: You must specify exactly ONE schedule type: at_seconds (one-time), every_seconds (recurring interval), or cron_expr (complex schedule).",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "A short descriptive name for this job (e.g., 'daily_backup', 'meeting_reminder', 'weekly_report')",
					},
					"message": map[string]interface{}{
						"type":        "string",
						"description": "The message to display or execute when the job is triggered (e.g., 'Time to take your medicine', 'Run daily backup script')",
					},
					// Schedule options - exactly one must be provided
					"at_seconds": map[string]interface{}{
						"type":        "integer",
						"description": "For ONE-TIME reminders only: seconds from now when to trigger. Examples: 600 (10 minutes), 3600 (1 hour), 86400 (1 day). Do NOT use this for recurring tasks.",
					},
					"every_seconds": map[string]interface{}{
						"type":        "integer",
						"description": "For RECURRING tasks only: interval in seconds between runs. Examples: 3600 (every hour), 86400 (every day), 604800 (every week). Do NOT use this for one-time reminders.",
					},
					"cron_expr": map[string]interface{}{
						"type":        "string",
						"description": "For COMPLEX recurring schedules: standard cron expression. Examples: '0 9 * * *' (daily at 9am), '0 9 * * 1-5' (weekdays at 9am), '*/30 * * * *' (every 30 minutes). Format: minute hour day month weekday.",
					},
				},
				"required": []string{"name", "message"},
			},
			t.execAddExplicit,
		),

		// cron_list - List all jobs
		NewBaseTool(
			"cron_list",
			"List all scheduled jobs with their details including ID, name, schedule, status, and next run time. Use this to show the user all their scheduled tasks.",
			map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
			t.execList,
		),

		// cron_remove - Remove a job
		NewBaseTool(
			"cron_remove",
			"Remove/delete a scheduled job permanently. Use this when the user wants to cancel or delete a scheduled task. You need the job_id from cron_list first.",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"job_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the job to remove (get it from cron_list command output, looks like 'job-abc123')",
					},
				},
				"required": []string{"job_id"},
			},
			t.execRemove,
		),

		// cron_enable - Enable a job
		NewBaseTool(
			"cron_enable",
			"Enable a disabled scheduled job so it will run according to its schedule. Use this when the user wants to resume a paused task.",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"job_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the job to enable (get it from cron_list)",
					},
				},
				"required": []string{"job_id"},
			},
			t.execEnable,
		),

		// cron_disable - Disable a job
		NewBaseTool(
			"cron_disable",
			"Disable a scheduled job (it will remain defined but won't run). Use this when the user wants to temporarily pause a scheduled task without deleting it.",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"job_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the job to disable (get it from cron_list)",
					},
				},
				"required": []string{"job_id"},
			},
			t.execDisable,
		),

		// cron_run - Run a job immediately
		NewBaseTool(
			"cron_run",
			"Run a scheduled job immediately, regardless of its schedule. Use this when the user wants to trigger a scheduled task right now instead of waiting for its next scheduled time.",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"job_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the job to run immediately (get it from cron_list)",
					},
					"force": map[string]interface{}{
						"type":        "boolean",
						"description": "If true, run even if the job is currently running (default: false). Use this to force-run a job that might already be running.",
					},
				},
				"required": []string{"job_id"},
			},
			t.execRun,
		),

		// cron_status - Show cron service status
		NewBaseTool(
			"cron_status",
			"Show the overall status of the cron service including total jobs, how many are enabled/disabled, and how many are currently running. Use this to check if the scheduling system is working.",
			map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
			t.execStatus,
		),

		// cron_runs - Show job run history
		NewBaseTool(
			"cron_runs",
			"Show the execution history for a specific job including past run times, status, duration, and any errors. Use this to diagnose problems with scheduled tasks.",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"job_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the job to show history for (get it from cron_list)",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of past runs to show (default: 10, maximum: 100)",
						"minimum":     1,
						"maximum":     100,
					},
				},
				"required": []string{"job_id"},
			},
			t.execRuns,
		),
	}

	return tools
}

// ============================================================
// Legacy Exec method -保持了所有原有功能（向后兼容）
// Supports: --every "1d", --at RFC3339, --system-event
// ============================================================

// Exec executes a cron command (legacy interface for backward compatibility)
func (t *CronTool) Exec(ctx context.Context, params map[string]interface{}) (string, error) {
	if !t.enabled {
		return "", fmt.Errorf("cron tool is disabled")
	}

	command, ok := params["command"].(string)
	if !ok {
		return "", fmt.Errorf("command parameter is required")
	}

	logger.Info("[CronTool] Executing command",
		zap.String("command", command))

	// Parse the command using shell-style parsing to preserve quoted strings
	parts, parseErr := parseCommandArgs(command)
	if parseErr != nil {
		return "", fmt.Errorf("failed to parse command: %w", parseErr)
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("empty command")
	}

	var result string
	var err error

	switch parts[0] {
	case "add":
		result, err = t.execAddLegacy(ctx, parts[1:])
	case "list", "ls":
		result, err = t.execList(ctx, nil)
	case "rm", "remove":
		result, err = t.execRemove(ctx, map[string]interface{}{"job_id": parts[1]})
	case "enable":
		result, err = t.execEnable(ctx, map[string]interface{}{"job_id": parts[1]})
	case "disable":
		result, err = t.execDisable(ctx, map[string]interface{}{"job_id": parts[1]})
	case "run":
		result, err = t.execRunLegacy(ctx, parts[1:])
	case "status":
		result, err = t.execStatus(ctx, nil)
	case "runs":
		result, err = t.execRunsLegacy(ctx, parts[1:])
	default:
		err = fmt.Errorf("unknown cron command: %s (available: add, list, rm, enable, disable, run, status, runs)", parts[0])
	}

	logger.Info("[CronTool] Command execution completed",
		zap.String("command", command),
		zap.Int("result_length", len(result)),
		zap.Bool("has_error", err != nil),
		zap.Error(err))

	if err != nil {
		return "", err
	}
	return result, nil
}

// execAddLegacy adds a new cron job (legacy version with all features)
func (t *CronTool) execAddLegacy(ctx context.Context, args []string) (string, error) {
	// Parse flags
	var name, message, systemEvent string
	var every, at, cronExpr string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--name":
			if i+1 < len(args) {
				name = args[i+1]
				i++
			}
		case "--message":
			if i+1 < len(args) {
				message = args[i+1]
				i++
			}
		case "--system-event":
			if i+1 < len(args) {
				systemEvent = args[i+1]
				i++
			}
		case "--every":
			if i+1 < len(args) {
				every = args[i+1]
				i++
			}
		case "--at":
			if i+1 < len(args) {
				at = args[i+1]
				i++
			}
		case "--cron":
			if i+1 < len(args) {
				cronExpr = args[i+1]
				i++
			}
		}
	}

	if name == "" {
		return "", fmt.Errorf("--name is required")
	}

	// Determine schedule
	var scheduleType cron.ScheduleType
	var scheduleConfig cron.Schedule

	count := 0
	if cronExpr != "" {
		count++
		scheduleType = cron.ScheduleTypeCron
		scheduleConfig.CronExpression = cronExpr
	}
	if every != "" {
		count++
		scheduleType = cron.ScheduleTypeEvery
		duration, err := cron.ParseHumanDuration(every)
		if err != nil {
			return "", fmt.Errorf("invalid interval: %w", err)
		}
		scheduleConfig.EveryDuration = duration
	}
	if at != "" {
		count++
		scheduleType = cron.ScheduleTypeAt
		parsedTime, err := time.Parse(time.RFC3339, at)
		if err != nil {
			return "", fmt.Errorf("invalid time format: %w", err)
		}
		scheduleConfig.At = parsedTime
	}

	if count == 0 {
		return "", fmt.Errorf("must specify one of: --cron <expr>, --every <duration>, --at <time>")
	}
	if count > 1 {
		return "", fmt.Errorf("can only specify one of: --cron, --every, --at")
	}

	scheduleConfig.Type = scheduleType

	// Determine payload
	var payload cron.Payload
	payloadCount := 0
	if message != "" {
		payloadCount++
		payload.Type = cron.PayloadTypeAgentTurn
		payload.Message = message
	}
	if systemEvent != "" {
		payloadCount++
		payload.Type = cron.PayloadTypeSystemEvent
		payload.SystemEventType = systemEvent
	}

	if payloadCount == 0 {
		return "", fmt.Errorf("must specify one of: --message <text>, --system-event <type>")
	}
	if payloadCount > 1 {
		return "", fmt.Errorf("can only specify one of: --message, --system-event")
	}

	job := &cron.Job{
		Name:          name,
		Schedule:      scheduleConfig,
		SessionTarget: cron.SessionTargetMain,
		WakeMode:      cron.WakeModeNow,
		Payload:       payload,
		State: cron.JobState{
			Enabled: true,
		},
	}

	if err := t.service.AddJob(job); err != nil {
		return "", fmt.Errorf("failed to add job: %w", err)
	}

	return fmt.Sprintf("Job '%s' added with ID: %s\nSchedule: %s\nPayload: %s",
		name, job.ID, formatSchedule(scheduleConfig), formatPayload(payload)), nil
}

// execRunLegacy runs a job immediately (legacy version)
func (t *CronTool) execRunLegacy(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("job ID is required")
	}

	jobID := args[0]
	force := false
	for _, arg := range args[1:] {
		if arg == "--force" {
			force = true
		}
	}

	if err := t.service.RunJob(ctx, jobID, force); err != nil {
		return "", fmt.Errorf("failed to run job: %w", err)
	}

	return fmt.Sprintf("Job '%s' executed successfully", jobID), nil
}

// execRunsLegacy shows run history for a job (legacy version)
func (t *CronTool) execRunsLegacy(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("job ID is required")
	}

	jobID := args[0]
	limit := 10

	filter := cron.RunLogFilter{
		JobID: jobID,
		Limit: limit,
	}

	runs, err := t.service.GetRunLogs(jobID, filter)
	if err != nil {
		return "", fmt.Errorf("failed to get run history: %w", err)
	}

	if len(runs) == 0 {
		return fmt.Sprintf("No run history found for job '%s'", jobID), nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Run History for Job '%s' (last %d runs):\n\n", jobID, len(runs)))

	for i, run := range runs {
		output.WriteString(fmt.Sprintf("%d. %s\n", i+1, run.StartedAt.Format(time.RFC3339)))
		output.WriteString(fmt.Sprintf("   Status: %s\n", run.Status))
		output.WriteString(fmt.Sprintf("   Duration: %v\n", run.Duration))
		if run.Error != "" {
			output.WriteString(fmt.Sprintf("   Error: %s\n", run.Error))
		}
		output.WriteString("\n")
	}

	return output.String(), nil
}

// ============================================================
// New explicit tool methods - better LLM compatibility
// ============================================================

// execAddExplicit adds a new cron job (new explicit parameters version)
func (t *CronTool) execAddExplicit(ctx context.Context, params map[string]interface{}) (string, error) {
	if !t.enabled {
		return "", fmt.Errorf("cron tool is disabled")
	}

	name, _ := params["name"].(string)
	message, _ := params["message"].(string)

	// Check schedule parameters - support both float64 and int types
	var hasAt, hasEvery, hasCron bool
	var atSeconds, everySeconds float64
	var cronExpr string

	if v, ok := params["at_seconds"]; ok {
		switch val := v.(type) {
		case float64:
			atSeconds = val
			hasAt = true
		case int:
			atSeconds = float64(val)
			hasAt = true
		case int64:
			atSeconds = float64(val)
			hasAt = true
		}
	}

	if v, ok := params["every_seconds"]; ok {
		switch val := v.(type) {
		case float64:
			everySeconds = val
			hasEvery = true
		case int:
			everySeconds = float64(val)
			hasEvery = true
		case int64:
			everySeconds = float64(val)
			hasEvery = true
		}
	}

	if v, ok := params["cron_expr"]; ok {
		if str, ok := v.(string); ok {
			cronExpr = str
			hasCron = true
		}
	}

	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	if message == "" {
		return "", fmt.Errorf("message is required")
	}

	logger.Info("[CronTool] Adding job",
		zap.String("name", name),
		zap.Bool("has_at_seconds", hasAt),
		zap.Bool("has_every_seconds", hasEvery),
		zap.Bool("has_cron_expr", hasCron))

	// Determine schedule - exactly one must be provided
	var scheduleType cron.ScheduleType
	var scheduleConfig cron.Schedule
	scheduleCount := 0

	if hasAt {
		scheduleCount++
		scheduleType = cron.ScheduleTypeAt
		scheduleConfig.At = time.Now().Add(time.Duration(atSeconds) * time.Second)
	}
	if hasEvery {
		scheduleCount++
		scheduleType = cron.ScheduleTypeEvery
		scheduleConfig.EveryDuration = time.Duration(everySeconds) * time.Second
	}
	if hasCron {
		scheduleCount++
		scheduleType = cron.ScheduleTypeCron
		scheduleConfig.CronExpression = cronExpr
	}

	if scheduleCount == 0 {
		return "", fmt.Errorf("must specify exactly one schedule type: at_seconds (for one-time), every_seconds (for recurring), or cron_expr (for complex schedules)")
	}
	if scheduleCount > 1 {
		return "", fmt.Errorf("can only specify one schedule type: use either at_seconds, every_seconds, or cron_expr (not multiple)")
	}

	scheduleConfig.Type = scheduleType

	// Create payload
	payload := cron.Payload{
		Type:    cron.PayloadTypeAgentTurn,
		Message: message,
	}

	job := &cron.Job{
		Name:          name,
		Schedule:      scheduleConfig,
		SessionTarget: cron.SessionTargetMain,
		WakeMode:      cron.WakeModeNow,
		Payload:       payload,
		State: cron.JobState{
			Enabled: true,
		},
	}

	if err := t.service.AddJob(job); err != nil {
		return "", fmt.Errorf("failed to add job: %w", err)
	}

	return fmt.Sprintf("Job '%s' added successfully\nID: %s\nSchedule: %s\nMessage: %s",
		name, job.ID, formatSchedule(scheduleConfig), formatPayload(payload)), nil
}

// execList lists all cron jobs
func (t *CronTool) execList(ctx context.Context, params map[string]interface{}) (string, error) {
	if !t.enabled {
		return "", fmt.Errorf("cron tool is disabled")
	}

	jobs := t.service.ListJobs()

	if len(jobs) == 0 {
		return "No scheduled jobs found. Use cron_add or 'cron command=\"add ...\"' to create one.", nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Found %d scheduled job(s):\n\n", len(jobs)))

	for _, job := range jobs {
		status := "✓ enabled"
		if !job.State.Enabled {
			status = "✗ disabled"
		}
		output.WriteString(fmt.Sprintf("%s (%s)\n", job.ID, status))
		output.WriteString(fmt.Sprintf("  Name: %s\n", job.Name))
		output.WriteString(fmt.Sprintf("  Schedule: %s\n", formatSchedule(job.Schedule)))
		output.WriteString(fmt.Sprintf("  Message: %s\n", job.Payload.Message))
		output.WriteString(fmt.Sprintf("  Next Run: %s\n", formatTimePtr(job.State.NextRunAt)))
		output.WriteString(fmt.Sprintf("  Last Run: %s\n", formatTimePtr(job.State.LastRunAt)))
		if job.State.ConsecutiveErrors > 0 {
			output.WriteString(fmt.Sprintf("  ⚠ Consecutive Errors: %d\n", job.State.ConsecutiveErrors))
		}
		output.WriteString("\n")
	}

	return output.String(), nil
}

// execRemove removes a cron job
func (t *CronTool) execRemove(ctx context.Context, params map[string]interface{}) (string, error) {
	if !t.enabled {
		return "", fmt.Errorf("cron tool is disabled")
	}

	jobID, ok := params["job_id"].(string)
	if !ok || jobID == "" {
		return "", fmt.Errorf("job_id is required")
	}

	logger.Info("[CronTool] Removing job", zap.String("job_id", jobID))

	if err := t.service.RemoveJob(jobID); err != nil {
		return "", fmt.Errorf("failed to remove job: %w", err)
	}

	return fmt.Sprintf("Job '%s' has been removed", jobID), nil
}

// execEnable enables a cron job
func (t *CronTool) execEnable(ctx context.Context, params map[string]interface{}) (string, error) {
	if !t.enabled {
		return "", fmt.Errorf("cron tool is disabled")
	}

	jobID, ok := params["job_id"].(string)
	if !ok || jobID == "" {
		return "", fmt.Errorf("job_id is required")
	}

	logger.Info("[CronTool] Enabling job", zap.String("job_id", jobID))

	if err := t.service.EnableJob(jobID); err != nil {
		return "", fmt.Errorf("failed to enable job: %w", err)
	}

	return fmt.Sprintf("Job '%s' has been enabled", jobID), nil
}

// execDisable disables a cron job
func (t *CronTool) execDisable(ctx context.Context, params map[string]interface{}) (string, error) {
	if !t.enabled {
		return "", fmt.Errorf("cron tool is disabled")
	}

	jobID, ok := params["job_id"].(string)
	if !ok || jobID == "" {
		return "", fmt.Errorf("job_id is required")
	}

	logger.Info("[CronTool] Disabling job", zap.String("job_id", jobID))

	if err := t.service.DisableJob(jobID); err != nil {
		return "", fmt.Errorf("failed to disable job: %w", err)
	}

	return fmt.Sprintf("Job '%s' has been disabled", jobID), nil
}

// execRun runs a job immediately
func (t *CronTool) execRun(ctx context.Context, params map[string]interface{}) (string, error) {
	if !t.enabled {
		return "", fmt.Errorf("cron tool is disabled")
	}

	jobID, ok := params["job_id"].(string)
	if !ok || jobID == "" {
		return "", fmt.Errorf("job_id is required")
	}

	force, _ := params["force"].(bool)

	logger.Info("[CronTool] Running job",
		zap.String("job_id", jobID),
		zap.Bool("force", force))

	if err := t.service.RunJob(ctx, jobID, force); err != nil {
		return "", fmt.Errorf("failed to run job: %w", err)
	}

	return fmt.Sprintf("Job '%s' has been executed successfully", jobID), nil
}

// execStatus shows cron service status
func (t *CronTool) execStatus(ctx context.Context, params map[string]interface{}) (string, error) {
	if !t.enabled {
		return "", fmt.Errorf("cron tool is disabled")
	}

	status := t.service.GetStatus()

	var output strings.Builder
	output.WriteString("📅 Cron Service Status\n")
	output.WriteString(fmt.Sprintf("  Running: %v\n", status["running"]))
	output.WriteString(fmt.Sprintf("  Total Jobs: %v\n", status["total_jobs"]))
	output.WriteString(fmt.Sprintf("  Enabled Jobs: %v\n", status["enabled_jobs"]))
	output.WriteString(fmt.Sprintf("  Disabled Jobs: %v\n", status["disabled_jobs"]))
	output.WriteString(fmt.Sprintf("  Running Jobs: %v\n", status["running_jobs"]))

	return output.String(), nil
}

// execRuns shows run history for a job
func (t *CronTool) execRuns(ctx context.Context, params map[string]interface{}) (string, error) {
	if !t.enabled {
		return "", fmt.Errorf("cron tool is disabled")
	}

	jobID, ok := params["job_id"].(string)
	if !ok || jobID == "" {
		return "", fmt.Errorf("job_id is required")
	}

	limit := 10
	if limitFloat, ok := params["limit"].(float64); ok {
		limit = int(limitFloat)
		if limit < 1 {
			limit = 1
		}
		if limit > 100 {
			limit = 100
		}
	}

	logger.Info("[CronTool] Getting run history",
		zap.String("job_id", jobID),
		zap.Int("limit", limit))

	filter := cron.RunLogFilter{
		JobID: jobID,
		Limit: limit,
	}

	runs, err := t.service.GetRunLogs(jobID, filter)
	if err != nil {
		return "", fmt.Errorf("failed to get run history: %w", err)
	}

	if len(runs) == 0 {
		return fmt.Sprintf("No run history found for job '%s'", jobID), nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("📜 Run History for Job '%s' (showing last %d runs):\n\n", jobID, len(runs)))

	for i, run := range runs {
		statusIcon := "✓"
		if run.Status != "success" {
			statusIcon = "✗"
		}
		output.WriteString(fmt.Sprintf("%d. %s %s\n", i+1, statusIcon, run.StartedAt.Format(time.RFC3339)))
		output.WriteString(fmt.Sprintf("   Status: %s\n", run.Status))
		output.WriteString(fmt.Sprintf("   Duration: %v\n", run.Duration))
		if run.Error != "" {
			output.WriteString(fmt.Sprintf("   Error: %s\n", run.Error))
		}
		output.WriteString("\n")
	}

	return output.String(), nil
}

// ============================================================
// Helper functions
// ============================================================

func formatSchedule(schedule cron.Schedule) string {
	switch schedule.Type {
	case cron.ScheduleTypeAt:
		return fmt.Sprintf("one-time at %s", schedule.At.Format("2006-01-02 15:04:05"))
	case cron.ScheduleTypeEvery:
		return fmt.Sprintf("recurring every %s", cron.FormatDuration(schedule.EveryDuration))
	case cron.ScheduleTypeCron:
		return fmt.Sprintf("cron expression: %s", schedule.CronExpression)
	default:
		return "unknown"
	}
}

func formatPayload(payload cron.Payload) string {
	switch payload.Type {
	case cron.PayloadTypeAgentTurn:
		return payload.Message
	case cron.PayloadTypeSystemEvent:
		return "event: " + payload.SystemEventType
	default:
		return "unknown"
	}
}

func formatTimePtr(t *time.Time) string {
	if t == nil {
		return "not scheduled"
	}
	return t.Format("2006-01-02 15:04:05")
}

// parseCommandArgs parses a command string with support for quoted arguments
// This handles shell-style quoting to preserve spaces within quoted strings
func parseCommandArgs(command string) ([]string, error) {
	var args []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)

	for _, ch := range command {
		switch {
		case ch == '"' || ch == '\'':
			if !inQuote {
				// Start of quoted section
				inQuote = true
				quoteChar = ch
			} else if ch == quoteChar {
				// End of quoted section
				inQuote = false
				quoteChar = 0
			} else {
				// Different quote character inside quotes
				current.WriteRune(ch)
			}
		case ch == ' ' || ch == '\t':
			if inQuote {
				// Space inside quotes - preserve it
				current.WriteRune(ch)
			} else if current.Len() > 0 {
				// Space outside quotes - end of argument
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(ch)
		}
	}

	// Add final argument if any
	if current.Len() > 0 {
		args = append(args, current.String())
	}

	// Check for unclosed quote
	if inQuote {
		return nil, fmt.Errorf("unclosed quote in command")
	}

	return args, nil
}
