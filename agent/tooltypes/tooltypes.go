package tooltypes

import "context"

// AgentToolInterface defines the interface for agent tools.
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
	Content []ContentBlock
	Details map[string]any
	Error   error
}

// ContentBlock represents a content block in a message.
type ContentBlock interface {
	ContentType() string
}

// TextContent represents text content.
type TextContent struct {
	Text string
}

func (t TextContent) ContentType() string {
	return "text"
}

// AgentTextContent is a text content block (alias for tools package compatibility).
type AgentTextContent = TextContent

// AgentContentBlock represents a content block from agent tools (alias).
type AgentContentBlock = ContentBlock

// ToolResult is an alias for AgentToolResult (for tools package compatibility).
type ToolResult = AgentToolResult
