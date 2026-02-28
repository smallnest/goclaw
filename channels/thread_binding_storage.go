package channels

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ThreadBindingStorage defines the interface for persistent thread binding storage.
type ThreadBindingStorage interface {
	// Load loads all bindings from storage.
	Load() ([]*ThreadBindingRecord, error)
	// Save saves a binding to storage.
	Save(record *ThreadBindingRecord) error
	// Delete removes a binding from storage.
	Delete(bindingID string) error
	// List returns all bindings.
	List() ([]*ThreadBindingRecord, error)
}

// JSONFileStorage implements ThreadBindingStorage using JSON files.
type JSONFileStorage struct {
	filePath string
	mu       sync.Mutex
}

// NewJSONFileStorage creates a new JSON file-based storage.
func NewJSONFileStorage(dataDir string) (*JSONFileStorage, error) {
	if dataDir == "" {
		dataDir = "./data"
	}

	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	filePath := filepath.Join(dataDir, "thread_bindings.json")

	storage := &JSONFileStorage{
		filePath: filePath,
	}

	// Create file if it doesn't exist
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		if err := os.WriteFile(filePath, []byte("[]"), 0644); err != nil {
			return nil, fmt.Errorf("failed to create storage file: %w", err)
		}
	}

	return storage, nil
}

// Load loads all bindings from the JSON file.
func (s *JSONFileStorage) Load() ([]*ThreadBindingRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read storage file: %w", err)
	}

	var bindings []*ThreadBindingRecord
	if err := json.Unmarshal(data, &bindings); err != nil {
		return nil, fmt.Errorf("failed to parse storage file: %w", err)
	}

	return bindings, nil
}

// Save saves a single binding to the JSON file.
// This loads all bindings, adds the new one, and saves back.
func (s *JSONFileStorage) Save(record *ThreadBindingRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Load existing bindings
	bindings, err := s.loadWithoutLock()
	if err != nil {
		return err
	}

	// Check if binding already exists
	found := false
	for i, b := range bindings {
		if b.ID == record.ID {
			bindings[i] = record
			found = true
			break
		}
	}

	if !found {
		bindings = append(bindings, record)
	}

	// Save all bindings
	return s.saveWithoutLock(bindings)
}

// Delete removes a binding from storage.
func (s *JSONFileStorage) Delete(bindingID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	bindings, err := s.loadWithoutLock()
	if err != nil {
		return err
	}

	// Find and remove the binding
	updatedCap := len(bindings) - 1
	if updatedCap < 0 {
		updatedCap = 0
	}
	updated := make([]*ThreadBindingRecord, 0, updatedCap)
	for _, b := range bindings {
		if b.ID != bindingID {
			updated = append(updated, b)
		}
	}

	if len(updated) == len(bindings) {
		return fmt.Errorf("binding not found: %s", bindingID)
	}

	return s.saveWithoutLock(updated)
}

// List returns all bindings from storage.
func (s *JSONFileStorage) List() ([]*ThreadBindingRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadWithoutLock()
}

// loadWithoutLock loads bindings without locking (caller must hold lock).
func (s *JSONFileStorage) loadWithoutLock() ([]*ThreadBindingRecord, error) {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read storage file: %w", err)
	}

	var bindings []*ThreadBindingRecord
	if err := json.Unmarshal(data, &bindings); err != nil {
		return nil, fmt.Errorf("failed to parse storage file: %w", err)
	}

	return bindings, nil
}

// saveWithoutLock saves bindings without locking (caller must hold lock).
func (s *JSONFileStorage) saveWithoutLock(bindings []*ThreadBindingRecord) error {
	data, err := json.MarshalIndent(bindings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal bindings: %w", err)
	}

	if err := os.WriteFile(s.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write storage file: %w", err)
	}

	return nil
}

// CleanupExpired removes expired bindings from storage.
func (s *JSONFileStorage) CleanupExpired() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	bindings, err := s.loadWithoutLock()
	if err != nil {
		return err
	}

	now := time.Now()
	updated := make([]*ThreadBindingRecord, 0, len(bindings))
	for _, b := range bindings {
		// Keep bindings that haven't expired or have no expiration
		if b.ExpiresAt == nil || b.ExpiresAt.After(now) {
			updated = append(updated, b)
		}
	}

	return s.saveWithoutLock(updated)
}
