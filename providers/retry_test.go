package providers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// retryMockProvider is a mock implementation of Provider for retry testing
type retryMockProvider struct {
	chatFunc          func(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error)
	chatWithToolsFunc func(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error)
	closeErr          error
	callCount         int
}

func (m *retryMockProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
	m.callCount++
	if m.chatFunc != nil {
		return m.chatFunc(ctx, messages, tools, options...)
	}
	return &Response{Content: "success"}, nil
}

func (m *retryMockProvider) ChatWithTools(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
	m.callCount++
	if m.chatWithToolsFunc != nil {
		return m.chatWithToolsFunc(ctx, messages, tools, options...)
	}
	return &Response{Content: "success with tools"}, nil
}

func (m *retryMockProvider) Close() error {
	return m.closeErr
}

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	assert.Equal(t, 3, config.MaxRetries)
	assert.Equal(t, 1*time.Second, config.InitialDelay)
	assert.Equal(t, 30*time.Second, config.MaxDelay)
	assert.Equal(t, 2.0, config.BackoffFactor)
}

func TestNewRetryableProvider(t *testing.T) {
	mock := &retryMockProvider{}
	config := RetryConfig{
		MaxRetries:    5,
		InitialDelay:  2 * time.Second,
		MaxDelay:      60 * time.Second,
		BackoffFactor: 1.5,
	}

	rp := NewRetryableProvider(mock, config)

	require.NotNil(t, rp)
	assert.Equal(t, mock, rp.provider)
	assert.Equal(t, config, rp.config)
}

func TestRetryableProvider_Chat_Success(t *testing.T) {
	tests := []struct {
		name          string
		callCount     int
		failUntilCall int
		config        RetryConfig
		expectedCalls int
	}{
		{
			name:          "success on first attempt",
			callCount:     0,
			config:        DefaultRetryConfig(),
			expectedCalls: 1,
		},
		{
			name:          "success on second attempt",
			failUntilCall: 1,
			config:        RetryConfig{MaxRetries: 3, InitialDelay: 10 * time.Millisecond, MaxDelay: 100 * time.Millisecond, BackoffFactor: 2.0},
			expectedCalls: 2,
		},
		{
			name:          "success on third attempt",
			failUntilCall: 2,
			config:        RetryConfig{MaxRetries: 3, InitialDelay: 10 * time.Millisecond, MaxDelay: 100 * time.Millisecond, BackoffFactor: 2.0},
			expectedCalls: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			mock := &retryMockProvider{
				chatFunc: func(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
					callCount++
					if callCount <= tt.failUntilCall {
						return nil, errors.New("rate limit exceeded")
					}
					return &Response{Content: "success"}, nil
				},
			}

			rp := NewRetryableProvider(mock, tt.config)
			resp, err := rp.Chat(context.Background(), nil, nil)

			require.NoError(t, err)
			assert.Equal(t, "success", resp.Content)
			assert.Equal(t, tt.expectedCalls, callCount)
		})
	}
}

func TestRetryableProvider_Chat_NonRetryableError(t *testing.T) {
	tests := []struct {
		name            string
		errMsg          string
		expectedRetries int
	}{
		{
			name:            "unauthorized error",
			errMsg:          "unauthorized: invalid api key",
			expectedRetries: 1,
		},
		{
			name:            "forbidden error",
			errMsg:          "403 forbidden",
			expectedRetries: 1,
		},
		{
			name:            "authentication error",
			errMsg:          "authentication failed",
			expectedRetries: 1,
		},
		{
			name:            "invalid api key",
			errMsg:          "invalid api key provided",
			expectedRetries: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			mock := &retryMockProvider{
				chatFunc: func(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
					callCount++
					return nil, errors.New(tt.errMsg)
				},
			}

			rp := NewRetryableProvider(mock, RetryConfig{
				MaxRetries:    3,
				InitialDelay:  10 * time.Millisecond,
				MaxDelay:      100 * time.Millisecond,
				BackoffFactor: 2.0,
			})

			resp, err := rp.Chat(context.Background(), nil, nil)

			require.Error(t, err)
			assert.Nil(t, resp)
			assert.Equal(t, tt.expectedRetries, callCount)
		})
	}
}

func TestRetryableProvider_Chat_RetryableErrors(t *testing.T) {
	tests := []struct {
		name   string
		errMsg string
	}{
		{"rate limit", "rate limit exceeded"},
		{"429 error", "429 too many requests"},
		{"timeout", "timeout waiting for response"},
		{"deadline exceeded", "deadline exceeded"},
		{"connection refused", "connection refused"},
		{"connection reset", "connection reset by peer"},
		{"network error", "network is unreachable"},
		{"temporary error", "temporary failure"},
		{"500 error", "500 internal server error"},
		{"502 error", "502 bad gateway"},
		{"503 error", "503 service unavailable"},
		{"504 error", "504 gateway timeout"},
		{"internal error", "internal error occurred"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			mock := &retryMockProvider{
				chatFunc: func(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
					callCount++
					if callCount < 3 {
						return nil, errors.New(tt.errMsg)
					}
					return &Response{Content: "success"}, nil
				},
			}

			rp := NewRetryableProvider(mock, RetryConfig{
				MaxRetries:    3,
				InitialDelay:  10 * time.Millisecond,
				MaxDelay:      100 * time.Millisecond,
				BackoffFactor: 2.0,
			})

			resp, err := rp.Chat(context.Background(), nil, nil)

			require.NoError(t, err)
			assert.Equal(t, "success", resp.Content)
			assert.Equal(t, 3, callCount)
		})
	}
}

func TestRetryableProvider_Chat_MaxRetriesExhausted(t *testing.T) {
	callCount := 0
	mock := &retryMockProvider{
		chatFunc: func(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
			callCount++
			return nil, errors.New("rate limit exceeded")
		},
	}

	config := RetryConfig{
		MaxRetries:    2,
		InitialDelay:  10 * time.Millisecond,
		MaxDelay:      100 * time.Millisecond,
		BackoffFactor: 2.0,
	}
	rp := NewRetryableProvider(mock, config)

	resp, err := rp.Chat(context.Background(), nil, nil)

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, 3, callCount)
}

func TestRetryableProvider_Chat_ContextCancellation(t *testing.T) {
	callCount := 0
	mock := &retryMockProvider{
		chatFunc: func(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
			callCount++
			return nil, errors.New("timeout occurred")
		},
	}

	config := RetryConfig{
		MaxRetries:    5,
		InitialDelay:  1 * time.Second,
		MaxDelay:      10 * time.Second,
		BackoffFactor: 2.0,
	}
	rp := NewRetryableProvider(mock, config)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	resp, err := rp.Chat(ctx, nil, nil)

	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	assert.Nil(t, resp)
	assert.Equal(t, 1, callCount)
}

func TestRetryableProvider_Chat_ContextTimeout(t *testing.T) {
	mock := &retryMockProvider{
		chatFunc: func(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
			return nil, errors.New("timeout occurred")
		},
	}

	config := RetryConfig{
		MaxRetries:    5,
		InitialDelay:  1 * time.Second,
		MaxDelay:      10 * time.Second,
		BackoffFactor: 2.0,
	}
	rp := NewRetryableProvider(mock, config)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	resp, err := rp.Chat(ctx, nil, nil)

	require.Error(t, err)
	assert.Equal(t, context.DeadlineExceeded, err)
	assert.Nil(t, resp)
}

func TestRetryableProvider_ChatWithTools_Success(t *testing.T) {
	callCount := 0
	mock := &retryMockProvider{
		chatWithToolsFunc: func(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
			callCount++
			if callCount < 2 {
				return nil, errors.New("rate limit")
			}
			return &Response{Content: "success with tools"}, nil
		},
	}

	rp := NewRetryableProvider(mock, RetryConfig{
		MaxRetries:    3,
		InitialDelay:  10 * time.Millisecond,
		MaxDelay:      100 * time.Millisecond,
		BackoffFactor: 2.0,
	})

	resp, err := rp.ChatWithTools(context.Background(), nil, nil)

	require.NoError(t, err)
	assert.Equal(t, "success with tools", resp.Content)
	assert.Equal(t, 2, callCount)
}

func TestRetryableProvider_ChatWithTools_NonRetryableError(t *testing.T) {
	callCount := 0
	mock := &retryMockProvider{
		chatWithToolsFunc: func(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
			callCount++
			return nil, errors.New("unauthorized access")
		},
	}

	rp := NewRetryableProvider(mock, RetryConfig{
		MaxRetries:    3,
		InitialDelay:  10 * time.Millisecond,
		MaxDelay:      100 * time.Millisecond,
		BackoffFactor: 2.0,
	})

	resp, err := rp.ChatWithTools(context.Background(), nil, nil)

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, 1, callCount)
}

func TestRetryableProvider_ChatWithTools_MaxRetriesExhausted(t *testing.T) {
	callCount := 0
	mock := &retryMockProvider{
		chatWithToolsFunc: func(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
			callCount++
			return nil, errors.New("service unavailable")
		},
	}

	config := RetryConfig{
		MaxRetries:    2,
		InitialDelay:  10 * time.Millisecond,
		MaxDelay:      100 * time.Millisecond,
		BackoffFactor: 2.0,
	}
	rp := NewRetryableProvider(mock, config)

	resp, err := rp.ChatWithTools(context.Background(), nil, nil)

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, 3, callCount)
}

func TestRetryableProvider_Close(t *testing.T) {
	tests := []struct {
		name     string
		closeErr error
	}{
		{"success", nil},
		{"error", errors.New("close failed")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &retryMockProvider{closeErr: tt.closeErr}
			rp := NewRetryableProvider(mock, DefaultRetryConfig())

			err := rp.Close()

			if tt.closeErr != nil {
				require.Error(t, err)
				assert.Equal(t, tt.closeErr, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRetryableProvider_CalculateDelay(t *testing.T) {
	tests := []struct {
		name        string
		config      RetryConfig
		attempt     int
		expectedMin time.Duration
		expectedMax time.Duration
	}{
		{
			name:        "first retry attempt",
			config:      RetryConfig{InitialDelay: 1 * time.Second, MaxDelay: 30 * time.Second, BackoffFactor: 2.0},
			attempt:     1,
			expectedMin: 1 * time.Second,
			expectedMax: 1 * time.Second,
		},
		{
			name:        "second retry attempt",
			config:      RetryConfig{InitialDelay: 1 * time.Second, MaxDelay: 30 * time.Second, BackoffFactor: 2.0},
			attempt:     2,
			expectedMin: 2 * time.Second,
			expectedMax: 2 * time.Second,
		},
		{
			name:        "third retry attempt",
			config:      RetryConfig{InitialDelay: 1 * time.Second, MaxDelay: 30 * time.Second, BackoffFactor: 2.0},
			attempt:     3,
			expectedMin: 4 * time.Second,
			expectedMax: 4 * time.Second,
		},
		{
			name:        "hit max delay cap",
			config:      RetryConfig{InitialDelay: 1 * time.Second, MaxDelay: 5 * time.Second, BackoffFactor: 2.0},
			attempt:     4,
			expectedMin: 5 * time.Second,
			expectedMax: 5 * time.Second,
		},
		{
			name:        "different backoff factor",
			config:      RetryConfig{InitialDelay: 100 * time.Millisecond, MaxDelay: 10 * time.Second, BackoffFactor: 1.5},
			attempt:     3,
			expectedMin: 225 * time.Millisecond,
			expectedMax: 225 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rp := NewRetryableProvider(&retryMockProvider{}, tt.config)
			delay := rp.calculateDelay(tt.attempt)

			assert.GreaterOrEqual(t, delay, tt.expectedMin)
			assert.LessOrEqual(t, delay, tt.expectedMax)
		})
	}
}

func TestRetryableProvider_IsRetryable(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		expectedRetry bool
	}{
		{"nil error", nil, false},
		{"rate limit", errors.New("rate limit exceeded"), true},
		{"429 error", errors.New("429 too many requests"), true},
		{"too many requests", errors.New("too many requests"), true},
		{"timeout", errors.New("timeout"), true},
		{"deadline exceeded", errors.New("deadline exceeded"), true},
		{"context deadline", errors.New("context deadline exceeded"), true},
		{"connection refused", errors.New("connection refused"), true},
		{"connection reset", errors.New("connection reset"), true},
		{"network error", errors.New("network error"), true},
		{"temporary error", errors.New("temporary failure"), true},
		{"500 error", errors.New("500 internal server error"), true},
		{"502 error", errors.New("502 bad gateway"), true},
		{"503 error", errors.New("503 service unavailable"), true},
		{"504 error", errors.New("504 gateway timeout"), true},
		{"internal error", errors.New("internal error"), true},
		{"service unavailable", errors.New("service unavailable"), true},
		{"401 unauthorized", errors.New("401 unauthorized"), false},
		{"403 forbidden", errors.New("403 forbidden"), false},
		{"unauthorized", errors.New("unauthorized access"), false},
		{"forbidden", errors.New("forbidden"), false},
		{"invalid api key", errors.New("invalid api key"), false},
		{"authentication failed", errors.New("authentication failed"), false},
		{"unknown error", errors.New("some random error"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rp := NewRetryableProvider(&retryMockProvider{}, DefaultRetryConfig())
			result := rp.isRetryable(tt.err)
			assert.Equal(t, tt.expectedRetry, result)
		})
	}
}

func TestContainsAny(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		substrs  []string
		expected bool
	}{
		{"match found", "error: rate limit exceeded", []string{"rate limit", "timeout"}, true},
		{"match at start", "timeout error", []string{"timeout", "error"}, true},
		{"match at end", "error timeout", []string{"timeout"}, true},
		{"no match", "invalid request", []string{"timeout", "rate limit"}, false},
		{"empty string", "", []string{"error"}, false},
		{"empty substrings", "error message", []string{}, false},
		{"substring longer than string", "err", []string{"error message"}, false},
		{"exact match", "timeout", []string{"timeout"}, true},
		{"case sensitive", "TIMEOUT", []string{"timeout"}, false},
		{"multiple substrings first match", "rate limit", []string{"rate limit", "timeout"}, true},
		{"multiple substrings second match", "timeout", []string{"rate limit", "timeout"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsAny(tt.s, tt.substrs)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRetryableProvider_Chat_WithOptions(t *testing.T) {
	mock := &retryMockProvider{
		chatFunc: func(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
			return &Response{Content: "success"}, nil
		},
	}

	rp := NewRetryableProvider(mock, DefaultRetryConfig())
	opts := []ChatOption{WithModel("gpt-4"), WithTemperature(0.5)}

	resp, err := rp.Chat(context.Background(), nil, nil, opts...)

	require.NoError(t, err)
	assert.Equal(t, "success", resp.Content)
}

func TestRetryableProvider_ChatWithTools_ContextCancellation(t *testing.T) {
	callCount := 0
	mock := &retryMockProvider{
		chatWithToolsFunc: func(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
			callCount++
			return nil, errors.New("timeout occurred")
		},
	}

	config := RetryConfig{
		MaxRetries:    5,
		InitialDelay:  1 * time.Second,
		MaxDelay:      10 * time.Second,
		BackoffFactor: 2.0,
	}
	rp := NewRetryableProvider(mock, config)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	resp, err := rp.ChatWithTools(ctx, nil, nil)

	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	assert.Nil(t, resp)
	assert.Equal(t, 1, callCount)
}
