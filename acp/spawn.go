package acp

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/smallnest/goclaw/acp/runtime"
	"github.com/smallnest/goclaw/channels"
	"github.com/smallnest/goclaw/config"
)

const (
	// AcpSpawnAcceptedNote is the note for accepted oneshot spawns.
	AcpSpawnAcceptedNote = "initial ACP task queued in isolated session; follow-ups continue in the bound thread."

	// AcpSpawnSessionAcceptedNote is the note for accepted persistent session spawns.
	AcpSpawnSessionAcceptedNote = "thread-bound ACP session stays active after this task; continue in-thread for follow-ups."
)

// SpawnAcpMode represents the spawn mode.
type SpawnAcpMode string

const (
	// SpawnModeRun is oneshot mode - session closes after task completion.
	SpawnModeRun SpawnAcpMode = "run"

	// SpawnModeSession is persistent mode - session stays active.
	SpawnModeSession SpawnAcpMode = "session"
)

// SpawnAcpParams contains parameters for spawning an ACP session.
type SpawnAcpParams struct {
	Task    string       // The task to execute
	Label   string       // Optional label for the session
	AgentID string       // Target agent ID
	Cwd     string       // Working directory
	Mode    SpawnAcpMode // "run" or "session"
	Thread  bool         // Whether to bind to a thread
}

// SpawnAcpContext contains context for spawning an ACP session.
type SpawnAcpContext struct {
	AgentSessionKey string // Parent session key
	AgentChannel    string // Channel type
	AgentAccountID  string // Channel account ID
	AgentTo         string // Target recipient
	AgentThreadID   string // Thread ID
}

// SpawnAcpResult represents the result of spawning an ACP session.
type SpawnAcpResult struct {
	Status          string       // "accepted", "forbidden", "error"
	ChildSessionKey string       // The session key of the spawned ACP session
	RunID           string       // The run ID
	Mode            SpawnAcpMode // The spawn mode
	Note            string       // Optional note
	Error           string       // Error message if status is "error"
}

// SpawnAcpDirect spawns a new ACP session.
func SpawnAcpDirect(ctx context.Context, cfg *config.Config, params SpawnAcpParams, spawnCtx SpawnAcpContext) (*SpawnAcpResult, error) {
	// Check if ACP is enabled by policy
	if !IsAcpEnabledByPolicy(cfg) {
		return &SpawnAcpResult{
			Status: "forbidden",
			Error:  "ACP is disabled by policy (acp.enabled=false).",
		}, nil
	}

	// Resolve spawn mode
	requestThreadBinding := params.Thread
	spawnMode := resolveSpawnMode(params.Mode, requestThreadBinding)

	// Validate session mode requires thread
	if spawnMode == SpawnModeSession && !requestThreadBinding {
		return &SpawnAcpResult{
			Status: "error",
			Error:  `mode="session" requires thread=true so the ACP session can stay bound to a thread.`,
		}, nil
	}

	// Resolve target agent
	targetAgentID, err := resolveTargetAcpAgentID(params.AgentID, cfg)
	if err != nil {
		return &SpawnAcpResult{
			Status: "error",
			Error:  err.Error(),
		}, nil
	}

	// Check agent policy
	if policyErr := ResolveAcpAgentPolicyError(cfg, targetAgentID); policyErr != nil {
		return &SpawnAcpResult{
			Status: "forbidden",
			Error:  policyErr.Error(),
		}, nil
	}

	manager := GetOrCreateGlobalManager(cfg)

	// Generate session key
	sessionKey := fmt.Sprintf("agent:%s:acp:%s", targetAgentID, uuid.New().String())
	runtimeMode := resolveAcpSessionMode(spawnMode)

	// Prepare thread binding if requested
	var preparedBinding *channels.PreparedAcpThreadBinding
	if requestThreadBinding {
		prepared, err := prepareAcpThreadBinding(cfg, spawnCtx)
		if err != nil {
			return &SpawnAcpResult{
				Status: "error",
				Error:  err.Error(),
			}, nil
		}
		preparedBinding = prepared
	}

	// Initialize session
	handle, meta, initErr := manager.InitializeSession(ctx, InitializeSessionInput{
		Cfg:        cfg,
		SessionKey: sessionKey,
		Agent:      targetAgentID,
		Mode:       runtimeMode,
		Cwd:        params.Cwd,
		BackendID:  cfg.ACP.Backend,
	})

	if initErr != nil {
		// Cleanup on failure
		_ = cleanupFailedAcpSpawn(ctx, cfg, sessionKey, true, true, handle)
		return &SpawnAcpResult{
			Status: "error",
			Error:  initErr.Error(),
		}, nil
	}

	// Create thread binding if requested
	var binding *channels.ThreadBindingRecord
	if preparedBinding != nil {
		// Use the global thread binding service
		threadBindingService := GetGlobalThreadBindingService()
		if threadBindingService == nil {
			_ = cleanupFailedAcpSpawn(ctx, cfg, sessionKey, true, true, handle)
			return &SpawnAcpResult{
				Status: "error",
				Error:  "thread binding service is unavailable",
			}, nil
		}

		binding, err = threadBindingService.Bind(channels.BindInput{
			TargetSessionKey: sessionKey,
			TargetKind:       "session",
			Conversation: channels.ThreadBindingConversation{
				Channel:        preparedBinding.Channel,
				AccountID:      preparedBinding.AccountID,
				ConversationID: preparedBinding.ConversationID,
			},
			Placement: "child",
			Metadata: channels.ThreadBindingMetadata{
				ThreadName: channels.ResolveThreadBindingThreadName(targetAgentID, params.Label),
				AgentID:    targetAgentID,
				Label:      params.Label,
				BoundBy:    "system",
				IntroText: channels.ResolveThreadBindingIntroText(
					targetAgentID,
					params.Label,
					channels.ResolveThreadBindingIdleTimeoutMsForChannel(cfg, preparedBinding.Channel, preparedBinding.AccountID),
					channels.ResolveThreadBindingMaxAgeMsForChannel(cfg, preparedBinding.Channel, preparedBinding.AccountID),
					meta.Cwd,
					[]string{}, // TODO: add session details
				),
				SessionCwd: meta.Cwd,
			},
		})
		if err != nil || binding == nil {
			// Cleanup on failure
			_ = cleanupFailedAcpSpawn(ctx, cfg, sessionKey, true, true, handle)
			return &SpawnAcpResult{
				Status: "error",
				Error:  fmt.Sprintf("failed to create thread binding: %v", err),
			}, nil
		}
	}

	// Generate run ID
	runID := uuid.New().String()

	turnResult, err := manager.RunTrackedTurn(ctx, RunTrackedTurnInput{
		Cfg:        cfg,
		SessionKey: sessionKey,
		Text:       params.Task,
		Mode:       runtime.AcpPromptModePrompt,
		RequestID:  runID,
	})
	if err != nil {
		// Cleanup on failure
		_ = cleanupFailedAcpSpawn(ctx, cfg, sessionKey, true, true, handle)
		return &SpawnAcpResult{
			Status: "error",
			Error:  fmt.Sprintf("failed to run ACP turn: %v", err),
		}, nil
	}

	// Always consume turn events so tracked channels never block.
	// In oneshot mode, terminal events trigger automatic session close.
	go func() {
		for event := range turnResult.EventChan {
			if spawnMode != SpawnModeRun {
				continue
			}
			switch event.(type) {
			case *runtime.AcpEventDone:
				_, _ = manager.CloseSession(ctx, CloseSessionInput{
					Cfg:               cfg,
					SessionKey:        sessionKey,
					Reason:            "oneshot-task-complete",
					RequireAcpSession: false,
					ClearMeta:         false,
				})
			case *runtime.AcpEventError:
				_, _ = manager.CloseSession(ctx, CloseSessionInput{
					Cfg:               cfg,
					SessionKey:        sessionKey,
					Reason:            "oneshot-task-failed",
					RequireAcpSession: false,
					ClearMeta:         false,
				})
			}
		}
	}()

	note := AcpSpawnAcceptedNote
	if spawnMode == SpawnModeSession {
		note = AcpSpawnSessionAcceptedNote
	}

	return &SpawnAcpResult{
		Status:          "accepted",
		ChildSessionKey: sessionKey,
		RunID:           runID,
		Mode:            spawnMode,
		Note:            note,
	}, nil
}

// resolveSpawnMode resolves the spawn mode.
func resolveSpawnMode(requestedMode SpawnAcpMode, threadRequested bool) SpawnAcpMode {
	if requestedMode == SpawnModeRun || requestedMode == SpawnModeSession {
		return requestedMode
	}
	// Thread-bound spawns should default to persistent sessions.
	if threadRequested {
		return SpawnModeSession
	}
	return SpawnModeRun
}

// resolveAcpSessionMode converts spawn mode to runtime session mode.
func resolveAcpSessionMode(mode SpawnAcpMode) runtime.AcpRuntimeSessionMode {
	if mode == SpawnModeSession {
		return runtime.AcpSessionModePersistent
	}
	return runtime.AcpSessionModeOneshot
}

// resolveTargetAcpAgentID resolves the target agent ID.
func resolveTargetAcpAgentID(requestedAgentID string, cfg *config.Config) (string, error) {
	normalized := normalizeSpawnAgentID(requestedAgentID)
	if normalized != "" {
		return normalized, nil
	}

	// Use configured default
	configuredDefault := normalizeSpawnAgentID(cfg.ACP.DefaultAgent)
	if configuredDefault != "" {
		return configuredDefault, nil
	}

	return "", fmt.Errorf("ACP target agent is not configured. Pass agentId in spawn params or set acp.defaultAgent in config")
}

// normalizeSpawnAgentID normalizes an agent ID for spawning.
func normalizeSpawnAgentID(agentID string) string {
	if agentID == "" {
		return ""
	}
	// Simple trim - in production this would be more sophisticated
	return agentID
}

// prepareAcpThreadBinding prepares a thread binding for ACP spawn.
func prepareAcpThreadBinding(cfg *config.Config, spawnCtx SpawnAcpContext) (*channels.PreparedAcpThreadBinding, error) {
	channel := spawnCtx.AgentChannel
	if channel == "" {
		return nil, fmt.Errorf("thread=true for ACP sessions requires a channel context")
	}

	accountID := spawnCtx.AgentAccountID
	if accountID == "" {
		accountID = "default"
	}

	// Check policy
	policy := ResolveThreadBindingSpawnPolicy(cfg, channel, accountID, "acp")
	if !policy.Enabled {
		return nil, fmt.Errorf("%s", channels.FormatThreadBindingDisabledError(channel, accountID, "acp"))
	}
	if !policy.SpawnEnabled {
		return nil, fmt.Errorf("%s", channels.FormatThreadBindingSpawnDisabledError(channel, accountID, "acp"))
	}

	// Resolve conversation ID
	conversationID := resolveConversationID(spawnCtx)
	if conversationID == "" {
		return nil, fmt.Errorf("could not resolve a %s conversation for ACP thread spawn", channel)
	}

	return &channels.PreparedAcpThreadBinding{
		Channel:        channel,
		AccountID:      accountID,
		ConversationID: conversationID,
	}, nil
}

// resolveConversationID resolves the conversation ID from spawn context.
func resolveConversationID(spawnCtx SpawnAcpContext) string {
	if spawnCtx.AgentThreadID != "" {
		return spawnCtx.AgentThreadID
	}
	// In a real implementation, this would also check spawnCtx.AgentTo
	return ""
}

// cleanupFailedAcpSpawn cleans up resources after a failed spawn.
func cleanupFailedAcpSpawn(ctx context.Context, cfg *config.Config, sessionKey string, shouldDeleteSession, deleteTranscript bool, handle *runtime.AcpRuntimeHandle) error {
	// Use the global manager to ensure we can find the session
	manager := GetOrCreateGlobalManager(cfg)

	// Close runtime if provided
	if handle != nil {
		_, _ = manager.CloseSession(ctx, CloseSessionInput{
			Cfg:               cfg,
			SessionKey:        sessionKey,
			Reason:            "spawn-failed",
			RequireAcpSession: false,
			ClearMeta:         false,
		})
	}
	unbindThreadBindingsForSession(sessionKey)

	// In a real implementation, this would also delete the session from storage
	// and clean up the transcript

	return nil
}
