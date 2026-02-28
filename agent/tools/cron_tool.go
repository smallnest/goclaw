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

// Exec executes a cron command
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
		result, err = t.execAdd(ctx, parts[1:])
	case "list", "ls":
		result, err = t.execList(ctx)
	case "rm", "remove":
		result, err = t.execRemove(ctx, parts[1:])
	case "enable":
		result, err = t.execEnable(ctx, parts[1:])
	case "disable":
		result, err = t.execDisable(ctx, parts[1:])
	case "run":
		result, err = t.execRun(ctx, parts[1:])
	case "status":
		result, err = t.execStatus(ctx)
	case "runs":
		result, err = t.execRuns(ctx, parts[1:])
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

// execAdd adds a new cron job
func (t *CronTool) execAdd(ctx context.Context, args []string) (string, error) {
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

// execList lists all cron jobs
func (t *CronTool) execList(ctx context.Context) (string, error) {
	jobs := t.service.ListJobs()

	if len(jobs) == 0 {
		return "No jobs found", nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Found %d job(s):\n\n", len(jobs)))

	for _, job := range jobs {
		status := "enabled"
		if !job.State.Enabled {
			status = "disabled"
		}
		output.WriteString(fmt.Sprintf("%s (%s)\n", job.ID, status))
		output.WriteString(fmt.Sprintf("  Name: %s\n", job.Name))
		output.WriteString(fmt.Sprintf("  Schedule: %s\n", formatSchedule(job.Schedule)))
		output.WriteString(fmt.Sprintf("  Payload: %s\n", formatPayload(job.Payload)))
		output.WriteString(fmt.Sprintf("  Next Run: %s\n", formatTimePtr(job.State.NextRunAt)))
		output.WriteString(fmt.Sprintf("  Last Run: %s\n", formatTimePtr(job.State.LastRunAt)))
		if job.State.ConsecutiveErrors > 0 {
			output.WriteString(fmt.Sprintf("  Consecutive Errors: %d\n", job.State.ConsecutiveErrors))
		}
		output.WriteString("\n")
	}

	return output.String(), nil
}

// execRemove removes a cron job
func (t *CronTool) execRemove(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("job ID is required")
	}

	jobID := args[0]

	if err := t.service.RemoveJob(jobID); err != nil {
		return "", fmt.Errorf("failed to remove job: %w", err)
	}

	return fmt.Sprintf("Job '%s' removed", jobID), nil
}

// execEnable enables a cron job
func (t *CronTool) execEnable(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("job ID is required")
	}

	jobID := args[0]

	if err := t.service.EnableJob(jobID); err != nil {
		return "", fmt.Errorf("failed to enable job: %w", err)
	}

	return fmt.Sprintf("Job '%s' enabled", jobID), nil
}

// execDisable disables a cron job
func (t *CronTool) execDisable(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("job ID is required")
	}

	jobID := args[0]

	if err := t.service.DisableJob(jobID); err != nil {
		return "", fmt.Errorf("failed to disable job: %w", err)
	}

	return fmt.Sprintf("Job '%s' disabled", jobID), nil
}

// execRun runs a job immediately
func (t *CronTool) execRun(ctx context.Context, args []string) (string, error) {
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

// execStatus shows cron service status
func (t *CronTool) execStatus(ctx context.Context) (string, error) {
	status := t.service.GetStatus()

	var output strings.Builder
	output.WriteString("Cron Service Status:\n")
	output.WriteString(fmt.Sprintf("  Running: %v\n", status["running"]))
	output.WriteString(fmt.Sprintf("  Total Jobs: %v\n", status["total_jobs"]))
	output.WriteString(fmt.Sprintf("  Enabled Jobs: %v\n", status["enabled_jobs"]))
	output.WriteString(fmt.Sprintf("  Disabled Jobs: %v\n", status["disabled_jobs"]))
	output.WriteString(fmt.Sprintf("  Running Jobs: %v\n", status["running_jobs"]))

	return output.String(), nil
}

// execRuns shows run history for a job
func (t *CronTool) execRuns(ctx context.Context, args []string) (string, error) {
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

// GetTools returns the tool definitions for the cron tool
func (t *CronTool) GetTools() []Tool {
	if !t.enabled {
		return []Tool{}
	}

	return []Tool{
		NewBaseTool(
			"cron",
			"Manage goclaw's built-in cron/scheduler service. This is the ONLY WAY to manage scheduled tasks in goclaw. DO NOT use system 'crontab' commands or any other scheduling methods. All scheduled task operations (create, list, view, edit, delete, enable, disable, run) MUST be done through this tool. Supported commands: add (create job), list/ls (list all jobs), rm/remove (delete job), enable (enable job), disable (disable job), run (execute job immediately), status (show service status), runs (show run history).",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{
						"type":        "string",
						"description": "Cron command to execute. Examples: 'add --name \"daily backup\" --every \"1d\" --message \"run backup.sh\"', 'add --name \"daily check\" --cron \"0 8,20 * * *\" --message \"check GitHub issues\"', 'list' (view all jobs), 'rm job-abc123' (delete), 'enable job-abc123', 'disable job-abc123', 'run job-abc123 --force', 'status', 'runs job-abc123'",
					},
				},
				"required": []string{"command"},
			},
			t.Exec,
		),
	}
}

// Helper functions

func formatSchedule(schedule cron.Schedule) string {
	switch schedule.Type {
	case cron.ScheduleTypeAt:
		return "at " + schedule.At.Format(time.RFC3339)
	case cron.ScheduleTypeEvery:
		return "every " + cron.FormatDuration(schedule.EveryDuration)
	case cron.ScheduleTypeCron:
		return schedule.CronExpression
	default:
		return "unknown"
	}
}

func formatPayload(payload cron.Payload) string {
	switch payload.Type {
	case cron.PayloadTypeAgentTurn:
		return "message: " + payload.Message
	case cron.PayloadTypeSystemEvent:
		return "event: " + payload.SystemEventType
	default:
		return "unknown"
	}
}

func formatTimePtr(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format(time.RFC3339)
}

// parseCommandArgs parses a command string with support for quoted arguments
// This handles shell-style quoting to preserve spaces within quoted strings
func parseCommandArgs(command string) ([]string, error) {
	var args []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)

	for i, ch := range command {
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
		_ = i // avoid unused variable warning
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
