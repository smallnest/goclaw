package runtime

import (
	"fmt"
	"sync"
)

// AcpRuntimeBackend represents a registered ACP runtime backend.
type AcpRuntimeBackend struct {
	// ID is the unique identifier for this backend
	ID string

	// Runtime is the ACP runtime implementation
	Runtime AcpRuntime

	// Healthy is an optional function to check if the backend is healthy.
	// If nil, the backend is always considered healthy.
	Healthy func() bool
}

// acpRuntimeRegistryState holds the global registry state.
type acpRuntimeRegistryState struct {
	backendsByID map[string]*AcpRuntimeBackend
	mu           sync.RWMutex
}

var (
	globalRegistry = &acpRuntimeRegistryState{
		backendsByID: make(map[string]*AcpRuntimeBackend),
	}
)

// normalizeBackendId normalizes a backend ID for consistent lookup.
func normalizeBackendID(id string) string {
	return trimAndLower(id)
}

// trimAndLower trims whitespace and converts to lowercase.
func trimAndLower(s string) string {
	if s == "" {
		return ""
	}
	// Simple implementation - for Unicode safety in production, use cases
	// or a proper Unicode string library
	result := []rune(s)
	start := 0
	end := len(result)

	// Trim leading whitespace
	for start < end && (result[start] == ' ' || result[start] == '\t' || result[start] == '\n' || result[start] == '\r') {
		start++
	}

	// Trim trailing whitespace
	for end > start && (result[end-1] == ' ' || result[end-1] == '\t' || result[end-1] == '\n' || result[end-1] == '\r') {
		end--
	}

	// Convert to lowercase (ASCII only for simplicity)
	trimmed := result[start:end]
	for i := range trimmed {
		if trimmed[i] >= 'A' && trimmed[i] <= 'Z' {
			trimmed[i] = trimmed[i] + ('a' - 'A')
		}
	}

	return string(trimmed)
}

// isBackendHealthy checks if a backend is healthy.
// Returns true if the backend has no health check or the health check passes.
func isBackendHealthy(backend *AcpRuntimeBackend) bool {
	if backend.Healthy == nil {
		return true
	}
	defer func() {
		// Recover from panics in health check functions
		_ = recover()
	}()
	return backend.Healthy()
}

// RegisterAcpRuntimeBackend registers an ACP runtime backend.
// If a backend with the same ID already exists, it will be replaced.
func RegisterAcpRuntimeBackend(backend AcpRuntimeBackend) error {
	id := normalizeBackendID(backend.ID)
	if id == "" {
		return fmt.Errorf("ACP runtime backend ID is required")
	}
	if backend.Runtime == nil {
		return fmt.Errorf("ACP runtime backend '%s' is missing runtime implementation", id)
	}

	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()

	globalRegistry.backendsByID[id] = &AcpRuntimeBackend{
		ID:      id,
		Runtime: backend.Runtime,
		Healthy: backend.Healthy,
	}

	return nil
}

// UnregisterAcpRuntimeBackend removes an ACP runtime backend from the registry.
func UnregisterAcpRuntimeBackend(id string) {
	normalized := normalizeBackendID(id)
	if normalized == "" {
		return
	}

	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()

	delete(globalRegistry.backendsByID, normalized)
}

// GetAcpRuntimeBackend retrieves an ACP runtime backend by ID.
// If ID is empty, returns the first healthy backend.
// Returns nil if no matching backend is found.
func GetAcpRuntimeBackend(id string) *AcpRuntimeBackend {
	normalized := normalizeBackendID(id)

	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	// If specific ID requested, try to find it
	if normalized != "" {
		if backend, ok := globalRegistry.backendsByID[normalized]; ok {
			if isBackendHealthy(backend) {
				return backend
			}
		}
		return nil
	}

	// No specific ID - return first healthy backend
	for _, backend := range globalRegistry.backendsByID {
		if isBackendHealthy(backend) {
			return backend
		}
	}

	// If no healthy backends, return the first one (if any)
	for _, backend := range globalRegistry.backendsByID {
		return backend
	}

	return nil
}

// RequireAcpRuntimeBackend retrieves an ACP runtime backend or returns an error.
// If ID is empty, returns the first healthy backend.
// Returns an error if no matching backend is found or if the backend is unhealthy.
func RequireAcpRuntimeBackend(id string) (*AcpRuntimeBackend, error) {
	normalized := normalizeBackendID(id)
	backend := GetAcpRuntimeBackend(normalized)

	if backend == nil {
		if normalized != "" {
			return nil, NewBackendMissingError(normalized)
		}
		return nil, NewBackendMissingError("(default)")
	}

	if !isBackendHealthy(backend) {
		return nil, NewBackendUnavailableError(backend.ID)
	}

	// If specific ID was requested, verify it matches
	if normalized != "" && backend.ID != normalized {
		return nil, NewBackendMissingError(normalized)
	}

	return backend, nil
}

// ListAcpRuntimeBackends returns all registered backend IDs.
func ListAcpRuntimeBackends() []string {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	ids := make([]string, 0, len(globalRegistry.backendsByID))
	for id := range globalRegistry.backendsByID {
		ids = append(ids, id)
	}
	return ids
}

// GetAcpRuntimeBackendCount returns the number of registered backends.
func GetAcpRuntimeBackendCount() int {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	return len(globalRegistry.backendsByID)
}

// ResetAcpRuntimeRegistry clears all registered backends.
// This is primarily intended for testing purposes.
func ResetAcpRuntimeRegistry() {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()

	globalRegistry.backendsByID = make(map[string]*AcpRuntimeBackend)
}

// GetAcpRuntimeBackendStats returns statistics about registered backends.
type AcpRuntimeBackendStats struct {
	Total      int // Total number of registered backends
	Healthy    int // Number of healthy backends
	Unhealthy  int // Number of unhealthy backends
	BackendIDs []string
}

// GetAcpRuntimeBackendStats returns statistics about all registered backends.
func GetAcpRuntimeBackendStats() AcpRuntimeBackendStats {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	stats := AcpRuntimeBackendStats{
		Total:      len(globalRegistry.backendsByID),
		BackendIDs: make([]string, 0, len(globalRegistry.backendsByID)),
	}

	for id, backend := range globalRegistry.backendsByID {
		stats.BackendIDs = append(stats.BackendIDs, id)
		if isBackendHealthy(backend) {
			stats.Healthy++
		} else {
			stats.Unhealthy++
		}
	}

	return stats
}
