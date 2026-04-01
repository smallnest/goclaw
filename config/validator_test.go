package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewValidator(t *testing.T) {
	tests := []struct {
		name       string
		strictMode bool
	}{
		{name: "strict mode enabled", strictMode: true},
		{name: "strict mode disabled", strictMode: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(tt.strictMode)
			assert.NotNil(t, v)
			assert.Equal(t, tt.strictMode, v.strictMode)
		})
	}
}

func TestValidator_Validate_NilConfig(t *testing.T) {
	v := NewValidator(true)
	err := v.Validate(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "configuration cannot be nil")
}

func TestValidator_ValidateRetryConfig(t *testing.T) {
	tests := []struct {
		name    string
		retry   *RetryConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid retry config",
			retry: &RetryConfig{
				MaxRetries:    3,
				InitialDelay:  2 * time.Second,
				MaxDelay:      60 * time.Second,
				BackoffFactor: 2.0,
			},
			wantErr: false,
		},
		{
			name: "max_retries at lower boundary",
			retry: &RetryConfig{
				MaxRetries:    0,
				InitialDelay:  1 * time.Second,
				MaxDelay:      10 * time.Second,
				BackoffFactor: 1.5,
			},
			wantErr: false,
		},
		{
			name: "max_retries at upper boundary",
			retry: &RetryConfig{
				MaxRetries:    10,
				InitialDelay:  1 * time.Second,
				MaxDelay:      10 * time.Second,
				BackoffFactor: 1.5,
			},
			wantErr: false,
		},
		{
			name: "max_retries negative",
			retry: &RetryConfig{
				MaxRetries:    -1,
				InitialDelay:  1 * time.Second,
				MaxDelay:      10 * time.Second,
				BackoffFactor: 1.5,
			},
			wantErr: true,
			errMsg:  "retry.max_retries must be between 0 and 10",
		},
		{
			name: "max_retries exceeds maximum",
			retry: &RetryConfig{
				MaxRetries:    11,
				InitialDelay:  1 * time.Second,
				MaxDelay:      10 * time.Second,
				BackoffFactor: 1.5,
			},
			wantErr: true,
			errMsg:  "retry.max_retries must be between 0 and 10",
		},
		{
			name: "initial_delay at lower boundary",
			retry: &RetryConfig{
				MaxRetries:    3,
				InitialDelay:  0,
				MaxDelay:      10 * time.Second,
				BackoffFactor: 1.5,
			},
			wantErr: false,
		},
		{
			name: "initial_delay at upper boundary",
			retry: &RetryConfig{
				MaxRetries:    3,
				InitialDelay:  60 * time.Second,
				MaxDelay:      60 * time.Second,
				BackoffFactor: 1.5,
			},
			wantErr: false,
		},
		{
			name: "initial_delay negative",
			retry: &RetryConfig{
				MaxRetries:    3,
				InitialDelay:  -1 * time.Second,
				MaxDelay:      10 * time.Second,
				BackoffFactor: 1.5,
			},
			wantErr: true,
			errMsg:  "retry.initial_delay must be between 0 and 60 seconds",
		},
		{
			name: "initial_delay exceeds maximum",
			retry: &RetryConfig{
				MaxRetries:    3,
				InitialDelay:  61 * time.Second,
				MaxDelay:      70 * time.Second,
				BackoffFactor: 1.5,
			},
			wantErr: true,
			errMsg:  "retry.initial_delay must be between 0 and 60 seconds",
		},
		{
			name: "max_delay at lower boundary",
			retry: &RetryConfig{
				MaxRetries:    3,
				InitialDelay:  1 * time.Second,
				MaxDelay:      0,
				BackoffFactor: 1.5,
			},
			wantErr: false,
		},
		{
			name: "max_delay at upper boundary",
			retry: &RetryConfig{
				MaxRetries:    3,
				InitialDelay:  1 * time.Second,
				MaxDelay:      300 * time.Second,
				BackoffFactor: 1.5,
			},
			wantErr: false,
		},
		{
			name: "max_delay negative",
			retry: &RetryConfig{
				MaxRetries:    3,
				InitialDelay:  1 * time.Second,
				MaxDelay:      -1 * time.Second,
				BackoffFactor: 1.5,
			},
			wantErr: true,
			errMsg:  "retry.max_delay must be between 0 and 300 seconds",
		},
		{
			name: "max_delay exceeds maximum",
			retry: &RetryConfig{
				MaxRetries:    3,
				InitialDelay:  1 * time.Second,
				MaxDelay:      301 * time.Second,
				BackoffFactor: 1.5,
			},
			wantErr: true,
			errMsg:  "retry.max_delay must be between 0 and 300 seconds",
		},
		{
			name: "backoff_factor at lower boundary",
			retry: &RetryConfig{
				MaxRetries:    3,
				InitialDelay:  1 * time.Second,
				MaxDelay:      10 * time.Second,
				BackoffFactor: 1.0,
			},
			wantErr: false,
		},
		{
			name: "backoff_factor at upper boundary",
			retry: &RetryConfig{
				MaxRetries:    3,
				InitialDelay:  1 * time.Second,
				MaxDelay:      10 * time.Second,
				BackoffFactor: 10.0,
			},
			wantErr: false,
		},
		{
			name: "backoff_factor below minimum",
			retry: &RetryConfig{
				MaxRetries:    3,
				InitialDelay:  1 * time.Second,
				MaxDelay:      10 * time.Second,
				BackoffFactor: 0.9,
			},
			wantErr: true,
			errMsg:  "retry.backoff_factor must be between 1.0 and 10.0",
		},
		{
			name: "backoff_factor exceeds maximum",
			retry: &RetryConfig{
				MaxRetries:    3,
				InitialDelay:  1 * time.Second,
				MaxDelay:      10 * time.Second,
				BackoffFactor: 10.1,
			},
			wantErr: true,
			errMsg:  "retry.backoff_factor must be between 1.0 and 10.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(true)
			err := v.validateRetryConfig(tt.retry)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateSubagentsDefaults(t *testing.T) {
	tests := []struct {
		name      string
		subagents *SubagentsConfig
		wantErr   bool
		errMsg    string
	}{
		{
			name: "valid subagents config",
			subagents: &SubagentsConfig{
				MaxConcurrent:       5,
				ArchiveAfterMinutes: 60,
				TimeoutSeconds:      300,
			},
			wantErr: false,
		},
		{
			name: "max_concurrent at lower boundary",
			subagents: &SubagentsConfig{
				MaxConcurrent:       1,
				ArchiveAfterMinutes: 60,
				TimeoutSeconds:      300,
			},
			wantErr: false,
		},
		{
			name: "max_concurrent at upper boundary",
			subagents: &SubagentsConfig{
				MaxConcurrent:       10,
				ArchiveAfterMinutes: 60,
				TimeoutSeconds:      300,
			},
			wantErr: false,
		},
		{
			name: "max_concurrent below minimum",
			subagents: &SubagentsConfig{
				MaxConcurrent:       0,
				ArchiveAfterMinutes: 60,
				TimeoutSeconds:      300,
			},
			wantErr: true,
			errMsg:  "subagents.max_concurrent must be between 1 and 10",
		},
		{
			name: "max_concurrent exceeds maximum",
			subagents: &SubagentsConfig{
				MaxConcurrent:       11,
				ArchiveAfterMinutes: 60,
				TimeoutSeconds:      300,
			},
			wantErr: true,
			errMsg:  "subagents.max_concurrent must be between 1 and 10",
		},
		{
			name: "archive_after_minutes at lower boundary",
			subagents: &SubagentsConfig{
				MaxConcurrent:       5,
				ArchiveAfterMinutes: 1,
				TimeoutSeconds:      300,
			},
			wantErr: false,
		},
		{
			name: "archive_after_minutes at upper boundary (24 hours)",
			subagents: &SubagentsConfig{
				MaxConcurrent:       5,
				ArchiveAfterMinutes: 1440,
				TimeoutSeconds:      300,
			},
			wantErr: false,
		},
		{
			name: "archive_after_minutes below minimum",
			subagents: &SubagentsConfig{
				MaxConcurrent:       5,
				ArchiveAfterMinutes: 0,
				TimeoutSeconds:      300,
			},
			wantErr: true,
			errMsg:  "subagents.archive_after_minutes must be between 1 and 1440 (24 hours)",
		},
		{
			name: "archive_after_minutes exceeds maximum",
			subagents: &SubagentsConfig{
				MaxConcurrent:       5,
				ArchiveAfterMinutes: 1441,
				TimeoutSeconds:      300,
			},
			wantErr: true,
			errMsg:  "subagents.archive_after_minutes must be between 1 and 1440 (24 hours)",
		},
		{
			name: "timeout_seconds at lower boundary",
			subagents: &SubagentsConfig{
				MaxConcurrent:       5,
				ArchiveAfterMinutes: 60,
				TimeoutSeconds:      60,
			},
			wantErr: false,
		},
		{
			name: "timeout_seconds at upper boundary",
			subagents: &SubagentsConfig{
				MaxConcurrent:       5,
				ArchiveAfterMinutes: 60,
				TimeoutSeconds:      3600,
			},
			wantErr: false,
		},
		{
			name: "timeout_seconds below minimum",
			subagents: &SubagentsConfig{
				MaxConcurrent:       5,
				ArchiveAfterMinutes: 60,
				TimeoutSeconds:      59,
			},
			wantErr: true,
			errMsg:  "subagents.timeout_seconds must be between 60 and 3600",
		},
		{
			name: "timeout_seconds exceeds maximum",
			subagents: &SubagentsConfig{
				MaxConcurrent:       5,
				ArchiveAfterMinutes: 60,
				TimeoutSeconds:      3601,
			},
			wantErr: true,
			errMsg:  "subagents.timeout_seconds must be between 60 and 3600",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(true)
			err := v.validateSubagentsDefaults(tt.subagents)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateModelsMode(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "mode merge",
			mode:    "merge",
			wantErr: false,
		},
		{
			name:    "mode replace",
			mode:    "replace",
			wantErr: false,
		},
		{
			name:    "mode empty (valid)",
			mode:    "",
			wantErr: false,
		},
		{
			name:    "mode invalid",
			mode:    "invalid",
			wantErr: true,
			errMsg:  "models.mode must be 'merge' or 'replace'",
		},
		{
			name:    "mode random string",
			mode:    "random",
			wantErr: true,
			errMsg:  "models.mode must be 'merge' or 'replace'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(true)
			cfg := createValidConfig()
			cfg.Models.Mode = tt.mode

			err := v.validateProviders(cfg)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateWebSocketAuthToken(t *testing.T) {
	tests := []struct {
		name    string
		ws      *WebSocketConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "auth disabled with empty token",
			ws: &WebSocketConfig{
				Host:       "localhost",
				Port:       8080,
				EnableAuth: false,
				AuthToken:  "",
			},
			wantErr: false,
		},
		{
			name: "auth enabled with valid token",
			ws: &WebSocketConfig{
				Host:       "localhost",
				Port:       8080,
				EnableAuth: true,
				AuthToken:  "valid-token-123",
			},
			wantErr: false,
		},
		{
			name: "auth enabled with empty token",
			ws: &WebSocketConfig{
				Host:       "localhost",
				Port:       8080,
				EnableAuth: true,
				AuthToken:  "",
			},
			wantErr: true,
			errMsg:  "websocket auth_token is required when enable_auth is true",
		},
		{
			name: "auth enabled with whitespace token",
			ws: &WebSocketConfig{
				Host:       "localhost",
				Port:       8080,
				EnableAuth: true,
				AuthToken:  "   ",
			},
			wantErr: true,
			errMsg:  "websocket auth_token is required when enable_auth is true",
		},
		{
			name: "no host configured",
			ws: &WebSocketConfig{
				Host:       "",
				Port:       8080,
				EnableAuth: true,
				AuthToken:  "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(true)
			err := v.validateWebSocketConfig(tt.ws)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidatePortWithZero(t *testing.T) {
	tests := []struct {
		name    string
		port    int
		wantErr bool
	}{
		{
			name:    "port 0 should be allowed (not configured)",
			port:    0,
			wantErr: false,
		},
		{
			name:    "port at lower boundary",
			port:    1024,
			wantErr: false,
		},
		{
			name:    "port at upper boundary",
			port:    65535,
			wantErr: false,
		},
		{
			name:    "port below valid range",
			port:    1023,
			wantErr: true,
		},
		{
			name:    "port above valid range",
			port:    65536,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(true)

			cfg := createValidConfig()
			cfg.Gateway.Port = tt.port
			err := v.validateGateway(cfg)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateWebSocketPortWithZero(t *testing.T) {
	tests := []struct {
		name    string
		port    int
		wantErr bool
	}{
		{
			name:    "port 0 should be allowed (use default)",
			port:    0,
			wantErr: false,
		},
		{
			name:    "port at lower boundary",
			port:    1024,
			wantErr: false,
		},
		{
			name:    "port at upper boundary",
			port:    65535,
			wantErr: false,
		},
		{
			name:    "port below valid range",
			port:    1023,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(true)

			ws := &WebSocketConfig{
				Host: "localhost",
				Port: tt.port,
			}
			err := v.validateWebSocketConfig(ws)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateWebToolTimeout(t *testing.T) {
	tests := []struct {
		name    string
		timeout int
		wantErr bool
		errMsg  string
	}{
		{
			name:    "timeout 0 should be allowed (not configured)",
			timeout: 0,
			wantErr: false,
		},
		{
			name:    "timeout at lower boundary",
			timeout: 1,
			wantErr: false,
		},
		{
			name:    "timeout at upper boundary",
			timeout: 300,
			wantErr: false,
		},
		{
			name:    "timeout below minimum",
			timeout: -1,
			wantErr: true,
			errMsg:  "web timeout must be between 1 and 300 seconds",
		},
		{
			name:    "timeout exceeds maximum",
			timeout: 301,
			wantErr: true,
			errMsg:  "web timeout must be between 1 and 300 seconds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(true)
			web := &WebToolConfig{
				Timeout: tt.timeout,
			}
			err := v.validateWebTool(web)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateGatewayTimeouts(t *testing.T) {
	tests := []struct {
		name         string
		readTimeout  time.Duration
		writeTimeout time.Duration
		wantErr      bool
	}{
		{
			name:         "read timeout 0 should be allowed",
			readTimeout:  0,
			writeTimeout: 30 * time.Second,
			wantErr:      false,
		},
		{
			name:         "read timeout at lower boundary",
			readTimeout:  1 * time.Nanosecond,
			writeTimeout: 30 * time.Second,
			wantErr:      false,
		},
		{
			name:         "read timeout at upper boundary",
			readTimeout:  300 * time.Second,
			writeTimeout: 30 * time.Second,
			wantErr:      false,
		},
		{
			name:         "read timeout negative",
			readTimeout:  -1 * time.Second,
			writeTimeout: 30 * time.Second,
			wantErr:      true,
		},
		{
			name:         "read timeout exceeds maximum",
			readTimeout:  301 * time.Second,
			writeTimeout: 30 * time.Second,
			wantErr:      true,
		},
		{
			name:         "write timeout 0 should be allowed",
			readTimeout:  30 * time.Second,
			writeTimeout: 0,
			wantErr:      false,
		},
		{
			name:         "write timeout at upper boundary",
			readTimeout:  30 * time.Second,
			writeTimeout: 300 * time.Second,
			wantErr:      false,
		},
		{
			name:         "write timeout negative",
			readTimeout:  30 * time.Second,
			writeTimeout: -1 * time.Second,
			wantErr:      true,
		},
		{
			name:         "write timeout exceeds maximum",
			readTimeout:  30 * time.Second,
			writeTimeout: 301 * time.Second,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(true)
			cfg := createValidConfig()
			cfg.Gateway.ReadTimeout = tt.readTimeout
			cfg.Gateway.WriteTimeout = tt.writeTimeout

			err := v.validateGateway(cfg)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateAgentDefaults(t *testing.T) {
	tests := []struct {
		name     string
		defaults *AgentDefaults
		wantErr  bool
		errMsg   string
	}{
		{
			name: "valid defaults",
			defaults: &AgentDefaults{
				Model: ModelSelection{
					Primary: "qianfan:test-model",
				},
				MaxIterations: 10,
				Temperature:   1.0,
				MaxTokens:     4096,
			},
			wantErr: false,
		},
		{
			name: "empty model",
			defaults: &AgentDefaults{
				Model: ModelSelection{
					Primary: "",
				},
				MaxIterations: 10,
				Temperature:   1.0,
				MaxTokens:     4096,
			},
			wantErr: true,
			errMsg:  "default agent model cannot be empty",
		},
		{
			name: "whitespace model",
			defaults: &AgentDefaults{
				Model: ModelSelection{
					Primary: "   ",
				},
				MaxIterations: 10,
				Temperature:   1.0,
				MaxTokens:     4096,
			},
			wantErr: true,
			errMsg:  "default agent model cannot be empty",
		},
		{
			name: "max_iterations at lower boundary",
			defaults: &AgentDefaults{
				Model: ModelSelection{
					Primary: "qianfan:test-model",
				},
				MaxIterations: 1,
				Temperature:   1.0,
				MaxTokens:     4096,
			},
			wantErr: false,
		},
		{
			name: "max_iterations at upper boundary",
			defaults: &AgentDefaults{
				Model: ModelSelection{
					Primary: "qianfan:test-model",
				},
				MaxIterations: 100,
				Temperature:   1.0,
				MaxTokens:     4096,
			},
			wantErr: false,
		},
		{
			name: "max_iterations below minimum",
			defaults: &AgentDefaults{
				Model: ModelSelection{
					Primary: "qianfan:test-model",
				},
				MaxIterations: 0,
				Temperature:   1.0,
				MaxTokens:     4096,
			},
			wantErr: true,
			errMsg:  "max_iterations must be between 1 and 100",
		},
		{
			name: "max_iterations exceeds maximum",
			defaults: &AgentDefaults{
				Model: ModelSelection{
					Primary: "qianfan:test-model",
				},
				MaxIterations: 101,
				Temperature:   1.0,
				MaxTokens:     4096,
			},
			wantErr: true,
			errMsg:  "max_iterations must be between 1 and 100",
		},
		{
			name: "temperature at lower boundary",
			defaults: &AgentDefaults{
				Model: ModelSelection{
					Primary: "qianfan:test-model",
				},
				MaxIterations: 10,
				Temperature:   0,
				MaxTokens:     4096,
			},
			wantErr: false,
		},
		{
			name: "temperature at upper boundary",
			defaults: &AgentDefaults{
				Model: ModelSelection{
					Primary: "qianfan:test-model",
				},
				MaxIterations: 10,
				Temperature:   2,
				MaxTokens:     4096,
			},
			wantErr: false,
		},
		{
			name: "temperature negative",
			defaults: &AgentDefaults{
				Model: ModelSelection{
					Primary: "qianfan:test-model",
				},
				MaxIterations: 10,
				Temperature:   -0.1,
				MaxTokens:     4096,
			},
			wantErr: true,
			errMsg:  "temperature must be between 0 and 2",
		},
		{
			name: "temperature exceeds maximum",
			defaults: &AgentDefaults{
				Model: ModelSelection{
					Primary: "qianfan:test-model",
				},
				MaxIterations: 10,
				Temperature:   2.1,
				MaxTokens:     4096,
			},
			wantErr: true,
			errMsg:  "temperature must be between 0 and 2",
		},
		{
			name: "max_tokens at lower boundary",
			defaults: &AgentDefaults{
				Model: ModelSelection{
					Primary: "qianfan:test-model",
				},
				MaxIterations: 10,
				Temperature:   1.0,
				MaxTokens:     1,
			},
			wantErr: false,
		},
		{
			name: "max_tokens at upper boundary",
			defaults: &AgentDefaults{
				Model: ModelSelection{
					Primary: "qianfan:test-model",
				},
				MaxIterations: 10,
				Temperature:   1.0,
				MaxTokens:     128000,
			},
			wantErr: false,
		},
		{
			name: "max_tokens below minimum",
			defaults: &AgentDefaults{
				Model: ModelSelection{
					Primary: "qianfan:test-model",
				},
				MaxIterations: 10,
				Temperature:   1.0,
				MaxTokens:     0,
			},
			wantErr: true,
			errMsg:  "max_tokens must be between 1 and 128000",
		},
		{
			name: "max_tokens exceeds maximum",
			defaults: &AgentDefaults{
				Model: ModelSelection{
					Primary: "qianfan:test-model",
				},
				MaxIterations: 10,
				Temperature:   1.0,
				MaxTokens:     128001,
			},
			wantErr: true,
			errMsg:  "max_tokens must be between 1 and 128000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(true)
			err := v.validateAgentDefaults(tt.defaults)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateAgentConfig(t *testing.T) {
	tests := []struct {
		name    string
		agent   *AgentConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid agent config",
			agent: &AgentConfig{
				ID:    "agent-1",
				Model: "qianfan:test-model",
			},
			wantErr: false,
		},
		{
			name: "empty model",
			agent: &AgentConfig{
				ID:    "agent-1",
				Model: "",
			},
			wantErr: true,
			errMsg:  "agent model cannot be empty",
		},
		{
			name: "whitespace model",
			agent: &AgentConfig{
				ID:    "agent-1",
				Model: "   ",
			},
			wantErr: true,
			errMsg:  "agent model cannot be empty",
		},
		{
			name: "subagent timeout at lower boundary",
			agent: &AgentConfig{
				ID:    "agent-1",
				Model: "qianfan:test-model",
				Subagents: &AgentSubagentConfig{
					TimeoutSeconds: 1,
				},
			},
			wantErr: false,
		},
		{
			name: "subagent timeout at upper boundary",
			agent: &AgentConfig{
				ID:    "agent-1",
				Model: "qianfan:test-model",
				Subagents: &AgentSubagentConfig{
					TimeoutSeconds: 3600,
				},
			},
			wantErr: false,
		},
		{
			name: "subagent timeout below minimum",
			agent: &AgentConfig{
				ID:    "agent-1",
				Model: "qianfan:test-model",
				Subagents: &AgentSubagentConfig{
					TimeoutSeconds: 0,
				},
			},
			wantErr: true,
			errMsg:  "subagent timeout must be between 1 and 3600 seconds",
		},
		{
			name: "subagent timeout exceeds maximum",
			agent: &AgentConfig{
				ID:    "agent-1",
				Model: "qianfan:test-model",
				Subagents: &AgentSubagentConfig{
					TimeoutSeconds: 3601,
				},
			},
			wantErr: true,
			errMsg:  "subagent timeout must be between 1 and 3600 seconds",
		},
		{
			name: "tool in both allow and deny lists",
			agent: &AgentConfig{
				ID:    "agent-1",
				Model: "qianfan:test-model",
				Subagents: &AgentSubagentConfig{
					TimeoutSeconds: 60,
					AllowTools:     []string{"tool1", "tool2"},
					DenyTools:      []string{"tool2", "tool3"},
				},
			},
			wantErr: true,
			errMsg:  "tool 'tool2' is both allowed and denied",
		},
		{
			name: "no overlapping tools",
			agent: &AgentConfig{
				ID:    "agent-1",
				Model: "qianfan:test-model",
				Subagents: &AgentSubagentConfig{
					TimeoutSeconds: 60,
					AllowTools:     []string{"tool1"},
					DenyTools:      []string{"tool2"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(true)
			err := v.validateAgentConfig(tt.agent)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateAgentsList(t *testing.T) {
	tests := []struct {
		name    string
		agents  []AgentConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid agents list",
			agents: []AgentConfig{
				{ID: "agent-1", Model: "qianfan:test-model"},
				{ID: "agent-2", Model: "qianfan:test-model"},
			},
			wantErr: false,
		},
		{
			name: "agent with empty ID",
			agents: []AgentConfig{
				{ID: "", Model: "qianfan:test-model"},
			},
			wantErr: true,
			errMsg:  "agent at index 0 has empty ID",
		},
		{
			name: "duplicate agent IDs",
			agents: []AgentConfig{
				{ID: "agent-1", Model: "qianfan:test-model"},
				{ID: "agent-1", Model: "qianfan:test-model"},
			},
			wantErr: true,
			errMsg:  "duplicate agent ID: agent-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(true)
			cfg := createValidConfig()
			cfg.Agents.List = tt.agents

			err := v.validateAgents(cfg)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateBindings(t *testing.T) {
	tests := []struct {
		name     string
		agents   []AgentConfig
		bindings []BindingConfig
		wantErr  bool
		errMsg   string
	}{
		{
			name:   "valid binding",
			agents: []AgentConfig{{ID: "agent-1", Model: "qianfan:test-model"}},
			bindings: []BindingConfig{
				{AgentID: "agent-1"},
			},
			wantErr: false,
		},
		{
			name:   "binding to non-existent agent",
			agents: []AgentConfig{{ID: "agent-1", Model: "qianfan:test-model"}},
			bindings: []BindingConfig{
				{AgentID: "agent-2"},
			},
			wantErr: true,
			errMsg:  "binding references non-existent agent: agent-2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(true)
			cfg := createValidConfig()
			cfg.Agents.List = tt.agents
			cfg.Bindings = tt.bindings

			err := v.validateAgents(cfg)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateProviders(t *testing.T) {
	tests := []struct {
		name      string
		providers map[string]ModelProviderConfig
		wantErr   bool
		errMsg    string
	}{
		{
			name: "valid providers",
			providers: map[string]ModelProviderConfig{
				"qianfan": {
					BaseURL: "https://qianfan.baidubce.com/v2",
					APIKey:  "valid-api-key-12345",
					Models: []ModelDefinitionConfig{
						{ID: "model-1", Name: "Model 1"},
					},
				},
			},
			wantErr: false,
		},
		{
			name:      "no providers configured",
			providers: map[string]ModelProviderConfig{},
			wantErr:   true,
			errMsg:    "at least one LLM provider must be configured in models.providers",
		},
		{
			name: "provider with empty base URL",
			providers: map[string]ModelProviderConfig{
				"qianfan": {
					BaseURL: "",
					APIKey:  "valid-api-key-12345",
					Models: []ModelDefinitionConfig{
						{ID: "model-1", Name: "Model 1"},
					},
				},
			},
			wantErr: true,
			errMsg:  "provider 'qianfan' has empty baseUrl",
		},
		{
			name: "provider with short API key",
			providers: map[string]ModelProviderConfig{
				"qianfan": {
					BaseURL: "https://qianfan.baidubce.com/v2",
					APIKey:  "short",
					Models: []ModelDefinitionConfig{
						{ID: "model-1", Name: "Model 1"},
					},
				},
			},
			wantErr: true,
			errMsg:  "API key too short",
		},
		{
			name: "provider with API key containing spaces",
			providers: map[string]ModelProviderConfig{
				"qianfan": {
					BaseURL: "https://qianfan.baidubce.com/v2",
					APIKey:  "api key with spaces",
					Models: []ModelDefinitionConfig{
						{ID: "model-1", Name: "Model 1"},
					},
				},
			},
			wantErr: true,
			errMsg:  "API key cannot contain spaces",
		},
		{
			name: "model with empty ID",
			providers: map[string]ModelProviderConfig{
				"qianfan": {
					BaseURL: "https://qianfan.baidubce.com/v2",
					APIKey:  "valid-api-key-12345",
					Models: []ModelDefinitionConfig{
						{ID: "", Name: "Model 1"},
					},
				},
			},
			wantErr: true,
			errMsg:  "model at index 0 has empty id",
		},
		{
			name: "model with empty name",
			providers: map[string]ModelProviderConfig{
				"qianfan": {
					BaseURL: "https://qianfan.baidubce.com/v2",
					APIKey:  "valid-api-key-12345",
					Models: []ModelDefinitionConfig{
						{ID: "model-1", Name: ""},
					},
				},
			},
			wantErr: true,
			errMsg:  "model 'model-1' has empty name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(true)
			cfg := createValidConfig()
			cfg.Models.Providers = tt.providers

			err := v.validateProviders(cfg)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateChannels(t *testing.T) {
	tests := []struct {
		name     string
		setupCfg func(*Config)
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "disabled channel",
			setupCfg: func(cfg *Config) {},
			wantErr:  false,
		},
		{
			name: "Telegram enabled without token",
			setupCfg: func(cfg *Config) {
				cfg.Channels.Telegram.Enabled = true
				cfg.Channels.Telegram.Token = ""
			},
			wantErr: true,
			errMsg:  "telegram token is required when enabled",
		},
		{
			name: "Telegram enabled with token",
			setupCfg: func(cfg *Config) {
				cfg.Channels.Telegram.Enabled = true
				cfg.Channels.Telegram.Token = "valid-token"
			},
			wantErr: false,
		},
		{
			name: "WhatsApp enabled without bridge URL",
			setupCfg: func(cfg *Config) {
				cfg.Channels.WhatsApp.Enabled = true
				cfg.Channels.WhatsApp.BridgeURL = ""
			},
			wantErr: true,
			errMsg:  "whatsapp bridge_url is required when enabled",
		},
		{
			name: "WhatsApp enabled with invalid URL",
			setupCfg: func(cfg *Config) {
				cfg.Channels.WhatsApp.Enabled = true
				cfg.Channels.WhatsApp.BridgeURL = "://invalid"
			},
			wantErr: true,
			errMsg:  "invalid whatsapp bridge_url",
		},
		{
			name: "Feishu enabled without app_id",
			setupCfg: func(cfg *Config) {
				cfg.Channels.Feishu.Enabled = true
				cfg.Channels.Feishu.AppID = ""
			},
			wantErr: true,
			errMsg:  "feishu app_id is required when enabled",
		},
		{
			name: "Feishu enabled without app_secret",
			setupCfg: func(cfg *Config) {
				cfg.Channels.Feishu.Enabled = true
				cfg.Channels.Feishu.AppID = "app-123"
				cfg.Channels.Feishu.AppSecret = ""
			},
			wantErr: true,
			errMsg:  "feishu app_secret is required when enabled",
		},
		{
			name: "Feishu with invalid webhook port",
			setupCfg: func(cfg *Config) {
				cfg.Channels.Feishu.Enabled = true
				cfg.Channels.Feishu.AppID = "app-123"
				cfg.Channels.Feishu.AppSecret = "secret-123"
				cfg.Channels.Feishu.WebhookPort = 80
			},
			wantErr: true,
			errMsg:  "feishu webhook_port must be between 1024 and 65535",
		},
		{
			name: "Feishu with valid webhook port",
			setupCfg: func(cfg *Config) {
				cfg.Channels.Feishu.Enabled = true
				cfg.Channels.Feishu.AppID = "app-123"
				cfg.Channels.Feishu.AppSecret = "secret-123"
				cfg.Channels.Feishu.WebhookPort = 8080
			},
			wantErr: false,
		},
		{
			name: "QQ enabled without app_id",
			setupCfg: func(cfg *Config) {
				cfg.Channels.QQ.Enabled = true
				cfg.Channels.QQ.AppID = ""
			},
			wantErr: true,
			errMsg:  "qq app_id is required when enabled",
		},
		{
			name: "QQ enabled without app_secret",
			setupCfg: func(cfg *Config) {
				cfg.Channels.QQ.Enabled = true
				cfg.Channels.QQ.AppID = "app-123"
				cfg.Channels.QQ.AppSecret = ""
			},
			wantErr: true,
			errMsg:  "qq app_secret is required when enabled",
		},
		{
			name: "WeWork enabled without corp_id",
			setupCfg: func(cfg *Config) {
				cfg.Channels.WeWork.Enabled = true
				cfg.Channels.WeWork.CorpID = ""
			},
			wantErr: true,
			errMsg:  "wework corp_id is required when enabled",
		},
		{
			name: "WeWork with invalid webhook port",
			setupCfg: func(cfg *Config) {
				cfg.Channels.WeWork.Enabled = true
				cfg.Channels.WeWork.CorpID = "corp-123"
				cfg.Channels.WeWork.Secret = "secret-123"
				cfg.Channels.WeWork.AgentID = "agent-123"
				cfg.Channels.WeWork.WebhookPort = 80
			},
			wantErr: true,
			errMsg:  "wework webhook_port must be between 1024 and 65535",
		},
		{
			name: "DingTalk enabled without client_id",
			setupCfg: func(cfg *Config) {
				cfg.Channels.DingTalk.Enabled = true
				cfg.Channels.DingTalk.ClientID = ""
			},
			wantErr: true,
			errMsg:  "dingtalk client_id is required when enabled",
		},
		{
			name: "Infoflow enabled without webhook URL",
			setupCfg: func(cfg *Config) {
				cfg.Channels.Infoflow.Enabled = true
				cfg.Channels.Infoflow.WebhookURL = ""
			},
			wantErr: true,
			errMsg:  "infoflow webhook_url is required when enabled",
		},
		{
			name: "Infoflow with invalid webhook URL",
			setupCfg: func(cfg *Config) {
				cfg.Channels.Infoflow.Enabled = true
				cfg.Channels.Infoflow.WebhookURL = "://invalid"
			},
			wantErr: true,
			errMsg:  "invalid infoflow webhook_url",
		},
		{
			name: "Infoflow with invalid webhook port",
			setupCfg: func(cfg *Config) {
				cfg.Channels.Infoflow.Enabled = true
				cfg.Channels.Infoflow.WebhookURL = "http://example.com"
				cfg.Channels.Infoflow.WebhookPort = 80
			},
			wantErr: true,
			errMsg:  "infoflow webhook_port must be between 1024 and 65535",
		},
		{
			name: "WeWork enabled without secret",
			setupCfg: func(cfg *Config) {
				cfg.Channels.WeWork.Enabled = true
				cfg.Channels.WeWork.CorpID = "corp-123"
				cfg.Channels.WeWork.Secret = ""
			},
			wantErr: true,
			errMsg:  "wework secret is required when enabled",
		},
		{
			name: "WeWork enabled without agent_id",
			setupCfg: func(cfg *Config) {
				cfg.Channels.WeWork.Enabled = true
				cfg.Channels.WeWork.CorpID = "corp-123"
				cfg.Channels.WeWork.Secret = "secret-123"
				cfg.Channels.WeWork.AgentID = ""
			},
			wantErr: true,
			errMsg:  "wework agent_id is required when enabled",
		},
		{
			name: "DingTalk enabled without client_secret",
			setupCfg: func(cfg *Config) {
				cfg.Channels.DingTalk.Enabled = true
				cfg.Channels.DingTalk.ClientID = "client-123"
				cfg.Channels.DingTalk.ClientSecret = ""
			},
			wantErr: true,
			errMsg:  "dingtalk client_secret is required when enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(true)
			cfg := createValidConfig()
			tt.setupCfg(cfg)

			err := v.validateChannels(cfg)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateTools(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*Config)
		wantErr bool
		errMsg  string
	}{
		{
			name:    "disabled shell tool",
			setup:   func(cfg *Config) {},
			wantErr: false,
		},
		{
			name: "shell tool with invalid timeout",
			setup: func(cfg *Config) {
				cfg.Tools.Shell.Enabled = true
				cfg.Tools.Shell.Timeout = 0
				cfg.Tools.Shell.DeniedCmds = []string{"rm -rf", "dd", "mkfs"}
			},
			wantErr: true,
			errMsg:  "shell timeout must be between 1 and 3600 seconds",
		},
		{
			name: "shell tool missing dangerous commands",
			setup: func(cfg *Config) {
				cfg.Tools.Shell.Enabled = true
				cfg.Tools.Shell.Timeout = 30
				cfg.Tools.Shell.DeniedCmds = []string{"rm -rf"}
			},
			wantErr: true,
			errMsg:  "dangerous command 'dd' should be in denied_cmds list",
		},
		{
			name: "shell tool with sandbox enabled but no image",
			setup: func(cfg *Config) {
				cfg.Tools.Shell.Enabled = true
				cfg.Tools.Shell.Timeout = 30
				cfg.Tools.Shell.DeniedCmds = []string{"rm -rf", "dd", "mkfs"}
				cfg.Tools.Shell.Sandbox.Enabled = true
				cfg.Tools.Shell.Sandbox.Image = ""
			},
			wantErr: true,
			errMsg:  "sandbox image is required when enabled",
		},
		{
			name: "browser tool with invalid timeout",
			setup: func(cfg *Config) {
				cfg.Tools.Browser.Enabled = true
				cfg.Tools.Browser.Timeout = 0
			},
			wantErr: true,
			errMsg:  "browser timeout must be between 1 and 600 seconds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(true)
			cfg := createValidConfig()
			tt.setup(cfg)

			err := v.validateTools(cfg)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateMemory(t *testing.T) {
	tests := []struct {
		name    string
		backend string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "builtin backend",
			backend: "builtin",
			wantErr: false,
		},
		{
			name:    "qmd backend",
			backend: "qmd",
			wantErr: false,
		},
		{
			name:    "empty backend",
			backend: "",
			wantErr: true,
			errMsg:  "memory backend cannot be empty",
		},
		{
			name:    "invalid backend",
			backend: "invalid",
			wantErr: true,
			errMsg:  "invalid memory backend: invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(true)
			cfg := createValidConfig()
			cfg.Memory.Backend = tt.backend

			err := v.validateMemory(cfg)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateAPIKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid API key",
			key:     "valid-api-key-12345",
			wantErr: false,
		},
		{
			name:    "short API key",
			key:     "short",
			wantErr: true,
			errMsg:  "API key too short (minimum 10 characters)",
		},
		{
			name:    "API key with spaces",
			key:     "api key with spaces",
			wantErr: true,
			errMsg:  "API key cannot contain spaces",
		},
		{
			name:    "empty API key",
			key:     "",
			wantErr: true,
			errMsg:  "API key too short (minimum 10 characters)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(true)
			err := v.validateAPIKey(tt.key)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateWebSocketConfig(t *testing.T) {
	tests := []struct {
		name    string
		ws      WebSocketConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid WebSocket config",
			ws: WebSocketConfig{
				Host:         "localhost",
				Port:         8080,
				PingInterval: 30 * time.Second,
				PongTimeout:  10 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "no host configured",
			ws: WebSocketConfig{
				Host: "",
			},
			wantErr: false,
		},
		{
			name: "ping interval at upper boundary",
			ws: WebSocketConfig{
				Host:         "localhost",
				PingInterval: 5 * time.Minute,
			},
			wantErr: false,
		},
		{
			name: "ping interval exceeds maximum",
			ws: WebSocketConfig{
				Host:         "localhost",
				PingInterval: 6 * time.Minute,
			},
			wantErr: true,
			errMsg:  "websocket ping_interval must be between 0 and 5m",
		},
		{
			name: "ping interval negative",
			ws: WebSocketConfig{
				Host:         "localhost",
				PingInterval: -1 * time.Second,
			},
			wantErr: true,
			errMsg:  "websocket ping_interval must be between 0 and 5m",
		},
		{
			name: "pong timeout at upper boundary",
			ws: WebSocketConfig{
				Host:        "localhost",
				PongTimeout: 5 * time.Minute,
			},
			wantErr: false,
		},
		{
			name: "pong timeout exceeds maximum",
			ws: WebSocketConfig{
				Host:        "localhost",
				PongTimeout: 6 * time.Minute,
			},
			wantErr: true,
			errMsg:  "websocket pong_timeout must be between 0 and 5m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(true)
			err := v.validateWebSocketConfig(&tt.ws)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateWorkspace(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "empty path (valid)",
			path:    "",
			wantErr: false,
		},
		{
			name:    "relative path (invalid)",
			path:    "relative/path",
			wantErr: true,
			errMsg:  "workspace path must be absolute",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(true)
			cfg := createValidConfig()
			cfg.Workspace.Path = tt.path

			err := v.validateWorkspace(cfg)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_Integration(t *testing.T) {
	t.Run("complete valid configuration", func(t *testing.T) {
		v := NewValidator(true)
		cfg := createValidConfig()

		err := v.Validate(cfg)
		require.NoError(t, err)
	})

	t.Run("configuration with all components", func(t *testing.T) {
		v := NewValidator(true)
		cfg := createValidConfig()

		cfg.Agents.Defaults.Retry = &RetryConfig{
			MaxRetries:    3,
			InitialDelay:  2 * time.Second,
			MaxDelay:      60 * time.Second,
			BackoffFactor: 2.0,
		}

		cfg.Agents.Defaults.Subagents = &SubagentsConfig{
			MaxConcurrent:       5,
			ArchiveAfterMinutes: 60,
			TimeoutSeconds:      300,
		}

		cfg.Agents.List = []AgentConfig{
			{
				ID:    "agent-1",
				Model: "qianfan:test-model",
				Subagents: &AgentSubagentConfig{
					TimeoutSeconds: 60,
					AllowTools:     []string{"tool1"},
					DenyTools:      []string{"tool2"},
				},
			},
		}

		cfg.Gateway.WebSocket = WebSocketConfig{
			Host:         "localhost",
			Port:         8080,
			EnableAuth:   true,
			AuthToken:    "valid-token",
			PingInterval: 30 * time.Second,
			PongTimeout:  10 * time.Second,
		}

		err := v.Validate(cfg)
		require.NoError(t, err)
	})

	t.Run("configuration with multiple errors", func(t *testing.T) {
		v := NewValidator(true)
		cfg := createValidConfig()

		cfg.Agents.Defaults.MaxIterations = 0
		cfg.Models.Mode = "invalid"
		cfg.Memory.Backend = "invalid"

		err := v.Validate(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "max_iterations must be between 1 and 100")
	})
}

func createValidConfig() *Config {
	return &Config{
		Workspace: WorkspaceConfig{
			Path: "",
		},
		Agents: AgentsConfig{
			Defaults: AgentDefaults{
				Model: ModelSelection{
					Primary: "qianfan:test-model",
				},
				MaxIterations: 10,
				Temperature:   1.0,
				MaxTokens:     4096,
			},
			List: []AgentConfig{},
		},
		Models: ModelsConfig{
			Mode: "merge",
			Providers: map[string]ModelProviderConfig{
				"qianfan": {
					BaseURL: "https://qianfan.baidubce.com/v2",
					APIKey:  "test-valid-api-key-12345",
					API:     ModelAPIOpenAICompletions,
					Models: []ModelDefinitionConfig{
						{
							ID:            "test-model",
							Name:          "Test Model",
							ContextWindow: 128000,
							MaxTokens:     8192,
							Input:         []string{"text", "image"},
						},
					},
				},
			},
		},
		Tools: ToolsConfig{
			Web: WebToolConfig{
				Timeout: 30,
			},
			Shell: ShellToolConfig{
				Enabled: false,
			},
			Browser: BrowserToolConfig{
				Enabled: false,
			},
		},
		Gateway: GatewayConfig{
			Port:         8080,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		},
		Memory: MemoryConfig{
			Backend: "builtin",
		},
		Bindings: []BindingConfig{},
	}
}
