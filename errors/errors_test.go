package errors

import (
	"errors"
	"testing"
)

func TestNewError(t *testing.T) {
	err := New(ErrCodeInvalidInput, "test error")
	if err.Code != ErrCodeInvalidInput {
		t.Errorf("expected code %s, got %s", ErrCodeInvalidInput, err.Code)
	}
	if err.Message != "test error" {
		t.Errorf("expected message 'test error', got '%s'", err.Message)
	}
}

func TestWrapError(t *testing.T) {
	original := errors.New("original error")
	wrapped := Wrap(original, ErrCodeToolExecution, "failed to execute")

	if wrapped.Code != ErrCodeToolExecution {
		t.Errorf("expected code %s, got %s", ErrCodeToolExecution, wrapped.Code)
	}
	if wrapped.Message != "failed to execute" {
		t.Errorf("expected message 'failed to execute', got '%s'", wrapped.Message)
	}
	if !errors.Is(wrapped, original) {
		t.Error("wrapped error should contain original error")
	}
}

func TestErrorWithContext(t *testing.T) {
	err := New(ErrCodeNotFound, "resource not found")
	err = err.WithContext("resource_type", "tool").
		WithContext("resource_name", "bash")

	if err.Context["resource_type"] != "tool" {
		t.Error("context not set correctly")
	}
	if err.Context["resource_name"] != "bash" {
		t.Error("context not set correctly")
	}
}

func TestGetCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorCode
	}{
		{"app error", New(ErrCodeInvalidInput, "test"), ErrCodeInvalidInput},
		{"standard error", errors.New("standard"), ErrCodeUnknown},
		{"nil error", nil, ErrCodeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				return
			}
			if code := GetCode(tt.err); code != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, code)
			}
		})
	}
}

func TestIs(t *testing.T) {
	err := New(ErrCodeTimeout, "operation timed out")

	if !Is(err, ErrCodeTimeout) {
		t.Error("Is should return true for matching error code")
	}
	if Is(err, ErrCodeInvalidInput) {
		t.Error("Is should return false for non-matching error code")
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{"timeout error", New(ErrCodeTimeout, "timeout"), true},
		{"rate limit", New(ErrCodeRateLimit, "rate limit"), true},
		{"invalid input", New(ErrCodeInvalidInput, "bad input"), false},
		{"permission", New(ErrCodePermission, "denied"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if retryable := IsRetryable(tt.err); retryable != tt.retryable {
				t.Errorf("expected retryable=%v, got %v", tt.retryable, retryable)
			}
		})
	}
}

func TestIsFatal(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		fatal bool
	}{
		{"config error", New(ErrCodeInvalidConfig, "bad config"), true},
		{"timeout", New(ErrCodeTimeout, "timeout"), false},
		{"permission", New(ErrCodePermission, "denied"), true},
		{"not found", New(ErrCodeNotFound, "missing"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if fatal := IsFatal(tt.err); fatal != tt.fatal {
				t.Errorf("expected fatal=%v, got %v", tt.fatal, fatal)
			}
		})
	}
}

func TestGetUserMessage(t *testing.T) {
	tests := []struct {
		name             string
		err              error
		expectedContains string
	}{
		{"invalid input", New(ErrCodeInvalidInput, "bad input"), "invalid"},
		{"timeout", New(ErrCodeTimeout, "timed out"), "timeout"},
		{"unknown error", New(ErrCodeUnknown, "unknown"), "unexpected"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := GetUserMessage(tt.err)
			if msg == "" {
				t.Error("user message should not be empty")
			}
		})
	}
}

func TestConvenienceFunctions(t *testing.T) {
	t.Run("ToolNotFound", func(t *testing.T) {
		err := ToolNotFound("bash")
		if err.Code != ErrCodeToolNotFound {
			t.Errorf("expected code %s, got %s", ErrCodeToolNotFound, err.Code)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		err := NotFound("session")
		if err.Code != ErrCodeNotFound {
			t.Errorf("expected code %s, got %s", ErrCodeNotFound, err.Code)
		}
	})

	t.Run("Timeout", func(t *testing.T) {
		err := Timeout("connect to provider")
		if err.Code != ErrCodeTimeout {
			t.Errorf("expected code %s, got %s", ErrCodeTimeout, err.Code)
		}
	})
}
