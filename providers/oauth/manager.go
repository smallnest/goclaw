package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Manager manages OAuth credentials and providers
type Manager struct {
	// mu protects concurrent access to credentials
	mu sync.RWMutex

	// credentials stores OAuth credentials by profile ID
	credentials map[string]*OAuthCredentials

	// providers stores registered OAuth providers
	providers map[string]*OAuthProvider

	// storagePath is the path to the credentials storage file
	storagePath string
}

// NewManager creates a new OAuth manager
func NewManager(storagePath string) (*Manager, error) {
	m := &Manager{
		credentials: make(map[string]*OAuthCredentials),
		providers:   make(map[string]*OAuthProvider),
		storagePath: storagePath,
	}

	// Register default providers
	m.registerDefaultProviders()

	// Load existing credentials
	if err := m.loadCredentials(); err != nil {
		return nil, fmt.Errorf("failed to load credentials: %w", err)
	}

	return m, nil
}

// registerDefaultProviders registers the default OAuth providers
func (m *Manager) registerDefaultProviders() {
	// Register Anthropic
	m.RegisterProvider(AnthropicProvider())
	// Register OpenAI
	m.RegisterProvider(OpenAIProvider())
}

// RegisterProvider registers an OAuth provider
func (m *Manager) RegisterProvider(provider *OAuthProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.providers[provider.ID] = provider
}

// GetCredentials returns credentials for a profile ID
func (m *Manager) GetCredentials(profileID string) (*OAuthCredentials, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	creds, exists := m.credentials[profileID]
	if !exists {
		return nil, fmt.Errorf("credentials not found for profile: %s", profileID)
	}

	// Check if token is expired and needs refresh
	if creds.IsExpired() && creds.RefreshToken != "" {
		provider, providerExists := m.providers[creds.Provider]
		if !providerExists {
			return nil, fmt.Errorf("provider not registered: %s", creds.Provider)
		}

		// Refresh the token
		newCreds, err := provider.RefreshFunc(creds)
		if err != nil {
			return nil, fmt.Errorf("failed to refresh token: %w", err)
		}

		// Update stored credentials
		m.credentials[profileID] = newCreds
		if saveErr := m.saveCredentials(); saveErr != nil {
			// Log error but return valid credentials
			fmt.Fprintf(os.Stderr, "Warning: failed to save refreshed credentials: %v\n", saveErr)
		}

		return newCreds, nil
	}

	return creds, nil
}

// StoreCredentials stores OAuth credentials
func (m *Manager) StoreCredentials(profileID string, creds *OAuthCredentials) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.credentials[profileID] = creds
	return m.saveCredentials()
}

// DeleteCredentials removes stored credentials
func (m *Manager) DeleteCredentials(profileID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.credentials, profileID)
	return m.saveCredentials()
}

// ListProfiles returns all stored profile IDs
func (m *Manager) ListProfiles() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	profiles := make([]string, 0, len(m.credentials))
	for profileID := range m.credentials {
		profiles = append(profiles, profileID)
	}
	return profiles
}

// GetProvider returns a registered OAuth provider
func (m *Manager) GetProvider(providerID string) (*OAuthProvider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	provider, exists := m.providers[providerID]
	if !exists {
		return nil, fmt.Errorf("provider not registered: %s", providerID)
	}
	return provider, nil
}

// GetAPIKey returns an API key for a profile, refreshing if necessary
func (m *Manager) GetAPIKey(ctx context.Context, profileID string) (string, error) {
	creds, err := m.GetCredentials(profileID)
	if err != nil {
		return "", err
	}

	// Validate the token if possible
	provider, err := m.GetProvider(creds.Provider)
	if err == nil {
		// Try to validate the token
		var validator TokenValidator
		switch creds.Provider {
		case "anthropic":
			anthropicOAuth := NewAnthropicOAuth()
			validator = anthropicOAuth
		case "openai":
			openaiOAuth := NewOpenAIOAuth()
			validator = openaiOAuth
		}

		if validator != nil {
			if valid, err := validator.ValidateToken(ctx, creds.AccessToken); err == nil && !valid {
				// Token is invalid, try to refresh
				if creds.RefreshToken != "" {
					newCreds, refreshErr := provider.RefreshFunc(creds)
					if refreshErr != nil {
						return "", fmt.Errorf("token validation failed and refresh failed: %w", refreshErr)
					}
					m.credentials[profileID] = newCreds
					if saveErr := m.saveCredentials(); saveErr != nil {
						fmt.Fprintf(os.Stderr, "Warning: failed to save refreshed credentials: %v\n", saveErr)
					}
					creds = newCreds
				}
			}
		}
	}

	return BuildAPIKey(creds), nil
}

// loadCredentials loads credentials from storage
func (m *Manager) loadCredentials() error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(m.storagePath), 0700); err != nil {
		return fmt.Errorf("failed to create credentials directory: %w", err)
	}

	// Read credentials file
	data, err := os.ReadFile(m.storagePath)
	if err != nil {
		if os.IsNotExist(err) {
			// No credentials file yet, that's OK
			return nil
		}
		return fmt.Errorf("failed to read credentials file: %w", err)
	}

	// Parse credentials
	var stored struct {
		Credentials map[string]*OAuthCredentials `json:"credentials"`
	}

	if err := json.Unmarshal(data, &stored); err != nil {
		return fmt.Errorf("failed to parse credentials: %w", err)
	}

	m.credentials = stored.Credentials
	return nil
}

// saveCredentials saves credentials to storage
func (m *Manager) saveCredentials() error {
	stored := struct {
		Credentials map[string]*OAuthCredentials `json:"credentials"`
	}{
		Credentials: m.credentials,
	}

	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	// Write to temporary file first
	tmpPath := m.storagePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write credentials: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, m.storagePath); err != nil {
		os.Remove(tmpPath) // Clean up temp file
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	return nil
}

// TokenValidator is an interface for validating OAuth tokens
type TokenValidator interface {
	ValidateToken(ctx context.Context, accessToken string) (bool, error)
}

// StartAutoRefresh starts automatic token refresh in the background
func (m *Manager) StartAutoRefresh(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				m.refreshExpiredCredentials(ctx)
			}
		}
	}()
}

// refreshExpiredCredentials refreshes any expired tokens
func (m *Manager) refreshExpiredCredentials(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for profileID, creds := range m.credentials {
		if creds.IsExpired() && creds.RefreshToken != "" {
			provider, exists := m.providers[creds.Provider]
			if !exists {
				continue
			}

			newCreds, err := provider.RefreshFunc(creds)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to refresh credentials for %s: %v\n", profileID, err)
				continue
			}

			m.credentials[profileID] = newCreds
		}
	}

	if err := m.saveCredentials(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save refreshed credentials: %v\n", err)
	}
}
