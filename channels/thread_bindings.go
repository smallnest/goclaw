package channels

import (
	"fmt"
	"sync"
	"time"

	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/session"
)

// ThreadBindingRecord represents a thread binding between a channel thread and an ACP session.
type ThreadBindingRecord struct {
	ID               string                    // Unique binding ID
	TargetSessionKey string                    // The ACP session key
	TargetKind       string                    // "session" or "subagent"
	Conversation     ThreadBindingConversation // Channel conversation info
	Placement        string                    // "child" or "peer"
	Metadata         ThreadBindingMetadata     // Binding metadata
	CreatedAt        time.Time                 // Creation time
	ExpiresAt        *time.Time                // Optional expiration
}

// ThreadBindingConversation identifies a channel conversation.
type ThreadBindingConversation struct {
	Channel        string // Channel type (telegram, discord, etc.)
	AccountID      string // Account identifier
	ConversationID string // Thread/conversation identifier
}

// ThreadBindingMetadata contains additional binding metadata.
type ThreadBindingMetadata struct {
	ThreadName string         // Display name for the thread
	AgentID    string         // Associated agent ID
	Label      string         // Optional label
	BoundBy    string         // Who created the binding ("user" or "system")
	IntroText  string         // Introduction text
	SessionCwd string         // Working directory
	Details    map[string]any // Additional details
}

// ThreadBindingPolicy represents thread binding policy for a channel.
type ThreadBindingPolicy struct {
	Channel       string // Channel type
	AccountID     string // Account identifier
	Kind          string // Binding kind ("acp" or "subagent")
	Enabled       bool   // Whether thread bindings are enabled
	SpawnEnabled  bool   // Whether spawning new bindings is allowed
	IdleTimeoutMs int    // Idle timeout in milliseconds
	MaxAgeMs      int    // Maximum age in milliseconds
}

// ThreadBindingCapabilities describes channel's thread binding support.
type ThreadBindingCapabilities struct {
	AdapterAvailable bool     // Whether the adapter is available
	BindSupported    bool     // Whether thread binding is supported
	Placements       []string // Supported placement types
}

// PreparedAcpThreadBinding represents a prepared thread binding for ACP spawn.
type PreparedAcpThreadBinding struct {
	Channel        string
	AccountID      string
	ConversationID string
}

// ThreadBindingService manages thread bindings.
type ThreadBindingService struct {
	mu       sync.RWMutex
	bindings map[string]*ThreadBindingRecord   // by ID
	byTarget map[string][]*ThreadBindingRecord // by session key
	byThread map[string]*ThreadBindingRecord   // by conversation

	cfg      *config.Config
	sessions *session.Manager
	storage  ThreadBindingStorage // Persistent storage backend
}

// NewThreadBindingService creates a new thread binding service.
func NewThreadBindingService(cfg *config.Config, sessions *session.Manager) *ThreadBindingService {
	service := &ThreadBindingService{
		bindings: make(map[string]*ThreadBindingRecord),
		byTarget: make(map[string][]*ThreadBindingRecord),
		byThread: make(map[string]*ThreadBindingRecord),
		cfg:      cfg,
		sessions: sessions,
	}

	// Initialize persistent storage
	if cfg != nil && cfg.Workspace.Path != "" {
		storage, err := NewJSONFileStorage(cfg.Workspace.Path)
		if err == nil {
			service.storage = storage
			// Load existing bindings from storage
			service.loadFromStorage()
		}
	}

	return service
}

// loadFromStorage loads bindings from persistent storage into memory.
func (s *ThreadBindingService) loadFromStorage() {
	if s.storage == nil {
		return
	}

	bindings, err := s.storage.Load()
	if err != nil {
		return
	}

	// Populate in-memory indexes
	for _, record := range bindings {
		s.bindings[record.ID] = record
		s.byTarget[record.TargetSessionKey] = append(s.byTarget[record.TargetSessionKey], record)
		conversationKey := s.conversationKey(record.Conversation)
		s.byThread[conversationKey] = record
	}
}

// GetCapabilities returns the thread binding capabilities for a channel/account.
func (s *ThreadBindingService) GetCapabilities(channel, accountID string) ThreadBindingCapabilities {
	// Default capabilities - in a real implementation, this would check
	// the specific channel adapter's capabilities
	return ThreadBindingCapabilities{
		AdapterAvailable: true,
		BindSupported:    true,
		Placements:       []string{"child"},
	}
}

// Bind creates a new thread binding.
type BindInput struct {
	TargetSessionKey string
	TargetKind       string
	Conversation     ThreadBindingConversation
	Placement        string
	Metadata         ThreadBindingMetadata
}

func (s *ThreadBindingService) Bind(input BindInput) (*ThreadBindingRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if binding already exists for this conversation
	conversationKey := s.conversationKey(input.Conversation)
	if existing, exists := s.byThread[conversationKey]; exists {
		return existing, fmt.Errorf("thread binding already exists for conversation %s", conversationKey)
	}

	// Create binding record
	now := time.Now()
	record := &ThreadBindingRecord{
		ID:               generateBindingID(),
		TargetSessionKey: input.TargetSessionKey,
		TargetKind:       input.TargetKind,
		Conversation:     input.Conversation,
		Placement:        input.Placement,
		Metadata:         input.Metadata,
		CreatedAt:        now,
	}

	// Set expiration if configured
	policy := s.resolvePolicy(input.Conversation.Channel, input.Conversation.AccountID, input.TargetKind)
	if policy.MaxAgeMs > 0 {
		expiresAt := now.Add(time.Duration(policy.MaxAgeMs) * time.Millisecond)
		record.ExpiresAt = &expiresAt
	}

	// Store binding in memory
	s.bindings[record.ID] = record
	s.byTarget[input.TargetSessionKey] = append(s.byTarget[input.TargetSessionKey], record)
	s.byThread[conversationKey] = record

	// Persist to storage
	if s.storage != nil {
		_ = s.storage.Save(record)
	}

	return record, nil
}

// Unbind removes a thread binding.
func (s *ThreadBindingService) Unbind(bindingID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, exists := s.bindings[bindingID]
	if !exists {
		return fmt.Errorf("binding not found: %s", bindingID)
	}

	// Remove from indexes
	conversationKey := s.conversationKey(record.Conversation)

	// Remove from byTarget
	targetBindings := s.byTarget[record.TargetSessionKey]
	for i, b := range targetBindings {
		if b.ID == bindingID {
			s.byTarget[record.TargetSessionKey] = append(targetBindings[:i], targetBindings[i+1:]...)
			break
		}
	}

	// Remove from byThread
	delete(s.byThread, conversationKey)

	// Remove from bindings
	delete(s.bindings, bindingID)

	// Delete from persistent storage
	if s.storage != nil {
		_ = s.storage.Delete(bindingID)
	}

	return nil
}

// GetBySession retrieves all bindings for a session.
func (s *ThreadBindingService) GetBySession(sessionKey string) []*ThreadBindingRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	bindings := s.byTarget[sessionKey]
	if bindings == nil {
		return []*ThreadBindingRecord{}
	}

	result := make([]*ThreadBindingRecord, len(bindings))
	copy(result, bindings)
	return result
}

// GetByConversation retrieves a binding by conversation.
func (s *ThreadBindingService) GetByConversation(channel, accountID, conversationID string) *ThreadBindingRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conversationKey := fmt.Sprintf("%s:%s:%s", channel, accountID, conversationID)
	return s.byThread[conversationKey]
}

// Get retrieves a binding by ID.
func (s *ThreadBindingService) Get(bindingID string) *ThreadBindingRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.bindings[bindingID]
}

// CleanupExpired removes expired bindings.
func (s *ThreadBindingService) CleanupExpired() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	expired := 0

	for id, record := range s.bindings {
		if record.ExpiresAt != nil && now.After(*record.ExpiresAt) {
			conversationKey := s.conversationKey(record.Conversation)

			// Remove from byTarget
			targetBindings := s.byTarget[record.TargetSessionKey]
			for i, b := range targetBindings {
				if b.ID == id {
					s.byTarget[record.TargetSessionKey] = append(targetBindings[:i], targetBindings[i+1:]...)
					break
				}
			}

			// Remove from byThread
			delete(s.byThread, conversationKey)

			// Remove from bindings
			delete(s.bindings, id)

			// Delete from persistent storage as well.
			if s.storage != nil {
				_ = s.storage.Delete(id)
			}

			expired++
		}
	}

	return expired
}

// ResolvePolicy resolves the thread binding policy for a channel/account/kind.
func (s *ThreadBindingService) ResolvePolicy(channel, accountID, kind string) ThreadBindingPolicy {
	return s.resolvePolicy(channel, accountID, kind)
}

// resolvePolicy is the internal policy resolver.
func (s *ThreadBindingService) resolvePolicy(channel, accountID, kind string) ThreadBindingPolicy {
	// Default policy
	policy := ThreadBindingPolicy{
		Channel:       channel,
		AccountID:     accountID,
		Kind:          kind,
		Enabled:       true,
		SpawnEnabled:  true,
		IdleTimeoutMs: 5 * 60 * 1000,  // 5 minutes
		MaxAgeMs:      60 * 60 * 1000, // 1 hour
	}

	if s.cfg == nil {
		return policy
	}

	// Check channel-specific configuration
	bindingKey := fmt.Sprintf("%s:%s", channel, accountID)
	if channelConfig, exists := s.cfg.ACP.ThreadBindings[bindingKey]; exists {
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

// conversationKey generates a unique key for a conversation.
func (s *ThreadBindingService) conversationKey(conv ThreadBindingConversation) string {
	return fmt.Sprintf("%s:%s:%s", conv.Channel, conv.AccountID, conv.ConversationID)
}

// generateBindingID generates a unique binding ID.
func generateBindingID() string {
	return fmt.Sprintf("binding-%d", time.Now().UnixNano())
}

// Helper functions for thread binding messages (similar to openclaw)

// ResolveThreadBindingThreadName generates a thread name for a binding.
func ResolveThreadBindingThreadName(agentID, label string) string {
	if label != "" {
		return fmt.Sprintf("%s (%s)", label, agentID)
	}
	return fmt.Sprintf("ACP: %s", agentID)
}

// ResolveThreadBindingIntroText generates intro text for a thread binding.
func ResolveThreadBindingIntroText(agentID, label string, idleTimeoutMs, maxAgeMs int, sessionCwd string, sessionDetails []string) string {
	intro := "📌 **Thread-bound ACP session created**\n\n"
	intro += fmt.Sprintf("**Agent:** %s\n", agentID)
	if label != "" {
		intro += fmt.Sprintf("**Label:** %s\n", label)
	}
	if sessionCwd != "" {
		intro += fmt.Sprintf("**Working Directory:** `%s`\n", sessionCwd)
	}
	intro += "\n**Session Settings:**\n"
	intro += fmt.Sprintf("- Idle timeout: %s\n", formatDuration(idleTimeoutMs))
	intro += fmt.Sprintf("- Max age: %s\n", formatDuration(maxAgeMs))
	if len(sessionDetails) > 0 {
		intro += "\n**Details:**\n"
		for _, detail := range sessionDetails {
			intro += fmt.Sprintf("- %s\n", detail)
		}
	}
	return intro
}

// formatDuration formats milliseconds as a human-readable duration.
func formatDuration(ms int) string {
	duration := time.Duration(ms) * time.Millisecond

	if duration < time.Minute {
		return duration.String()
	}
	if duration < time.Hour {
		return fmt.Sprintf("%.1fm", duration.Minutes())
	}
	return fmt.Sprintf("%.1fh", duration.Hours())
}

// ResolveThreadBindingIdleTimeoutMsForChannel returns the idle timeout for a channel.
func ResolveThreadBindingIdleTimeoutMsForChannel(cfg *config.Config, channel, accountID string) int {
	if cfg == nil {
		return 5 * 60 * 1000 // Default 5 minutes
	}

	bindingKey := fmt.Sprintf("%s:%s", channel, accountID)
	if channelConfig, exists := cfg.ACP.ThreadBindings[bindingKey]; exists && channelConfig.IdleTimeoutMs > 0 {
		return channelConfig.IdleTimeoutMs
	}

	return 5 * 60 * 1000 // Default 5 minutes
}

// ResolveThreadBindingMaxAgeMsForChannel returns the max age for a channel.
func ResolveThreadBindingMaxAgeMsForChannel(cfg *config.Config, channel, accountID string) int {
	if cfg == nil {
		return 60 * 60 * 1000 // Default 1 hour
	}

	bindingKey := fmt.Sprintf("%s:%s", channel, accountID)
	if channelConfig, exists := cfg.ACP.ThreadBindings[bindingKey]; exists && channelConfig.MaxAgeMs > 0 {
		return channelConfig.MaxAgeMs
	}

	return 60 * 60 * 1000 // Default 1 hour
}

// FormatThreadBindingDisabledError formats an error message for disabled thread bindings.
func FormatThreadBindingDisabledError(channel, accountID, kind string) string {
	return fmt.Sprintf("Thread bindings are disabled for %s:%s (kind: %s)", channel, accountID, kind)
}

// FormatThreadBindingSpawnDisabledError formats an error message for disabled spawn.
func FormatThreadBindingSpawnDisabledError(channel, accountID, kind string) string {
	return fmt.Sprintf("Thread-bound spawning is disabled for %s:%s (kind: %s)", channel, accountID, kind)
}

// ResolveThreadBindingSpawnPolicy resolves the spawn policy for a channel.
func ResolveThreadBindingSpawnPolicy(cfg *config.Config, channel, accountID, kind string) ThreadBindingPolicy {
	policy := ThreadBindingPolicy{
		Channel:       channel,
		AccountID:     accountID,
		Kind:          kind,
		Enabled:       true,
		SpawnEnabled:  true,
		IdleTimeoutMs: 5 * 60 * 1000,
		MaxAgeMs:      60 * 60 * 1000,
	}

	if cfg == nil {
		return policy
	}

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
