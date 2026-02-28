package memory

import (
	"container/list"
	"sync"
)

// LRUCache is a thread-safe LRU (Least Recently Used) cache
type LRUCache struct {
	mu      sync.RWMutex
	maxSize int
	items   map[string]*list.Element // id -> list element
	lruList *list.List               // LRU order: front = most recent, back = least recent
	onEvict func(key string, value *VectorEmbedding)
}

// lruEntry represents a cache entry with its key and value
type lruEntry struct {
	key   string
	value *VectorEmbedding
}

// NewLRUCache creates a new LRU cache with the specified maximum size
func NewLRUCache(maxSize int) *LRUCache {
	if maxSize <= 0 {
		maxSize = 1000
	}

	return &LRUCache{
		maxSize: maxSize,
		items:   make(map[string]*list.Element),
		lruList: list.New(),
	}
}

// Get retrieves a value from the cache, moving it to the front (most recently used)
func (c *LRUCache) Get(key string) (*VectorEmbedding, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, exists := c.items[key]; exists {
		// Move to front (most recently used)
		c.lruList.MoveToFront(elem)
		return elem.Value.(*lruEntry).value, true
	}
	return nil, false
}

// Put adds a value to the cache, evicting the least recently used item if necessary
func (c *LRUCache) Put(key string, value *VectorEmbedding) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, exists := c.items[key]; exists {
		// Update existing entry and move to front
		c.lruList.MoveToFront(elem)
		elem.Value.(*lruEntry).value = value
		return
	}

	// Add new entry
	elem := c.lruList.PushFront(&lruEntry{key: key, value: value})
	c.items[key] = elem

	// Evict if over capacity
	if c.lruList.Len() > c.maxSize {
		c.evictOldest()
	}
}

// evictOldest removes the least recently used item from the cache
func (c *LRUCache) evictOldest() {
	elem := c.lruList.Back()
	if elem != nil {
		c.removeElement(elem)
	}
}

// removeElement removes an element from the cache
func (c *LRUCache) removeElement(elem *list.Element) {
	if elem == nil {
		return
	}

	entry := elem.Value.(*lruEntry)
	delete(c.items, entry.key)
	c.lruList.Remove(elem)

	if c.onEvict != nil {
		c.onEvict(entry.key, entry.value)
	}
}

// Delete removes a key from the cache
func (c *LRUCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, exists := c.items[key]; exists {
		c.removeElement(elem)
	}
}

// Len returns the number of items in the cache
func (c *LRUCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lruList.Len()
}

// Clear removes all items from the cache
func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element)
	c.lruList.Init()
}

// Keys returns all keys in the cache (ordered from most to least recent)
func (c *LRUCache) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([]string, 0, c.lruList.Len())
	for elem := c.lruList.Front(); elem != nil; elem = elem.Next() {
		keys = append(keys, elem.Value.(*lruEntry).key)
	}
	return keys
}

// SetOnEvict sets a callback function to be called when items are evicted
func (c *LRUCache) SetOnEvict(fn func(key string, value *VectorEmbedding)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onEvict = fn
}

// Peek retrieves a value without updating its LRU position
func (c *LRUCache) Peek(key string) (*VectorEmbedding, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if elem, exists := c.items[key]; exists {
		return elem.Value.(*lruEntry).value, true
	}
	return nil, false
}
