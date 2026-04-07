package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/smallnest/goclaw/internal/logger"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/ollama"
	"go.uber.org/zap"
)

// OllamaProvider Ollama 提供商
type OllamaProvider struct {
	llm       *ollama.LLM
	model     string
	maxTokens int
	timeout   time.Duration
}

// NewOllamaProvider 创建 Ollama 提供商
func NewOllamaProvider(apiKey, baseURL, model string, maxTokens int) (*OllamaProvider, error) {
	return NewOllamaProviderWithTimeout(apiKey, baseURL, model, maxTokens, 0)
}

// NewOllamaProviderWithTimeout 创建带超时的 Ollama 提供商
func NewOllamaProviderWithTimeout(apiKey, baseURL, model string, maxTokens int, timeout time.Duration) (*OllamaProvider, error) {
	if model == "" {
		model = "llama3.2"
	}

	opts := []ollama.Option{
		ollama.WithModel(model),
	}

	if baseURL != "" {
		opts = append(opts, ollama.WithServerURL(baseURL))
	}

	if timeout > 0 {
		httpClient := &http.Client{
			Timeout: timeout,
		}
		opts = append(opts, ollama.WithHTTPClient(httpClient))
		logger.Info("Ollama provider configured with timeout",
			zap.Duration("timeout", timeout))
	}

	llm, err := ollama.New(opts...)
	if err != nil {
		return nil, err
	}

	return &OllamaProvider{
		llm:       llm,
		model:     model,
		maxTokens: maxTokens,
		timeout:   timeout,
	}, nil
}

// Chat 聊天
func (p *OllamaProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
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
	llmOpts := []llms.CallOption{}
	if opts.Temperature > 0 {
		llmOpts = append(llmOpts, llms.WithTemperature(opts.Temperature))
	}
	if opts.MaxTokens > 0 {
		llmOpts = append(llmOpts, llms.WithMaxTokens(opts.MaxTokens))
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
		// 记录是否有工具调用
		if len(completion.Choices[0].ToolCalls) > 0 {
			logger.Debug("Found tool calls from LLM",
				zap.Int("count", len(completion.Choices[0].ToolCalls)))
			for _, tc := range completion.Choices[0].ToolCalls {
				logger.Debug("Tool call",
					zap.String("id", tc.ID),
					zap.String("name", tc.FunctionCall.Name),
					zap.String("args", tc.FunctionCall.Arguments))
			}
		}
		for _, tc := range completion.Choices[0].ToolCalls {
			var params map[string]interface{}
			if err := json.Unmarshal([]byte(tc.FunctionCall.Arguments), &params); err != nil {
				// 如果参数解析失败，记录错误但继续
				logger.Error("Failed to unmarshal tool arguments",
					zap.String("tool", tc.FunctionCall.Name),
					zap.String("id", tc.ID),
					zap.Error(err),
					zap.String("raw_args", tc.FunctionCall.Arguments),
					zap.Int("args_length", len(tc.FunctionCall.Arguments)))

				// 创建一个包含错误信息的参数对象
				params = map[string]interface{}{
					"__error__":         fmt.Sprintf("Failed to parse arguments: %v", err),
					"__raw_arguments__": tc.FunctionCall.Arguments,
				}
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
		FinishReason: "stop", // Simplified
	}

	return response, nil
}

// ChatWithTools 聊天（带工具）
func (p *OllamaProvider) ChatWithTools(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
	return p.Chat(ctx, messages, tools, options...)
}

// Close 关闭连接
func (p *OllamaProvider) Close() error {
	return nil
}

// NewOllamaProviderFromLangChain 从 LangChain 创建提供商
func NewOllamaProviderFromLangChain(apiKey, baseURL, model string, maxTokens int) (Provider, error) {
	return NewOllamaProvider(apiKey, baseURL, model, maxTokens)
}
