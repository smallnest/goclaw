package agent

import (
	"context"
	"time"

	"github.com/smallnest/goclaw/providers"
	"github.com/smallnest/goclaw/session"
)

// MessageRole represents the role of a message
type MessageRole string

const (
	RoleUser       MessageRole = "user"
	RoleAssistant  MessageRole = "assistant"
	RoleToolResult MessageRole = "tool"
	RoleSystem     MessageRole = "system"
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

// ToolCallContent represents a tool call from assistant
type ToolCallContent struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

func (t ToolCallContent) ContentType() string {
	return "tool_call"
}

// ThinkingContent represents thinking/reasoning content
type ThinkingContent struct {
	Thinking string `json:"thinking"`
}

func (t ThinkingContent) ContentType() string {
	return "thinking"
}

// AgentMessage represents a message in the agent conversation (renamed to avoid conflict with context.go)
type AgentMessage struct {
	ID        string         `json:"id,omitempty"`
	Role      MessageRole     `json:"role"`
	Content   []ContentBlock `json:"content"`
	Timestamp int64           `json:"timestamp,omitempty"`
	Metadata  map[string]any  `json:"metadata,omitempty"`
}

// ToolResult represents the result of a tool execution
type ToolResult struct {
	Content []ContentBlock `json:"content"`
	Details map[string]any `json:"details"`
	Error   error           `json:"error,omitempty"`
}

// Tool represents an executable tool
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]any

	// Execute runs the tool with optional streaming updates
	Execute(ctx context.Context, params map[string]any, onUpdate func(ToolResult)) (ToolResult, error)
}

// AgentState represents the current state of the agent
type AgentState struct {
	SystemPrompt   string
	Model         string
	Provider      string
	ThinkingLevel string // off, minimal, low, medium, high, xhigh
	Tools         []Tool
	Messages      []AgentMessage
	IsStreaming   bool
	PendingTools  map[string]bool
	Error         error

	// Queues for message injection (inspired by pi-mono)
	SteeringQueue  []AgentMessage
	FollowUpQueue []AgentMessage

	// Session key
	SessionKey string
}

// EventType represents types of events emitted by the agent
type EventType string

const (
	EventAgentStart         EventType = "agent_start"
	EventAgentEnd           EventType = "agent_end"
	EventTurnStart          EventType = "turn_start"
	EventTurnEnd            EventType = "turn_end"
	EventMessageStart       EventType = "message_start"
	EventMessageUpdate      EventType = "message_update"
	EventMessageEnd         EventType = "message_end"
	EventToolExecutionStart EventType = "tool_execution_start"
	EventToolExecutionUpdate EventType = "tool_execution_update"
	EventToolExecutionEnd   EventType = "tool_execution_end"
)

// Event represents an event from the agent
type Event struct {
	Type      EventType       `json:"type"`
	Message   *Message         `json:"message,omitempty"`
	Timestamp int64            `json:"timestamp"`
	// Tool execution fields
	ToolID       string     `json:"tool_id,omitempty"`
	ToolName    string     `json:"tool_name,omitempty"`
	ToolArgs     map[string]any `json:"tool_args,omitempty"`
	ToolResult   *ToolResult `json:"tool_result,omitempty"`
	ToolError    bool       `json:"tool_error,omitempty"`
	// Turn end fields
	StopReason   string     `json:"stop_reason,omitempty"`
	FinalMessages []AgentMessage `json:"final_messages,omitempty"`
}

// LoopConfig contains configuration for the agent loop
type LoopConfig struct {
	Model           string
	Provider        providers.Provider
	SessionMgr      *session.Manager
	MaxIterations   int
	SessionID       string

	// Hooks for message transformation
	ConvertToLLM     func([]AgentMessage) ([]providers.Message, error)
	TransformContext func([]AgentMessage) ([]AgentMessage, error)

	// Queues for message injection
	GetSteeringMessages  func() ([]AgentMessage, error)
	GetFollowUpMessages func() ([]AgentMessage, error)
}

// NewAgentState creates a new agent state
func NewAgentState() *AgentState {
	return &AgentState{
		Tools:        make([]Tool, 0),
		Messages:     make([]AgentMessage, 0),
		PendingTools: make(map[string]bool),
		SteeringQueue: make([]AgentMessage, 0),
		FollowUpQueue: make([]AgentMessage, 0),
	}
}

// AddMessage adds a message to the agent state
func (s *AgentState) AddMessage(msg AgentMessage) {
	s.Messages = append(s.Messages, msg)
}

// AddMessages adds multiple messages to the agent state
func (s *AgentState) AddMessages(msgs []AgentMessage) {
	s.Messages = append(s.Messages, msgs...)
}

// ClearMessages clears all messages from the agent state
func (s *AgentState) ClearMessages() {
	s.Messages = make([]AgentMessage, 0)
}

// GetLastMessage returns the last message in the state
func (s *AgentState) GetLastMessage() *AgentMessage {
	if len(s.Messages) == 0 {
		return nil
	}
	return &s.Messages[len(s.Messages)-1]
}

// HasPendingToolCalls checks if there are pending tool executions
func (s *AgentState) HasPendingToolCalls() bool {
	return len(s.PendingTools) > 0
}

// AddPendingTool adds a tool to the pending set
func (s *AgentState) AddPendingTool(toolID string) {
	if s.PendingTools == nil {
		s.PendingTools = make(map[string]bool)
	}
	s.PendingTools[toolID] = true
}

// RemovePendingTool removes a tool from the pending set
func (s *AgentState) RemovePendingTool(toolID string) {
	delete(s.PendingTools, toolID)
}

// ClearPendingTools clears all pending tools
func (s *AgentState) ClearPendingTools() {
	s.PendingTools = make(map[string]bool)
}

// Steer adds a steering message to interrupt the agent mid-run
func (s *AgentState) Steer(msg AgentMessage) {
	s.SteeringQueue = append(s.SteeringQueue, msg)
}

// FollowUp adds a follow-up message to be processed after agent finishes
func (s *AgentState) FollowUp(msg AgentMessage) {
	s.FollowUpQueue = append(s.FollowUpQueue, msg)
}

// DequeueSteeringMessages gets and clears steering messages
func (s *AgentState) DequeueSteeringMessages() []AgentMessage {
	msgs := s.SteeringQueue
	s.SteeringQueue = make([]AgentMessage, 0)
	return msgs
}

// DequeueFollowUpMessages gets and clears follow-up messages
func (s *AgentState) DequeueFollowUpMessages() []AgentMessage {
	msgs := s.FollowUpQueue
	s.FollowUpQueue = make([]AgentMessage, 0)
	return msgs
}

// HasQueuedMessages checks if there are queued messages
func (s *AgentState) HasQueuedMessages() bool {
	return len(s.SteeringQueue) > 0 || len(s.FollowUpQueue) > 0
}

// Clone creates a deep copy of the agent state
func (s *AgentState) Clone() *AgentState {
	messages := make([]AgentMessage, len(s.Messages))
	copy(messages, s.Messages)

	steering := make([]AgentMessage, len(s.SteeringQueue))
	copy(steering, s.SteeringQueue)

	followUp := make([]AgentMessage, len(s.FollowUpQueue))
	copy(followUp, s.FollowUpQueue)

	pendingTools := make(map[string]bool, len(s.PendingTools))
	for k, v := range s.PendingTools {
		pendingTools[k] = v
	}

	return &AgentState{
		SystemPrompt:   s.SystemPrompt,
		Model:         s.Model,
		Provider:      s.Provider,
		ThinkingLevel: s.ThinkingLevel,
		Tools:         append([]Tool{}, s.Tools...),
		Messages:      messages,
		IsStreaming:   s.IsStreaming,
		PendingTools:  pendingTools,
		Error:         s.Error,
		SteeringQueue:  steering,
		FollowUpQueue:  followUp,
		SessionKey:    s.SessionKey,
	}
}

// NewEvent creates a new event with current timestamp
func NewEvent(eventType EventType) *Event {
	return &Event{
		Type:      eventType,
		Timestamp: time.Now().UnixMilli(),
	}
}

// WithMessage adds message to the event
func (e *Event) WithMessage(msg *Message) *Event {
	e.Message = msg
	return e
}

// WithToolExecution adds tool execution info to the event
func (e *Event) WithToolExecution(toolID, toolName string, args map[string]any) *Event {
	e.ToolID = toolID
	e.ToolName = toolName
	e.ToolArgs = args
	return e
}

// WithToolResult adds tool result to the event
func (e *Event) WithToolResult(result *ToolResult, isError bool) *Event {
	e.ToolResult = result
	e.ToolError = isError
	return e
}

// WithStopReason adds stop reason to the event
func (e *Event) WithStopReason(reason string) *Event {
	e.StopReason = reason
	return e
}

// WithFinalMessages adds final messages to the event
func (e *Event) WithFinalMessages(msgs []AgentMessage) *Event {
	e.FinalMessages = msgs
	return e
}
