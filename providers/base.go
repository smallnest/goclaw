package providers

import (
	"context"

	"github.com/tmc/langchaingo/llms"
)

// Message 消息
type Message struct {
	Role       string     `json:"role"` // user, assistant, system, tool
	Content    string     `json:"content"`
	Images     []string   `json:"images,omitempty"`       // Image URLs or Base64
	ToolCallID string     `json:"tool_call_id,omitempty"` // For tool role
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // For assistant role
}

// ToolCall 工具调用
type ToolCall struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Params   map[string]interface{} `json:"params"`
	Response string                 `json:"response,omitempty"`
}

// Response LLM 响应
type Response struct {
	Content      string     `json:"content"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	FinishReason string     `json:"finish_reason"`
	Usage        Usage      `json:"usage"`
}

// Usage 使用情况
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Provider LLM 提供商接口
type Provider interface {
	// Chat 聊天
	Chat(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error)

	// ChatWithTools 聊天（带工具）
	ChatWithTools(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error)

	// Close 关闭连接
	Close() error
}

// ToolDefinition 工具定义
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ChatOption 聊天选项
type ChatOption func(*ChatOptions)

// ChatOptions 聊天配置
type ChatOptions struct {
	Model       string
	Temperature float64
	MaxTokens   int
	Stream      bool
}

// WithModel 设置模型
func WithModel(model string) ChatOption {
	return func(o *ChatOptions) {
		o.Model = model
	}
}

// WithTemperature 设置温度
func WithTemperature(temp float64) ChatOption {
	return func(o *ChatOptions) {
		o.Temperature = temp
	}
}

// WithMaxTokens 设置最大 tokens
func WithMaxTokens(maxTokens int) ChatOption {
	return func(o *ChatOptions) {
		o.MaxTokens = maxTokens
	}
}

// WithStream 设置流式输出
func WithStream(stream bool) ChatOption {
	return func(o *ChatOptions) {
		o.Stream = stream
	}
}

// ConvertToLangChainMessages 转换为 LangChain 消息格式
func ConvertToLangChainMessages(messages []Message) []llms.MessageContent {
	result := make([]llms.MessageContent, len(messages))
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

		if len(msg.Images) > 0 {
			parts := []llms.ContentPart{
				llms.TextPart(msg.Content),
			}
			for _, img := range msg.Images {
				parts = append(parts, llms.ImageURLPart(img))
			}
			result[i] = llms.MessageContent{
				Role:  role,
				Parts: parts,
			}
		} else {
			result[i] = llms.TextParts(role, msg.Content)
		}
	}
	return result
}

// ConvertToLangChainTools 转换为 LangChain 工具格式
func ConvertToLangChainTools(tools []ToolDefinition) []llms.Tool {
	result := make([]llms.Tool, len(tools))
	for i, tool := range tools {
		result[i] = llms.Tool{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		}
	}
	return result
}
