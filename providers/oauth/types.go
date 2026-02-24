package oauth

import "time"

// OAuthCredentials represents OAuth token information
type OAuthCredentials struct {
	// Access is the access token
	AccessToken string `json:"access_token"`

	// Refresh is the refresh token
	RefreshToken string `json:"refresh_token"`

	// Expires is when the access token expires
	Expires time.Time `json:"expires_at"`

	// Provider is the OAuth provider ID
	Provider string `json:"provider"`

	// ProjectID is for Google Gemini (optional)
	ProjectID string `json:"project_id,omitempty"`

	// Email is the user's email (optional)
	Email string `json:"email,omitempty"`
}

// OAuthProvider represents an OAuth provider configuration
type OAuthProvider struct {
	// ID is the unique provider identifier
	ID string

	// Name is the human-readable name
	Name string

	// AuthURL is the URL to start OAuth flow
	AuthURL string

	// TokenURL is the URL to exchange tokens
	TokenURL string

	// Scopes is the OAuth scopes required
	Scopes []string

	// RefreshFunc is the function to refresh tokens
	RefreshFunc func(creds *OAuthCredentials) (*OAuthCredentials, error)
}

// OAuthConfig represents OAuth configuration
type OAuthConfig struct {
	// ClientID is the OAuth client ID
	ClientID string

	// ClientSecret is the OAuth client secret
	ClientSecret string

	// RedirectURL is the OAuth redirect URL
	RedirectURL string

	// Providers is the list of configured providers
	Providers map[string]*OAuthProvider
}

// AuthProfile represents a stored authentication profile
type AuthProfile struct {
	// Type is the auth type: "oauth" or "api_key"
	Type string `json:"type"`

	// Provider is the provider ID
	Provider string `json:"provider"`

	// Access is the access token or API key
	Access string `json:"access"`

	// Expires is when the credential expires
	Expires time.Time `json:"expires_at"`

	// Email is the user's email
	Email string `json:"email,omitempty"`

	// ProjectID is for Google Gemini
	ProjectID string `json:"project_id,omitempty"`

	// RefreshToken is for OAuth refresh
	RefreshToken string `json:"refresh_token,omitempty"`
}

// TokenResponse represents an OAuth token response
type TokenResponse struct {
	// AccessToken is the access token
	AccessToken string `json:"access_token"`

	// RefreshToken is the refresh token
	RefreshToken string `json:"refresh_token"`

	// ExpiresIn is the token lifetime in seconds
	ExpiresIn int `json:"expires_in"`

	// Scope is the granted scope
	Scope string `json:"scope"`

	// TokenType is usually "Bearer"
	TokenType string `json:"token_type"`
}

// OAuthError represents an OAuth error
type OAuthError struct {
	// Code is the error code
	Code string

	// Description is the error description
	Description string

	// URI is a link to more information
	URI string
}

// Error implements the error interface
func (e *OAuthError) Error() string {
	if e.Description != "" {
		return e.Description
	}
	return "oauth error: " + e.Code
}

// Common OAuth errors
var (
	ErrInvalidGrant  = &OAuthError{Code: "invalid_grant", Description: "Invalid grant"}
	ErrInvalidClient = &OAuthError{Code: "invalid_client", Description: "Invalid client"}
	ErrUnauthorized  = &OAuthError{Code: "unauthorized", Description: "Unauthorized"}
	ErrExpiredToken  = &OAuthError{Code: "expired_token", Description: "Token expired"}
)

// IsOAuthProvider checks if a provider is an OAuth provider
func IsOAuthProvider(provider string) bool {
	oauthProviders := map[string]bool{
		"anthropic":     true,
		"openai":        true,
		"google-gemini": true,
		"chutes":        true,
	}
	return oauthProviders[provider]
}

// BuildAPIKey builds an API key string from OAuth credentials
func BuildAPIKey(creds *OAuthCredentials) string {
	if creds.ProjectID != "" {
		// For Google Gemini, include project ID
		// In real implementation, marshal to JSON
		// For now, just return the access token
		return creds.AccessToken
	}
	return creds.AccessToken
}

// IsExpired checks if credentials are expired or will expire soon
func (c *OAuthCredentials) IsExpired() bool {
	// Consider expired if less than 5 minutes remaining
	return time.Until(c.Expires) < 5*time.Minute
}
