package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/smallnest/goclaw/acp"
	"github.com/smallnest/goclaw/config"
)

// Ensure SpawnAcpTool implements Tool
var _ Tool = (*SpawnAcpTool)(nil)

// SpawnAcpTool is a tool that spawns new ACP sessions for coding tasks.
type SpawnAcpTool struct {
	cfg        *config.Config
	acpManager *acp.Manager
}

// NewSpawnAcpTool creates a new spawn_acp tool.
func NewSpawnAcpTool(cfg *config.Config, acpManager *acp.Manager) *SpawnAcpTool {
	return &SpawnAcpTool{
		cfg:        cfg,
		acpManager: acpManager,
	}
}

// Name returns the tool name.
func (t *SpawnAcpTool) Name() string {
	return "spawn_acp"
}

// Description returns the tool description.
func (t *SpawnAcpTool) Description() string {
	return `Spawns a new ACP (Agent Client Protocol) session for coding tasks. ACP sessions provide advanced coding capabilities with direct IDE integration and file system access.

Parameters:
- task (required): The coding task or description to execute
- label (optional): A descriptive label for the ACP session
- agent_id (optional): The agent ID to use (defaults to configured default)
- cwd (optional): Working directory for the session
- mode (optional): "run" for oneshot (default) or "session" for persistent
- thread (optional): Whether to bind to the current thread for persistent sessions

Modes:
- "run": Creates a oneshot session that closes after completing the task
- "session": Creates a persistent session that stays active (requires thread=true)

Thread-bound sessions:
- When thread=true, the ACP session is bound to the current conversation thread
- Follow-up messages in the thread will be sent to the ACP session
- The session remains active until idle timeout or max age is reached

Example usage:
{ "task": "Refactor the user authentication code", "mode": "run" }
{ "task": "Set up a development environment", "mode": "session", "thread": true }`
}

// Label returns the tool label.
func (t *SpawnAcpTool) Label() string {
	return "ACP Session Spawner"
}

// Parameters returns the tool parameters schema.
func (t *SpawnAcpTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task": map[string]any{
				"type":        "string",
				"description": "The coding task or description to execute",
			},
			"label": map[string]any{
				"type":        "string",
				"description": "A descriptive label for the ACP session",
			},
			"agent_id": map[string]any{
				"type":        "string",
				"description": "The agent ID to use (defaults to configured default)",
			},
			"cwd": map[string]any{
				"type":        "string",
				"description": "Working directory for the session",
			},
			"mode": map[string]any{
				"type":        "string",
				"enum":        []string{"run", "session"},
				"description": "Session mode: 'run' for oneshot, 'session' for persistent",
			},
			"thread": map[string]any{
				"type":        "boolean",
				"description": "Whether to bind to the current thread (for persistent sessions)",
			},
		},
		"required": []string{"task"},
	}
}

// Execute executes the spawn_acp tool.
func (t *SpawnAcpTool) Execute(ctx context.Context, params map[string]any, onUpdate func(ToolResult)) (ToolResult, error) {
	// Extract parameters
	task, ok := params["task"].(string)
	if !ok || task == "" {
		return ToolResult{}, fmt.Errorf("task parameter is required and must be a string")
	}

	label, _ := params["label"].(string)
	agentID, _ := params["agent_id"].(string)
	cwd, _ := params["cwd"].(string)
	modeStr, _ := params["mode"].(string)
	thread, _ := params["thread"].(bool)

	// Determine spawn mode
	var mode acp.SpawnAcpMode
	switch modeStr {
	case "session":
		mode = acp.SpawnModeSession
	case "run", "":
		mode = acp.SpawnModeRun
	default:
		return ToolResult{}, fmt.Errorf("invalid mode: %s (must be 'run' or 'session')", modeStr)
	}

	// Validate session mode requires thread
	if mode == acp.SpawnModeSession && !thread {
		return ToolResult{}, fmt.Errorf(`mode="session" requires thread=true so the ACP session can stay bound to a thread`)
	}

	// Build spawn context from current agent session context.
	spawnCtx := acp.SpawnAcpContext{}
	if sessionKey, ok := ctx.Value(SessionKeyContextKey).(string); ok && sessionKey != "" {
		derivedCtx, deriveErr := buildSpawnContextFromSessionKey(sessionKey)
		if deriveErr != nil && thread {
			return ToolResult{}, deriveErr
		}
		if deriveErr == nil {
			spawnCtx = derivedCtx
		}
	}
	if thread && (spawnCtx.AgentChannel == "" || spawnCtx.AgentAccountID == "" || spawnCtx.AgentThreadID == "") {
		return ToolResult{}, fmt.Errorf("thread=true requires a channel session context, but it is unavailable")
	}

	// Build spawn params
	spawnParams := acp.SpawnAcpParams{
		Task:    task,
		Label:   label,
		AgentID: agentID,
		Cwd:     cwd,
		Mode:    mode,
		Thread:  thread,
	}

	// Spawn ACP session
	result, err := acp.SpawnAcpDirect(ctx, t.cfg, spawnParams, spawnCtx)
	if err != nil {
		return ToolResult{}, fmt.Errorf("failed to spawn ACP session: %w", err)
	}

	// Build result message
	var message string
	switch result.Status {
	case "accepted":
		message = fmt.Sprintf("✅ ACP session spawned successfully\n\n")
		if result.Note != "" {
			message += fmt.Sprintf("**Note:** %s\n\n", result.Note)
		}
		message += fmt.Sprintf("**Session Key:** `%s`\n", result.ChildSessionKey)
		message += fmt.Sprintf("**Run ID:** `%s`\n", result.RunID)
		message += fmt.Sprintf("**Mode:** %s\n", result.Mode)
		if result.Mode == acp.SpawnModeSession && thread {
			message += "\n💬 This session is bound to this thread. Continue here for follow-ups."
		}
	case "forbidden":
		message = fmt.Sprintf("❌ ACP spawn forbidden: %s", result.Error)
	case "error":
		message = fmt.Sprintf("❌ ACP spawn failed: %s", result.Error)
	default:
		message = fmt.Sprintf("⚠️ ACP spawn returned unknown status: %s", result.Status)
	}

	// Format result as JSON for machine readability
	resultJSON, _ := json.Marshal(map[string]any{
		"status":           result.Status,
		"child_session_key": result.ChildSessionKey,
		"run_id":           result.RunID,
		"mode":             string(result.Mode),
		"note":             result.Note,
		"error":            result.Error,
	})

	// Build content blocks
	content := []ContentBlock{
		TextContent{Text: message},
	}

	// Build error if status is not accepted
	var toolErr error
	if result.Status != "accepted" && result.Error != "" {
		toolErr = fmt.Errorf("%s: %s", result.Status, result.Error)
	}

	agentResult := ToolResult{
		Content: content,
		Details: map[string]any{
			"status":           result.Status,
			"child_session_key": result.ChildSessionKey,
			"run_id":           result.RunID,
			"mode":             string(result.Mode),
			"note":             result.Note,
			"raw_error":        result.Error,
			"json":             string(resultJSON),
		},
		Error: toolErr,
	}

	// Call update callback if provided
	if onUpdate != nil {
		onUpdate(agentResult)
	}

	return agentResult, nil
}

func buildSpawnContextFromSessionKey(sessionKey string) (acp.SpawnAcpContext, error) {
	parts := strings.SplitN(sessionKey, ":", 3)
	if len(parts) != 3 {
		return acp.SpawnAcpContext{}, fmt.Errorf("invalid session key format for ACP thread binding: %s", sessionKey)
	}

	channel := strings.TrimSpace(parts[0])
	accountID := strings.TrimSpace(parts[1])
	conversationID := strings.TrimSpace(parts[2])
	if channel == "" || accountID == "" || conversationID == "" {
		return acp.SpawnAcpContext{}, fmt.Errorf("session key missing channel context for ACP thread binding: %s", sessionKey)
	}

	return acp.SpawnAcpContext{
		AgentSessionKey: sessionKey,
		AgentChannel:    channel,
		AgentAccountID:  accountID,
		AgentTo:         conversationID,
		AgentThreadID:   conversationID,
	}, nil
}
