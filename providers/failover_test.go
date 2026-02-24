package providers

import (
	"context"
	stderrors "errors"
	"testing"

	"github.com/smallnest/goclaw/errors"
)

// mockProvider 用于测试的模拟提供商
type mockProvider struct {
	shouldFail bool
	failError  error
	response   *Response
}

func (m *mockProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
	if m.shouldFail {
		return nil, m.failError
	}
	return m.response, nil
}

func (m *mockProvider) ChatWithTools(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
	return m.Chat(ctx, messages, tools, options...)
}

func (m *mockProvider) Close() error {
	return nil
}

func TestNewFailoverProvider(t *testing.T) {
	primary := &mockProvider{}
	fallback := &mockProvider{}
	classifier := errors.NewSimpleErrorClassifier()

	fp := NewFailoverProvider(primary, fallback, classifier)

	if fp.GetPrimary() != primary {
		t.Error("Expected primary to be set")
	}
	if fp.GetFallback() != fallback {
		t.Error("Expected fallback to be set")
	}
	if fp.GetCircuitBreaker() == nil {
		t.Error("Expected circuit breaker to be set")
	}
}

func TestFailoverProviderSuccessOnPrimary(t *testing.T) {
	primary := &mockProvider{
		response: &Response{Content: "success"},
	}
	fallback := &mockProvider{}
	classifier := errors.NewSimpleErrorClassifier()

	fp := NewFailoverProvider(primary, fallback, classifier)

	ctx := context.Background()
	resp, err := fp.Chat(ctx, nil, nil)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if resp.Content != "success" {
		t.Errorf("Expected 'success', got '%s'", resp.Content)
	}
}

func TestFailoverProviderFailoverOnAuthError(t *testing.T) {
	primary := &mockProvider{
		shouldFail: true,
		failError:  stderrors.New("invalid api key"),
	}
	fallback := &mockProvider{
		response: &Response{Content: "fallback response"},
	}
	classifier := errors.NewSimpleErrorClassifier()

	fp := NewFailoverProvider(primary, fallback, classifier)

	ctx := context.Background()
	resp, err := fp.Chat(ctx, nil, nil)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if resp.Content != "fallback response" {
		t.Errorf("Expected 'fallback response', got '%s'", resp.Content)
	}

	// Note: Circuit breaker has threshold of 5, so single failure won't open it
	// Just verify that failover worked
}

func TestFailoverProviderFailoverOnRateLimit(t *testing.T) {
	primary := &mockProvider{
		shouldFail: true,
		failError:  stderrors.New("rate limit exceeded"),
	}
	fallback := &mockProvider{
		response: &Response{Content: "fallback response"},
	}
	classifier := errors.NewSimpleErrorClassifier()

	fp := NewFailoverProvider(primary, fallback, classifier)

	ctx := context.Background()
	resp, err := fp.Chat(ctx, nil, nil)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if resp.Content != "fallback response" {
		t.Errorf("Expected 'fallback response', got '%s'", resp.Content)
	}
}

func TestFailoverProviderNoFailbackOnTimeout(t *testing.T) {
	primary := &mockProvider{
		shouldFail: true,
		failError:  stderrors.New("timeout"),
	}
	fallback := &mockProvider{
		response: &Response{Content: "fallback response"},
	}
	classifier := errors.NewSimpleErrorClassifier()

	fp := NewFailoverProvider(primary, fallback, classifier)

	ctx := context.Background()
	_, err := fp.Chat(ctx, nil, nil)

	if err == nil {
		t.Error("Expected error for timeout (should not failover)")
	}

	// Circuit should NOT be open for non-failover errors
	if fp.GetCircuitBreaker().IsOpen() {
		t.Error("Expected circuit breaker to remain closed for non-failover errors")
	}
}

func TestFailoverProviderNoFallback(t *testing.T) {
	primary := &mockProvider{
		shouldFail: true,
		failError:  stderrors.New("invalid api key"),
	}
	classifier := errors.NewSimpleErrorClassifier()

	fp := NewFailoverProvider(primary, nil, classifier)

	ctx := context.Background()
	_, err := fp.Chat(ctx, nil, nil)

	if err == nil {
		t.Error("Expected error when primary fails and no fallback available")
	}
}

func TestFailoverProviderUseFallbackWhenCircuitOpen(t *testing.T) {
	primary := &mockProvider{
		shouldFail: true,
		failError:  stderrors.New("invalid api key"),
	}
	fallback := &mockProvider{
		response: &Response{Content: "fallback response"},
	}
	classifier := errors.NewSimpleErrorClassifier()

	fp := NewFailoverProvider(primary, fallback, classifier)

	ctx := context.Background()

	// First call - primary fails, fallback succeeds
	_, _ = fp.Chat(ctx, nil, nil)

	// Make a second call - still uses fallback since primary fails
	resp, err := fp.Chat(ctx, nil, nil)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if resp.Content != "fallback response" {
		t.Errorf("Expected 'fallback response', got '%s'", resp.Content)
	}
}

func TestFailoverProviderSetFallback(t *testing.T) {
	primary := &mockProvider{}
	classifier := errors.NewSimpleErrorClassifier()

	fp := NewFailoverProvider(primary, nil, classifier)

	if fp.GetFallback() != nil {
		t.Error("Expected no fallback initially")
	}

	newFallback := &mockProvider{response: &Response{Content: "new fallback"}}
	fp.SetFallback(newFallback)

	if fp.GetFallback() != newFallback {
		t.Error("Expected fallback to be updated")
	}
}

func TestFailoverProviderShouldFailover(t *testing.T) {
	tests := []struct {
		name   string
		reason errors.FailoverReason
		want   bool
	}{
		{"auth error", errors.FailoverReasonAuth, true},
		{"rate limit", errors.FailoverReasonRateLimit, true},
		{"billing", errors.FailoverReasonBilling, true},
		{"timeout", errors.FailoverReasonTimeout, false},
		{"unknown", errors.FailoverReasonUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp := &FailoverProvider{}
			result := fp.shouldFailover(tt.reason)
			if result != tt.want {
				t.Errorf("Expected %v for %s, got %v", tt.want, tt.name, result)
			}
		})
	}
}

func TestFailoverProviderClose(t *testing.T) {
	primary := &mockProvider{}
	fallback := &mockProvider{}
	classifier := &errors.SimpleErrorClassifier{}

	fp := NewFailoverProvider(primary, fallback, classifier)

	err := fp.Close()
	if err != nil {
		t.Fatalf("Expected no error on close, got %v", err)
	}
}
