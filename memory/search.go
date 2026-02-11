package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// MemoryManager manages memory storage and retrieval
type MemoryManager struct {
	store        Store
	provider     EmbeddingProvider
	mu           sync.RWMutex
	cache        map[string]*VectorEmbedding
	cacheMaxSize int
	initialized  bool
}

// ManagerConfig configures the memory manager
type ManagerConfig struct {
	Store        Store
	Provider     EmbeddingProvider
	CacheMaxSize int
}

// DefaultManagerConfig returns default manager configuration
func DefaultManagerConfig(store Store, provider EmbeddingProvider) ManagerConfig {
	return ManagerConfig{
		Store:        store,
		Provider:     provider,
		CacheMaxSize: 1000,
	}
}

// NewMemoryManager creates a new memory manager
func NewMemoryManager(config ManagerConfig) (*MemoryManager, error) {
	if config.Store == nil {
		return nil, fmt.Errorf("store is required")
	}
	if config.Provider == nil {
		return nil, fmt.Errorf("provider is required")
	}

	if config.CacheMaxSize == 0 {
		config.CacheMaxSize = 1000
	}

	return &MemoryManager{
		store:        config.Store,
		provider:     config.Provider,
		cache:        make(map[string]*VectorEmbedding),
		cacheMaxSize: config.CacheMaxSize,
		initialized:  true,
	}, nil
}

// AddMemory adds a new memory with automatic embedding generation
func (m *MemoryManager) AddMemory(ctx context.Context, text string, source MemorySource, memType MemoryType, metadata MemoryMetadata) (*VectorEmbedding, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Generate embedding
	embedding, err := m.provider.Embed(text)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	ve := &VectorEmbedding{
		Vector:    embedding,
		Dimension: len(embedding),
		Text:      text,
		Source:    source,
		Type:      memType,
		Metadata:  metadata,
	}

	// Store the memory
	if err := m.store.Add(ve); err != nil {
		return nil, fmt.Errorf("failed to store memory: %w", err)
	}

	// Update cache
	m.updateCache(ve)

	return ve, nil
}

// AddMemoryBatch adds multiple memories with automatic embedding generation
func (m *MemoryManager) AddMemoryBatch(ctx context.Context, items []MemoryItem) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Prepare texts for embedding
	texts := make([]string, len(items))
	for i, item := range items {
		texts[i] = item.Text
	}

	// Generate embeddings in batch
	embeddings, err := m.provider.EmbedBatch(texts)
	if err != nil {
		return fmt.Errorf("failed to generate embeddings: %w", err)
	}

	// Create VectorEmbedding objects
	ves := make([]*VectorEmbedding, len(items))
	for i, item := range items {
		ves[i] = &VectorEmbedding{
			Vector:    embeddings[i],
			Dimension: len(embeddings[i]),
			Text:      item.Text,
			Source:    item.Source,
			Type:      item.Type,
			Metadata:  item.Metadata,
		}
	}

	// Store the memories
	if err := m.store.AddBatch(ves); err != nil {
		return fmt.Errorf("failed to store memories: %w", err)
	}

	// Update cache
	for _, ve := range ves {
		m.updateCache(ve)
	}

	return nil
}

// MemoryItem represents a memory to be added
type MemoryItem struct {
	Text     string
	Source   MemorySource
	Type     MemoryType
	Metadata MemoryMetadata
}

// Search searches for similar memories
func (m *MemoryManager) Search(ctx context.Context, query string, opts SearchOptions) ([]*SearchResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Generate query embedding
	queryVec, err := m.provider.Embed(query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	// Perform search
	results, err := m.store.Search(queryVec, opts)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	return results, nil
}

// Get retrieves a memory by ID
func (m *MemoryManager) Get(ctx context.Context, id string) (*VectorEmbedding, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check cache first
	if ve, ok := m.cache[id]; ok {
		return ve, nil
	}

	// Fetch from store
	ve, err := m.store.Get(id)
	if err != nil {
		return nil, err
	}

	// Update cache
	m.updateCache(ve)

	return ve, nil
}

// Update updates an existing memory
func (m *MemoryManager) Update(ctx context.Context, ve *VectorEmbedding) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Re-generate embedding if text changed
	if len(ve.Vector) == 0 {
		embedding, err := m.provider.Embed(ve.Text)
		if err != nil {
			return fmt.Errorf("failed to generate embedding: %w", err)
		}
		ve.Vector = embedding
		ve.Dimension = len(embedding)
	}

	// Update in store
	if err := m.store.Update(ve); err != nil {
		return fmt.Errorf("failed to update memory: %w", err)
	}

	// Update cache
	m.cache[ve.ID] = ve

	return nil
}

// Delete removes a memory
func (m *MemoryManager) Delete(ctx context.Context, id string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Delete from store
	if err := m.store.Delete(id); err != nil {
		return fmt.Errorf("failed to delete memory: %w", err)
	}

	// Remove from cache
	delete(m.cache, id)

	return nil
}

// List lists all memories with optional filtering
func (m *MemoryManager) List(ctx context.Context, filter func(*VectorEmbedding) bool) ([]*VectorEmbedding, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.store.List(filter)
}

// SearchBySource searches memories by source
func (m *MemoryManager) SearchBySource(ctx context.Context, source MemorySource) ([]*VectorEmbedding, error) {
	return m.List(ctx, func(ve *VectorEmbedding) bool {
		return ve.Source == source
	})
}

// SearchByType searches memories by type
func (m *MemoryManager) SearchByType(ctx context.Context, memType MemoryType) ([]*VectorEmbedding, error) {
	return m.List(ctx, func(ve *VectorEmbedding) bool {
		return ve.Type == memType
	})
}

// SearchByTag searches memories by tag
func (m *MemoryManager) SearchByTag(ctx context.Context, tag string) ([]*VectorEmbedding, error) {
	return m.List(ctx, func(ve *VectorEmbedding) bool {
		for _, t := range ve.Metadata.Tags {
			if strings.EqualFold(t, tag) {
				return true
			}
		}
		return false
	})
}

// SearchByText searches memories by text content (simple substring match)
func (m *MemoryManager) SearchByText(ctx context.Context, query string) ([]*VectorEmbedding, error) {
	queryLower := strings.ToLower(query)
	return m.List(ctx, func(ve *VectorEmbedding) bool {
		return strings.Contains(strings.ToLower(ve.Text), queryLower)
	})
}

// GetStats returns statistics about the memory store
func (m *MemoryManager) GetStats(ctx context.Context) (*MemoryStats, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	all, err := m.store.List(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list memories: %w", err)
	}

	stats := &MemoryStats{
		TotalCount:   len(all),
		SourceCounts: make(map[MemorySource]int),
		TypeCounts:   make(map[MemoryType]int),
		CacheSize:    len(m.cache),
		CacheMaxSize: m.cacheMaxSize,
	}

	for _, ve := range all {
		stats.SourceCounts[ve.Source]++
		stats.TypeCounts[ve.Type]++
	}

	return stats, nil
}

// MemoryStats contains statistics about the memory store
type MemoryStats struct {
	TotalCount   int                  `json:"total_count"`
	SourceCounts map[MemorySource]int `json:"source_counts"`
	TypeCounts   map[MemoryType]int   `json:"type_counts"`
	CacheSize    int                  `json:"cache_size"`
	CacheMaxSize int                  `json:"cache_max_size"`
}

// ClearCache clears the in-memory cache
func (m *MemoryManager) ClearCache() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cache = make(map[string]*VectorEmbedding)
}

// Close closes the memory manager
func (m *MemoryManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.initialized {
		return nil
	}

	m.initialized = false

	// Clear cache
	m.cache = nil

	// Close store
	return m.store.Close()
}

// updateCache updates the cache with a new memory
func (m *MemoryManager) updateCache(ve *VectorEmbedding) {
	// If cache is full, remove oldest entries
	if len(m.cache) >= m.cacheMaxSize {
		// Simple FIFO eviction
		for k := range m.cache {
			delete(m.cache, k)
			break
		}
	}

	m.cache[ve.ID] = ve
}
