package providers

import (
	"context"
	"time"

	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

// RetryConfig configures retry behavior for provider calls
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts
	MaxRetries int `mapstructure:"max_retries" json:"max_retries"`
	// InitialDelay is the initial backoff delay
	InitialDelay time.Duration `mapstructure:"initial_delay" json:"initial_delay"`
	// MaxDelay is the maximum backoff delay
	MaxDelay time.Duration `mapstructure:"max_delay" json:"max_delay"`
	// BackoffFactor is the multiplier for each retry
	BackoffFactor float64 `mapstructure:"backoff_factor" json:"backoff_factor"`
}

// DefaultRetryConfig returns sensible retry defaults
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:    3,
		InitialDelay:  1 * time.Second,
		MaxDelay:      30 * time.Second,
		BackoffFactor: 2.0,
	}
}

// RetryableProvider wraps a Provider with retry logic
type RetryableProvider struct {
	provider Provider
	config   RetryConfig
}

// NewRetryableProvider creates a provider with retry logic
func NewRetryableProvider(provider Provider, config RetryConfig) *RetryableProvider {
	return &RetryableProvider{
		provider: provider,
		config:   config,
	}
}

// Chat implements Provider interface with retry logic
func (p *RetryableProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
	var lastErr error

	for attempt := 0; attempt <= p.config.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := p.calculateDelay(attempt)
			logger.Info("Retrying provider call",
				zap.Int("attempt", attempt),
				zap.Duration("delay", delay))

			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		resp, err := p.provider.Chat(ctx, messages, tools, options...)
		if err == nil {
			if attempt > 0 {
				logger.Info("Provider call succeeded after retry",
					zap.Int("attempts", attempt+1))
			}
			return resp, nil
		}

		lastErr = err

		// Check if error is retryable
		if !p.isRetryable(err) {
			logger.Warn("Non-retryable error from provider",
				zap.Error(err))
			return nil, err
		}

		logger.Warn("Provider call failed, will retry",
			zap.Int("attempt", attempt),
			zap.Int("max_retries", p.config.MaxRetries),
			zap.Error(err))
	}

	return nil, lastErr
}

// ChatWithTools implements Provider interface with retry logic
func (p *RetryableProvider) ChatWithTools(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
	var lastErr error

	for attempt := 0; attempt <= p.config.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := p.calculateDelay(attempt)

			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		resp, err := p.provider.ChatWithTools(ctx, messages, tools, options...)
		if err == nil {
			if attempt > 0 {
				logger.Info("Provider call with tools succeeded after retry",
					zap.Int("attempts", attempt+1))
			}
			return resp, nil
		}

		lastErr = err

		if !p.isRetryable(err) {
			return nil, err
		}
	}

	return nil, lastErr
}

// Close implements Provider interface
func (p *RetryableProvider) Close() error {
	return p.provider.Close()
}

// calculateDelay calculates the backoff delay for a given attempt
func (p *RetryableProvider) calculateDelay(attempt int) time.Duration {
	delay := float64(p.config.InitialDelay)
	for i := 1; i < attempt; i++ {
		delay *= p.config.BackoffFactor
		if delay > float64(p.config.MaxDelay) {
			delay = float64(p.config.MaxDelay)
			break
		}
	}
	return time.Duration(delay)
}

// isRetryable determines if an error is retryable
func (p *RetryableProvider) isRetryable(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Rate limit errors
	if containsAny(errStr, []string{"rate limit", "429", "too many requests"}) {
		return true
	}

	// Timeout errors
	if containsAny(errStr, []string{"timeout", "deadline exceeded", "context deadline"}) {
		return true
	}

	// Network errors
	if containsAny(errStr, []string{"connection refused", "connection reset", "network", "temporary"}) {
		return true
	}

	// Server errors (5xx)
	if containsAny(errStr, []string{"500", "502", "503", "504", "internal error", "service unavailable"}) {
		return true
	}

	// Auth errors are not retryable
	if containsAny(errStr, []string{"401", "403", "unauthorized", "forbidden", "invalid api key", "authentication"}) {
		return false
	}

	// Default to retryable for unknown errors
	return true
}

// containsAny checks if s contains any of the substrings
func containsAny(s string, substrings []string) bool {
	for _, substr := range substrings {
		if len(s) >= len(substr) {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}
