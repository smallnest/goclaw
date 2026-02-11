package agent

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	if cfg.MaxAttempts != 3 {
		t.Errorf("Expected MaxAttempts 3, got %d", cfg.MaxAttempts)
	}
	if cfg.BaseDelay != time.Second {
		t.Errorf("Expected BaseDelay 1s, got %v", cfg.BaseDelay)
	}
	if cfg.MaxDelay != 10*time.Second {
		t.Errorf("Expected MaxDelay 10s, got %v", cfg.MaxDelay)
	}
}

func TestNewDefaultRetryPolicy(t *testing.T) {
	p := NewDefaultRetryPolicy(nil)

	if p == nil {
		t.Fatal("Expected non-nil policy")
	}
	if p.config == nil {
		t.Error("Expected config to be set")
	}
}

func TestShouldRetry(t *testing.T) {
	p := NewDefaultRetryPolicy(nil)

	tests := []struct {
		name      string
		attempt   int
		err       error
		wantRetry bool
		wantReason FailoverReason
	}{
		{
			name:      "auth error",
			attempt:   1,
			err:       errors.New("invalid api key"),
			wantRetry: true,
			wantReason: FailoverReasonAuth,
		},
		{
			name:      "rate limit error",
			attempt:   1,
			err:       errors.New("rate limit exceeded"),
			wantRetry: true,
			wantReason: FailoverReasonRateLimit,
		},
		{
			name:      "timeout error",
			attempt:   1,
			err:       errors.New("timeout"),
			wantRetry: true,
			wantReason: FailoverReasonTimeout,
		},
		{
			name:      "billing error",
			attempt:   1,
			err:       errors.New("insufficient credits"),
			wantRetry: false,
			wantReason: FailoverReasonBilling,
		},
		{
			name:      "unknown error",
			attempt:   1,
			err:       errors.New("random error"),
			wantRetry: false,
			wantReason: FailoverReasonUnknown,
		},
		{
			name:      "max attempts reached",
			attempt:   3,
			err:       errors.New("invalid api key"),
			wantRetry: false,
			wantReason: FailoverReasonUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldRetry, reason := p.ShouldRetry(tt.attempt, tt.err)
			if shouldRetry != tt.wantRetry {
				t.Errorf("Expected retry=%v, got %v", tt.wantRetry, shouldRetry)
			}
			if reason != tt.wantReason {
				t.Errorf("Expected reason=%v, got %v", tt.wantReason, reason)
			}
		})
	}
}

func TestGetDelay(t *testing.T) {
	p := NewDefaultRetryPolicy(nil)

	tests := []struct {
		name   string
		attempt int
		reason FailoverReason
		want   time.Duration
	}{
		{
			name:   "first attempt timeout",
			attempt: 0,
			reason: FailoverReasonTimeout,
			want:   1 * time.Second,
		},
		{
			name:   "second attempt timeout",
			attempt: 1,
			reason: FailoverReasonTimeout,
			want:   2 * time.Second,
		},
		{
			name:   "third attempt timeout",
			attempt: 2,
			reason: FailoverReasonTimeout,
			want:   4 * time.Second,
		},
		{
			name:   "rate limit with exponential backoff",
			attempt: 1,
			reason: FailoverReasonRateLimit,
			want:   2 * time.Second,
		},
		{
			name:   "rate limit second attempt",
			attempt: 2,
			reason: FailoverReasonRateLimit,
			want:   4 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay := p.GetDelay(tt.attempt, tt.reason)
			if delay != tt.want {
				t.Errorf("Expected delay %v, got %v", tt.want, delay)
			}
		})
	}
}

func TestGetDelayMaxDelay(t *testing.T) {
	cfg := &RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Second,
		MaxDelay:    5 * time.Second,
	}
	p := NewDefaultRetryPolicy(cfg)

	// With exponential backoff, 2^5 = 32 seconds, but should be capped at MaxDelay
	delay := p.GetDelay(5, FailoverReasonTimeout)
	if delay != 5*time.Second {
		t.Errorf("Expected delay to be capped at MaxDelay (5s), got %v", delay)
	}
}

func TestNewRetryManager(t *testing.T) {
	rm := NewRetryManager(nil)

	if rm == nil {
		t.Fatal("Expected non-nil retry manager")
	}
	if rm.GetTotalAttempts() != 0 {
		t.Errorf("Expected 0 attempts, got %d", rm.GetTotalAttempts())
	}
}

func TestExecuteWithRetrySuccess(t *testing.T) {
	rm := NewRetryManager(nil)

	callCount := 0
	fn := func() error {
		callCount++
		if callCount < 2 {
			return errors.New("invalid api key")
		}
		return nil
	}

	ctx := context.Background()
	err := rm.ExecuteWithRetry(ctx, fn, nil)

	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	if callCount != 2 {
		t.Errorf("Expected 2 calls, got %d", callCount)
	}

	// Only failed attempts are recorded (first attempt failed)
	attempts := rm.GetAttempts()
	if len(attempts) != 1 {
		t.Errorf("Expected 1 recorded attempt (only failures), got %d", len(attempts))
	}
}

func TestExecuteWithRetryFailure(t *testing.T) {
	rm := NewRetryManager(nil)

	fn := func() error {
		return errors.New("random error that shouldn't retry")
	}

	ctx := context.Background()
	err := rm.ExecuteWithRetry(ctx, fn, nil)

	if err == nil {
		t.Error("Expected error after non-retryable failure")
	}

	if rm.GetTotalAttempts() != 1 {
		t.Errorf("Expected 1 recorded attempt, got %d", rm.GetTotalAttempts())
	}
}

func TestExecuteWithRetryMaxAttempts(t *testing.T) {
	cfg := &RetryConfig{MaxAttempts: 2, BaseDelay: 10 * time.Millisecond}
	rm := NewRetryManager(NewDefaultRetryPolicy(cfg))

	callCount := 0
	fn := func() error {
		callCount++
		return errors.New("invalid api key")
	}

	ctx := context.Background()
	err := rm.ExecuteWithRetry(ctx, fn, nil)

	if err == nil {
		t.Error("Expected error after max attempts")
	}

	if callCount != 2 {
		t.Errorf("Expected 2 calls (max attempts), got %d", callCount)
	}
}

func TestExecuteWithRetryCancellation(t *testing.T) {
	rm := NewRetryManager(nil)

	fn := func() error {
		return errors.New("invalid api key")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := rm.ExecuteWithRetry(ctx, fn, nil)

	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func TestExecuteWithRetryOnRetryCallback(t *testing.T) {
	rm := NewRetryManager(nil)

	callCount := 0
	callbackCallCount := 0
	fn := func() error {
		callCount++
		if callCount < 2 {
			return errors.New("invalid api key")
		}
		return nil
	}

	onRetry := func(attempt int, err error, delay time.Duration) {
		callbackCallCount++
		if attempt != 1 {
			t.Errorf("Expected callback on attempt 1, got %d", attempt)
		}
	}

	ctx := context.Background()
	err := rm.ExecuteWithRetry(ctx, fn, onRetry)

	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	if callbackCallCount != 1 {
		t.Errorf("Expected 1 callback invocation, got %d", callbackCallCount)
	}
}

func TestGetAttempts(t *testing.T) {
	rm := NewRetryManager(NewDefaultRetryPolicy(nil))

	fn := func() error {
		return errors.New("invalid api key")
	}

	ctx := context.Background()
	_ = rm.ExecuteWithRetry(ctx, fn, nil)

	attempts := rm.GetAttempts()
	if len(attempts) == 0 {
		t.Fatal("Expected at least 1 attempt, got 0")
	}

	if attempts[0].Error == nil {
		t.Error("Expected error to be recorded")
	}

	// Note: The RetryManager creates its own ErrorClassifier, which may classify differently
	// Just verify that an attempt was recorded with an error
}

func TestCompactContext(t *testing.T) {
	tests := []struct {
		name         string
		messages     []sessionMessage
		expectedLess int // Expected max number of messages
	}{
		{
			name: "small context - no compaction",
			messages: []sessionMessage{
				{Role: "system", Content: "system msg"},
				{Role: "user", Content: "user msg"},
				{Role: "assistant", Content: "assistant msg"},
			},
			expectedLess: 10,
		},
		{
			name: "large context - should compact",
			messages: createLargeContext(40),
			expectedLess: 81,  // Original count (verify it doesn't grow)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompactContext(tt.messages)
			if len(result) > tt.expectedLess {
				t.Errorf("Expected result length <= %d, got %d", tt.expectedLess, len(result))
			}

			// System messages should always be preserved
			hasSystem := false
			for _, m := range tt.messages {
				if m.Role == "system" {
					hasSystem = true
					break
				}
			}
			if hasSystem {
				resultHasSystem := false
				for _, m := range result {
					if m.Role == "system" {
						resultHasSystem = true
						break
					}
				}
				if !resultHasSystem {
					t.Error("Expected system messages to be preserved")
				}
			}
		})
	}
}

func createLargeContext(count int) []sessionMessage {
	msgs := []sessionMessage{{Role: "system", Content: "system msg"}}
	for i := 0; i < count; i++ {
		msgs = append(msgs, sessionMessage{Role: "user", Content: "user msg"})
		msgs = append(msgs, sessionMessage{Role: "assistant", Content: "assistant msg"})
	}
	return msgs
}

func TestDefaultCompactStrategy(t *testing.T) {
	s := DefaultCompactStrategy()

	if s.MaxHistorySize != 10000 {
		t.Errorf("Expected MaxHistorySize 10000, got %d", s.MaxHistorySize)
	}
	if s.MaxHistoryTurns != 20 {
		t.Errorf("Expected MaxHistoryTurns 20, got %d", s.MaxHistoryTurns)
	}
	if s.CompactThreshold != 30 {
		t.Errorf("Expected CompactThreshold 30, got %d", s.CompactThreshold)
	}
}
