package acp

import (
	"testing"
	"time"

	"github.com/smallnest/goclaw/config"
	"github.com/stretchr/testify/assert"
)

// TestGetGlobalManager tests the global manager singleton.
func TestGetGlobalManager(t *testing.T) {
	// Reset global manager
	globalManager = nil

	cfg := &config.Config{}

	// First call should create a new manager
	mgr1 := GetOrCreateGlobalManager(cfg)
	assert.NotNil(t, mgr1)

	// Second call should return the same instance
	mgr2 := GetOrCreateGlobalManager(cfg)
	assert.Same(t, mgr1, mgr2)

	// GetGlobalManager should return the instance
	mgr3 := GetGlobalManager()
	assert.Same(t, mgr1, mgr3)
}

// TestNormalizeSessionKey tests session key normalization.
func TestNormalizeSessionKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"test-session", "test-session"},
		{"  Test Session  ", "  Test Session  "}, // Note: normalizeSessionKey currently just returns the key as-is
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeSessionKey(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestNormalizeAgentID tests agent ID normalization.
func TestNormalizeAgentID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"main", "main"},
		{"", "main"},
		{"test-agent", "test-agent"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeAgentID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestResolveRuntimeIdleTTL tests runtime idle TTL resolution.
func TestResolveRuntimeIdleTTL(t *testing.T) {
	cfg := &config.Config{}
	cfg.ACP.IdleTimeoutMs = 300000 // 5 minutes

	ttl := resolveRuntimeIdleTTL(cfg)
	assert.Equal(t, 300000*time.Millisecond, ttl)

	// Test with no config (default 5 minutes)
	ttl2 := resolveRuntimeIdleTTL(nil)
	assert.Equal(t, 5*time.Minute, ttl2)
}

// TestSessionResolution tests session resolution result structure.
func TestSessionResolution(t *testing.T) {
	cfg := &config.Config{}
	manager := NewManager(cfg)

	// Test with empty session key
	result := manager.ResolveSession("")
	assert.Equal(t, "none", result.Kind)
	assert.Equal(t, "", result.SessionKey)

	// Test with non-existent session
	result = manager.ResolveSession("test-session")
	assert.Equal(t, "none", result.Kind)
	assert.Equal(t, "test-session", result.SessionKey)
}

// TestGetObservabilitySnapshot tests getting observability snapshot.
func TestGetObservabilitySnapshot(t *testing.T) {
	cfg := &config.Config{}
	manager := NewManager(cfg)

	snapshot := manager.GetObservabilitySnapshot()

	assert.NotNil(t, snapshot)
	assert.NotNil(t, snapshot.RuntimeCache)
	assert.NotNil(t, snapshot.Turns)
	assert.NotNil(t, snapshot.ErrorsByCode)
}

func TestActorQueueCleansIdleQueues(t *testing.T) {
	q := NewActorQueue()

	err := q.Run("session-a", func() error { return nil })
	assert.NoError(t, err)
	assert.Equal(t, 0, q.GetTotalPendingCount())
	assert.Len(t, q.GetTailMapForTesting(), 0)
}
