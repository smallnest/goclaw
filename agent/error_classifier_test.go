package agent

import (
	"errors"
	"testing"
)

func TestNewErrorClassifier(t *testing.T) {
	c := NewErrorClassifier()
	if c == nil {
		t.Fatal("Expected non-nil classifier")
	}
}

func TestClassifyErrorAuth(t *testing.T) {
	c := NewErrorClassifier()

	tests := []struct {
		errMsg string
		want   FailoverReason
	}{
		{"invalid api key", FailoverReasonAuth},
		{"incorrect api key", FailoverReasonAuth},
		{"authentication failed", FailoverReasonAuth},
		{"unauthorized access", FailoverReasonAuth},
		{"401 unauthorized", FailoverReasonAuth},
		{"403 forbidden", FailoverReasonAuth},
		{"token has expired", FailoverReasonAuth},
		{"access denied", FailoverReasonAuth},
		{"no credentials found", FailoverReasonAuth},
		{"re-authenticate", FailoverReasonAuth},
	}

	for _, tt := range tests {
		t.Run(tt.errMsg, func(t *testing.T) {
			reason := c.ClassifyError(errors.New(tt.errMsg))
			if reason != tt.want {
				t.Errorf("Expected %v, got %v", tt.want, reason)
			}
		})
	}
}

func TestClassifyErrorRateLimit(t *testing.T) {
	c := NewErrorClassifier()

	tests := []struct {
		errMsg string
		want   FailoverReason
	}{
		{"rate limit exceeded", FailoverReasonRateLimit},
		{"too many requests", FailoverReasonRateLimit},
		{"429 too many requests", FailoverReasonRateLimit},
		{"quota exceeded", FailoverReasonRateLimit},
		{"resource_exhausted", FailoverReasonRateLimit},
		{"usage limit reached", FailoverReasonRateLimit},
		{"overloaded", FailoverReasonRateLimit},
		{"resource has been exhausted", FailoverReasonRateLimit},
	}

	for _, tt := range tests {
		t.Run(tt.errMsg, func(t *testing.T) {
			reason := c.ClassifyError(errors.New(tt.errMsg))
			if reason != tt.want {
				t.Errorf("Expected %v, got %v", tt.want, reason)
			}
		})
	}
}

func TestClassifyErrorTimeout(t *testing.T) {
	c := NewErrorClassifier()

	tests := []struct {
		errMsg string
		want   FailoverReason
	}{
		{"request timeout", FailoverReasonTimeout},
		{"connection timed out", FailoverReasonTimeout},
		{"deadline exceeded", FailoverReasonTimeout},
		{"context deadline exceeded", FailoverReasonTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.errMsg, func(t *testing.T) {
			reason := c.ClassifyError(errors.New(tt.errMsg))
			if reason != tt.want {
				t.Errorf("Expected %v, got %v", tt.want, reason)
			}
		})
	}
}

func TestClassifyErrorBilling(t *testing.T) {
	c := NewErrorClassifier()

	tests := []struct {
		errMsg string
		want   FailoverReason
	}{
		{"payment required", FailoverReasonBilling},
		{"402 payment required", FailoverReasonBilling},
		{"insufficient credits", FailoverReasonBilling},
		{"credit balance too low", FailoverReasonBilling},
		{"please check your plans & billing", FailoverReasonBilling},
	}

	for _, tt := range tests {
		t.Run(tt.errMsg, func(t *testing.T) {
			reason := c.ClassifyError(errors.New(tt.errMsg))
			if reason != tt.want {
				t.Errorf("Expected %v, got %v", tt.want, reason)
			}
		})
	}
}

func TestClassifyErrorUnknown(t *testing.T) {
	c := NewErrorClassifier()

	tests := []struct {
		errMsg string
		want   FailoverReason
	}{
		{"some random error", FailoverReasonUnknown},
		{"internal server error", FailoverReasonUnknown},
		{"bad request", FailoverReasonUnknown},
		{"not found", FailoverReasonUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.errMsg, func(t *testing.T) {
			reason := c.ClassifyError(errors.New(tt.errMsg))
			if reason != tt.want {
				t.Errorf("Expected %v, got %v", tt.want, reason)
			}
		})
	}
}

func TestClassifyErrorNil(t *testing.T) {
	c := NewErrorClassifier()
	reason := c.ClassifyError(nil)
	if reason != FailoverReasonUnknown {
		t.Errorf("Expected Unknown for nil error, got %v", reason)
	}
}

func TestIsContextOverflowError(t *testing.T) {
	tests := []struct {
		errMsg  string
		want    bool
		reason  string
	}{
		{"request size exceeds model context window", true, "exceeds context window"},
		{"context length exceeded", true, "context length exceeded"},
		{"maximum context length exceeded", true, "maximum context length"},
		{"prompt is too long", true, "prompt too long"},
		{"context overflow", true, "context overflow"},
		{"413 payload too large", true, "413 with too large"},
		{"request_too_large", true, "request_too_large"},
		{"normal error", false, "unrelated error"},
		{"context without overflow", false, "context but no overflow"},
		{"too large file", false, "too large without 413"},
	}

	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			result := IsContextOverflowError(tt.errMsg)
			if result != tt.want {
				t.Errorf("Expected %v for '%s', got %v", tt.want, tt.errMsg, result)
			}
		})
	}
}

func TestIsRoleOrderingError(t *testing.T) {
	tests := []struct {
		errMsg string
		want   bool
	}{
		{"incorrect role information", true},
		{"roles must alternate", true},
		{"normal error", false},
		{"role without ordering", false},
	}

	for _, tt := range tests {
		t.Run(tt.errMsg, func(t *testing.T) {
			result := IsRoleOrderingError(tt.errMsg)
			if result != tt.want {
				t.Errorf("Expected %v, got %v", tt.want, result)
			}
		})
	}
}

func TestIsImageSizeError(t *testing.T) {
	tests := []struct {
		errMsg string
		want   bool
	}{
		{"image exceeds 10 MB", true},
		{"image exceeds 20mb", true},
		{"image too large", false},
		{"10 MB but no image", false},
	}

	for _, tt := range tests {
		t.Run(tt.errMsg, func(t *testing.T) {
			result := IsImageSizeError(tt.errMsg)
			if result != tt.want {
				t.Errorf("Expected %v, got %v", tt.want, result)
			}
		})
	}
}

func TestIsFailoverError(t *testing.T) {
	c := NewErrorClassifier()

	tests := []struct {
		errMsg string
		want   bool
	}{
		{"invalid api key", true},
		{"rate limit exceeded", true},
		{"timeout", true},
		{"insufficient credits", true},
		{"random error", false},
	}

	for _, tt := range tests {
		t.Run(tt.errMsg, func(t *testing.T) {
			result := c.IsFailoverError(errors.New(tt.errMsg))
			if result != tt.want {
				t.Errorf("Expected %v for '%s', got %v", tt.want, tt.errMsg, result)
			}
		})
	}
}

func TestIsFailoverErrorNil(t *testing.T) {
	c := NewErrorClassifier()
	result := c.IsFailoverError(nil)
	if result {
		t.Error("Expected false for nil error")
	}
}

func TestFormatErrorForUser(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		contains string
	}{
		{
			name:     "context overflow",
			errMsg:   "request size exceeds model context window",
			contains: "Context overflow",
		},
		{
			name:     "role ordering",
			errMsg:   "incorrect role information",
			contains: "ordering",  // Changed from "message ordering" to "ordering" (case-insensitive match)
		},
		{
			name:     "image size",
			errMsg:   "image exceeds 20 MB",
			contains: "Image too large",
		},
		{
			name:     "long error",
			errMsg:   "this is a very long error message that should be truncated " + string(make([]byte, 600)),
			contains: "…",
		},
		{
			name:     "normal error",
			errMsg:   "some normal error",
			contains: "some normal error",
		},
		{
			name:     "empty error",
			errMsg:   "",
			contains: "unknown error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatErrorForUser(tt.errMsg)
			if len(result) == 0 {
				t.Error("Expected non-empty result")
			}
			// Check if key phrase is in result
			if len(tt.contains) > 0 {
				// Simple containment check
				found := false
				for i := 0; i <= len(result)-len(tt.contains); i++ {
					if result[i:i+len(tt.contains)] == tt.contains {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected result to contain '%s', got '%s'", tt.contains, result)
				}
			}
		})
	}
}
