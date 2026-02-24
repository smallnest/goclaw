package agent

import (
	"context"
	"fmt"
)

// AgentTool is the unified tool interface for the agent
// Inspired by pi-mono's AgentTool<TParameters, TDetails> interface
type AgentTool interface {
	// Name returns the tool name (used for tool calls)
	Name() string

	// Description returns what the tool does
	Description() string

	// Parameters returns JSON Schema for the tool's parameters
	Parameters() map[string]any

	// Label returns a human-readable label for UI display
	// Inspired by pi-mono's AgentTool.label
	Label() string

	// Execute runs the tool with streaming update support
	// toolCallId: unique identifier for this tool call
	// params: validated parameters
	// signal: cancellation signal
	// onUpdate: callback for streaming updates
	Execute(ctx context.Context, toolCallId string, params map[string]any, signal context.Context, onUpdate func(AgentToolResult)) (AgentToolResult, error)
}

// AgentToolResult represents the result of a tool execution
// Inspired by pi-mono's AgentToolResult<T>
type AgentToolResult struct {
	// Content blocks supporting text and images
	Content []ContentBlock `json:"content"`
	// Details to be displayed in a UI or logged
	Details map[string]any `json:"details"`
}

// NewAgentToolResult creates a new tool result
func NewAgentToolResult(content string) AgentToolResult {
	return AgentToolResult{
		Content: []ContentBlock{TextContent{Text: content}},
		Details: make(map[string]any),
	}
}

// NewAgentToolResultWithDetails creates a new tool result with details
func NewAgentToolResultWithDetails(content string, details map[string]any) AgentToolResult {
	return AgentToolResult{
		Content: []ContentBlock{TextContent{Text: content}},
		Details: details,
	}
}

// BaseAgentTool provides a base implementation of AgentTool
type BaseAgentTool struct {
	name        string
	label       string
	description string
	parameters  map[string]any
	executeFunc func(ctx context.Context, toolCallId string, params map[string]any, signal context.Context, onUpdate func(AgentToolResult)) (AgentToolResult, error)
}

// NewBaseAgentTool creates a new base agent tool
func NewBaseAgentTool(name, label, description string, parameters map[string]any, executeFunc func(ctx context.Context, toolCallId string, params map[string]any, signal context.Context, onUpdate func(AgentToolResult)) (AgentToolResult, error)) *BaseAgentTool {
	if label == "" {
		label = name
	}
	return &BaseAgentTool{
		name:        name,
		label:       label,
		description: description,
		parameters:  parameters,
		executeFunc: executeFunc,
	}
}

// Name returns the tool name
func (t *BaseAgentTool) Name() string {
	return t.name
}

// Label returns the tool label
func (t *BaseAgentTool) Label() string {
	return t.label
}

// Description returns the tool description
func (t *BaseAgentTool) Description() string {
	return t.description
}

// Parameters returns the tool parameters
func (t *BaseAgentTool) Parameters() map[string]any {
	return t.parameters
}

// Execute executes the tool
func (t *BaseAgentTool) Execute(ctx context.Context, toolCallId string, params map[string]any, signal context.Context, onUpdate func(AgentToolResult)) (AgentToolResult, error) {
	return t.executeFunc(ctx, toolCallId, params, signal, onUpdate)
}

// AdaptTool converts an existing Tool to AgentTool
func AdaptTool(tool Tool) AgentTool {
	return &toolAgentAdapter{tool: tool}
}

// toolAgentAdapter adapts Tool to AgentTool interface
type toolAgentAdapter struct {
	tool Tool
}

func (a *toolAgentAdapter) Name() string {
	return a.tool.Name()
}

func (a *toolAgentAdapter) Label() string {
	return a.tool.Label()
}

func (a *toolAgentAdapter) Description() string {
	return a.tool.Description()
}

func (a *toolAgentAdapter) Parameters() map[string]any {
	return a.tool.Parameters()
}

func (a *toolAgentAdapter) Execute(ctx context.Context, toolCallId string, params map[string]any, signal context.Context, onUpdate func(AgentToolResult)) (AgentToolResult, error) {
	// Call the tool's Execute method
	result, err := a.tool.Execute(ctx, params, func(tr ToolResult) {
		if onUpdate != nil {
			onUpdate(AgentToolResult{
				Content: tr.Content,
				Details: tr.Details,
			})
		}
	})

	if err != nil {
		return AgentToolResult{}, err
	}

	return AgentToolResult{
		Content: result.Content,
		Details: result.Details,
	}, nil
}

// AgentToolFromFunc creates an AgentTool from a simple function
func AgentToolFromFunc(name, label, description string, parameters map[string]any, fn func(ctx context.Context, params map[string]any) (string, error)) AgentTool {
	return NewBaseAgentTool(name, label, description, parameters,
		func(ctx context.Context, toolCallId string, params map[string]any, signal context.Context, onUpdate func(AgentToolResult)) (AgentToolResult, error) {
			result, err := fn(ctx, params)
			if err != nil {
				return AgentToolResult{}, err
			}
			return AgentToolResult{
				Content: []ContentBlock{TextContent{Text: result}},
				Details: make(map[string]any),
			}, nil
		},
	)
}

// ToAgentTools converts a slice of Tool to AgentTool
func ToAgentToolsSlice(tools []Tool) []AgentTool {
	result := make([]AgentTool, len(tools))
	for i, t := range tools {
		result[i] = AdaptTool(t)
	}
	return result
}

// ToTools converts a slice of AgentTool to Tool (for compatibility)
func ToTools(agentTools []AgentTool) []Tool {
	result := make([]Tool, len(agentTools))
	for i, t := range agentTools {
		result[i] = &agentToolAdapter{tool: t}
	}
	return result
}

// agentToolAdapter adapts AgentTool to Tool interface
type agentToolAdapter struct {
	tool AgentTool
}

func (a *agentToolAdapter) Name() string {
	return a.tool.Name()
}

func (a *agentToolAdapter) Label() string {
	return a.tool.Label()
}

func (a *agentToolAdapter) Description() string {
	return a.tool.Description()
}

func (a *agentToolAdapter) Parameters() map[string]any {
	return a.tool.Parameters()
}

func (a *agentToolAdapter) Execute(ctx context.Context, params map[string]any, onUpdate func(ToolResult)) (ToolResult, error) {
	result, err := a.tool.Execute(ctx, "", params, nil, func(atr AgentToolResult) {
		if onUpdate != nil {
			onUpdate(ToolResult{
				Content: atr.Content,
				Details: atr.Details,
			})
		}
	})

	if err != nil {
		return ToolResult{Error: err}, err
	}

	return ToolResult{
		Content: result.Content,
		Details: result.Details,
	}, nil
}

// ValidateToolParameters validates tool parameters against schema
func ValidateToolParameters(params map[string]any, schema map[string]any) error {
	required := []string{}
	if req, ok := schema["required"].([]any); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				required = append(required, s)
			}
		}
	}

	for _, field := range required {
		if _, ok := params[field]; !ok {
			return &ToolValidationError{
				Field:   field,
				Message: "required field missing",
			}
		}
	}

	return nil
}

// ToolValidationError is returned when parameter validation fails
type ToolValidationError struct {
	Field   string
	Message string
}

func (e *ToolValidationError) Error() string {
	return fmt.Sprintf("validation error for field '%s': %s", e.Field, e.Message)
}
