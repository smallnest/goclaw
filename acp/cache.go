package acp

import (
	"sync"
	"time"

	"github.com/smallnest/goclaw/acp/runtime"
)

// RuntimeCache caches active runtime handles.
type RuntimeCache struct {
	mu            sync.RWMutex
	states        map[string]*CachedRuntimeState
	evictedTotal  int
	lastEvictedAt *int64
}

// CachedRuntimeState represents a cached runtime state.
type CachedRuntimeState struct {
	runtime       runtime.AcpRuntime
	handle        runtime.AcpRuntimeHandle
	backend       string
	agent         string
	mode          runtime.AcpRuntimeSessionMode
	cwd           string
	lastTouchedAt time.Time
}

// NewRuntimeCache creates a new runtime cache.
func NewRuntimeCache() *RuntimeCache {
	return &RuntimeCache{
		states: make(map[string]*CachedRuntimeState),
	}
}

// Get retrieves a cached runtime state and updates lastTouchedAt.
func (c *RuntimeCache) Get(sessionKey string) *CachedRuntimeState {
	c.mu.Lock()
	defer c.mu.Unlock()

	if state, exists := c.states[sessionKey]; exists {
		state.lastTouchedAt = time.Now()
		return state
	}
	return nil
}

// Set stores a runtime state in the cache.
func (c *RuntimeCache) Set(sessionKey string, state *CachedRuntimeState) {
	c.mu.Lock()
	defer c.mu.Unlock()

	state.lastTouchedAt = time.Now()
	c.states[sessionKey] = state
}

// Peek retrieves a cached state without updating lastTouchedAt.
func (c *RuntimeCache) Peek(sessionKey string) *CachedRuntimeState {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.states[sessionKey]
}

// Clear removes a state from the cache.
func (c *RuntimeCache) Clear(sessionKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.states, sessionKey)
}

// Has checks if a session key is in the cache.
func (c *RuntimeCache) Has(sessionKey string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	_, exists := c.states[sessionKey]
	return exists
}

// Size returns the number of cached states.
func (c *RuntimeCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.states)
}

// GetLastTouchedAt returns the last touched time for a session.
func (c *RuntimeCache) GetLastTouchedAt(sessionKey string) time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if state, exists := c.states[sessionKey]; exists {
		return state.lastTouchedAt
	}
	return time.Time{}
}

// IdleCandidate represents a candidate for idle eviction.
type IdleCandidate struct {
	SessionKey    string
	LastTouchedAt time.Time
	Handle        *runtime.AcpRuntimeHandle
}

// CollectIdleCandidates collects sessions that have been idle longer than maxIdleMs.
func (c *RuntimeCache) CollectIdleCandidates(maxIdle time.Duration, now time.Time) []IdleCandidate {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var candidates []IdleCandidate
	for sessionKey, state := range c.states {
		idleTime := now.Sub(state.lastTouchedAt)
		if idleTime >= maxIdle {
			candidates = append(candidates, IdleCandidate{
				SessionKey:    sessionKey,
				LastTouchedAt: state.lastTouchedAt,
				Handle:        &state.handle,
			})
		}
	}

	return candidates
}

// IncrementEvicted increments the evicted counter.
func (c *RuntimeCache) IncrementEvicted() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.evictedTotal++
	now := time.Now().UnixMilli()
	c.lastEvictedAt = &now
}

// GetSnapshot returns a snapshot of the cache statistics.
func (c *RuntimeCache) GetSnapshot() RuntimeCacheSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()

	snapshot := RuntimeCacheSnapshot{
		ActiveSessions: len(c.states),
		EvictedTotal:   c.evictedTotal,
	}

	if c.lastEvictedAt != nil {
		snapshot.LastEvictedAt = c.lastEvictedAt
	}

	return snapshot
}
