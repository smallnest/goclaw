package gateway

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/smallnest/goclaw/acp"
	"github.com/smallnest/goclaw/config"
)

// RegisterAcpMethods registers ACP-related gateway methods.
func RegisterAcpMethods(registry *MethodRegistry, cfg *config.Config, acpManager *acp.Manager) {
	// acp/spawn - Create a new ACP session
	registry.Register("acp_spawn", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		return handleAcpSpawn(cfg, acpManager, sessionID, params)
	})

	// acp_status - Get ACP session status
	registry.Register("acp_status", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		return handleAcpStatus(cfg, acpManager, sessionID, params)
	})

	// acp_set_mode - Set runtime mode for ACP session
	registry.Register("acp_set_mode", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		return handleAcpSetMode(cfg, acpManager, sessionID, params)
	})

	// acp_set_config_option - Set config option on ACP session
	registry.Register("acp_set_config_option", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		return handleAcpSetConfigOption(cfg, acpManager, sessionID, params)
	})

	// acp_cancel - Cancel active ACP turn
	registry.Register("acp_cancel", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		return handleAcpCancel(cfg, acpManager, sessionID, params)
	})

	// acp_close - Close ACP session
	registry.Register("acp_close", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		return handleAcpClose(cfg, acpManager, sessionID, params)
	})

	// acp_list - List all ACP sessions
	registry.Register("acp_list", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		return handleAcpList(cfg, acpManager, sessionID, params)
	})
}

// AcpSpawnParams parameters for acp_spawn method
type AcpSpawnParams struct {
	Task    string `json:"task"`
	Label   string `json:"label,omitempty"`
	AgentID string `json:"agent_id,omitempty"`
	Cwd     string `json:"cwd,omitempty"`
	Mode    string `json:"mode,omitempty"`   // "run" or "session"
	Thread  bool   `json:"thread,omitempty"` // Whether to bind to a thread

	// Context for thread binding
	AgentChannel   string `json:"agent_channel,omitempty"`
	AgentAccountID string `json:"agent_account_id,omitempty"`
	AgentTo        string `json:"agent_to,omitempty"`
	AgentThreadID  string `json:"agent_thread_id,omitempty"`
}

// handleAcpSpawn handles the acp_spawn gateway method.
func handleAcpSpawn(cfg *config.Config, acpManager *acp.Manager, sessionID string, params map[string]interface{}) (interface{}, error) {
	// Parse parameters
	var spawnParams AcpSpawnParams
	paramsBytes, _ := json.Marshal(params)
	if err := json.Unmarshal(paramsBytes, &spawnParams); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	// Validate required parameters
	if spawnParams.Task == "" {
		return nil, fmt.Errorf("task parameter is required")
	}

	// Determine spawn mode
	var mode acp.SpawnAcpMode
	switch spawnParams.Mode {
	case "session":
		mode = acp.SpawnModeSession
	case "run", "":
		mode = acp.SpawnModeRun
	default:
		return nil, fmt.Errorf("invalid mode: %s (must be 'run' or 'session')", spawnParams.Mode)
	}

	// Build spawn context
	spawnCtx := acp.SpawnAcpContext{
		AgentSessionKey: sessionID,
		AgentChannel:    spawnParams.AgentChannel,
		AgentAccountID:  spawnParams.AgentAccountID,
		AgentTo:         spawnParams.AgentTo,
		AgentThreadID:   spawnParams.AgentThreadID,
	}

	// Build spawn params
	inputParams := acp.SpawnAcpParams{
		Task:    spawnParams.Task,
		Label:   spawnParams.Label,
		AgentID: spawnParams.AgentID,
		Cwd:     spawnParams.Cwd,
		Mode:    mode,
		Thread:  spawnParams.Thread,
	}

	// Spawn ACP session
	ctx := context.Background()
	result, err := acp.SpawnAcpDirect(ctx, cfg, inputParams, spawnCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to spawn ACP session: %w", err)
	}

	// Return result
	return map[string]interface{}{
		"status":            result.Status,
		"child_session_key": result.ChildSessionKey,
		"run_id":            result.RunID,
		"mode":              string(result.Mode),
		"note":              result.Note,
		"error":             result.Error,
	}, nil
}

// AcpStatusParams parameters for acp_status method
type AcpStatusParams struct {
	SessionKey string `json:"session_key"`
}

// handleAcpStatus handles the acp_status gateway method.
func handleAcpStatus(cfg *config.Config, acpManager *acp.Manager, sessionID string, params map[string]interface{}) (interface{}, error) {
	// Parse parameters
	var statusParams AcpStatusParams
	paramsBytes, _ := json.Marshal(params)
	if err := json.Unmarshal(paramsBytes, &statusParams); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	// Validate required parameters
	if statusParams.SessionKey == "" {
		return nil, fmt.Errorf("session_key parameter is required")
	}

	// Get session status
	ctx := context.Background()
	status, err := acpManager.GetSessionStatus(ctx, acp.GetSessionStatusInput{
		Cfg:        cfg,
		SessionKey: statusParams.SessionKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get ACP session status: %w", err)
	}

	// Return status
	return map[string]interface{}{
		"session_key":      status.SessionKey,
		"backend":          status.Backend,
		"agent":            status.Agent,
		"identity":         status.Identity,
		"state":            status.State,
		"mode":             string(status.Mode),
		"runtime_options":  status.RuntimeOptions,
		"capabilities":     status.Capabilities,
		"runtime_status":   status.RuntimeStatus,
		"last_activity_at": status.LastActivityAt,
		"last_error":       status.LastError,
	}, nil
}

// AcpSetModeParams parameters for acp_set_mode method
type AcpSetModeParams struct {
	SessionKey  string `json:"session_key"`
	RuntimeMode string `json:"runtime_mode"`
}

// handleAcpSetMode handles the acp_set_mode gateway method.
func handleAcpSetMode(cfg *config.Config, acpManager *acp.Manager, sessionID string, params map[string]interface{}) (interface{}, error) {
	// Parse parameters
	var setModeParams AcpSetModeParams
	paramsBytes, _ := json.Marshal(params)
	if err := json.Unmarshal(paramsBytes, &setModeParams); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	// Validate required parameters
	if setModeParams.SessionKey == "" {
		return nil, fmt.Errorf("session_key parameter is required")
	}
	if setModeParams.RuntimeMode == "" {
		return nil, fmt.Errorf("runtime_mode parameter is required")
	}

	// Set runtime mode
	ctx := context.Background()
	options, err := acpManager.SetSessionRuntimeMode(ctx, acp.SetSessionRuntimeModeInput{
		Cfg:         cfg,
		SessionKey:  setModeParams.SessionKey,
		RuntimeMode: setModeParams.RuntimeMode,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to set runtime mode: %w", err)
	}

	return map[string]interface{}{
		"runtime_options": options,
	}, nil
}

// AcpSetConfigOptionParams parameters for acp_set_config_option method
type AcpSetConfigOptionParams struct {
	SessionKey string `json:"session_key"`
	Key        string `json:"key"`
	Value      string `json:"value"`
}

// handleAcpSetConfigOption handles the acp_set_config_option gateway method.
func handleAcpSetConfigOption(cfg *config.Config, acpManager *acp.Manager, sessionID string, params map[string]interface{}) (interface{}, error) {
	// Parse parameters
	var setOptionParams AcpSetConfigOptionParams
	paramsBytes, _ := json.Marshal(params)
	if err := json.Unmarshal(paramsBytes, &setOptionParams); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	// Validate required parameters
	if setOptionParams.SessionKey == "" {
		return nil, fmt.Errorf("session_key parameter is required")
	}
	if setOptionParams.Key == "" {
		return nil, fmt.Errorf("key parameter is required")
	}
	if setOptionParams.Value == "" {
		return nil, fmt.Errorf("value parameter is required")
	}

	// Set config option
	ctx := context.Background()
	options, err := acpManager.SetSessionConfigOption(ctx, acp.SetSessionConfigOptionInput{
		Cfg:        cfg,
		SessionKey: setOptionParams.SessionKey,
		Key:        setOptionParams.Key,
		Value:      setOptionParams.Value,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to set config option: %w", err)
	}

	return map[string]interface{}{
		"runtime_options": options,
	}, nil
}

// AcpCancelParams parameters for acp_cancel method
type AcpCancelParams struct {
	SessionKey string `json:"session_key"`
	Reason     string `json:"reason,omitempty"`
}

// handleAcpCancel handles the acp_cancel gateway method.
func handleAcpCancel(cfg *config.Config, acpManager *acp.Manager, sessionID string, params map[string]interface{}) (interface{}, error) {
	// Parse parameters
	var cancelParams AcpCancelParams
	paramsBytes, _ := json.Marshal(params)
	if err := json.Unmarshal(paramsBytes, &cancelParams); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	// Validate required parameters
	if cancelParams.SessionKey == "" {
		return nil, fmt.Errorf("session_key parameter is required")
	}

	// Cancel session
	ctx := context.Background()
	err := acpManager.CancelSession(ctx, acp.CancelSessionInput{
		Cfg:        cfg,
		SessionKey: cancelParams.SessionKey,
		Reason:     cancelParams.Reason,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to cancel session: %w", err)
	}

	return map[string]interface{}{
		"success": true,
	}, nil
}

// AcpCloseParams parameters for acp_close method
type AcpCloseParams struct {
	SessionKey        string `json:"session_key"`
	Reason            string `json:"reason,omitempty"`
	RequireAcpSession bool   `json:"require_acp_session,omitempty"`
	ClearMeta         bool   `json:"clear_meta,omitempty"`
}

// handleAcpClose handles the acp_close gateway method.
func handleAcpClose(cfg *config.Config, acpManager *acp.Manager, sessionID string, params map[string]interface{}) (interface{}, error) {
	// Parse parameters
	var closeParams AcpCloseParams
	paramsBytes, _ := json.Marshal(params)
	if err := json.Unmarshal(paramsBytes, &closeParams); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	// Validate required parameters
	if closeParams.SessionKey == "" {
		return nil, fmt.Errorf("session_key parameter is required")
	}

	// Close session
	ctx := context.Background()
	result, err := acpManager.CloseSession(ctx, acp.CloseSessionInput{
		Cfg:               cfg,
		SessionKey:        closeParams.SessionKey,
		Reason:            closeParams.Reason,
		RequireAcpSession: closeParams.RequireAcpSession,
		ClearMeta:         closeParams.ClearMeta,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to close session: %w", err)
	}

	return map[string]interface{}{
		"runtime_closed": result.RuntimeClosed,
		"runtime_notice": result.RuntimeNotice,
		"meta_cleared":   result.MetaCleared,
	}, nil
}

// AcpListParams parameters for acp_list method
type AcpListParams struct {
	Limit int `json:"limit,omitempty"`
}

// handleAcpList handles the acp_list gateway method.
func handleAcpList(cfg *config.Config, acpManager *acp.Manager, sessionID string, params map[string]interface{}) (interface{}, error) {
	// Parse parameters
	var listParams AcpListParams
	paramsBytes, _ := json.Marshal(params)
	if err := json.Unmarshal(paramsBytes, &listParams); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	// Get observability snapshot
	snapshot := acpManager.GetObservabilitySnapshot()

	return map[string]interface{}{
		"runtime_cache":  snapshot.RuntimeCache,
		"turns":          snapshot.Turns,
		"errors_by_code": snapshot.ErrorsByCode,
	}, nil
}
