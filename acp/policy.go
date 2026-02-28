package acp

import (
	"fmt"

	"github.com/smallnest/goclaw/acp/runtime"
	"github.com/smallnest/goclaw/config"
)

// IsAcpEnabledByPolicy checks if ACP is enabled by policy.
func IsAcpEnabledByPolicy(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	return cfg.ACP.Enabled
}

// ResolveAcpAgentPolicyError checks if an agent is authorized to use ACP.
// Returns an error if the agent is not authorized.
func ResolveAcpAgentPolicyError(cfg *config.Config, agentID string) error {
	if cfg == nil {
		return nil // No config means no restrictions
	}

	// If no allowed agents list, all agents are allowed
	if len(cfg.ACP.AllowedAgents) == 0 {
		return nil
	}

	// Check if agent is in the allowed list
	for _, allowed := range cfg.ACP.AllowedAgents {
		if allowed == agentID {
			return nil
		}
	}

	return runtime.NewAcpRuntimeError(
		runtime.ErrCodeAgentUnauthorized,
		fmt.Sprintf("Agent '%s' is not authorized to use ACP", agentID),
		nil,
	)
}

// ResolveAcpDefaultAgent resolves the default agent ID for ACP sessions.
func ResolveAcpDefaultAgent(cfg *config.Config) string {
	if cfg == nil || cfg.ACP.DefaultAgent == "" {
		return "main"
	}
	return cfg.ACP.DefaultAgent
}

// ResolveAcpBackend resolves the ACP backend to use.
func ResolveAcpBackend(cfg *config.Config, requestedBackend string) string {
	if cfg == nil {
		return requestedBackend
	}

	// Use requested backend if provided
	if requestedBackend != "" {
		return requestedBackend
	}

	// Use configured backend
	if cfg.ACP.Backend != "" {
		return cfg.ACP.Backend
	}

	// Return empty to let the system choose the default
	return ""
}

// ResolveAcpMaxConcurrentSessions resolves the maximum concurrent sessions limit.
func ResolveAcpMaxConcurrentSessions(cfg *config.Config) int {
	if cfg == nil || cfg.ACP.MaxConcurrentSessions <= 0 {
		return 0 // No limit
	}
	return cfg.ACP.MaxConcurrentSessions
}

// ThreadBindingPolicy represents thread binding policy for a channel.
type ThreadBindingPolicy struct {
	Channel       string // Channel type
	AccountID     string // Account identifier
	Kind          string // "acp" or "subagent"
	Enabled       bool   // Whether thread bindings are enabled
	SpawnEnabled  bool   // Whether spawning new bindings is allowed
	IdleTimeoutMs int    // Idle timeout in milliseconds
	MaxAgeMs      int    // Maximum age in milliseconds
}

// ResolveThreadBindingSpawnPolicy resolves the spawn policy for thread bindings.
func ResolveThreadBindingSpawnPolicy(cfg *config.Config, channel, accountID, kind string) ThreadBindingPolicy {
	policy := ThreadBindingPolicy{
		Channel:       channel,
		AccountID:     accountID,
		Kind:          kind,
		Enabled:       true,
		SpawnEnabled:  true,
		IdleTimeoutMs: 5 * 60 * 1000, // Default 5 minutes
		MaxAgeMs:      60 * 60 * 1000, // Default 1 hour
	}

	if cfg == nil {
		return policy
	}

	// Check channel-specific configuration
	bindingKey := fmt.Sprintf("%s:%s", channel, accountID)
	if channelConfig, exists := cfg.ACP.ThreadBindings[bindingKey]; exists {
		policy.Enabled = channelConfig.Enabled
		policy.SpawnEnabled = channelConfig.SpawnEnabled
		if channelConfig.IdleTimeoutMs > 0 {
			policy.IdleTimeoutMs = channelConfig.IdleTimeoutMs
		}
		if channelConfig.MaxAgeMs > 0 {
			policy.MaxAgeMs = channelConfig.MaxAgeMs
		}
	}

	return policy
}

// CheckAcpAgentAuthorization checks if an agent is authorized for ACP operations.
func CheckAcpAgentAuthorization(cfg *config.Config, agentID string) error {
	return ResolveAcpAgentPolicyError(cfg, agentID)
}

// IsAcpAgentAuthorized checks if an agent is authorized for ACP operations.
func IsAcpAgentAuthorized(cfg *config.Config, agentID string) bool {
	return ResolveAcpAgentPolicyError(cfg, agentID) == nil
}

// FormatAcpPolicyError formats an ACP policy error message.
func FormatAcpPolicyError(code, message string) string {
	return fmt.Sprintf("[%s] %s", code, message)
}

// ResolveAcpIdleTimeoutMs resolves the idle timeout for ACP sessions.
func ResolveAcpIdleTimeoutMs(cfg *config.Config) int {
	if cfg == nil || cfg.ACP.IdleTimeoutMs <= 0 {
		return 5 * 60 * 1000 // Default 5 minutes
	}
	return cfg.ACP.IdleTimeoutMs
}
