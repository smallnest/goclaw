package tools

import (
	"context"
	"fmt"
	"sync"

	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

// Registry 工具注册表
type Registry struct {
	tools map[string]Tool
	mu    sync.RWMutex
}

// NewRegistry 创建工具注册表
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register 注册工具
func (r *Registry) Register(tool Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := tool.Name()
	if _, ok := r.tools[name]; ok {
		return fmt.Errorf("tool %s already registered", name)
	}

	r.tools[name] = tool
	logger.Info("Tool registered", zap.String("tool", name))
	return nil
}

// AgentToolInterface defines the interface for agent tools without importing agent package.
// This avoids circular dependency between agent/tools and agent packages.
type AgentToolInterface interface {
	Name() string
	Description() string
	Label() string
	Parameters() map[string]any
	Execute(ctx context.Context, params map[string]any, onUpdate func(AgentToolResult)) (AgentToolResult, error)
}

// AgentToolResult represents the result of an agent tool execution.
type AgentToolResult struct {
	Content []AgentContentBlock
	Details map[string]any
	Error   error
}

// AgentContentBlock represents a content block from agent tools.
type AgentContentBlock interface {
	ContentType() string
}

// AgentTextContent is a text content block.
type AgentTextContent struct {
	Text string
}

func (a AgentTextContent) ContentType() string { return "text" }

// RegisterAgentTool 注册 agent.Tool 类型的工具（使用接口避免循环导入）
func (r *Registry) RegisterAgentTool(tool AgentToolInterface) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := tool.Name()
	if _, ok := r.tools[name]; ok {
		return fmt.Errorf("tool %s already registered", name)
	}

	// Wrap agent tool as tools.Tool using adapter
	wrapped := &AgentToolAdapter{tool: tool}
	r.tools[name] = wrapped
	logger.Info("Agent tool registered", zap.String("tool", name))
	return nil
}

// AgentToolAdapter 将 AgentToolInterface 适配为 tools.Tool
type AgentToolAdapter struct {
	tool AgentToolInterface
}

func (a *AgentToolAdapter) Name() string {
	return a.tool.Name()
}

func (a *AgentToolAdapter) Description() string {
	return a.tool.Description()
}

func (a *AgentToolAdapter) Parameters() map[string]interface{} {
	return a.tool.Parameters()
}

func (a *AgentToolAdapter) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	result, err := a.tool.Execute(ctx, params, nil)

	// Convert AgentToolResult to string
	if err != nil {
		return "", err
	}

	// Serialize content blocks
	var output string
	for _, block := range result.Content {
		switch b := block.(type) {
		case AgentTextContent:
			output += b.Text
		default:
			if block != nil {
				output += fmt.Sprintf("[%s content]", block.ContentType())
			}
		}
	}

	if output == "" {
		output = "Tool executed successfully"
	}

	return output, nil
}

// Unregister 注销工具
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.tools, name)
	logger.Info("Tool unregistered", zap.String("tool", name))
}

// Get 获取工具
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, ok := r.tools[name]
	return tool, ok
}

// List 列出所有工具
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}

// GetDefinitions 获取所有工具的 OpenAI 格式定义
func (r *Registry) GetDefinitions() []map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	definitions := make([]map[string]interface{}, 0, len(r.tools))
	for _, tool := range r.tools {
		definitions = append(definitions, ToSchema(tool))
	}
	return definitions
}

// Execute 执行工具
func (r *Registry) Execute(ctx context.Context, name string, params map[string]interface{}) (string, error) {
	tool, ok := r.Get(name)
	if !ok {
		return "", fmt.Errorf("tool %s not found", name)
	}

	// 验证参数
	if err := ValidateParameters(params, tool.Parameters()); err != nil {
		return "", fmt.Errorf("parameter validation failed: %w", err)
	}

	// 执行工具
	logger.Debug("Executing tool",
		zap.String("tool", name),
		zap.Any("params", params),
	)

	result, err := tool.Execute(ctx, params)
	if err != nil {
		logger.Error("Tool execution failed",
			zap.String("tool", name),
			zap.Error(err),
		)
		return "", err
	}

	logger.Debug("Tool executed successfully",
		zap.String("tool", name),
		zap.Int("result_length", len(result)),
	)

	return result, nil
}

// Count 返回工具数量
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// Has 检查工具是否存在
func (r *Registry) Has(name string) bool {
	_, ok := r.Get(name)
	return ok
}

// Clear 清空所有工具
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tools = make(map[string]Tool)
}
