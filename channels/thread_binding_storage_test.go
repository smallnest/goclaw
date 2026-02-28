package channels

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestJSONFileStorage tests the JSON file storage implementation.
func TestJSONFileStorage(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "thread-binding-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create storage
	storage, err := NewJSONFileStorage(tempDir)
	require.NoError(t, err)

	// Test Save
	record := &ThreadBindingRecord{
		ID:               "test-binding-1",
		TargetSessionKey: "test-session",
		TargetKind:       "session",
		Conversation: ThreadBindingConversation{
			Channel:        "telegram",
			AccountID:      "test-account",
			ConversationID: "test-conversation",
		},
		Placement: "child",
		Metadata: ThreadBindingMetadata{
			ThreadName: "Test Thread",
			AgentID:    "main",
			Label:      "test-label",
			BoundBy:    "user",
		},
		CreatedAt: time.Now(),
	}

	err = storage.Save(record)
	require.NoError(t, err)

	// Test Load
	bindings, err := storage.Load()
	require.NoError(t, err)
	require.Len(t, bindings, 1)

	assert.Equal(t, record.ID, bindings[0].ID)
	assert.Equal(t, record.TargetSessionKey, bindings[0].TargetSessionKey)
	assert.Equal(t, record.TargetKind, bindings[0].TargetKind)
	assert.Equal(t, record.Conversation.Channel, bindings[0].Conversation.Channel)
	assert.Equal(t, record.Conversation.AccountID, bindings[0].Conversation.AccountID)
	assert.Equal(t, record.Conversation.ConversationID, bindings[0].Conversation.ConversationID)
	assert.Equal(t, record.Placement, bindings[0].Placement)
	assert.Equal(t, record.Metadata.ThreadName, bindings[0].Metadata.ThreadName)
	assert.Equal(t, record.Metadata.AgentID, bindings[0].Metadata.AgentID)
	assert.Equal(t, record.Metadata.Label, bindings[0].Metadata.Label)
	assert.Equal(t, record.Metadata.BoundBy, bindings[0].Metadata.BoundBy)

	// Test Update
	record.Metadata.Label = "updated-label"
	err = storage.Save(record)
	require.NoError(t, err)

	bindings, err = storage.Load()
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	assert.Equal(t, "updated-label", bindings[0].Metadata.Label)

	// Test Delete
	err = storage.Delete(record.ID)
	require.NoError(t, err)

	bindings, err = storage.Load()
	require.NoError(t, err)
	assert.Len(t, bindings, 0)

	// Test Delete non-existent
	err = storage.Delete("non-existent")
	assert.Error(t, err)

	// Test List
	record2 := &ThreadBindingRecord{
		ID:               "test-binding-2",
		TargetSessionKey: "test-session-2",
		TargetKind:       "session",
		Conversation: ThreadBindingConversation{
			Channel:        "discord",
			AccountID:      "test-account",
			ConversationID: "test-conversation-2",
		},
		CreatedAt: time.Now(),
	}
	err = storage.Save(record2)
	require.NoError(t, err)

	list, err := storage.List()
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, record2.ID, list[0].ID)
}

// TestJSONFileStorageCleanupExpired tests cleanup of expired bindings.
func TestJSONFileStorageCleanupExpired(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "thread-binding-expire-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	storage, err := NewJSONFileStorage(tempDir)
	require.NoError(t, err)

	now := time.Now()
	past := now.Add(-1 * time.Hour)
	future := now.Add(1 * time.Hour)

	// Create bindings: one expired, one not expired, one with no expiration
	expiredRecord := &ThreadBindingRecord{
		ID:               "expired",
		TargetSessionKey: "session-1",
		TargetKind:       "session",
		Conversation: ThreadBindingConversation{
			Channel:        "telegram",
			AccountID:      "test",
			ConversationID: "conv-1",
		},
		CreatedAt: past,
		ExpiresAt: &past,
	}

	activeRecord := &ThreadBindingRecord{
		ID:               "active",
		TargetSessionKey: "session-2",
		TargetKind:       "session",
		Conversation: ThreadBindingConversation{
			Channel:        "telegram",
			AccountID:      "test",
			ConversationID: "conv-2",
		},
		CreatedAt: now,
		ExpiresAt: &future,
	}

	noExpiryRecord := &ThreadBindingRecord{
		ID:               "no-expiry",
		TargetSessionKey: "session-3",
		TargetKind:       "session",
		Conversation: ThreadBindingConversation{
			Channel:        "telegram",
			AccountID:      "test",
			ConversationID: "conv-3",
		},
		CreatedAt: now,
		// ExpiresAt is nil
	}

	// Save all bindings
	require.NoError(t, storage.Save(expiredRecord))
	require.NoError(t, storage.Save(activeRecord))
	require.NoError(t, storage.Save(noExpiryRecord))

	// Verify 3 bindings
	bindings, err := storage.Load()
	require.NoError(t, err)
	assert.Len(t, bindings, 3)

	// Cleanup expired
	err = storage.CleanupExpired()
	require.NoError(t, err)

	// Verify only 2 bindings remain
	bindings, err = storage.Load()
	require.NoError(t, err)
	assert.Len(t, bindings, 2)

	// Verify the correct bindings remain
	ids := make(map[string]bool)
	for _, b := range bindings {
		ids[b.ID] = true
	}
	assert.True(t, ids["active"])
	assert.True(t, ids["no-expiry"])
	assert.False(t, ids["expired"])
}

// TestJSONFileStorageFilePath tests that storage uses the correct file path.
func TestJSONFileStorageFilePath(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "thread-binding-path-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	storage, err := NewJSONFileStorage(tempDir)
	require.NoError(t, err)

	expectedPath := filepath.Join(tempDir, "thread_bindings.json")
	assert.Equal(t, expectedPath, storage.filePath)

	// Verify file exists
	_, err = os.Stat(expectedPath)
	assert.NoError(t, err)
}

// TestJSONFileStorageEmptyDir tests storage with empty directory (uses default).
func TestJSONFileStorageEmptyDir(t *testing.T) {
	storage, err := NewJSONFileStorage("")
	require.NoError(t, err)

	// Should create default ./data directory
	assert.Contains(t, storage.filePath, "data")

	// Cleanup
	os.RemoveAll("./data")
}
