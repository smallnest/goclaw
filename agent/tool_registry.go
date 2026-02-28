package agent

import (
	"context"
	"fmt"

	"github.com/smallnest/goclaw/agent/tools"
)

// ToolRegistry wraps the existing tools.Registry and provides helper methods
type ToolRegistry struct {
	registry *tools.Registry
}

// NewToolRegistry creates a new tool registry
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		registry: tools.NewRegistry(),
	}
}

// RegisterExisting registers an existing tool from tools package
func (r *ToolRegistry) RegisterExisting(tool tools.Tool) error {
	return r.registry.Register(tool)
}

// RegisterAgentTool registers an agent.Tool (with onUpdate callback support)
func (r *ToolRegistry) RegisterAgentTool(tool Tool) {
	// Adapt agent.Tool to tools.Tool and register into the shared registry.
	// Ignore duplicate registration errors to keep startup resilient.
	_ = r.registry.Register(&reverseToolAdapter{tool: tool})
}

// Unregister removes a tool
func (r *ToolRegistry) Unregister(name string) {
	r.registry.Unregister(name)
}

// GetExisting retrieves a tool as existing type
func (r *ToolRegistry) GetExisting(name string) (tools.Tool, bool) {
	return r.registry.Get(name)
}

// ListExisting returns tools as existing type
func (r *ToolRegistry) ListExisting() []tools.Tool {
	return r.registry.List()
}

// Count returns the number of registered tools
func (r *ToolRegistry) Count() int {
	return r.registry.Count()
}

// Has checks if a tool is registered
func (r *ToolRegistry) Has(name string) bool {
	return r.registry.Has(name)
}

// Clear removes all tools
func (r *ToolRegistry) Clear() {
	r.registry.Clear()
}

// Execute executes a tool using the existing registry
func (r *ToolRegistry) Execute(ctx context.Context, name string, params map[string]interface{}) (string, error) {
	return r.registry.Execute(ctx, name, params)
}

// ToAgentTools converts existing tools to agent.Tool format (with adapter)
func ToAgentTools(existingTools []tools.Tool) []Tool {
	result := make([]Tool, 0, len(existingTools))
	for _, t := range existingTools {
		result = append(result, &toolAdapter{tool: t})
	}
	return result
}

// toolAdapter adapts tools.Tool to agent.Tool interface
type toolAdapter struct {
	tool tools.Tool
}

func (a *toolAdapter) Name() string {
	return a.tool.Name()
}

func (a *toolAdapter) Description() string {
	return a.tool.Description()
}

func (a *toolAdapter) Label() string {
	// Use name as label if no specific label is defined
	// Existing tools don't have Label(), so we use Name() as fallback
	return a.tool.Name()
}

func (a *toolAdapter) Parameters() map[string]any {
	params := a.tool.Parameters()
	result := make(map[string]any)
	for k, v := range params {
		result[k] = v
	}
	return result
}

func (a *toolAdapter) Execute(ctx context.Context, params map[string]any, onUpdate func(ToolResult)) (ToolResult, error) {
	// Convert params to existing format
	existingParams := make(map[string]interface{})
	for k, v := range params {
		existingParams[k] = v
	}

	// Execute using existing tool
	resultStr, err := a.tool.Execute(ctx, existingParams)

	result := ToolResult{
		Content: []ContentBlock{TextContent{Text: resultStr}},
		Details: make(map[string]any),
	}

	if err != nil {
		result.Error = err
		result.Details["error"] = err.Error()
	}

	// Call update callback if provided
	if onUpdate != nil {
		onUpdate(result)
	}

	return result, nil
}

// ToExistingTools converts agent tools to existing tools.Tool format
func ToExistingTools(agentTools []Tool) []tools.Tool {
	result := make([]tools.Tool, 0, len(agentTools))
	for _, t := range agentTools {
		if adapter, ok := t.(*toolAdapter); ok {
			result = append(result, adapter.tool)
		} else {
			// Create a reverse adapter
			result = append(result, &reverseToolAdapter{tool: t})
		}
	}
	return result
}

// reverseToolAdapter adapts agent.Tool to tools.Tool interface
type reverseToolAdapter struct {
	tool Tool
}

func (a *reverseToolAdapter) Name() string {
	return a.tool.Name()
}

func (a *reverseToolAdapter) Description() string {
	return a.tool.Description()
}

func (a *reverseToolAdapter) Label() string {
	return a.tool.Label()
}

func (a *reverseToolAdapter) Parameters() map[string]interface{} {
	params := a.tool.Parameters()
	result := make(map[string]interface{})
	for k, v := range params {
		result[k] = v
	}
	return result
}

func (a *reverseToolAdapter) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	// Convert params
	agentParams := make(map[string]any)
	for k, v := range params {
		agentParams[k] = v
	}

	result, err := a.tool.Execute(ctx, agentParams, nil)
	if err != nil {
		return "", err
	}

	// Extract text from content
	for _, block := range result.Content {
		if text, ok := block.(TextContent); ok {
			return text.Text, nil
		}
	}

	return "", fmt.Errorf("tool result has no text content")
}
