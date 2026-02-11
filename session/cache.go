package session

import (
	"sync"
	"time"
)

// CacheConfig configures the session cache
type CacheConfig struct {
	MaxSize      int           // Maximum number of sessions to cache
	DefaultTTL   time.Duration // Default TTL for cache entries
	CleanupIntvl time.Duration // Interval for cleanup routine
}

// DefaultCacheConfig returns default cache configuration
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		MaxSize:      100,
		DefaultTTL:   1 * time.Hour,
		CleanupIntvl: 5 * time.Minute,
	}
}

// CachedSession represents a cached session with metadata
type CachedSession struct {
	Session      *Session
	AccessCount  int
	CreatedAt    time.Time
	LastAccessed time.Time
	ExpiresAt    time.Time
	Size         int // Estimated size in bytes
}

// Cache implements an LRU cache with TTL for sessions
type Cache struct {
	sessions map[string]*CachedSession
	lruList  []string // LRU tracking (least recently used at end)
	mu       sync.RWMutex
	config   CacheConfig
	stats    CacheStats
	stopChan chan struct{}
}

// CacheStats contains cache statistics
type CacheStats struct {
	Hits        int64
	Misses      int64
	Evictions   int64
	Expirations int64
	Size        int
	MaxSize     int
}

// NewCache creates a new session cache
func NewCache(config CacheConfig) *Cache {
	if config.MaxSize <= 0 {
		config.MaxSize = 100
	}
	if config.DefaultTTL <= 0 {
		config.DefaultTTL = 1 * time.Hour
	}
	if config.CleanupIntvl <= 0 {
		config.CleanupIntvl = 5 * time.Minute
	}

	cache := &Cache{
		sessions: make(map[string]*CachedSession),
		lruList:  make([]string, 0),
		config:   config,
		stopChan: make(chan struct{}),
		stats: CacheStats{
			MaxSize: config.MaxSize,
		},
	}

	// Start cleanup routine
	go cache.cleanupRoutine()

	return cache
}

// Get retrieves a session from cache
func (c *Cache) Get(key string) (*Session, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	cached, ok := c.sessions[key]
	if !ok {
		c.stats.Misses++
		return nil, false
	}

	// Check expiration
	if time.Now().After(cached.ExpiresAt) {
		c.remove(key)
		c.stats.Expirations++
		c.stats.Misses++
		return nil, false
	}

	// Update access info
	cached.LastAccessed = time.Now()
	cached.AccessCount++
	c.stats.Hits++

	// Move to front of LRU list
	c.updateLRU(key)

	return cached.Session, true
}

// Set adds or updates a session in the cache
func (c *Cache) Set(key string, session *Session) {
	c.SetWithTTL(key, session, c.config.DefaultTTL)
}

// SetWithTTL adds or updates a session with a specific TTL
func (c *Cache) SetWithTTL(key string, session *Session, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	expiresAt := time.Now().Add(ttl)

	// Estimate session size
	size := c.estimateSessionSize(session)

	if existing, ok := c.sessions[key]; ok {
		// Update existing entry
		existing.Session = session
		existing.LastAccessed = time.Now()
		existing.ExpiresAt = expiresAt
		existing.Size = size
		c.updateLRU(key)
		return
	}

	// Check if we need to evict
	if len(c.sessions) >= c.config.MaxSize {
		c.evictLRU()
	}

	// Add new entry
	c.sessions[key] = &CachedSession{
		Session:      session,
		AccessCount:  1,
		CreatedAt:    time.Now(),
		LastAccessed: time.Now(),
		ExpiresAt:    expiresAt,
		Size:         size,
	}

	// Add to front of LRU list
	c.lruList = append([]string{key}, c.lruList...)
	c.stats.Size = len(c.sessions)
}

// Delete removes a session from the cache
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.remove(key)
}

// remove removes a session from the cache (must be called with lock held)
func (c *Cache) remove(key string) {
	delete(c.sessions, key)

	// Remove from LRU list
	for i, k := range c.lruList {
		if k == key {
			c.lruList = append(c.lruList[:i], c.lruList[i+1:]...)
			break
		}
	}

	c.stats.Size = len(c.sessions)
}

// evictLRU evicts the least recently used session
func (c *Cache) evictLRU() {
	if len(c.lruList) == 0 {
		return
	}

	// Get LRU key (last element)
	lruKey := c.lruList[len(c.lruList)-1]

	// Remove from cache
	delete(c.sessions, lruKey)

	// Remove from LRU list
	c.lruList = c.lruList[:len(c.lruList)-1]

	c.stats.Evictions++
	c.stats.Size = len(c.sessions)
}

// updateLRU moves a key to the front of the LRU list
func (c *Cache) updateLRU(key string) {
	// Remove from current position
	for i, k := range c.lruList {
		if k == key {
			c.lruList = append(c.lruList[:i], c.lruList[i+1:]...)
			break
		}
	}

	// Add to front
	c.lruList = append([]string{key}, c.lruList...)
}

// estimateSessionSize estimates the size of a session in bytes
func (c *Cache) estimateSessionSize(session *Session) int {
	size := 0

	// Base session overhead
	size += 100 // Rough estimate for struct overhead

	// Messages
	for _, msg := range session.Messages {
		size += len(msg.Content)
		size += len(msg.Role) * 2
		size += 50 // Per message overhead

		// Media
		for _, media := range msg.Media {
			size += len(media.Type)
			size += len(media.URL)
			size += len(media.Base64)
			size += len(media.MimeType)
			size += 20 // Per media overhead
		}

		// Tool calls
		for _, tc := range msg.ToolCalls {
			size += len(tc.ID)
			size += len(tc.Name)
			size += 20 // Per tool call overhead
		}

		// Metadata
		for k, v := range msg.Metadata {
			size += len(k)
			size += len(toString(v))
		}
	}

	return size
}

// toString converts interface{} to string for size estimation
func toString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// Contains checks if a key exists in the cache
func (c *Cache) Contains(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	_, ok := c.sessions[key]
	return ok
}

// Keys returns all keys in the cache
func (c *Cache) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([]string, 0, len(c.sessions))
	for k := range c.sessions {
		keys = append(keys, k)
	}
	return keys
}

// Clear removes all entries from the cache
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.sessions = make(map[string]*CachedSession)
	c.lruList = make([]string, 0)
	c.stats.Size = 0
}

// Size returns the current number of cached sessions
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.sessions)
}

// Stats returns cache statistics
func (c *Cache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.stats
}

// cleanupRoutine periodically expires stale cache entries
func (c *Cache) cleanupRoutine() {
	ticker := time.NewTicker(c.config.CleanupIntvl)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopChan:
			return
		}
	}
}

// cleanup removes expired entries
func (c *Cache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	expiredKeys := make([]string, 0)

	for key, cached := range c.sessions {
		if now.After(cached.ExpiresAt) {
			expiredKeys = append(expiredKeys, key)
		}
	}

	for _, key := range expiredKeys {
		c.remove(key)
		c.stats.Expirations++
	}
}

// Close stops the cleanup routine
func (c *Cache) Close() {
	close(c.stopChan)
}

// RefreshTTL refreshes the TTL for a cached session
func (c *Cache) RefreshTTL(key string, ttl time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	cached, ok := c.sessions[key]
	if !ok {
		return false
	}

	cached.ExpiresAt = time.Now().Add(ttl)
	cached.LastAccessed = time.Now()
	c.updateLRU(key)

	return true
}

// GetSession retrieves cached session without updating access time
func (c *Cache) GetSession(key string) (*Session, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cached, ok := c.sessions[key]
	if !ok {
		return nil, false
	}

	// Check expiration
	if time.Now().After(cached.ExpiresAt) {
		return nil, false
	}

	return cached.Session, true
}

// PruneBySize removes sessions until total size is under the limit
func (c *Cache) PruneBySize(maxTotalSize int) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	totalSize := 0
	for _, cached := range c.sessions {
		totalSize += cached.Size
	}

	if totalSize <= maxTotalSize {
		return 0
	}

	evicted := 0
	for totalSize > maxTotalSize && len(c.lruList) > 0 {
		// Evict LRU
		lruKey := c.lruList[len(c.lruList)-1]
		if cached, ok := c.sessions[lruKey]; ok {
			totalSize -= cached.Size
		}

		c.evictLRU()
		evicted++
	}

	return evicted
}

// PruneByAccessCount removes sessions with access count below threshold
func (c *Cache) PruneByAccessCount(minAccessCount int) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	toRemove := make([]string, 0)
	for key, cached := range c.sessions {
		if cached.AccessCount < minAccessCount {
			toRemove = append(toRemove, key)
		}
	}

	for _, key := range toRemove {
		c.remove(key)
	}

	return len(toRemove)
}

// GetOldest returns the oldest cached session
func (c *Cache) GetOldest() (*CachedSession, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.lruList) == 0 {
		return nil, false
	}

	oldestKey := c.lruList[len(c.lruList)-1]
	cached, ok := c.sessions[oldestKey]
	if !ok {
		return nil, false
	}

	return cached, true
}

// GetNewest returns the newest cached session
func (c *Cache) GetNewest() (*CachedSession, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.lruList) == 0 {
		return nil, false
	}

	newestKey := c.lruList[0]
	cached, ok := c.sessions[newestKey]
	if !ok {
		return nil, false
	}

	return cached, true
}

// HitRate returns the cache hit rate as a percentage
func (c *Cache) HitRate() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.stats.Hits + c.stats.Misses
	if total == 0 {
		return 0.0
	}

	return float64(c.stats.Hits) / float64(total) * 100.0
}
