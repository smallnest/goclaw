package agent

import (
	"errors"
	"testing"
	"time"

	gerrors "github.com/smallnest/goclaw/errors"
)

// mockErrorClassifier is a mock error classifier for testing
type mockErrorClassifier struct {
	reason gerrors.FailoverReason
}

func (m *mockErrorClassifier) ClassifyError(err error) gerrors.FailoverReason {
	return m.reason
}

func (m *mockErrorClassifier) IsFailoverError(err error) bool {
	return m.reason != gerrors.FailoverReasonUnknown
}

func TestNewRetryManager(t *testing.T) {
	tests := []struct {
		name       string
		config     *RetryConfig
		classifier gerrors.ErrorClassifier
	}{
		{
			name:       "nil config uses defaults",
			config:     nil,
			classifier: nil,
		},
		{
			name: "custom config",
			config: &RetryConfig{
				Enabled:         true,
				MaxRetries:      5,
				InitialDelay:    1 * time.Second,
				MaxDelay:        30 * time.Second,
				BackoffFactor:   1.5,
				RetryableErrors: []string{"auth", "rate_limit"},
			},
			classifier: &mockErrorClassifier{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rm := NewRetryManager(tt.config, tt.classifier)
			if rm == nil {
				t.Error("NewRetryManager returned nil")
			}
		})
	}
}

func TestRetryManager_ShouldRetry(t *testing.T) {
	tests := []struct {
		name       string
		config     *RetryConfig
		classifier gerrors.ErrorClassifier
		err        error
		attempts   int // number of times to call RecordError before ShouldRetry
		want       bool
	}{
		{
			name: "no error should not retry",
			config: &RetryConfig{
				Enabled:         true,
				MaxRetries:      3,
				RetryableErrors: []string{"auth"},
			},
			classifier: &mockErrorClassifier{reason: gerrors.FailoverReasonAuth},
			err:        nil,
			want:       false,
		},
		{
			name: "disabled should not retry",
			config: &RetryConfig{
				Enabled:         false,
				MaxRetries:      3,
				RetryableErrors: []string{"auth"},
			},
			classifier: &mockErrorClassifier{reason: gerrors.FailoverReasonAuth},
			err:        errors.New("test error"),
			want:       false,
		},
		{
			name: "retryable error should retry",
			config: &RetryConfig{
				Enabled:         true,
				MaxRetries:      3,
				RetryableErrors: []string{"auth", "rate_limit"},
			},
			classifier: &mockErrorClassifier{reason: gerrors.FailoverReasonAuth},
			err:        errors.New("auth error"),
			want:       true,
		},
		{
			name: "non-retryable error should not retry",
			config: &RetryConfig{
				Enabled:         true,
				MaxRetries:      3,
				RetryableErrors: []string{"auth"},
			},
			classifier: &mockErrorClassifier{reason: gerrors.FailoverReasonUnknown},
			err:        errors.New("unknown error"),
			want:       false,
		},
		{
			name: "max retries exceeded should not retry",
			config: &RetryConfig{
				Enabled:         true,
				MaxRetries:      2,
				RetryableErrors: []string{"auth"},
			},
			classifier: &mockErrorClassifier{reason: gerrors.FailoverReasonAuth},
			err:        errors.New("auth error"),
			attempts:   2,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rm := NewRetryManager(tt.config, tt.classifier)

			// Simulate previous attempts
			for i := 0; i < tt.attempts; i++ {
				rm.RecordError(tt.err)
			}

			got := rm.ShouldRetry(tt.err)
			if got != tt.want {
				t.Errorf("ShouldRetry() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRetryManager_GetDelay(t *testing.T) {
	config := &RetryConfig{
		Enabled:         true,
		MaxRetries:      5,
		InitialDelay:    1 * time.Second,
		MaxDelay:        10 * time.Second,
		BackoffFactor:   2.0,
		RetryableErrors: []string{"auth"},
	}

	rm := NewRetryManager(config, nil).(*retryManager)

	tests := []struct {
		attempt int
		wantMin time.Duration
		wantMax time.Duration
	}{
		{0, 1 * time.Second, 1 * time.Second},
		{1, 2 * time.Second, 2 * time.Second},
		{2, 4 * time.Second, 4 * time.Second},
		{3, 8 * time.Second, 8 * time.Second},
		{4, 10 * time.Second, 10 * time.Second}, // capped at MaxDelay
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			rm.state.Attempt = tt.attempt
			got := rm.GetDelay()
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("GetDelay() for attempt %d = %v, want between %v and %v", tt.attempt, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestRetryManager_RecordError(t *testing.T) {
	tests := []struct {
		name            string
		config          *RetryConfig
		classifier      gerrors.ErrorClassifier
		err             error
		wantShouldRetry bool
		wantAction      RecoveryAction
	}{
		{
			name: "auth error should rotate profile",
			config: &RetryConfig{
				Enabled:         true,
				MaxRetries:      3,
				InitialDelay:    1 * time.Second,
				RetryableErrors: []string{"auth"},
			},
			classifier:      &mockErrorClassifier{reason: gerrors.FailoverReasonAuth},
			err:             errors.New("invalid api key"),
			wantShouldRetry: true,
			wantAction:      RecoveryActionRotateProfile,
		},
		{
			name: "rate limit should backoff",
			config: &RetryConfig{
				Enabled:         true,
				MaxRetries:      3,
				InitialDelay:    1 * time.Second,
				RetryableErrors: []string{"rate_limit"},
			},
			classifier:      &mockErrorClassifier{reason: gerrors.FailoverReasonRateLimit},
			err:             errors.New("rate limit exceeded"),
			wantShouldRetry: true,
			wantAction:      RecoveryActionBackoff,
		},
		{
			name: "context overflow should compress",
			config: &RetryConfig{
				Enabled:         true,
				MaxRetries:      3,
				InitialDelay:    1 * time.Second,
				RetryableErrors: []string{"context_overflow"},
			},
			classifier:      &mockErrorClassifier{reason: gerrors.FailoverReasonContextOverflow},
			err:             errors.New("context length exceeded"),
			wantShouldRetry: true,
			wantAction:      RecoveryActionCompressContext,
		},
		{
			name: "unknown error should not retry",
			config: &RetryConfig{
				Enabled:         true,
				MaxRetries:      3,
				InitialDelay:    1 * time.Second,
				RetryableErrors: []string{"auth"},
			},
			classifier:      &mockErrorClassifier{reason: gerrors.FailoverReasonUnknown},
			err:             errors.New("unknown error"),
			wantShouldRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rm := NewRetryManager(tt.config, tt.classifier)
			decision := rm.RecordError(tt.err)

			if decision.ShouldRetry != tt.wantShouldRetry {
				t.Errorf("RecordError().ShouldRetry = %v, want %v", decision.ShouldRetry, tt.wantShouldRetry)
			}

			if tt.wantShouldRetry && decision.Action != tt.wantAction {
				t.Errorf("RecordError().Action = %v, want %v", decision.Action, tt.wantAction)
			}
		})
	}
}

func TestRetryManager_RecordSuccess(t *testing.T) {
	config := &RetryConfig{
		Enabled:         true,
		MaxRetries:      3,
		InitialDelay:    1 * time.Second,
		RetryableErrors: []string{"auth"},
	}
	classifier := &mockErrorClassifier{reason: gerrors.FailoverReasonAuth}

	rm := NewRetryManager(config, classifier).(*retryManager)

	// Simulate some attempts
	rm.RecordError(errors.New("auth error"))
	rm.RecordError(errors.New("auth error"))

	if rm.state.Attempt != 2 {
		t.Errorf("Expected attempt 2, got %d", rm.state.Attempt)
	}

	// Record success should reset
	rm.RecordSuccess()

	if rm.state.Attempt != 0 {
		t.Errorf("RecordSuccess() did not reset state, attempt = %d", rm.state.Attempt)
	}
}

func TestRetryManager_Reset(t *testing.T) {
	config := &RetryConfig{
		Enabled:         true,
		MaxRetries:      3,
		InitialDelay:    1 * time.Second,
		RetryableErrors: []string{"auth"},
	}
	classifier := &mockErrorClassifier{reason: gerrors.FailoverReasonAuth}

	rm := NewRetryManager(config, classifier).(*retryManager)

	// Simulate some attempts
	rm.RecordError(errors.New("auth error"))
	rm.RecordError(errors.New("auth error"))

	if rm.state.Attempt != 2 {
		t.Errorf("Expected attempt 2, got %d", rm.state.Attempt)
	}

	// Reset
	rm.Reset()

	if rm.state.Attempt != 0 {
		t.Errorf("Reset() did not reset state, attempt = %d", rm.state.Attempt)
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	if !config.Enabled {
		t.Error("Default config should be enabled")
	}
	if config.MaxRetries != 3 {
		t.Errorf("Default MaxRetries = %d, want 3", config.MaxRetries)
	}
	if config.InitialDelay != 2*time.Second {
		t.Errorf("Default InitialDelay = %v, want 2s", config.InitialDelay)
	}
	if config.MaxDelay != 60*time.Second {
		t.Errorf("Default MaxDelay = %v, want 60s", config.MaxDelay)
	}
	if config.BackoffFactor != 2.0 {
		t.Errorf("Default BackoffFactor = %v, want 2.0", config.BackoffFactor)
	}
}
