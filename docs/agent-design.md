# GoClaw Agent Architecture Design

## Executive Summary

This document synthesizes the architectural patterns from **pi-mono** (TypeScript) and **openclaw** (TypeScript) implementations to design a new, idiomatic Go agent architecture for goclaw.

The design leverages:
- pi-mono's modular architecture and session management
- openclaw's multi-agent coordination patterns
- goclaw's existing Go infrastructure (bus, providers, session)

## 1. Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                           goclaw Agent System                             │
├─────────────────────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  ┌──────────────────┐    ┌──────────────────┐    ┌──────────────┐ │
│  │   CLI Layer     │    │  Agent Loop     │    │  Bus System  │ │
│  │  (cli/*.go)     │    │ (agent/loop.go)│    │  (bus/*.go)  │ │
│  └────────┬─────────┘    └────────┬─────────┘    └──────┬───────┘ │
│           │                       │                      │           │
│           │           ┌───────────▼───────────────┐           │
│           │           │  Agent Runtime              │           │
│           │           │  - State Machine              │           │
│           │           │  - Orchestration             │           │
│           │           │  - Tool Execution             │◄──────────┘ │
│           │           └───────────┬───────────────┘                     │
│           │                       │                                  │
│           │   ┌───────────────────▼───────────────────────────────┐   │
│           │   │                  Core Components                    │   │
│           │   │  ┌─────────────┐ ┌─────────────┐          │   │
│           │   │  │   Context    │ │   Tools      │          │   │
│           │   │  │   Builder    │ │   Registry   │          │   │
│           │   │  └─────────────┘ └─────────────┘          │   │
│           │   │  ┌─────────────┐ ┌─────────────┐          │   │
│           │   │  │   Skills     │ │ Subagents   │          │   │
│           │   │  │   Loader     │ │  Manager    │          │   │
│           │   │  └─────────────┘ └─────────────┘          │   │
│           │   │  ┌─────────────┐ ┌─────────────┐          │   │
│           │   │  │    Memory    │ │  Session    │          │   │
│           │   │  │    Store     │ │  Manager    │          │   │
│           │   │  └─────────────┘ └─────────────┘          │   │
│           │   └───────────────────────────────────────────────────┘   │
│           │                                              │
│  ┌────────▼──────────────────────────────────────────────────────┐  │
│  │          Provider Layer                               │  │
│  │  (providers/*.go) - LLM abstraction              │  │
│  │  - OpenAI, Anthropic, OpenRouter, etc.            │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                           │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │          Session Layer                               │  │
│  │  (session/*.go) - Persistent conversation state       │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                           │
└───────────────────────────────────────────────────────────────────┘
```

## 2. Package Structure

```
goclaw/
├── agent/                    # Core agent runtime
│   ├── loop.go             # Main agent loop (REPL)
│   ├── state.go             # State machine (NEW)
│   ├── orchestrator.go       # Execution orchestration (NEW)
│   ├── context.go           # Context/prompt builder
│   ├── tools/
│   │   ├── registry.go      # Tool registration/discovery
│   │   ├── base.go         # Tool interface
│   │   ├── filesystem.go    # File operations
│   │   ├── shell.go         # Shell execution
│   │   ├── browser.go       # Browser automation
│   │   └── ...
│   ├── skills.go            # Skill loader
│   ├── memory.go            # Memory store
│   ├── subagent.go         # Sub-agent spawning
│   ├── reflection.go        # Task reflection
│   ├── retry_manager.go    # Retry logic
│   └── error_classifier.go  # Error handling
├── bus/                      # Message bus (in-place)
│   ├── queue.go            # Message queuing
│   ├── events.go           # Event types
│   └── streaming.go        # Streaming support
├── providers/                # LLM providers (in-place)
│   ├── base.go             # Provider interface
│   ├── openai.go
│   ├── anthropic.go
│   ├── openrouter.go
│   ├── rotation.go          # Provider rotation
│   ├── failover.go         # Failover logic
│   └── circuit.go          # Circuit breaker
├── session/                  # Session management (in-place)
│   ├── manager.go
│   ├── tree.go
│   └── cache.go
├── cli/                     # CLI interface
│   ├── root.go             # Root command
│   ├── agent.go             # Agent command
│   ├── session.go           # Session management commands
│   └── ...
└── internal/
    ├── logger/             # Logging (zap)
    └── workspace/          # Workspace management
```

## 3. Core Interfaces

### 3.1 Tool Interface (Existing - Enhanced)

```go
// agent/tools/base.go
package tools

// Tool represents an executable tool/action
type Tool interface {
    // Name returns the tool's identifier
    Name() string

    // Description returns what the tool does (for LLM)
    Description() string

    // Parameters returns JSON Schema for validation
    Parameters() map[string]interface{}

    // Execute runs the tool with given parameters
    Execute(ctx context.Context, params map[string]interface{}) (string, error)
}

// ToolCapabilities describes what a tool can do
type ToolCapabilities struct {
    // RequiresNetwork indicates if tool needs network access
    RequiresNetwork bool

    // RequiresSandbox indicates if tool should run in sandbox
    RequiresSandbox bool

    // Streaming indicates if tool supports streaming output
    Streaming bool

    // Dangerous indicates if tool has destructive potential
    Dangerous bool
}
```

### 3.2 Provider Interface (Existing)

```go
// providers/base.go
package providers

// Message represents a chat message
type Message struct {
    Role       string                 `json:"role"`
    Content    string                 `json:"content"`
    Images     []string               `json:"images,omitempty"`
    ToolCalls  []ToolCall             `json:"tool_calls,omitempty"`
}

// ToolCall represents a tool invocation request
type ToolCall struct {
    ID     string                 `json:"id"`
    Name   string                 `json:"name"`
    Params map[string]interface{} `json:"params"`
}

// ToolDefinition defines a tool for the LLM
type ToolDefinition struct {
    Name        string                 `json:"name"`
    Description string                 `json:"description"`
    Parameters  map[string]interface{} `json:"parameters"`
}

// Provider is the LLM API interface
type Provider interface {
    // Chat sends messages and returns response
    Chat(ctx context.Context, messages []Message, tools []ToolDefinition) (*Response, error)

    // ChatStream sends messages with streaming
    ChatStream(ctx context.Context, messages []Message, tools []ToolDefinition, handler StreamHandler) error

    // EstimateTokens estimates token usage
    EstimateTokens(messages []Message) (int, error)
}
```

### 3.3 Session Interface (Existing)

```go
// session/manager.go
package session

// Message represents a single message in conversation
type Message struct {
    Role       string                 `json:"role"`
    Content    string                 `json:"content"`
    Media      []Media                `json:"media,omitempty"`
    Timestamp  time.Time              `json:"timestamp"`
    Metadata   map[string]interface{} `json:"metadata,omitempty"`
    ToolCallID string                 `json:"tool_call_id,omitempty"`
    ToolCalls  []ToolCall             `json:"tool_calls,omitempty"`
}

// Session represents a conversation session
type Session struct {
    Key       string
    Messages  []Message
    CreatedAt time.Time
    UpdatedAt time.Time
    Metadata  map[string]interface{}
    mu        sync.RWMutex
}

// Manager manages session lifecycle
type Manager interface {
    GetOrCreate(key string) (*Session, error)
    Save(session *Session) error
    Delete(key string) error
    List() ([]string, error)
}
```

## 4. New Components to Implement

### 4.1 State Machine (NEW)

```go
// agent/state.go
package agent

// AgentState represents the current state of the agent
type AgentState int

const (
    StateIdle AgentState = iota
    StateProcessing
    StateAwaitingToolResult
    StateStreaming
    StateError
    StateShuttingDown
)

// StateTransition represents a state change
type StateTransition struct {
    From     AgentState
    To       AgentState
    Reason   string
    Metadata map[string]interface{}
}

// StateListener is called on state changes
type StateListener interface {
    OnStateChange(old, new AgentState, transition StateTransition)
}

// StateMachine manages agent state transitions
type StateMachine struct {
    mu              sync.RWMutex
    current         AgentState
    listeners       []StateListener
    transitionCount  int
}

func (sm *StateMachine) Current() AgentState
func (sm *StateMachine) Transition(to AgentState, reason string) error
func (sm *StateMachine) AddListener(listener StateListener)
```

### 4.2 Orchestrator (NEW)

```go
// agent/orchestrator.go
package agent

// ExecutionPlan represents a plan for executing user request
type ExecutionPlan struct {
    Goal         string
    Steps        []ExecutionStep
    Dependencies map[string][]string  // step -> dependencies
}

// ExecutionStep represents a single step in execution
type ExecutionStep struct {
    ID           string
    Type         StepType  // ToolCall, LLMQuery, SubTask
    ToolName     string
    Parameters   map[string]interface{}
    CanFail      bool
    RetryPolicy  RetryPolicy
}

// StepType represents types of execution steps
type StepType string

const (
    StepToolCall   StepType = "tool_call"
    StepLLMQuery   StepType = "llm_query"
    StepSubTask    StepType = "sub_task"
    StepParallel   StepType = "parallel"
)

// Orchestrator manages execution flow
type Orchestrator struct {
    stateMachine  *StateMachine
    toolRegistry  *tools.Registry
    executor     *StepExecutor
    planner       *PlanGenerator  // Optional: uses LLM to plan
}

func (o *Orchestrator) Execute(ctx context.Context, userRequest string) (*ExecutionResult, error)
func (o *Orchestrator) Plan(ctx context.Context, userRequest string) (*ExecutionPlan, error)
```

### 4.3 Enhanced Tool Registry

```go
// agent/tools/registry.go (enhanced)
package tools

// ToolCategory groups related tools
type ToolCategory string

const (
    CategoryFileSystem  ToolCategory = "filesystem"
    CategoryShell      ToolCategory = "shell"
    CategoryBrowser    ToolCategory = "browser"
    CategorySearch     ToolCategory = "search"
    CategoryAgent      ToolCategory = "agent"
    CategorySkill      ToolCategory = "skill"
)

// ToolMetadata describes tool metadata
type ToolMetadata struct {
    Category    ToolCategory
    Version     string
    Author      string
    Deprecated  bool
    Experimental bool
}

// Registry manages available tools
type Registry struct {
    mu          sync.RWMutex
    tools       map[string]Tool
    categories  map[ToolCategory][]string
    aliases     map[string]string  // name -> canonical name
}

func (r *Registry) Register(tool Tool) error
func (r *Registry) Unregister(name string) error
func (r *Registry) Get(name string) (Tool, bool)
func (r *Registry) List() []Tool
func (r *Registry) ListByCategory(category ToolCategory) []Tool
func (r *Registry) Execute(ctx context.Context, name string, params map[string]interface{}) (string, error)
```

## 5. Data Structures

### 5.1 Message Flow

```go
// agent/messages.go
package agent

// InboundMessage represents a message from a user/channel
type InboundMessage struct {
    ID        string
    Channel   string  // e.g., "discord", "slack", "cli"
    ChatID    string  // Channel-specific conversation ID
    SenderID  string
    Content   string
    Media      []Media
    Metadata   map[string]interface{}
    Timestamp  time.Time
}

// OutboundMessage represents a message to send to user/channel
type OutboundMessage struct {
    Channel   string
    ChatID    string
    Content   string
    Media      []Media
    Timestamp  time.Time
}

// ToolResult represents the result of a tool execution
type ToolResult struct {
    ToolName   string
    CallID     string
    Success    bool
    Result     string
    Error      error
    Duration   time.Duration
    Metadata   map[string]interface{}
}
```

### 5.2 Skill Structure (Enhanced)

```go
// agent/skills.go (enhanced)
package agent

// Skill represents a loaded skill with metadata
type Skill struct {
    // Identity
    Name        string
    Version     string
    Author      string
    Homepage    string

    // Content
    Description string
    Content     string  // Markdown with YAML frontmatter

    // Dependencies
    Requires    SkillRequirements
    Install     []SkillInstall

    // Runtime
    Always      bool  // Always loaded
    Enabled     bool  // User enabled/disabled
    MissingDeps *MissingDeps  // Computed at load time

    // Metadata from YAML frontmatter
    Metadata    SkillMetadata
}

// SkillMetadata parsed from frontmatter
type SkillMetadata struct {
    Emoji    string
    Always    bool
    Requires  struct {
        Bins       []string
        AnyBins   []string
        Env        []string
        Config     []string
        OS         []string
        PythonPkgs []string
        NodePkgs   []string
    }
    Install []SkillInstall
}

// MissingDeps tracks what's missing for a skill
type MissingDeps struct {
    Bins       []string
    AnyBins    []string
    Env        []string
    PythonPkgs []string
    NodePkgs   []string
}
```

## 6. Concurrency Model

### 6.1 Goroutine Architecture

```
Main Goroutine                    Tool Execution Pool
┌──────────────────┐            ┌─────────────────────┐
│ Agent Loop       │            │ Worker 1           │
│ (loop.Start)   │            │ Worker 2           │
│                 │            │ Worker 3           │
│ ┌─────────────┐  │            │ Worker N           │
│ │ Message Bus  │  │            └─────────────────────┘
│ │ (channel)   │  │
│ └──────┬──────┘  │
│        │          │
│        ▼          │
│   Tool Execution │
│   Manager      │
└──────────────────┘
```

### 6.2 Channel Usage

```go
// agent/concurrency.go
package agent

// ExecutionQueue manages parallel tool execution
type ExecutionQueue struct {
    queue       chan *ToolTask
    workers     int
    wg          sync.WaitGroup
    ctx         context.Context
    results     map[string]chan *ToolResult
}

// ToolTask represents a queued tool execution
type ToolTask struct {
    ID          string
    ToolName    string
    Params      map[string]interface{}
    ResultChan  chan *ToolResult
    Context     context.Context
}

func NewExecutionQueue(workers int) *ExecutionQueue
func (eq *ExecutionQueue) Submit(task *ToolTask) error
func (eq *ExecutionQueue) Shutdown(ctx context.Context) error
```

## 7. Error Handling Strategy

### 7.1 Error Classification (Existing)

```go
// agent/error_classifier.go (enhanced)
package agent

// FailoverReason indicates why a request failed
type FailoverReason string

const (
    FailoverReasonUnknown    FailoverReason = "unknown"
    FailoverReasonAuth       FailoverReason = "auth"
    FailoverReasonRateLimit  FailoverReason = "rate_limit"
    FailoverReasonTimeout    FailoverReason = "timeout"
    FailoverReasonNetwork    FailoverReason = "network"
    FailoverReasonBilling    FailoverReason = "billing"
    FailoverReasonContext    FailoverReason = "context_overflow"
)

// ErrorClass represents categories of errors
type ErrorClass string

const (
    ErrorClassTransient  ErrorClass = "transient"  // Retryable
    ErrorClassPermanent  ErrorClass = "permanent"  // Non-retryable
    ErrorClassUser      ErrorClass = "user"       // User intervention needed
    ErrorClassFatal     ErrorClass = "fatal"      // Agent should stop
)

// ErrorClassifier categorizes errors for retry logic
type ErrorClassifier struct {
    patterns map[string]FailoverReason
}

func (ec *ErrorClassifier) ClassifyError(err error) FailoverReason
func (ec *ErrorClassifier) GetClass(reason FailoverReason) ErrorClass
```

### 7.2 Retry Strategy (Existing - Documented)

```go
// agent/retry_manager.go
// RetryPolicy defines retry behavior
type RetryPolicy interface {
    ShouldRetry(attempt int, err error) (bool, FailoverReason)
    GetDelay(attempt int, reason FailoverReason) time.Duration
}

// Exponential backoff calculation
func calculateExponentialBackoff(attempt int, baseDelay, maxDelay time.Duration) time.Duration
```

## 8. Integration with Existing GoClaw

### 8.1 Bus Integration (No Changes Needed)

The existing bus system works well:
- `InboundMessage` / `OutboundMessage` structures
- `MessageBus` with channels
- Streaming support via `StreamHandler`

### 8.2 Provider Integration (No Changes Needed)

The existing provider system is solid:
- Provider interface with `Chat()` and `ChatStream()`
- Rotation and failover logic
- Circuit breaker pattern
- Multiple providers supported

### 8.3 Session Integration (No Changes Needed)

The session manager is well-designed:
- JSONL file format for persistence
- In-memory caching with mutex
- Tree-based session navigation
- Pruning and compaction support

## 9. Implementation Plan

### Phase 1: Core State Machine
1. Implement `agent/state.go` with state machine
2. Add state listeners for observability
3. Integrate with existing `loop.go`

### Phase 2: Orchestrator
1. Implement `agent/orchestrator.go`
2. Add execution planning capabilities
3. Integrate with tool registry

### Phase 3: Enhanced Tools
1. Add tool categories to registry
2. Implement tool capabilities metadata
3. Add tool execution queue for parallel execution

### Phase 4: Testing
1. Unit tests for state machine
2. Integration tests for orchestrator
3. End-to-end tests for complete flow

## 10. Design Principles

### 10.1 Go Idioms
- Use interfaces for testability
- Context propagation for cancellation
- Proper mutex usage for shared state
- Channel-based communication over callbacks
- Defer for cleanup

### 10.2 Error Handling
- Wrap errors with context
- Classify errors for appropriate handling
- Never panic in production code
- Log errors with structured fields

### 10.3 Observability
- Structured logging with zap
- Metrics for key operations
- State transitions logged
- Tool execution timing tracked

### 10.4 Extensibility
- Plugin-based tool system
- Skill loading from directories
- Provider interface for new LLMs
- Modular architecture for easy additions

## 11. Comparison with Reference Implementations

### 11.1 pi-mono Patterns Adopted
1. **Session Management**: Tree-based session navigation
2. **Skills System**: Frontmatter-parsed skills
3. **Compaction**: Automatic context compression
4. **Resource Loading**: Unified resource discovery
5. **Extension System**: Event-driven extensions

### 11.2 openclaw Patterns Adopted
1. **Multi-Agent**: Sub-agent spawning for parallel tasks
2. **Message Bus**: Decoupled communication
3. **Provider Rotation**: Automatic failover
4. **Circuit Breaker**: Prevent cascade failures
5. **Streaming**: Real-time response streaming

### 11.3 goclaw Strengths Preserved
1. **Go Performance**: Native Go concurrency
2. **Type Safety**: Compile-time type checking
3. **Single Binary**: No runtime dependencies
4. **Bus System**: Clean message flow
5. **Provider Flexibility**: Easy to add new LLMs

## 12. Migration Path

### 12.1 Compatibility
- Maintain existing message formats
- Keep tool interface compatible
- Preserve session storage format
- Support existing skill definitions

### 12.2 Incremental Rollout
1. Deploy state machine alongside existing code
2. Gradually add orchestration features
3. Enable new features via configuration
4. Deprecate old code paths safely
