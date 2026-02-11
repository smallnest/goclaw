package session

import (
	"fmt"
	"sync"
	"time"
)

// PruneStrategy defines how sessions should be pruned
type PruneStrategy string

const (
	// PruneStrategyLRU removes least recently used sessions
	PruneStrategyLRU PruneStrategy = "lru"
	// PruneStrategyLFU removes least frequently used sessions
	PruneStrategyLFU PruneStrategy = "lfu"
	// PruneStrategyTTL removes sessions past their TTL
	PruneStrategyTTL PruneStrategy = "ttl"
	// PruneStrategySemantic uses semantic similarity to deduplicate
	PruneStrategySemantic PruneStrategy = "semantic"
	// PruneStrategySize removes largest sessions first
	PruneStrategySize PruneStrategy = "size"
)

// PruneConfig configures session pruning behavior
type PruneConfig struct {
	Strategy           PruneStrategy
	MaxTotalSessions   int
	MaxTotalMessages   int
	MaxTotalTokens     int
	DefaultMessageTTL  time.Duration
	DMPreserveCount    int // Minimum messages to preserve in DM
	GroupPreserveCount int // Minimum messages to preserve in group
}

// DefaultPruneConfig returns default pruning configuration
func DefaultPruneConfig() PruneConfig {
	return PruneConfig{
		Strategy:           PruneStrategyLRU,
		MaxTotalSessions:   1000,
		MaxTotalMessages:   10000,
		MaxTotalTokens:     1000000,
		DefaultMessageTTL:  24 * time.Hour,
		DMPreserveCount:    100,
		GroupPreserveCount: 50,
	}
}

// Pruner manages session pruning operations
type Pruner struct {
	config  PruneConfig
	mu      sync.RWMutex
	manager *Manager
	stats   PruneStats
}

// PruneStats contains pruning statistics
type PruneStats struct {
	TotalPrunes     int64
	MessagesPruned  int64
	SessionsPruned  int64
	TokensReclaimed int64
	LastPruneAt     time.Time
}

// NewPruner creates a new session pruner
func NewPruner(manager *Manager, config PruneConfig) *Pruner {
	if manager == nil {
		panic("manager cannot be nil")
	}

	return &Pruner{
		config:  config,
		manager: manager,
	}
}

// PruneSessions prunes sessions based on the configured strategy
func (p *Pruner) PruneSessions() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch p.config.Strategy {
	case PruneStrategyLRU:
		return p.pruneLRU()
	case PruneStrategyLFU:
		return p.pruneLFU()
	case PruneStrategyTTL:
		return p.pruneTTL()
	case PruneStrategySize:
		return p.pruneSize()
	case PruneStrategySemantic:
		return p.pruneSemantic()
	default:
		return fmt.Errorf("unknown prune strategy: %s", p.config.Strategy)
	}
}

// pruneLRU removes least recently used sessions
func (p *Pruner) pruneLRU() error {
	sessions, err := p.listAllSessions()
	if err != nil {
		return err
	}

	// If under limit, nothing to do
	if len(sessions) <= p.config.MaxTotalSessions {
		return nil
	}

	// Sort by last access time (assumes UpdatedAt reflects this)
	type sessionInfo struct {
		key          string
		updatedAt    time.Time
		messageCount int
	}

	sessionInfos := make([]sessionInfo, len(sessions))
	for i, key := range sessions {
		session, err := p.manager.GetOrCreate(key)
		if err != nil {
			continue
		}
		session.mu.RLock()
		sessionInfos[i] = sessionInfo{
			key:          key,
			updatedAt:    session.UpdatedAt,
			messageCount: len(session.Messages),
		}
		session.mu.RUnlock()
	}

	// Sort by UpdatedAt (oldest first)
	for i := 0; i < len(sessionInfos)-1; i++ {
		for j := i + 1; j < len(sessionInfos); j++ {
			if sessionInfos[i].updatedAt.After(sessionInfos[j].updatedAt) {
				sessionInfos[i], sessionInfos[j] = sessionInfos[j], sessionInfos[i]
			}
		}
	}

	// Remove oldest sessions
	toRemove := len(sessions) - p.config.MaxTotalSessions
	for i := 0; i < toRemove; i++ {
		if err := p.manager.Delete(sessionInfos[i].key); err != nil {
			continue
		}
		p.stats.SessionsPruned++
	}

	p.stats.TotalPrunes++
	p.stats.LastPruneAt = time.Now()

	return nil
}

// pruneLFU removes least frequently used sessions
func (p *Pruner) pruneLFU() error {
	// This would require tracking access count in the session
	// For now, fall back to LRU
	return p.pruneLRU()
}

// pruneTTL removes sessions past their TTL
func (p *Pruner) pruneTTL() error {
	sessions, err := p.listAllSessions()
	if err != nil {
		return err
	}

	now := time.Now()
	expiredKeys := []string{}

	for _, key := range sessions {
		session, err := p.manager.GetOrCreate(key)
		if err != nil {
			continue
		}

		session.mu.RLock()
		updatedAt := session.UpdatedAt
		session.mu.RUnlock()

		// Check if session is past TTL
		if now.Sub(updatedAt) > p.config.DefaultMessageTTL {
			expiredKeys = append(expiredKeys, key)
		}
	}

	// Delete expired sessions
	for _, key := range expiredKeys {
		if err := p.manager.Delete(key); err != nil {
			continue
		}
		p.stats.SessionsPruned++
	}

	if len(expiredKeys) > 0 {
		p.stats.TotalPrunes++
		p.stats.LastPruneAt = time.Now()
	}

	return nil
}

// pruneSize removes largest sessions first
func (p *Pruner) pruneSize() error {
	sessions, err := p.listAllSessions()
	if err != nil {
		return err
	}

	// Calculate total messages
	totalMessages := 0
	type sessionSize struct {
		key  string
		size int
	}

	sessionSizes := make([]sessionSize, len(sessions))
	for i, key := range sessions {
		session, err := p.manager.GetOrCreate(key)
		if err != nil {
			continue
		}

		session.mu.RLock()
		size := len(session.Messages)
		session.mu.RUnlock()

		sessionSizes[i] = sessionSize{key: key, size: size}
		totalMessages += size
	}

	// If under limit, nothing to do
	if totalMessages <= p.config.MaxTotalMessages {
		return nil
	}

	// Sort by size (largest first)
	for i := 0; i < len(sessionSizes)-1; i++ {
		for j := i + 1; j < len(sessionSizes); j++ {
			if sessionSizes[i].size < sessionSizes[j].size {
				sessionSizes[i], sessionSizes[j] = sessionSizes[j], sessionSizes[i]
			}
		}
	}

	// Remove largest sessions until under limit
	removed := 0
	for _, ss := range sessionSizes {
		if totalMessages <= p.config.MaxTotalMessages {
			break
		}

		if err := p.manager.Delete(ss.key); err != nil {
			continue
		}

		totalMessages -= ss.size
		p.stats.MessagesPruned += int64(ss.size)
		p.stats.SessionsPruned++
		removed++
	}

	if removed > 0 {
		p.stats.TotalPrunes++
		p.stats.LastPruneAt = time.Now()
	}

	return nil
}

// pruneSemantic uses semantic similarity to deduplicate sessions
func (p *Pruner) pruneSemantic() error {
	// This requires embedding support
	// For now, fall back to TTL
	return p.pruneTTL()
}

// PruneMessages prunes messages within a session based on TTL
func (p *Pruner) PruneMessages(sessionKey string, preserveCount int) error {
	session, err := p.manager.GetOrCreate(sessionKey)
	if err != nil {
		return err
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	if len(session.Messages) <= preserveCount {
		return nil
	}

	// Remove oldest messages
	prunedCount := len(session.Messages) - preserveCount
	session.Messages = session.Messages[preserveCount:]

	p.stats.MessagesPruned += int64(prunedCount)
	p.stats.LastPruneAt = time.Now()

	return nil
}

// PruneMessagesByTTL prunes messages based on their time-to-live
func (p *Pruner) PruneMessagesByTTL(sessionKey string) error {
	session, err := p.manager.GetOrCreate(sessionKey)
	if err != nil {
		return err
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-p.config.DefaultMessageTTL)

	// Find first message within TTL
	firstValid := 0
	for i, msg := range session.Messages {
		if msg.Timestamp.After(cutoff) {
			firstValid = i
			break
		}
	}

	if firstValid > 0 {
		prunedCount := firstValid
		session.Messages = session.Messages[firstValid:]
		p.stats.MessagesPruned += int64(prunedCount)
		p.stats.LastPruneAt = time.Now()
	}

	return nil
}

// PruneByType prunes messages based on session type (DM vs Group)
func (p *Pruner) PruneByType(sessionKey string, isDM bool) error {
	preserveCount := p.config.GroupPreserveCount
	if isDM {
		preserveCount = p.config.DMPreserveCount
	}

	return p.PruneMessages(sessionKey, preserveCount)
}

// SetConfig updates the prune configuration
func (p *Pruner) SetConfig(config PruneConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.config = config
}

// GetConfig returns the current prune configuration
func (p *Pruner) GetConfig() PruneConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.config
}

// GetStats returns pruning statistics
func (p *Pruner) GetStats() PruneStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.stats
}

// EstimateTokens estimates the token count for messages
func (p *Pruner) EstimateMessages(sessionKey string) int {
	session, err := p.manager.GetOrCreate(sessionKey)
	if err != nil {
		return 0
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	// Rough estimate: ~4 chars per token
	totalChars := 0
	for _, msg := range session.Messages {
		totalChars += len(msg.Content)
	}

	return totalChars / 4
}

// listAllSessions lists all session keys
func (p *Pruner) listAllSessions() ([]string, error) {
	return p.manager.List()
}

// ShouldCompact determines if a session should be compacted
func (p *Pruner) ShouldCompact(sessionKey string, estimatedTokens int) bool {
	session, err := p.manager.GetOrCreate(sessionKey)
	if err != nil {
		return false
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	// Check if approaching token limit
	if estimatedTokens > p.config.MaxTotalTokens*80/100 {
		return true
	}

	// Check if message count is high
	if len(session.Messages) > p.config.DMPreserveCount*2 {
		return true
	}

	return false
}

// CompactSession compacts a session by summarizing older messages
func (p *Pruner) CompactSession(sessionKey string) error {
	session, err := p.manager.GetOrCreate(sessionKey)
	if err != nil {
		return err
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	if len(session.Messages) == 0 {
		return nil
	}

	// Determine preserve count based on session type
	preserveCount := p.config.GroupPreserveCount
	// Assume DM if metadata says so (you could enhance this)
	if sessionType, ok := session.Metadata["type"].(string); ok && sessionType == "dm" {
		preserveCount = p.config.DMPreserveCount
	}

	if len(session.Messages) <= preserveCount {
		return nil
	}

	// Create summary of older messages
	olderMessages := session.Messages[:len(session.Messages)-preserveCount]
	summaryText := fmt.Sprintf("[Summary of %d earlier messages: ", len(olderMessages))

	// Simple summary: collect topics
	topics := make(map[string]bool)
	for _, msg := range olderMessages {
		if len(msg.Content) > 0 {
			// Extract first few words as topic
			words := 0
			topic := ""
			for _, word := range msg.Content {
				if word == ' ' {
					words++
					if words >= 3 {
						break
					}
				}
				topic += string(word)
			}
			if topic != "" {
				topics[topic] = true
			}
		}
	}

	// Add topics to summary
	for topic := range topics {
		summaryText += topic + ", "
	}
	summaryText += "end summary]"

	// Create summary message
	summaryMsg := Message{
		Role:      "system",
		Content:   summaryText,
		Timestamp: time.Now(),
		Metadata:  map[string]interface{}{"summary": true, "original_count": len(olderMessages)},
	}

	// Keep recent messages and add summary
	newMessages := []Message{summaryMsg}
	newMessages = append(newMessages, session.Messages[len(olderMessages):]...)

	session.Messages = newMessages
	session.UpdatedAt = time.Now()

	p.stats.MessagesPruned += int64(len(olderMessages) - 1)
	p.stats.LastPruneAt = time.Now()

	return nil
}

// Cleanup removes expired data and optimizes storage
func (p *Pruner) Cleanup() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Run TTL-based pruning first
	if err := p.pruneTTL(); err != nil {
		return err
	}

	// Then run strategy-based pruning
	if err := p.PruneSessions(); err != nil {
		return err
	}

	return nil
}
