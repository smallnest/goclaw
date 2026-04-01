package config

import (
	"testing"
)

func TestValidatorValidConfig(t *testing.T) {
	validator := NewValidator(true)

	cfg := &Config{
		Workspace: WorkspaceConfig{
			Path: "/tmp/test-workspace",
		},
		Agents: AgentsConfig{
			Defaults: AgentDefaults{
				Model: ModelSelection{
					Primary: "qianfan:test-model",
				},
				MaxIterations: 11,
				Temperature:   1.7,
				MaxTokens:     4096,
			},
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
		},
		Gateway: GatewayConfig{
			Port:         8080,
			ReadTimeout:  30,
			WriteTimeout: 30,
		},
		Memory: MemoryConfig{
			Backend: "builtin",
		},
	}

	err := validator.Validate(cfg)
	if err != nil {
		t.Fatalf("Expected valid config to pass, got error: %v", err)
	}
}
