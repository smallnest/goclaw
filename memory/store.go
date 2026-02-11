package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/glebarez/sqlite"
	"github.com/google/uuid"
)

// SQLiteStore implements the Store interface using SQLite
type SQLiteStore struct {
	db          *sql.DB
	dbPath      string
	provider    EmbeddingProvider
	mu          sync.RWMutex
	initialized bool
}

// StoreConfig configures the SQLite memory store
type StoreConfig struct {
	// DBPath is the path to the SQLite database file
	DBPath string
	// Provider is the embedding provider to use
	Provider EmbeddingProvider
	// EnableVectorSearch enables vector similarity search (requires sqlite-vec)
	EnableVectorSearch bool
	// EnableFTS enables full-text search
	EnableFTS bool
	// VectorExtensionPath is the path to the sqlite-vec extension
	VectorExtensionPath string
}

// DefaultStoreConfig returns default store configuration
func DefaultStoreConfig(dbPath string, provider EmbeddingProvider) StoreConfig {
	return StoreConfig{
		DBPath:              dbPath,
		Provider:            provider,
		EnableVectorSearch:  true,
		EnableFTS:           true,
		VectorExtensionPath: "",
	}
}

// NewSQLiteStore creates a new SQLite-based memory store
func NewSQLiteStore(config StoreConfig) (*SQLiteStore, error) {
	if config.DBPath == "" {
		return nil, fmt.Errorf("database path is required")
	}
	if config.Provider == nil {
		return nil, fmt.Errorf("embedding provider is required")
	}

	// Ensure directory exists
	dir := filepath.Dir(config.DBPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database connection
	db, err := sql.Open("sqlite", config.DBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(1) // SQLite works best with single connection
	db.SetMaxIdleConns(1)

	store := &SQLiteStore{
		db:       db,
		dbPath:   config.DBPath,
		provider: config.Provider,
	}

	// Initialize schema
	if err := store.initSchema(config); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	store.initialized = true
	return store, nil
}

// initSchema creates the database schema
func (s *SQLiteStore) initSchema(config StoreConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Enable foreign keys
	if _, err := s.db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Enable WAL mode for better concurrency
	if _, err := s.db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		return fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Create metadata table
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at INTEGER NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("failed to create meta table: %w", err)
	}

	// Create memories table
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS memories (
			id TEXT PRIMARY KEY,
			text TEXT NOT NULL,
			source TEXT NOT NULL,
			type TEXT NOT NULL,
			embedding BLOB,
			dimension INTEGER,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			file_path TEXT,
			line_number INTEGER,
			session_key TEXT,
			tags TEXT,
			importance REAL DEFAULT 0.5,
			access_count INTEGER DEFAULT 0,
			last_accessed INTEGER
		)
	`); err != nil {
		return fmt.Errorf("failed to create memories table: %w", err)
	}

	// Create indexes for common queries
	if _, err := s.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_memories_source
		ON memories(source)
	`); err != nil {
		return fmt.Errorf("failed to create source index: %w", err)
	}

	if _, err := s.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_memories_type
		ON memories(type)
	`); err != nil {
		return fmt.Errorf("failed to create type index: %w", err)
	}

	if _, err := s.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_memories_created_at
		ON memories(created_at)
	`); err != nil {
		return fmt.Errorf("failed to create created_at index: %w", err)
	}

	// Enable vector search if configured
	if config.EnableVectorSearch {
		if err := s.initVectorSearch(); err != nil {
			// Log warning but don't fail - vector search is optional
			fmt.Printf("Warning: vector search initialization failed: %v\n", err)
		}
	}

	// Enable FTS if configured
	if config.EnableFTS {
		if err := s.initFTS(); err != nil {
			// Log warning but don't fail - FTS is optional
			fmt.Printf("Warning: FTS initialization failed: %v\n", err)
		}
	}

	// Store schema version
	s.setMeta("schema_version", "1")
	s.setMeta("provider_dimension", fmt.Sprintf("%d", s.provider.Dimension()))

	return nil
}

// initVectorSearch initializes vector similarity search using sqlite-vec
func (s *SQLiteStore) initVectorSearch() error {
	// Try to load sqlite-vec extension
	extensionPath := os.Getenv("SQLITE_VEC_EXTENSION")
	if extensionPath == "" {
		// Try common locations
		paths := []string{
			"vec0",
			"./vec0.so",
			"./vec0.dylib",
			"/usr/local/lib/vec0",
			"/opt/homebrew/lib/vec0",
		}
		for _, path := range paths {
			if _, err := s.db.Exec(fmt.Sprintf("SELECT load_extension('%s')", path)); err == nil {
				extensionPath = path
				break
			}
		}
		if extensionPath == "" {
			return fmt.Errorf("sqlite-vec extension not found")
		}
	} else {
		if _, err := s.db.Exec(fmt.Sprintf("SELECT load_extension('%s')", extensionPath)); err != nil {
			return fmt.Errorf("failed to load sqlite-vec from %s: %w", extensionPath, err)
		}
	}

	dimension := s.provider.Dimension()

	// Create virtual table for vector search
	_, err := s.db.Exec(fmt.Sprintf(`
		CREATE VIRTUAL TABLE IF NOT EXISTS memory_vec USING vec0(
			id TEXT PRIMARY KEY,
			embedding FLOAT[%d]
		)
	`, dimension))

	if err != nil {
		return fmt.Errorf("failed to create vec0 table: %w", err)
	}

	s.setMeta("vector_enabled", "true")
	return nil
}

// initFTS initializes full-text search using FTS5
func (s *SQLiteStore) initFTS() error {
	_, err := s.db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(
			text,
			id UNINDEXED,
			source UNINDEXED,
			type UNINDEXED
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create FTS5 table: %w", err)
	}

	s.setMeta("fts_enabled", "true")
	return nil
}

// setMeta stores a key-value pair in the metadata table
func (s *SQLiteStore) setMeta(key, value string) {
	now := time.Now().Unix()
	_, _ = s.db.Exec(`
		INSERT INTO meta (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at
	`, key, value, now)
}

// Add adds a memory to the store
func (s *SQLiteStore) Add(embedding *VectorEmbedding) error {
	if embedding.ID == "" {
		embedding.ID = uuid.New().String()
	}
	if embedding.CreatedAt.IsZero() {
		embedding.CreatedAt = time.Now()
	}
	embedding.UpdatedAt = time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Serialize tags
	var tagsJSON string
	if len(embedding.Metadata.Tags) > 0 {
		tagsBytes, err := json.Marshal(embedding.Metadata.Tags)
		if err == nil {
			tagsJSON = string(tagsBytes)
		}
	}

	// Serialize embedding
	var embeddingJSON string
	if len(embedding.Vector) > 0 {
		embBytes, err := json.Marshal(embedding.Vector)
		if err != nil {
			return fmt.Errorf("failed to marshal embedding: %w", err)
		}
		embeddingJSON = string(embBytes)
	}

	// Insert into memories table
	_, err = tx.Exec(`
		INSERT INTO memories (
			id, text, source, type, embedding, dimension,
			created_at, updated_at, file_path, line_number,
			session_key, tags, importance, access_count, last_accessed
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, embedding.ID, embedding.Text, embedding.Source, embedding.Type,
		embeddingJSON, len(embedding.Vector), embedding.CreatedAt.Unix(),
		embedding.UpdatedAt.Unix(), embedding.Metadata.FilePath,
		embedding.Metadata.LineNumber, embedding.Metadata.SessionKey,
		tagsJSON, embedding.Metadata.Importance, embedding.Metadata.AccessCount,
		embedding.Metadata.LastAccessed.Unix())

	if err != nil {
		return fmt.Errorf("failed to insert memory: %w", err)
	}

	// Insert into vector table if enabled
	if s.isVectorEnabled() && len(embedding.Vector) > 0 {
		if err := s.insertVector(tx, embedding.ID, embedding.Vector); err != nil {
			return fmt.Errorf("failed to insert vector: %w", err)
		}
	}

	// Insert into FTS if enabled
	if s.isFTSEnabled() {
		if err := s.insertFTS(tx, embedding); err != nil {
			return fmt.Errorf("failed to insert FTS: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// AddBatch adds multiple memories in one transaction
func (s *SQLiteStore) AddBatch(embeddings []*VectorEmbedding) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`
		INSERT INTO memories (
			id, text, source, type, embedding, dimension,
			created_at, updated_at, file_path, line_number,
			session_key, tags, importance, access_count, last_accessed
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, emb := range embeddings {
		if emb.ID == "" {
			emb.ID = uuid.New().String()
		}
		if emb.CreatedAt.IsZero() {
			emb.CreatedAt = time.Now()
		}
		emb.UpdatedAt = time.Now()

		var tagsJSON string
		if len(emb.Metadata.Tags) > 0 {
			tagsBytes, _ := json.Marshal(emb.Metadata.Tags)
			tagsJSON = string(tagsBytes)
		}

		var embeddingJSON string
		if len(emb.Vector) > 0 {
			embBytes, _ := json.Marshal(emb.Vector)
			embeddingJSON = string(embBytes)
		}

		_, err := stmt.Exec(
			emb.ID, emb.Text, emb.Source, emb.Type,
			embeddingJSON, len(emb.Vector), emb.CreatedAt.Unix(),
			emb.UpdatedAt.Unix(), emb.Metadata.FilePath,
			emb.Metadata.LineNumber, emb.Metadata.SessionKey,
			tagsJSON, emb.Metadata.Importance, emb.Metadata.AccessCount,
			emb.Metadata.LastAccessed.Unix(),
		)
		if err != nil {
			return fmt.Errorf("failed to insert memory %s: %w", emb.ID, err)
		}

		if s.isVectorEnabled() && len(emb.Vector) > 0 {
			if err := s.insertVector(tx, emb.ID, emb.Vector); err != nil {
				return fmt.Errorf("failed to insert vector for %s: %w", emb.ID, err)
			}
		}

		if s.isFTSEnabled() {
			if err := s.insertFTS(tx, emb); err != nil {
				return fmt.Errorf("failed to insert FTS for %s: %w", emb.ID, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// insertVector inserts a vector into the vec0 table
func (s *SQLiteStore) insertVector(tx *sql.Tx, id string, vector []float32) error {
	// Convert float32 slice to comma-separated string
	vectorStr := float32SliceToString(vector)

	_, err := tx.Exec(`
		INSERT INTO memory_vec (id, embedding)
		VALUES (?, ?)
		ON CONFLICT(id) DO UPDATE SET
			embedding = excluded.embedding
	`, id, vectorStr)

	return err
}

// insertFTS inserts text into the FTS table
func (s *SQLiteStore) insertFTS(tx *sql.Tx, embedding *VectorEmbedding) error {
	_, err := tx.Exec(`
		INSERT INTO memory_fts (text, id, source, type)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			text = excluded.text
	`, embedding.Text, embedding.ID, embedding.Source, embedding.Type)

	return err
}

// Search performs vector similarity search
func (s *SQLiteStore) Search(query []float32, opts SearchOptions) ([]*SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(query) == 0 {
		return nil, fmt.Errorf("query vector is empty")
	}

	var results []*SearchResult

	// If vector search is enabled, use it
	if s.isVectorEnabled() {
		results, err := s.searchVector(query, opts)
		if err != nil {
			return nil, fmt.Errorf("vector search failed: %w", err)
		}

		// If hybrid search is enabled and FTS is available, combine results
		if opts.Hybrid && s.isFTSEnabled() {
			ftsResults, err := s.searchFTS(query, opts)
			if err == nil && len(ftsResults) > 0 {
				return s.mergeHybridResults(results, ftsResults, opts), nil
			}
		}

		return results, nil
	}

	// Fallback to basic text search
	if s.isFTSEnabled() {
		return s.searchFTS(query, opts)
	}

	// No search available, return empty
	return results, nil
}

// searchVector performs vector similarity search
func (s *SQLiteStore) searchVector(query []float32, opts SearchOptions) ([]*SearchResult, error) {
	queryStr := float32SliceToString(query)

	querySQL := `
		SELECT
			m.id,
			m.text,
			m.source,
			m.type,
			m.created_at,
			m.updated_at,
			m.file_path,
			m.line_number,
			m.session_key,
			m.tags,
			m.importance,
			m.access_count,
			m.last_accessed,
			distance
		FROM memory_vec v
		JOIN memories m ON m.id = v.id
		WHERE v.embedding MATCH ?
		AND m.source IN (` + sourcePlaceholders(opts.Sources) + `)
		AND m.type IN (` + typePlaceholders(opts.Types) + `)
		ORDER BY distance
		LIMIT ?
	`

	args := []interface{}{queryStr}
	args = appendSources(args, opts.Sources)
	args = appendTypes(args, opts.Types)
	args = append(args, opts.Limit)

	rows, err := s.db.Query(querySQL, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var results []*SearchResult
	for rows.Next() {
		var sr SearchResult
		var tagsJSON sql.NullString
		var lastAccessed sql.NullInt64
		var filePath sql.NullString
		var sessionKey sql.NullString
		var lineNumber sql.NullInt64
		var importance sql.NullFloat64
		var accessCount sql.NullInt64

		err := rows.Scan(
			&sr.ID,
			&sr.Text,
			&sr.Source,
			&sr.Type,
			&sr.CreatedAt,
			&sr.UpdatedAt,
			&filePath,
			&lineNumber,
			&sessionKey,
			&tagsJSON,
			&importance,
			&accessCount,
			&lastAccessed,
			&sr.Score,
		)
		if err != nil {
			continue
		}

		// Populate metadata fields
		sr.Metadata.FilePath = filePath.String
		sr.Metadata.LineNumber = int(lineNumber.Int64)
		sr.Metadata.SessionKey = sessionKey.String
		sr.Metadata.Importance = importance.Float64
		sr.Metadata.AccessCount = int(accessCount.Int64)

		if lastAccessed.Valid {
			sr.Metadata.LastAccessed = time.Unix(lastAccessed.Int64, 0)
		}

		if tagsJSON.Valid {
			_ = json.Unmarshal([]byte(tagsJSON.String), &sr.Metadata.Tags)
		}

		// Convert distance to similarity score (lower distance = higher score)
		// Assuming L2 distance, convert to 0-1 range
		sr.Score = 1.0 / (1.0 + sr.Score)

		if sr.Score >= opts.MinScore {
			results = append(results, &sr)
		}
	}

	return results, nil
}

// searchFTS performs full-text search
func (s *SQLiteStore) searchFTS(query []float32, opts SearchOptions) ([]*SearchResult, error) {
	// For now, this is a placeholder
	// In a full implementation, we'd convert the vector to text or use a separate text query
	return []*SearchResult{}, nil
}

// mergeHybridResults combines vector and FTS results
func (s *SQLiteStore) mergeHybridResults(vectorResults, ftsResults []*SearchResult, opts SearchOptions) []*SearchResult {
	// Create a map of results by ID
	resultMap := make(map[string]*SearchResult)

	// Add vector results
	for _, vr := range vectorResults {
		resultMap[vr.ID] = vr
	}

	// Merge FTS results
	for _, fr := range ftsResults {
		if existing, ok := resultMap[fr.ID]; ok {
			// Combine scores using weighted average
			existing.Score = existing.Score*opts.VectorWeight + fr.Score*opts.TextWeight
		} else {
			fr.Score = fr.Score * opts.TextWeight
			resultMap[fr.ID] = fr
		}
	}

	// Convert back to slice and sort by score
	var results []*SearchResult
	for _, r := range resultMap {
		if r.Score >= opts.MinScore {
			results = append(results, r)
		}
	}

	// Sort by score descending
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[i].Score < results[j].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	// Limit results
	if len(results) > opts.Limit {
		results = results[:opts.Limit]
	}

	return results
}

// Get retrieves a memory by ID
func (s *SQLiteStore) Get(id string) (*VectorEmbedding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var ve VectorEmbedding
	var embeddingJSON sql.NullString
	var tagsJSON sql.NullString
	var lastAccessed sql.NullInt64
	var filePath sql.NullString
	var sessionKey sql.NullString
	var lineNumber sql.NullInt64
	var importance sql.NullFloat64
	var accessCount sql.NullInt64

	err := s.db.QueryRow(`
		SELECT
			id, text, source, type, embedding, created_at, updated_at,
			file_path, line_number, session_key, tags, importance,
			access_count, last_accessed
		FROM memories
		WHERE id = ?
	`, id).Scan(
		&ve.ID,
		&ve.Text,
		&ve.Source,
		&ve.Type,
		&embeddingJSON,
		&ve.CreatedAt,
		&ve.UpdatedAt,
		&filePath,
		&lineNumber,
		&sessionKey,
		&tagsJSON,
		&importance,
		&accessCount,
		&lastAccessed,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("memory not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get memory: %w", err)
	}

	// Unmarshal embedding
	if embeddingJSON.Valid {
		_ = json.Unmarshal([]byte(embeddingJSON.String), &ve.Vector)
		ve.Dimension = len(ve.Vector)
	}

	// Populate metadata fields
	ve.Metadata.FilePath = filePath.String
	ve.Metadata.LineNumber = int(lineNumber.Int64)
	ve.Metadata.SessionKey = sessionKey.String
	ve.Metadata.Importance = importance.Float64
	ve.Metadata.AccessCount = int(accessCount.Int64)

	// Unmarshal tags
	if tagsJSON.Valid {
		_ = json.Unmarshal([]byte(tagsJSON.String), &ve.Metadata.Tags)
	}

	// Handle last accessed
	if lastAccessed.Valid {
		ve.Metadata.LastAccessed = time.Unix(lastAccessed.Int64, 0)
	}

	// Update access count
	go s.updateAccessCount(id)

	return &ve, nil
}

// Delete removes a memory by ID
func (s *SQLiteStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Delete from memories table
	if _, err := tx.Exec(`DELETE FROM memories WHERE id = ?`, id); err != nil {
		return fmt.Errorf("failed to delete memory: %w", err)
	}

	// Delete from vector table
	if s.isVectorEnabled() {
		if _, err := tx.Exec(`DELETE FROM memory_vec WHERE id = ?`, id); err != nil {
			return fmt.Errorf("failed to delete vector: %w", err)
		}
	}

	// Delete from FTS table
	if s.isFTSEnabled() {
		if _, err := tx.Exec(`DELETE FROM memory_fts WHERE id = ?`, id); err != nil {
			return fmt.Errorf("failed to delete FTS: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Update updates an existing memory
func (s *SQLiteStore) Update(embedding *VectorEmbedding) error {
	embedding.UpdatedAt = time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	var tagsJSON string
	if len(embedding.Metadata.Tags) > 0 {
		tagsBytes, _ := json.Marshal(embedding.Metadata.Tags)
		tagsJSON = string(tagsBytes)
	}

	var embeddingJSON string
	if len(embedding.Vector) > 0 {
		embBytes, _ := json.Marshal(embedding.Vector)
		embeddingJSON = string(embBytes)
	}

	_, err := s.db.Exec(`
		UPDATE memories SET
			text = ?,
			source = ?,
			type = ?,
			embedding = ?,
			dimension = ?,
			updated_at = ?,
			file_path = ?,
			line_number = ?,
			session_key = ?,
			tags = ?,
			importance = ?,
			access_count = ?,
			last_accessed = ?
		WHERE id = ?
	`, embedding.Text, embedding.Source, embedding.Type,
		embeddingJSON, len(embedding.Vector), embedding.UpdatedAt.Unix(),
		embedding.Metadata.FilePath, embedding.Metadata.LineNumber,
		embedding.Metadata.SessionKey, tagsJSON, embedding.Metadata.Importance,
		embedding.Metadata.AccessCount, embedding.Metadata.LastAccessed.Unix(),
		embedding.ID)

	if err != nil {
		return fmt.Errorf("failed to update memory: %w", err)
	}

	return nil
}

// List lists all memories with optional filtering
func (s *SQLiteStore) List(filter func(*VectorEmbedding) bool) ([]*VectorEmbedding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT
			id, text, source, type, embedding, created_at, updated_at,
			file_path, line_number, session_key, tags, importance,
			access_count, last_accessed
		FROM memories
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list memories: %w", err)
	}
	defer rows.Close()

	var results []*VectorEmbedding
	for rows.Next() {
		var ve VectorEmbedding
		var embeddingJSON sql.NullString
		var tagsJSON sql.NullString
		var lastAccessed sql.NullInt64
		var filePath sql.NullString
		var sessionKey sql.NullString
		var lineNumber sql.NullInt64
		var importance sql.NullFloat64
		var accessCount sql.NullInt64

		err := rows.Scan(
			&ve.ID,
			&ve.Text,
			&ve.Source,
			&ve.Type,
			&embeddingJSON,
			&ve.CreatedAt,
			&ve.UpdatedAt,
			&filePath,
			&lineNumber,
			&sessionKey,
			&tagsJSON,
			&importance,
			&accessCount,
			&lastAccessed,
		)
		if err != nil {
			continue
		}

		if embeddingJSON.Valid {
			_ = json.Unmarshal([]byte(embeddingJSON.String), &ve.Vector)
			ve.Dimension = len(ve.Vector)
		}

		// Populate metadata fields
		ve.Metadata.FilePath = filePath.String
		ve.Metadata.LineNumber = int(lineNumber.Int64)
		ve.Metadata.SessionKey = sessionKey.String
		ve.Metadata.Importance = importance.Float64
		ve.Metadata.AccessCount = int(accessCount.Int64)

		if tagsJSON.Valid {
			_ = json.Unmarshal([]byte(tagsJSON.String), &ve.Metadata.Tags)
		}

		if lastAccessed.Valid {
			ve.Metadata.LastAccessed = time.Unix(lastAccessed.Int64, 0)
		}

		if filter == nil || filter(&ve) {
			results = append(results, &ve)
		}
	}

	return results, nil
}

// Close closes the store
func (s *SQLiteStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// updateAccessCount updates the access count for a memory
func (s *SQLiteStore) updateAccessCount(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	_, _ = s.db.Exec(`
		UPDATE memories SET
			access_count = access_count + 1,
			last_accessed = ?
		WHERE id = ?
	`, now, id)
}

// isVectorEnabled checks if vector search is enabled
func (s *SQLiteStore) isVectorEnabled() bool {
	var value string
	err := s.db.QueryRow("SELECT value FROM meta WHERE key = ?", "vector_enabled").Scan(&value)
	return err == nil && value == "true"
}

// isFTSEnabled checks if FTS is enabled
func (s *SQLiteStore) isFTSEnabled() bool {
	var value string
	err := s.db.QueryRow("SELECT value FROM meta WHERE key = ?", "fts_enabled").Scan(&value)
	return err == nil && value == "true"
}

// Helper functions for SQL placeholders
func sourcePlaceholders(sources []MemorySource) string {
	if len(sources) == 0 {
		return "'longterm', 'session', 'daily'"
	}
	placeholders := make([]string, len(sources))
	for i := range placeholders {
		placeholders[i] = "?"
	}
	return "(" + joinString(placeholders, ",") + ")"
}

func typePlaceholders(types []MemoryType) string {
	if len(types) == 0 {
		return "'fact', 'preference', 'context', 'conversation'"
	}
	placeholders := make([]string, len(types))
	for i := range placeholders {
		placeholders[i] = "?"
	}
	return "(" + joinString(placeholders, ",") + ")"
}

func appendSources(args []interface{}, sources []MemorySource) []interface{} {
	if len(sources) == 0 {
		return args
	}
	for _, s := range sources {
		args = append(args, string(s))
	}
	return args
}

func appendTypes(args []interface{}, types []MemoryType) []interface{} {
	if len(types) == 0 {
		return args
	}
	for _, t := range types {
		args = append(args, string(t))
	}
	return args
}

func float32SliceToString(vec []float32) string {
	str := make([]byte, 0, len(vec)*8)
	for i, v := range vec {
		if i > 0 {
			str = append(str, ',')
		}
		str = append(str, fmt.Sprintf("%f", v)...)
	}
	return string(str)
}

func joinString(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	if len(strs) == 1 {
		return strs[0]
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
