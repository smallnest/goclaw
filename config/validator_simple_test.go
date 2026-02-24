package config

import (
	"testing"
	"time"

	"github.com/smallnest/goclaw/errors"
)

func TestValidatorValidConfig(t *testing.T) {
	validator := NewValidator(true)

	cfg := &Config{
		Workspace: WorkspaceConfig{
			Path: "/tmp/test-workspace",
		},
		Agents: AgentsConfig{
			Defaults: AgentDefaults{
				Model:         "test-model",
				MaxIterations: 10,
				Temperature:   0.7,
				MaxTokens:     2048,
			},
		},
		Providers: ProvidersConfig{
			OpenAI: OpenAIProviderConfig{
				APIKey: "sk-test-valid-api-key-12345",
			},
		},
		Gateway: GatewayConfig{
			Port:         8080,
			ReadTimeout:  30,
			WriteTimeout: 30,
			WebSocket: WebSocketConfig{
				Host:         "localhost",
				Port:         8081,
				PingInterval: 30 * time.Second,
				PongTimeout:  30 * time.Second,
				ReadTimeout:  30 * time.Second,
				WriteTimeout: 30 * time.Second,
			},
		},
		Tools: ToolsConfig{
			Web: WebToolConfig{
				Timeout: 10,
			},
		},
		Memory: MemoryConfig{
			Backend: "builtin",
		},
	}

	if err := validator.Validate(cfg); err != nil {
		t.Errorf("expected valid config, got error: %v", err)
	}
}

func TestValidatorInvalidModel(t *testing.T) {
	validator := NewValidator(true)

	cfg := &Config{
		Agents: AgentsConfig{
			Defaults: AgentDefaults{
				Model: "", // Invalid
			},
		},
		Memory: MemoryConfig{
			Backend: "builtin",
		},
	}

	err := validator.Validate(cfg)
	if err == nil {
		t.Error("expected error for empty model")
	}
	if !errors.Is(err, errors.ErrCodeInvalidConfig) {
		t.Errorf("expected ErrCodeInvalidConfig, got: %v", errors.GetCode(err))
	}
}

func TestValidatorInvalidTemperature(t *testing.T) {
	validator := NewValidator(true)

	cfg := &Config{
		Agents: AgentsConfig{
			Defaults: AgentDefaults{
				Model:       "test-model",
				Temperature: 3.0, // Invalid > 2
			},
		},
		Providers: ProvidersConfig{
			OpenAI: OpenAIProviderConfig{
				APIKey: "sk-test-valid-api-key-12345",
			},
		},
		Memory: MemoryConfig{
			Backend: "builtin",
		},
	}

	err := validator.Validate(cfg)
	if err == nil {
		t.Error("expected error for invalid temperature")
	}
}

func TestValidatorInvalidAPIKey(t *testing.T) {
	validator := NewValidator(true)

	cfg := &Config{
		Agents: AgentsConfig{
			Defaults: AgentDefaults{
				Model:         "test-model",
				MaxIterations: 10,
				Temperature:   0.7,
				MaxTokens:     2048,
			},
		},
		Providers: ProvidersConfig{
			OpenAI: OpenAIProviderConfig{
				APIKey: "short", // Invalid
			},
		},
		Memory: MemoryConfig{
			Backend: "builtin",
		},
	}

	err := validator.Validate(cfg)
	if err == nil {
		t.Error("expected error for short API key")
	}
}

func TestValidatorMissingProvider(t *testing.T) {
	validator := NewValidator(true)

	cfg := &Config{
		Agents: AgentsConfig{
			Defaults: AgentDefaults{
				Model:         "test-model",
				MaxIterations: 10,
				Temperature:   0.7,
				MaxTokens:     2048,
			},
		},
		Providers: ProvidersConfig{
			// No provider configured
		},
		Memory: MemoryConfig{
			Backend: "builtin",
		},
	}

	err := validator.Validate(cfg)
	if err == nil {
		t.Error("expected error when no provider is configured")
	}
}
