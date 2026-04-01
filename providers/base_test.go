package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"
)

func TestMessage_Structure(t *testing.T) {
	tests := []struct {
		name    string
		message Message
	}{
		{
			name: "user message",
			message: Message{
				Role:    "user",
				Content: "Hello",
			},
		},
		{
			name: "assistant message with tool calls",
			message: Message{
				Role:    "assistant",
				Content: "",
				ToolCalls: []ToolCall{
					{ID: "call1", Name: "search", Params: map[string]interface{}{"query": "test"}},
				},
			},
		},
		{
			name: "tool message",
			message: Message{
				Role:       "tool",
				Content:    `{"result": "success"}`,
				ToolCallID: "call1",
				ToolName:   "search",
			},
		},
		{
			name: "system message",
			message: Message{
				Role:    "system",
				Content: "You are a helpful assistant",
			},
		},
		{
			name: "message with images",
			message: Message{
				Role:    "user",
				Content: "What's in this image?",
				Images:  []string{"https://example.com/image.jpg", "data:image/png;base64,abc123"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEmpty(t, tt.message.Role)
		})
	}
}

func TestToolCall_Structure(t *testing.T) {
	toolCall := ToolCall{
		ID:       "call_123",
		Name:     "get_weather",
		Params:   map[string]interface{}{"location": "Beijing"},
		Response: `{"temp": 25, "condition": "sunny"}`,
	}

	assert.Equal(t, "call_123", toolCall.ID)
	assert.Equal(t, "get_weather", toolCall.Name)
	assert.Equal(t, "Beijing", toolCall.Params["location"])
	assert.Equal(t, `{"temp": 25, "condition": "sunny"}`, toolCall.Response)
}

func TestResponse_Structure(t *testing.T) {
	tests := []struct {
		name     string
		response Response
	}{
		{
			name: "simple response",
			response: Response{
				Content:      "Hello, how can I help?",
				FinishReason: "stop",
				Usage: Usage{
					PromptTokens:     10,
					CompletionTokens: 20,
					TotalTokens:      30,
				},
			},
		},
		{
			name: "response with tool calls",
			response: Response{
				Content: "",
				ToolCalls: []ToolCall{
					{ID: "call1", Name: "search", Params: map[string]interface{}{"q": "test"}},
				},
				FinishReason: "tool_use",
				Usage: Usage{
					PromptTokens:     15,
					CompletionTokens: 10,
					TotalTokens:      25,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEmpty(t, tt.response.FinishReason)
			assert.GreaterOrEqual(t, tt.response.Usage.TotalTokens, 0)
		})
	}
}

func TestUsage_Structure(t *testing.T) {
	usage := Usage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}

	assert.Equal(t, 100, usage.PromptTokens)
	assert.Equal(t, 50, usage.CompletionTokens)
	assert.Equal(t, 150, usage.TotalTokens)
}

func TestToolDefinition_Structure(t *testing.T) {
	tool := ToolDefinition{
		Name:        "get_weather",
		Description: "Get current weather for a location",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"location": map[string]interface{}{
					"type":        "string",
					"description": "City name",
				},
			},
			"required": []string{"location"},
		},
	}

	assert.Equal(t, "get_weather", tool.Name)
	assert.Equal(t, "Get current weather for a location", tool.Description)
	assert.NotNil(t, tool.Parameters)
}

func TestChatOptions_Default(t *testing.T) {
	opts := ChatOptions{}

	assert.Empty(t, opts.Model)
	assert.Equal(t, float64(0), opts.Temperature)
	assert.Equal(t, 0, opts.MaxTokens)
	assert.Equal(t, false, opts.Stream)
}

func TestWithModel(t *testing.T) {
	tests := []struct {
		name          string
		model         string
		expectedModel string
	}{
		{"gpt-4", "gpt-4", "gpt-4"},
		{"gpt-3.5-turbo", "gpt-3.5-turbo", "gpt-3.5-turbo"},
		{"claude-3", "claude-3-opus", "claude-3-opus"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &ChatOptions{}
			option := WithModel(tt.model)
			option(opts)

			assert.Equal(t, tt.expectedModel, opts.Model)
		})
	}
}

func TestWithTemperature(t *testing.T) {
	tests := []struct {
		name         string
		temperature  float64
		expectedTemp float64
	}{
		{"zero", 0.0, 0.0},
		{"low", 0.3, 0.3},
		{"medium", 0.7, 0.7},
		{"high", 1.0, 1.0},
		{"above one", 1.5, 1.5},
		{"negative", -0.5, -0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &ChatOptions{}
			option := WithTemperature(tt.temperature)
			option(opts)

			assert.Equal(t, tt.expectedTemp, opts.Temperature)
		})
	}
}

func TestWithMaxTokens(t *testing.T) {
	tests := []struct {
		name           string
		maxTokens      int
		expectedTokens int
	}{
		{"zero", 0, 0},
		{"small", 100, 100},
		{"medium", 2048, 2048},
		{"large", 8192, 8192},
		{"very large", 32000, 32000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &ChatOptions{}
			option := WithMaxTokens(tt.maxTokens)
			option(opts)

			assert.Equal(t, tt.expectedTokens, opts.MaxTokens)
		})
	}
}

func TestWithStream(t *testing.T) {
	tests := []struct {
		name           string
		stream         bool
		expectedStream bool
	}{
		{"enabled", true, true},
		{"disabled", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &ChatOptions{}
			option := WithStream(tt.stream)
			option(opts)

			assert.Equal(t, tt.expectedStream, opts.Stream)
		})
	}
}

func TestChatOptions_MultipleOptions(t *testing.T) {
	opts := &ChatOptions{}
	options := []ChatOption{
		WithModel("gpt-4"),
		WithTemperature(0.8),
		WithMaxTokens(4096),
		WithStream(true),
	}

	for _, option := range options {
		option(opts)
	}

	assert.Equal(t, "gpt-4", opts.Model)
	assert.Equal(t, 0.8, opts.Temperature)
	assert.Equal(t, 4096, opts.MaxTokens)
	assert.Equal(t, true, opts.Stream)
}

func TestChatOptions_OverrideOptions(t *testing.T) {
	opts := &ChatOptions{}

	WithModel("gpt-3.5-turbo")(opts)
	WithModel("gpt-4")(opts)

	assert.Equal(t, "gpt-4", opts.Model)
}

func TestConvertToLangChainMessages_BasicMessages(t *testing.T) {
	tests := []struct {
		name          string
		messages      []Message
		expectedRoles []llms.ChatMessageType
		expectedCount int
	}{
		{
			name: "user message",
			messages: []Message{
				{Role: "user", Content: "Hello"},
			},
			expectedRoles: []llms.ChatMessageType{llms.ChatMessageTypeHuman},
			expectedCount: 1,
		},
		{
			name: "assistant message",
			messages: []Message{
				{Role: "assistant", Content: "Hi there!"},
			},
			expectedRoles: []llms.ChatMessageType{llms.ChatMessageTypeAI},
			expectedCount: 1,
		},
		{
			name: "system message",
			messages: []Message{
				{Role: "system", Content: "You are helpful"},
			},
			expectedRoles: []llms.ChatMessageType{llms.ChatMessageTypeSystem},
			expectedCount: 1,
		},
		{
			name: "unknown role defaults to human",
			messages: []Message{
				{Role: "unknown", Content: "Test"},
			},
			expectedRoles: []llms.ChatMessageType{llms.ChatMessageTypeHuman},
			expectedCount: 1,
		},
		{
			name: "mixed messages",
			messages: []Message{
				{Role: "system", Content: "Be helpful"},
				{Role: "user", Content: "Hi"},
				{Role: "assistant", Content: "Hello!"},
				{Role: "user", Content: "Thanks"},
			},
			expectedRoles: []llms.ChatMessageType{
				llms.ChatMessageTypeSystem,
				llms.ChatMessageTypeHuman,
				llms.ChatMessageTypeAI,
				llms.ChatMessageTypeHuman,
			},
			expectedCount: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertToLangChainMessages(tt.messages)

			require.Len(t, result, tt.expectedCount)
			for i, msg := range result {
				assert.Equal(t, tt.expectedRoles[i], msg.Role)
			}
		})
	}
}

func TestConvertToLangChainMessages_WithImages(t *testing.T) {
	messages := []Message{
		{
			Role:    "user",
			Content: "What's in this image?",
			Images:  []string{"https://example.com/image.jpg"},
		},
	}

	result := ConvertToLangChainMessages(messages)

	require.Len(t, result, 1)
	assert.Equal(t, llms.ChatMessageTypeHuman, result[0].Role)
	assert.Len(t, result[0].Parts, 2)
}

func TestConvertToLangChainMessages_MultipleImages(t *testing.T) {
	messages := []Message{
		{
			Role:    "user",
			Content: "Compare these images",
			Images:  []string{"https://example.com/img1.jpg", "https://example.com/img2.jpg"},
		},
	}

	result := ConvertToLangChainMessages(messages)

	require.Len(t, result, 1)
	assert.Len(t, result[0].Parts, 3)
}

func TestConvertToLangChainMessages_EmptyMessages(t *testing.T) {
	result := ConvertToLangChainMessages([]Message{})

	require.Len(t, result, 0)
}

func TestConvertToLangChainMessages_ContentPreservation(t *testing.T) {
	tests := []struct {
		name     string
		messages []Message
	}{
		{
			name:     "simple content",
			messages: []Message{{Role: "user", Content: "Hello, world!"}},
		},
		{
			name:     "multiline content",
			messages: []Message{{Role: "user", Content: "Line 1\nLine 2\nLine 3"}},
		},
		{
			name:     "special characters",
			messages: []Message{{Role: "user", Content: "Special: <>&\"'"}},
		},
		{
			name:     "unicode content",
			messages: []Message{{Role: "user", Content: "你好世界 🌍"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertToLangChainMessages(tt.messages)
			require.Len(t, result, 1)

			textParts := 0
			for _, part := range result[0].Parts {
				if textPart, ok := part.(llms.TextContent); ok {
					assert.Equal(t, tt.messages[0].Content, textPart.Text)
					textParts++
				}
			}
			assert.Equal(t, 1, textParts)
		})
	}
}

func TestConvertToLangChainTools_BasicTools(t *testing.T) {
	tools := []ToolDefinition{
		{
			Name:        "search",
			Description: "Search the web",
			Parameters: map[string]interface{}{
				"type": "object",
			},
		},
	}

	result := ConvertToLangChainTools(tools)

	require.Len(t, result, 1)
	assert.Equal(t, "function", result[0].Type)
	assert.Equal(t, "search", result[0].Function.Name)
	assert.Equal(t, "Search the web", result[0].Function.Description)
	assert.NotNil(t, result[0].Function.Parameters)
}

func TestConvertToLangChainTools_MultipleTools(t *testing.T) {
	tools := []ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get weather info",
			Parameters:  map[string]interface{}{"type": "object"},
		},
		{
			Name:        "send_email",
			Description: "Send an email",
			Parameters:  map[string]interface{}{"type": "object"},
		},
		{
			Name:        "translate",
			Description: "Translate text",
			Parameters:  map[string]interface{}{"type": "object"},
		},
	}

	result := ConvertToLangChainTools(tools)

	require.Len(t, result, 3)
	names := []string{"get_weather", "send_email", "translate"}
	for i, tool := range result {
		assert.Equal(t, "function", tool.Type)
		assert.Equal(t, names[i], tool.Function.Name)
	}
}

func TestConvertToLangChainTools_EmptyTools(t *testing.T) {
	result := ConvertToLangChainTools([]ToolDefinition{})

	require.Len(t, result, 0)
}

func TestConvertToLangChainTools_ComplexParameters(t *testing.T) {
	tools := []ToolDefinition{
		{
			Name:        "complex_tool",
			Description: "A tool with complex parameters",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"string_param": map[string]interface{}{
						"type":        "string",
						"description": "A string parameter",
					},
					"number_param": map[string]interface{}{
						"type":        "number",
						"description": "A number parameter",
					},
					"nested": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"inner": map[string]interface{}{
								"type": "string",
							},
						},
					},
				},
				"required": []string{"string_param"},
			},
		},
	}

	result := ConvertToLangChainTools(tools)

	require.Len(t, result, 1)
	assert.NotNil(t, result[0].Function.Parameters)

	params, ok := result[0].Function.Parameters.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "object", params["type"])
	assert.NotNil(t, params["properties"])
}

func TestConvertToLangChainTools_NilParameters(t *testing.T) {
	tools := []ToolDefinition{
		{
			Name:        "no_params",
			Description: "Tool without parameters",
			Parameters:  nil,
		},
	}

	result := ConvertToLangChainTools(tools)

	require.Len(t, result, 1)
	assert.Nil(t, result[0].Function.Parameters)
}

func TestMessage_JSONTags(t *testing.T) {
	message := Message{
		Role:       "tool",
		Content:    "result",
		Images:     []string{"img1"},
		ToolCallID: "call123",
		ToolName:   "test_tool",
		ToolCalls: []ToolCall{
			{ID: "tc1", Name: "tool1", Params: map[string]interface{}{"k": "v"}},
		},
	}

	assert.Equal(t, "tool", message.Role)
	assert.Equal(t, "result", message.Content)
	assert.Len(t, message.Images, 1)
	assert.Equal(t, "call123", message.ToolCallID)
	assert.Equal(t, "test_tool", message.ToolName)
	assert.Len(t, message.ToolCalls, 1)
}

func TestToolCall_JSONTags(t *testing.T) {
	toolCall := ToolCall{
		ID:       "id123",
		Name:     "tool_name",
		Params:   map[string]interface{}{"key": "value"},
		Response: "response_data",
	}

	assert.Equal(t, "id123", toolCall.ID)
	assert.Equal(t, "tool_name", toolCall.Name)
	assert.Equal(t, "value", toolCall.Params["key"])
	assert.Equal(t, "response_data", toolCall.Response)
}

func TestResponse_JSONTags(t *testing.T) {
	response := Response{
		Content:      "response content",
		ToolCalls:    []ToolCall{{ID: "1", Name: "t"}},
		FinishReason: "stop",
		Usage:        Usage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30},
	}

	assert.Equal(t, "response content", response.Content)
	assert.Len(t, response.ToolCalls, 1)
	assert.Equal(t, "stop", response.FinishReason)
	assert.Equal(t, 10, response.Usage.PromptTokens)
}

func TestUsage_JSONTags(t *testing.T) {
	usage := Usage{
		PromptTokens:     100,
		CompletionTokens: 200,
		TotalTokens:      300,
	}

	assert.Equal(t, 100, usage.PromptTokens)
	assert.Equal(t, 200, usage.CompletionTokens)
	assert.Equal(t, 300, usage.TotalTokens)
}

func TestToolDefinition_JSONTags(t *testing.T) {
	tool := ToolDefinition{
		Name:        "tool_name",
		Description: "tool description",
		Parameters:  map[string]interface{}{"type": "object"},
	}

	assert.Equal(t, "tool_name", tool.Name)
	assert.Equal(t, "tool description", tool.Description)
	assert.NotNil(t, tool.Parameters)
}

func TestChatOptions_JSONTags(t *testing.T) {
	opts := ChatOptions{
		Model:       "gpt-4",
		Temperature: 0.7,
		MaxTokens:   4096,
		Stream:      true,
	}

	assert.Equal(t, "gpt-4", opts.Model)
	assert.Equal(t, 0.7, opts.Temperature)
	assert.Equal(t, 4096, opts.MaxTokens)
	assert.True(t, opts.Stream)
}

func TestConvertToLangChainMessages_ToolRole(t *testing.T) {
	messages := []Message{
		{
			Role:       "tool",
			Content:    `{"result": "success"}`,
			ToolCallID: "call_123",
			ToolName:   "get_data",
		},
	}

	result := ConvertToLangChainMessages(messages)

	require.Len(t, result, 1)
	assert.Equal(t, llms.ChatMessageTypeHuman, result[0].Role)
}

func TestConvertToLangChainMessages_Order(t *testing.T) {
	messages := []Message{
		{Role: "system", Content: "1"},
		{Role: "user", Content: "2"},
		{Role: "assistant", Content: "3"},
		{Role: "user", Content: "4"},
	}

	result := ConvertToLangChainMessages(messages)

	require.Len(t, result, 4)
	assert.Equal(t, llms.ChatMessageTypeSystem, result[0].Role)
	assert.Equal(t, llms.ChatMessageTypeHuman, result[1].Role)
	assert.Equal(t, llms.ChatMessageTypeAI, result[2].Role)
	assert.Equal(t, llms.ChatMessageTypeHuman, result[3].Role)
}
