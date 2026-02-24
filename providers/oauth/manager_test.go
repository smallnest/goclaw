package oauth

import (
	"path/filepath"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	// Create temp directory for testing
	tmpDir := t.TempDir()
	storagePath := filepath.Join(tmpDir, "credentials.json")

	// Create manager
	manager, err := NewManager(storagePath)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	if manager == nil {
		t.Fatal("Manager should not be nil")
	}

	// Check that default providers are registered
	providers := []string{"anthropic", "openai"}
	for _, providerID := range providers {
		provider, err := manager.GetProvider(providerID)
		if err != nil {
			t.Fatalf("Provider %s not registered: %v", providerID, err)
		}
		if provider == nil {
			t.Fatalf("Provider %s is nil", providerID)
		}
	}
}

func TestStoreAndGetCredentials(t *testing.T) {
	tmpDir := t.TempDir()
	storagePath := filepath.Join(tmpDir, "credentials.json")

	manager, err := NewManager(storagePath)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Store credentials
	creds := &OAuthCredentials{
		AccessToken:  "test_access_token",
		RefreshToken: "test_refresh_token",
		Expires:      time.Now().Add(1 * time.Hour),
		Provider:     "anthropic",
	}

	err = manager.StoreCredentials("test-profile", creds)
	if err != nil {
		t.Fatalf("Failed to store credentials: %v", err)
	}

	// Create new manager instance to test persistence
	manager2, err := NewManager(storagePath)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Get credentials
	retrieved, err := manager2.GetCredentials("test-profile")
	if err != nil {
		t.Fatalf("Failed to get credentials: %v", err)
	}

	if retrieved.AccessToken != creds.AccessToken {
		t.Errorf("Access token mismatch: got %s, want %s", retrieved.AccessToken, creds.AccessToken)
	}

	if retrieved.Provider != creds.Provider {
		t.Errorf("Provider mismatch: got %s, want %s", retrieved.Provider, creds.Provider)
	}
}

func TestDeleteCredentials(t *testing.T) {
	tmpDir := t.TempDir()
	storagePath := filepath.Join(tmpDir, "credentials.json")

	manager, err := NewManager(storagePath)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Store credentials
	creds := &OAuthCredentials{
		AccessToken: "test_token",
		Expires:     time.Now().Add(1 * time.Hour),
		Provider:    "openai",
	}

	err = manager.StoreCredentials("delete-test", creds)
	if err != nil {
		t.Fatalf("Failed to store credentials: %v", err)
	}

	// Verify it exists
	_, err = manager.GetCredentials("delete-test")
	if err != nil {
		t.Fatalf("Failed to get credentials before delete: %v", err)
	}

	// Delete credentials
	err = manager.DeleteCredentials("delete-test")
	if err != nil {
		t.Fatalf("Failed to delete credentials: %v", err)
	}

	// Verify it's gone
	_, err = manager.GetCredentials("delete-test")
	if err == nil {
		t.Error("Expected error when getting deleted credentials")
	}
}

func TestListProfiles(t *testing.T) {
	tmpDir := t.TempDir()
	storagePath := filepath.Join(tmpDir, "credentials.json")

	manager, err := NewManager(storagePath)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Store multiple credentials
	profiles := []string{"profile1", "profile2", "profile3"}
	for _, profile := range profiles {
		creds := &OAuthCredentials{
			AccessToken: "token_" + profile,
			Expires:     time.Now().Add(1 * time.Hour),
			Provider:    "anthropic",
		}
		err = manager.StoreCredentials(profile, creds)
		if err != nil {
			t.Fatalf("Failed to store credentials for %s: %v", profile, err)
		}
	}

	// List profiles
	listed := manager.ListProfiles()
	if len(listed) != len(profiles) {
		t.Fatalf("Profile count mismatch: got %d, want %d", len(listed), len(profiles))
	}

	// Verify all profiles are listed
	profileMap := make(map[string]bool)
	for _, profile := range listed {
		profileMap[profile] = true
	}

	for _, expected := range profiles {
		if !profileMap[expected] {
			t.Errorf("Profile %s not in list", expected)
		}
	}
}

func TestIsExpired(t *testing.T) {
	tests := []struct {
		name    string
		expires time.Time
		expired bool
	}{
		{
			name:    "Not expired",
			expires: time.Now().Add(1 * time.Hour),
			expired: false,
		},
		{
			name:    "Expired",
			expires: time.Now().Add(-1 * time.Hour),
			expired: true,
		},
		{
			name:    "Expires soon (< 5 min)",
			expires: time.Now().Add(3 * time.Minute),
			expired: true,
		},
		{
			name:    "Expires soon (> 5 min)",
			expires: time.Now().Add(10 * time.Minute),
			expired: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds := &OAuthCredentials{
				AccessToken: "test_token",
				Expires:     tt.expires,
			}

			result := creds.IsExpired()
			if result != tt.expired {
				t.Errorf("IsExpired() = %v, want %v", result, tt.expired)
			}
		})
	}
}

func TestBuildAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		creds    *OAuthCredentials
		expected string
	}{
		{
			name: "Simple token",
			creds: &OAuthCredentials{
				AccessToken: "sk-test-token",
			},
			expected: "sk-test-token",
		},
		{
			name: "Token with project ID",
			creds: &OAuthCredentials{
				AccessToken: "sk-test-token",
				ProjectID:   "project-123",
			},
			expected: "sk-test-token", // For now, just returns access token
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildAPIKey(tt.creds)
			if result != tt.expected {
				t.Errorf("BuildAPIKey() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestIsOAuthProvider(t *testing.T) {
	tests := []struct {
		provider string
		expected bool
	}{
		{"anthropic", true},
		{"openai", true},
		{"google-gemini", true},
		{"chutes", true},
		{"api-key", false},
		{"unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			result := IsOAuthProvider(tt.provider)
			if result != tt.expected {
				t.Errorf("IsOAuthProvider(%s) = %v, want %v", tt.provider, result, tt.expected)
			}
		})
	}
}

// Benchmark tests
func BenchmarkStoreCredentials(b *testing.B) {
	tmpDir := b.TempDir()
	storagePath := filepath.Join(tmpDir, "credentials.json")

	manager, err := NewManager(storagePath)
	if err != nil {
		b.Fatalf("Failed to create manager: %v", err)
	}

	creds := &OAuthCredentials{
		AccessToken: "test_token",
		Expires:     time.Now().Add(1 * time.Hour),
		Provider:    "anthropic",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		profile := "profile-" + string(rune(i%26))
		manager.StoreCredentials(profile, creds)
	}
}

func BenchmarkGetCredentials(b *testing.B) {
	tmpDir := b.TempDir()
	storagePath := filepath.Join(tmpDir, "credentials.json")

	manager, err := NewManager(storagePath)
	if err != nil {
		b.Fatalf("Failed to create manager: %v", err)
	}

	// Pre-populate with credentials
	for i := 0; i < 100; i++ {
		creds := &OAuthCredentials{
			AccessToken: "test_token",
			Expires:     time.Now().Add(1 * time.Hour),
			Provider:    "anthropic",
		}
		manager.StoreCredentials("profile-"+string(rune(i%26)), creds)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		profile := "profile-" + string(rune(i%100))
		manager.GetCredentials(profile)
	}
}
