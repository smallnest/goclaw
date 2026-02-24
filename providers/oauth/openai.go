package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// OpenAIOAuth handles OpenAI OAuth authentication
type OpenAIOAuth struct {
	// Client is the HTTP client for OAuth requests
	Client *http.Client

	// AuthURL is the OpenAI OAuth authorization URL
	AuthURL string

	// TokenURL is the OpenAI token endpoint
	TokenURL string
}

// NewOpenAIOAuth creates a new OpenAI OAuth handler
func NewOpenAIOAuth() *OpenAIOAuth {
	return &OpenAIOAuth{
		Client:   &http.Client{Timeout: 30 * time.Second},
		AuthURL:  "https://platform.openai.com/v1/oauth",
		TokenURL: "https://api.openai.com/v1/oauth/token",
	}
}

// GetAuthURL returns the authorization URL for OpenAI OAuth
// Parameters:
//   - clientID: OAuth client ID
//   - redirectURI: Callback URL after authorization
//   - state: CSRF protection value
func (o *OpenAIOAuth) GetAuthURL(clientID, redirectURI, state string) string {
	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("response_type", "code")
	params.Set("scope", "model.read,model.write")
	params.Set("state", state)

	return fmt.Sprintf("%s/authorize?%s", o.AuthURL, params.Encode())
}

// ExchangeCode exchanges an authorization code for an access token
func (o *OpenAIOAuth) ExchangeCode(ctx context.Context, clientID, clientSecret, code, redirectURI string) (*OAuthCredentials, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, "POST", o.TokenURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := o.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, o.handleError(resp.StatusCode, body)
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	// Calculate expiration
	expires := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return &OAuthCredentials{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		Expires:      expires,
		Provider:     "openai",
	}, nil
}

// RefreshToken refreshes an expired access token
func (o *OpenAIOAuth) RefreshToken(ctx context.Context, creds *OAuthCredentials) (*OAuthCredentials, error) {
	if creds.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available")
	}

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", creds.RefreshToken)

	req, err := http.NewRequestWithContext(ctx, "POST", o.TokenURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := o.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, o.handleError(resp.StatusCode, body)
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	// Calculate expiration
	expires := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return &OAuthCredentials{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		Expires:      expires,
		Provider:     "openai",
	}, nil
}

// ValidateToken checks if an access token is valid
func (o *OpenAIOAuth) ValidateToken(ctx context.Context, accessToken string) (bool, error) {
	// Use a simple API call to validate the token
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.openai.com/v1/models", nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	resp, err := o.Client.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to validate token: %w", err)
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// handleError processes OAuth error responses
func (o *OpenAIOAuth) handleError(statusCode int, body []byte) error {
	var errResp struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}

	if err := json.Unmarshal(body, &errResp); err != nil {
		return fmt.Errorf("oauth failed with status %d", statusCode)
	}

	switch statusCode {
	case http.StatusBadRequest:
		return &OAuthError{
			Code:        errResp.Error,
			Description: errResp.ErrorDescription,
		}
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusForbidden:
		return ErrInvalidGrant
	default:
		return &OAuthError{
			Code:        fmt.Sprintf("http_%d", statusCode),
			Description: errResp.ErrorDescription,
		}
	}
}

// OpenAIProvider returns the OAuth provider configuration for OpenAI
func OpenAIProvider() *OAuthProvider {
	return &OAuthProvider{
		ID:       "openai",
		Name:     "OpenAI",
		AuthURL:  "https://platform.openai.com/v1/oauth",
		TokenURL: "https://api.openai.com/v1/oauth/token",
		Scopes:   []string{"model.read", "model.write"},
		RefreshFunc: func(creds *OAuthCredentials) (*OAuthCredentials, error) {
			oauth := NewOpenAIOAuth()
			return oauth.RefreshToken(context.Background(), creds)
		},
	}
}
