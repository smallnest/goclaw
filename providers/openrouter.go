package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/smallnest/goclaw/internal/logger"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	"go.uber.org/zap"
)

// OpenRouterProvider OpenRouter 提供商
type OpenRouterProvider struct {
	llm       llms.Model
	model     string
	maxTokens int
	timeout   time.Duration
}

// NewOpenRouterProvider 创建 OpenRouter 提供商
func NewOpenRouterProvider(apiKey, baseURL, model string, maxTokens int) (*OpenRouterProvider, error) {
	return NewOpenRouterProviderWithTimeout(apiKey, baseURL, model, maxTokens, 0)
}

// NewOpenRouterProviderWithTimeout 创建带超时的 OpenRouter 提供商
func NewOpenRouterProviderWithTimeout(apiKey, baseURL, model string, maxTokens int, timeout time.Duration) (*OpenRouterProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	if model == "" {
		model = "anthropic/claude-opus-4-5"
	}

	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}

	opts := []openai.Option{
		openai.WithToken(apiKey),
		openai.WithModel(model),
		openai.WithBaseURL(baseURL),
	}

	// 设置超时
	if timeout > 0 {
		httpClient := &http.Client{
			Timeout: timeout,
		}
		opts = append(opts, openai.WithHTTPClient(httpClient))
		logger.Info("OpenRouter provider configured with timeout",
			zap.Duration("timeout", timeout))
	}

	llm, err := openai.New(opts...)
	if err != nil {
		return nil, err
	}

	return &OpenRouterProvider{
		llm:       llm,
		model:     model,
		maxTokens: maxTokens,
		timeout:   timeout,
	}, nil
}

// Chat 聊天
func (p *OpenRouterProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
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
		case "tool":
			role = llms.ChatMessageTypeTool
		default:
			role = llms.ChatMessageTypeHuman
		}

		// Handle tool result messages
		if msg.Role == "tool" {
			langchainMessages[i] = llms.MessageContent{
				Role: role,
				Parts: []llms.ContentPart{
					llms.ToolCallResponse{
						ToolCallID: msg.ToolCallID,
						Name:       msg.ToolName,
						Content:    msg.Content,
					},
				},
			}
		} else if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			// Handle assistant messages with tool calls
			parts := []llms.ContentPart{
				llms.TextPart(msg.Content),
			}
			for _, tc := range msg.ToolCalls {
				args, _ := json.Marshal(tc.Params)
				parts = append(parts, llms.ToolCall{
					ID:   tc.ID,
					Type: "function",
					FunctionCall: &llms.FunctionCall{
						Name:      tc.Name,
						Arguments: string(args),
					},
				})
			}
			langchainMessages[i] = llms.MessageContent{
				Role:  role,
				Parts: parts,
			}
		} else {
			langchainMessages[i] = llms.TextParts(role, msg.Content)
		}
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
		if len(completion.Choices[0].ToolCalls) > 0 {
			logger.Debug("Found tool calls from LLM",
				zap.Int("count", len(completion.Choices[0].ToolCalls)))
		}
		for _, tc := range completion.Choices[0].ToolCalls {
			var params map[string]interface{}
			if err := json.Unmarshal([]byte(tc.FunctionCall.Arguments), &params); err != nil {
				logger.Error("Failed to unmarshal tool arguments",
					zap.String("tool", tc.FunctionCall.Name),
					zap.String("id", tc.ID),
					zap.Error(err))
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
func (p *OpenRouterProvider) ChatWithTools(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
	return p.Chat(ctx, messages, tools, options...)
}

// Close 关闭连接
func (p *OpenRouterProvider) Close() error {
	return nil
}
