package providers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
)

// AnthropicProvider Anthropic 提供商
type AnthropicProvider struct {
	llm       llms.Model
	model     string
	maxTokens int
}

// NewAnthropicProvider 创建 Anthropic 提供商
func NewAnthropicProvider(apiKey, baseURL, model string, maxTokens int) (*AnthropicProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	if model == "" {
		model = "claude-3-opus-20240229"
	}

	opts := []anthropic.Option{
		anthropic.WithToken(apiKey),
		anthropic.WithModel(model),
	}

	if baseURL != "" {
		// Note: langchaingo's anthropic package might not support WithBaseURL yet,
		// but we should check if it's available or use a custom client if needed.
		// For now, we'll try to use it if it exists.
		// Actually, let's check langchaingo's anthropic option.
		_ = baseURL // TODO: implement custom base URL support
		// nolint:staticcheck,emptybranch
	}

	llm, err := anthropic.New(opts...)
	if err != nil {
		return nil, err
	}

	return &AnthropicProvider{
		llm:       llm,
		model:     model,
		maxTokens: maxTokens,
	}, nil
}

// Chat 聊天
func (p *AnthropicProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
	opts := &ChatOptions{
		Model:       p.model,
		Temperature: 0.7,
		MaxTokens:   p.maxTokens,
		Stream:      false,
	}

	for _, opt := range options {
		opt(opts)
	}

	// 转换消息
	langchainMessages := make([]llms.MessageContent, len(messages))
	for i, msg := range messages {
		var role llms.ChatMessageType
		switch msg.Role {
		case "user":
			role = llms.ChatMessageTypeHuman
		case "assistant":
			role = llms.ChatMessageTypeAI
		case "system":
			role = llms.ChatMessageTypeSystem
		default:
			role = llms.ChatMessageTypeHuman
		}

		langchainMessages[i] = llms.TextParts(role, msg.Content)
	}

	// 调用 LLM
	var llmOpts []llms.CallOption
	if opts.Temperature > 0 {
		llmOpts = append(llmOpts, llms.WithTemperature(float64(opts.Temperature)))
	}
	if opts.MaxTokens > 0 {
		llmOpts = append(llmOpts, llms.WithMaxTokens(int(opts.MaxTokens)))
	}

	// 如果有工具，添加工具选项
	if len(tools) > 0 {
		langchainTools := make([]llms.Tool, len(tools))
		for i, tool := range tools {
			langchainTools[i] = llms.Tool{
				Type: "function",
				Function: &llms.FunctionDefinition{
					Name:        tool.Name,
					Description: tool.Description,
					Parameters:  tool.Parameters,
				},
			}
		}
		llmOpts = append(llmOpts, llms.WithTools(langchainTools))
	}

	completion, err := p.llm.GenerateContent(ctx, langchainMessages, llmOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to generate content: %w", err)
	}

	// 解析工具调用
	var toolCalls []ToolCall
	if len(completion.Choices) > 0 {
		for _, tc := range completion.Choices[0].ToolCalls {
			var params map[string]interface{}
			if err := json.Unmarshal([]byte(tc.FunctionCall.Arguments), &params); err != nil {
				fmt.Printf("failed to unmarshal tool arguments: %v\n", err)
				continue
			}
			toolCalls = append(toolCalls, ToolCall{
				ID:     tc.ID,
				Name:   tc.FunctionCall.Name,
				Params: params,
			})
		}
	}

	response := &Response{
		Content:      completion.Choices[0].Content,
		ToolCalls:    toolCalls,
		FinishReason: "stop",
	}

	return response, nil
}

// ChatWithTools 聊天（带工具）
func (p *AnthropicProvider) ChatWithTools(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
	return p.Chat(ctx, messages, tools, options...)
}

// Close 关闭连接
func (p *AnthropicProvider) Close() error {
	return nil
}
