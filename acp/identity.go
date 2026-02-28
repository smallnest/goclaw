package acp

import (
	"context"
	"time"

	"github.com/smallnest/goclaw/acp/runtime"
)

// SessionIdentity tracks the identity of an ACP session from the runtime.
type SessionIdentity struct {
	State            string `json:"state"`  // "pending" or "resolved"
	Source           string `json:"source"` // "ensure" or "status"
	LastUpdatedAt    int64  `json:"last_updated_at"`
	BackendSessionID string `json:"backend_session_id,omitempty"`
	AgentSessionID   string `json:"agent_session_id,omitempty"`
}

// CreateIdentityFromEnsure creates a session identity from an ensure session response.
func CreateIdentityFromEnsure(handle runtime.AcpRuntimeHandle, now int64) *SessionIdentity {
	identity := &SessionIdentity{
		State:         "pending",
		Source:        "ensure",
		LastUpdatedAt: now,
	}

	// If handle has session IDs, consider it resolved
	if handle.BackendSessionId != "" || handle.AgentSessionId != "" {
		identity.State = "resolved"
		identity.BackendSessionID = handle.BackendSessionId
		identity.AgentSessionID = handle.AgentSessionId
	}

	return identity
}

// IsSessionIdentityPending checks if a session identity is pending.
func IsSessionIdentityPending(identity *SessionIdentity) bool {
	if identity == nil {
		return false
	}
	return identity.State == "pending"
}

// IdentityEquals checks if two session identities are equal.
func IdentityEquals(a, b *SessionIdentity) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	return a.State == b.State &&
		a.Source == b.Source &&
		a.BackendSessionID == b.BackendSessionID &&
		a.AgentSessionID == b.AgentSessionID
}

// MergeSessionIdentity merges an incoming identity with the current identity.
func MergeSessionIdentity(current *SessionIdentity, incoming *SessionIdentity, now int64) *SessionIdentity {
	if incoming == nil {
		if current != nil {
			// Update timestamp even if no new identity
			current.LastUpdatedAt = now
		}
		return current
	}

	if current == nil {
		incoming.LastUpdatedAt = now
		return incoming
	}

	// If incoming is resolved, use it
	if incoming.State == "resolved" {
		incoming.LastUpdatedAt = now
		return incoming
	}

	// If current is resolved and incoming is pending, keep current
	if current.State == "resolved" {
		current.LastUpdatedAt = now
		return current
	}

	// Both are pending, use the most recent one
	if incoming.LastUpdatedAt > current.LastUpdatedAt {
		incoming.LastUpdatedAt = now
		return incoming
	}

	current.LastUpdatedAt = now
	return current
}

// ResolveSessionIdentityFromMeta extracts session identity from metadata.
func ResolveSessionIdentityFromMeta(meta *SessionAcpMeta) *SessionIdentity {
	if meta == nil || meta.Identity == nil {
		return nil
	}

	return meta.Identity
}

// ResolveRuntimeHandleIdentifiersFromIdentity extracts runtime handle identifiers from identity.
func ResolveRuntimeHandleIdentifiersFromIdentity(identity *SessionIdentity) map[string]string {
	identifiers := make(map[string]string)

	if identity == nil {
		return identifiers
	}

	if identity.BackendSessionID != "" {
		identifiers["backend_session_id"] = identity.BackendSessionID
	}

	if identity.AgentSessionID != "" {
		identifiers["agent_session_id"] = identity.AgentSessionID
	}

	return identifiers
}

// UpdateIdentityFromStatus updates identity from a status response.
func UpdateIdentityFromStatus(identity *SessionIdentity, status *runtime.AcpRuntimeStatus) *SessionIdentity {
	if identity == nil {
		identity = &SessionIdentity{
			State:         "pending",
			Source:        "status",
			LastUpdatedAt: time.Now().UnixMilli(),
		}
	}

	if status == nil {
		return identity
	}

	// Update session IDs if available
	if status.BackendSessionId != "" {
		identity.BackendSessionID = status.BackendSessionId
	}

	if status.AgentSessionId != "" {
		identity.AgentSessionID = status.AgentSessionId
	}

	// If we have session IDs now, mark as resolved
	if status.BackendSessionId != "" || status.AgentSessionId != "" {
		identity.State = "resolved"
	}

	identity.LastUpdatedAt = time.Now().UnixMilli()

	return identity
}

// StartupIdentityReconcileResult represents the result of startup identity reconciliation.
type StartupIdentityReconcileResult struct {
	Checked  int
	Resolved int
	Failed   int
}

// ReconcilePendingSessionIdentities reconciles pending session identities on startup.
func (m *Manager) ReconcilePendingSessionIdentities(ctx context.Context) StartupIdentityReconcileResult {
	result := StartupIdentityReconcileResult{}

	// In a real implementation, this would:
	// 1. List all ACP sessions from storage
	// 2. Filter for sessions with "pending" identity
	// 3. For each pending session:
	//    a. Get or create runtime handle
	//    b. Call getStatus to reconcile session IDs
	//    c. Update metadata with resolved identity
	// 4. Track statistics

	// For now, return empty result
	return result
}

// HasLegacyAcpIdentityProjection checks if metadata has legacy ACP identity projection.
func HasLegacyAcpIdentityProjection(meta *SessionAcpMeta) bool {
	if meta == nil {
		return false
	}

	// Check for legacy identity fields that might exist in old metadata
	// This is a placeholder for future compatibility
	return false
}

// NormalizeText normalizes text values.
func NormalizeText(s string) string {
	if s == "" {
		return ""
	}
	// Simple trim - in production this would handle Unicode properly
	return s
}
