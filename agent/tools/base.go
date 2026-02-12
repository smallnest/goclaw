package tools

import (
	"context"
	"encoding/json"
)

// ContentBlock represents a block of content in a message
type ContentBlock interface {
	ContentType() string
}

// TextContent represents text content
type TextContent struct {
	Text string `json:"text"`
}

func (t TextContent) ContentType() string {
	return "text"
}

// ImageContent represents image content
type ImageContent struct {
	URL     string `json:"url,omitempty"`
	Data     string `json:"data,omitempty"` // base64
	MimeType string `json:"mimeType,omitempty"`
}

func (i ImageContent) ContentType() string {
	return "image"
}

// ToolResult represents the result of a tool execution
type ToolResult struct {
	Content []ContentBlock `json:"content"`
	Details map[string]any  `json:"details"`
	Error   error           `json:"error,omitempty"`
}

// Tool 工具接口
type Tool interface {
	// Name 工具名称
	Name() string

	// Description 工具描述
	Description() string

	// Parameters JSON Schema 参数定义
	Parameters() map[string]interface{}

	// Execute 执行工具
	Execute(ctx context.Context, params map[string]interface{}) (string, error)
}

// ToolCall 工具调用
type ToolCall struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Params   map[string]interface{} `json:"params"`
	Response string                 `json:"response,omitempty"`
	Error    string                 `json:"error,omitempty"`
}

// BaseTool 基础工具
type BaseTool struct {
	name        string
	description string
	parameters  map[string]interface{}
	executeFunc func(ctx context.Context, params map[string]interface{}) (string, error)
}

// NewBaseTool 创建基础工具
func NewBaseTool(name, description string, parameters map[string]interface{}, executeFunc func(ctx context.Context, params map[string]interface{}) (string, error)) *BaseTool {
	return &BaseTool{
		name:        name,
		description: description,
		parameters:  parameters,
		executeFunc: executeFunc,
	}
}

// Name 返回工具名称
func (t *BaseTool) Name() string {
	return t.name
}

// Description 返回工具描述
func (t *BaseTool) Description() string {
	return t.description
}

// Parameters 返回参数定义
func (t *BaseTool) Parameters() map[string]interface{} {
	return t.parameters
}

// Execute 执行工具
func (t *BaseTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	return t.executeFunc(ctx, params)
}

// ExecuteWithStreaming executes the tool with streaming support (new agent compatibility)
func (t *BaseTool) ExecuteWithStreaming(ctx context.Context, params map[string]interface{}, onUpdate func(ToolResult)) (ToolResult, error) {
	resultStr, err := t.executeFunc(ctx, params)

	result := ToolResult{
		Content: []ContentBlock{TextContent{Text: resultStr}},
		Details: map[string]any{},
	}

	if err != nil {
		result.Error = err
		result.Details = map[string]any{"error": err.Error()}
	}

	// Call update callback if provided
	if onUpdate != nil {
		onUpdate(result)
	}

	return result, nil
}

// ToSchema 转换为 OpenAI 函数格式
func ToSchema(tool Tool) map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        tool.Name(),
			"description": tool.Description(),
			"parameters":  tool.Parameters(),
		},
	}
}

// ValidateParameters 验证参数
func ValidateParameters(params map[string]interface{}, schema map[string]interface{}) error {
	// 获取 required 字段
	required := []string{}
	if req, ok := schema["required"].([]interface{}); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				required = append(required, s)
			}
		}
	}

	// 检查必需字段
	for _, field := range required {
		if _, ok := params[field]; !ok {
			return &ValidationError{
				Field:   field,
				Message: "required field missing",
			}
		}
	}

	return nil
}

// ValidationError 参数验证错误
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

// MarshalParams 序列化参数为 JSON
func MarshalParams(params map[string]interface{}) (string, error) {
	data, err := json.Marshal(params)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// UnmarshalParams 反序列化参数
func UnmarshalParams(data string) (map[string]interface{}, error) {
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(data), &params); err != nil {
		return nil, err
	}
	return params, nil
}
