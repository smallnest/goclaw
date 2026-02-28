# ACP (Agent Client Protocol) Architecture for goclaw

## Overview

This document describes the architecture for implementing ACP (Agent Client Protocol) support in goclaw, inspired by the openclaw implementation. ACP enables thread-bound agents as first-class runtimes with full lifecycle management.

## What is ACP?

ACP (Agent Client Protocol) is a standardized communication protocol between code editors/IDEs and AI-powered coding agents. Similar to how LSP standardized editor-language server communication, ACP standardizes agent-client interactions.

**Key Resources:**
- [Official ACP Documentation](https://agentclientprotocol.com)
- [Go SDK](https://pkg.go.dev/github.com/coder/acp-go-sdk)
- [TypeScript SDK](https://www.npmjs.com/package/@zed-industries/agent-client-protocol)

## Architecture Goals

1. **First-class ACP runtimes** - Treat ACP agents as native runtime backends
2. **Thread-bound sessions** - Bind ACP sessions to channel threads for persistent conversations
3. **Lifecycle management** - Full control over session initialization, execution, and cleanup
4. **Startup reconciliation** - Reconcile session identities after restarts
5. **Runtime cleanup** - Proper eviction and cleanup of idle sessions
6. **Coalesced replies** - Aggregate responses for thread-bound sessions

## Core Components

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          goclaw ACP Architecture                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────┐     ┌──────────────┐     ┌─────────────────────────────┐  │
│  │   Gateway   │────▶│ ACP Manager  │────▶│      Runtime Registry       │  │
│  │   (WebSocket)│     │  (lifecycle) │     │    (backend plugins)        │  │
│  └─────────────┘     └──────────────┘     └─────────────────────────────┘  │
│         │                    │                        │                     │
│         │                    ▼                        ▼                     │
│         │           ┌──────────────┐        ┌─────────────────┐            │
│         │           │Actor Queue   │        │  ACP SDK        │            │
│         │           │(per-session  │        │  Adapter        │            │
│         │           │ serialization)│       │  (acp-go-sdk)   │            │
│         │           └──────────────┘        └─────────────────┘            │
│         │                    │                        │                     │
│         ▼                    ▼                        ▼                     │
│  ┌─────────────┐     ┌──────────────┐     ┌─────────────────────────────┐  │
│  │   Agent     │────▶│Thread Binding│────▶│      ACP Runtime            │  │
│  │  (spawn_acp)│     │  Service     │     │   (stdio process)            │  │
│  └─────────────┘     └──────────────┘     └─────────────────────────────┘  │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Component Descriptions

### 1. ACP Runtime Types (`acp/runtime/types.go`)

Defines the core interfaces and types for ACP integration:

```go
// AcpRuntime - Interface that all ACP backends must implement
type AcpRuntime interface {
    ensureSession(input AcpRuntimeEnsureInput) (AcpRuntimeHandle, error)
    runTurn(input AcpRuntimeTurnInput) (<-chan AcpRuntimeEvent, error)
    getCapabilities(input AcpRuntimeHandle) (AcpRuntimeCapabilities, error)
    getStatus(input AcpRuntimeHandle) (AcpRuntimeStatus, error)
    setMode(input AcpRuntimeHandle, mode string) error
    setConfigOption(input AcpRuntimeHandle, key, value string) error
    doctor() (AcpRuntimeDoctorReport, error)
    cancel(input AcpRuntimeHandle, reason string) error
    close(input AcpRuntimeHandle, reason string) error
}

// AcpRuntimeHandle - Identifies an active ACP session
type AcpRuntimeHandle struct {
    SessionKey           string
    Backend              string
    RuntimeSessionName   string
    Cwd                  string
    AcpxRecordId         string
    BackendSessionId     string
    AgentSessionId       string
}
```

**Key Types:**
- `AcpRuntimeSessionMode`: "persistent" or "oneshot"
- `AcpRuntimePromptMode`: "prompt" or "steer"
- `AcpRuntimeEvent`: text_delta, status, tool_call, done, error
- `AcpRuntimeControl`: session/set_mode, session/set_config_option, session/status

### 2. Runtime Registry (`acp/runtime/registry.go`)

Manages available ACP runtime backends:

```go
type AcpRuntimeBackend struct {
    ID      string
    Runtime AcpRuntime
    Healthy func() bool
}

// Functions
func registerAcpRuntimeBackend(backend AcpRuntimeBackend)
func unregisterAcpRuntimeBackend(id string)
func getAcpRuntimeBackend(id string) *AcpRuntimeBackend
func requireAcpRuntimeBackend(id string) *AcpRuntimeBackend
```

**Features:**
- Global singleton registry
- Health checking for backends
- Backend ID normalization
- Default backend selection

### 3. ACP SDK Adapter (`acp/sdk/adapter.go`)

Bridges `github.com/coder/acp-go-sdk` to goclaw's `AcpRuntime` interface:

```go
type AcpGoSDKAdapter struct {
    agentPath string
    agentArgs []string
    env       []string
    timeout   time.Duration
}

func (a *AcpGoSDKAdapter) ensureSession(input AcpRuntimeEnsureInput) (AcpRuntimeHandle, error) {
    // 1. Start agent process
    // 2. Create AgentSideConnection
    // 3. Call initialize
    // 4. Call authenticate (if needed)
    // 5. Call session/new
    // 6. Return handle
}
```

**Protocol Flow:**
```
goclaw → Adapter → acp-go-sdk → Agent Process (stdio)
  │         │            │             │
  │         │            │             └─ JSON-RPC 2.0
  │         │            └─ AgentSideConnection
  │         └─ AcpRuntime interface
  └─ Channel/Agent
```

### 4. ACP Session Manager (`acp/manager.go`)

Core session lifecycle management:

```go
type AcpSessionManager struct {
    actorQueue          *SessionActorQueue
    runtimeCache        *RuntimeCache
    activeTurnBySession map[string]*ActiveTurnState
    turnLatencyStats    *TurnLatencyStats
    errorCountsByCode   map[string]int
}

type AcpSessionResolution struct {
    Kind  string // "none", "ready", "stale"
    SessionKey string
    Meta  *SessionAcpMeta
    Error error
}
```

**Responsibilities:**
- Initialize sessions (create runtime, persist metadata)
- Run turns (send prompts, stream events)
- Get session status
- Set runtime mode and config options
- Cancel active turns
- Close sessions
- Startup reconciliation (resolve pending identities)
- Evict idle runtime handles
- Track turn latency and errors

### 5. Session Actor Queue (`acp/queue.go`)

Serializes operations per session:

```go
type SessionActorQueue struct {
    mu          sync.Mutex
    queues      map[string]*chan struct{}
    totalPending int
}

func (q *SessionActorQueue) Run(sessionKey string, fn func() error) error {
    // Serialize operations per session key
    // Allow concurrent operations across different sessions
}
```

**Purpose:** Prevent race conditions when multiple operations target the same ACP session.

### 6. Runtime Cache (`acp/cache.go`)

Caches active runtime handles:

```go
type RuntimeCache struct {
    mu      sync.RWMutex
    states  map[string]*CachedRuntimeState
}

type CachedRuntimeState struct {
    Runtime        AcpRuntime
    Handle         AcpRuntimeHandle
    Backend        string
    Agent          string
    Mode           AcpRuntimeSessionMode
    Cwd            string
    LastTouchedAt  time.Time
}
```

**Features:**
- Get/Set/Clear/Has/Size operations
- Last accessed tracking
- Idle candidate collection for eviction

### 7. Thread Binding Support (`channels/thread_bindings.go`)

Binds ACP sessions to channel threads:

```go
type ThreadBindingRecord struct {
    ID              string
    TargetSessionKey string
    TargetKind      string // "session" or "subagent"
    Conversation    ThreadBindingConversation
    Placement       string // "child" or "peer"
    Metadata        ThreadBindingMetadata
    CreatedAt       time.Time
    ExpiresAt       *time.Time
}
```

**Responsibilities:**
- Create/bind threads to ACP sessions
- Manage binding lifecycle (idle timeout, max age)
- Generate thread names and intro text
- Handle channel-specific capabilities

### 8. Spawn System (`acp/spawn.go`)

Creates new ACP sessions:

```go
type SpawnAcpParams struct {
    Task    string
    Label   string
    AgentID string
    Cwd     string
    Mode    SpawnAcpMode // "run" or "session"
    Thread  bool
}

func spawnAcpDirect(params SpawnAcpParams, ctx SpawnAcpContext) (*SpawnAcpResult, error) {
    // 1. Validate ACP enabled by policy
    // 2. Resolve target agent
    // 3. Prepare thread binding (if requested)
    // 4. Initialize ACP session
    // 5. Bind to thread (if requested)
    // 6. Send bootstrap task
    // 7. Return result
}
```

**Modes:**
- `"run"` - Oneshot session (closes after task)
- `"session"` - Persistent session (stays bound to thread)

### 9. Runtime Options (`acp/options.go`)

Manages ACP runtime configuration:

```go
type AcpSessionRuntimeOptions struct {
    RuntimeMode string
    Cwd         string
}

func validateRuntimeOptionPatch(patch map[string]any) (AcpSessionRuntimeOptions, error)
func mergeRuntimeOptions(current, patch AcpSessionRuntimeOptions) AcpSessionRuntimeOptions
```

### 10. Identity Reconciliation (`acp/identity.go`)

Handles session identity tracking:

```go
type SessionIdentity struct {
    State            string // "pending" or "resolved"
    Source           string // "ensure" or "status"
    LastUpdatedAt    int64
    BackendSessionId string
    AgentSessionId   string
}

func reconcilePendingSessionIdentities(manager *AcpSessionManager, cfg *Config) (checked, resolved, failed int)
```

**Purpose:** Track backend/agent session IDs after restarts when they weren't available during initialization.

### 11. Configuration Schema (`config/acp.go`)

ACP configuration structure:

```go
type AcpConfig struct {
    Enabled             bool
    Backend             string
    DefaultAgent        string
    MaxConcurrentSessions int
    ThreadBindings      map[string]ChannelThreadBindingConfig
}

type ChannelThreadBindingConfig struct {
    Enabled         bool
    SpawnEnabled    bool
    IdleTimeoutMs   int
    MaxAgeMs        int
}
```

### 12. Policy and Authorization (`acp/policy.go`)

Validates ACP usage:

```go
func isAcpEnabledByPolicy(cfg *Config) bool
func resolveAcpAgentPolicyError(cfg *Config, agentID string) error
func resolveThreadBindingSpawnPolicy(cfg *Config, channel, accountID, kind string) ThreadBindingPolicy
```

## Integration Points

### Gateway Integration

**WebSocket Methods:**
- `sessions_acp_spawn` - Create new ACP session
- `sessions_acp_status` - Query session status
- `sessions_acp_set_mode` - Change runtime mode
- `sessions_acp_set_config_option` - Set config option
- `sessions_acp_cancel` - Cancel active turn
- `sessions_acp_close` - Close session

### Agent Tool Integration

**spawn_acp tool:**
```json
{
  "name": "spawn_acp",
  "description": "Spawn a new ACP session for coding tasks",
  "parameters": {
    "task": "string",
    "label": "string (optional)",
    "agentId": "string (optional)",
    "cwd": "string (optional)",
    "mode": "run | session",
    "thread": "boolean"
  }
}
```

### CLI Commands

```bash
goclaw acp doctor          # Check ACP backend health
goclaw acp list            # List active ACP sessions
goclaw acp status <key>    # Show session status
goclaw acp close <key>     # Close ACP session
goclaw acp spawn <task>    # Create new ACP session
```

## Session Key Format

ACP sessions use scoped session keys:

```
agent:{agentId}:acp:{uuid}
```

Thread-bound sessions:
```
agent:{agentId}:acp:{uuid}:thread:{channel}:{conversationId}
```

## Lifecycle Flow

### Initialization Flow

```
1. spawn_acp called with task
2. Validate ACP enabled by policy
3. Resolve target agent ID
4. Prepare thread binding (if thread=true)
5. Create unique session key
6. Initialize ACP session:
   a. Get runtime backend
   b. Call runtime.ensureSession()
   c. Persist session metadata
   d. Cache runtime handle
7. Bind to thread (if thread=true)
8. Send bootstrap task via gateway
9. Return result with session key
```

### Turn Execution Flow

```
1. Receive message in thread
2. Route to ACP session via binding
3. Acquire session actor lock
4. Get or create runtime handle
5. Apply runtime controls (mode, options)
6. Set session state to "running"
7. Run turn:
   a. Call runtime.runTurn()
   b. Stream events (text_delta, tool_call, etc.)
   c. Forward to channel
8. Set session state to "idle"
9. Reconcile session identity
10. Close runtime if oneshot mode
11. Release actor lock
```

### Startup Reconciliation Flow

```
1. List all ACP sessions from storage
2. Filter sessions with "pending" identity
3. For each pending session:
   a. Acquire session actor lock
   b. Get or create runtime handle
   c. Call runtime.getStatus()
   d. Update identity with backend/agent session IDs
   e. Persist updated metadata
4. Report reconciliation statistics
```

### Idle Eviction Flow

```
1. Periodic check (before each operation)
2. Collect candidates exceeding idle TTL
3. For each candidate:
   a. Acquire session actor lock
   b. Verify no active turn
   b. Call runtime.close()
   c. Clear cached state
   d. Increment eviction counter
```

## Error Handling

### Error Codes

- `ACP_BACKEND_MISSING` - No runtime backend configured
- `ACP_BACKEND_UNAVAILABLE` - Backend health check failed
- `ACP_SESSION_INIT_FAILED` - Session initialization failed
- `ACP_TURN_FAILED` - Turn execution failed
- `ACP_BACKEND_UNSUPPORTED_CONTROL` - Backend doesn't support control

### Retry Policy

- Transient errors: Retry with exponential backoff
- Backend unavailable: Treat as terminal, clear cached handle
- Session limit reached: Return error to caller

## Observability

### Metrics

- `acp.runtime_cache.active_sessions` - Currently cached sessions
- `acp.runtime_cache.evicted_total` - Total evictions
- `acp.turns.active` - Currently active turns
- `acp.turns.completed` - Total completed turns
- `acp.turns.failed` - Total failed turns
- `acp.turns.average_latency_ms` - Average turn latency
- `acp.turns.max_latency_ms` - Maximum turn latency
- `acp.errors.{code}` - Error count by code

### Logging

- Session lifecycle events (init, close, evict)
- Turn execution events (start, complete, fail)
- Identity reconciliation events
- Runtime backend events

## Security Considerations

1. **Allowlist authorization** - Only authorized agents can use ACP
2. **Channel-level policies** - Per-channel spawn permissions
3. **Thread binding validation** - Verify channel capabilities
4. **Sandbox enforcement** - Respect tool boundary policies
5. **Session isolation** - Separate runtime processes per session
6. **Resource limits** - Max concurrent sessions, idle TTL

## Dependencies

### New Go Dependencies

```go
require (
    github.com/coder/acp-go-sdk v0.1.0  // ACP protocol implementation
)
```

### Internal Dependencies

- `github.com/smallnest/goclaw/config` - Configuration loading
- `github.com/smallnest/goclaw/gateway` - WebSocket API
- `github.com/smallnest/goclaw/channels` - Thread binding
- `github.com/smallnest/goclaw/agent` - Tool registration
- `github.com/smallnest/goclaw/session` - Metadata persistence

## File Structure

```
acp/
├── runtime/
│   ├── types.go           # Core interfaces and types
│   ├── registry.go        # Backend registry
│   └── errors.go          # Error types
├── sdk/
│   └── adapter.go         # acp-go-sdk adapter
├── manager.go             # Session manager
├── spawn.go               # Spawn system
├── queue.go               # Session actor queue
├── cache.go               # Runtime cache
├── options.go             # Runtime options
├── identity.go            # Identity reconciliation
├── policy.go              # Policy and authorization
├── reconcile.go           # Startup reconciliation
└── errors.go              # Error utilities

channels/
└── thread_bindings.go     # Thread binding support

config/
└── acp.go                 # ACP configuration schema

gateway/
└── acp_methods.go         # Gateway RPC methods

agent/
└── acp_tool.go            # spawn_acp tool

cli/
└── acp.go                 # CLI commands

docs/
└── acp.md                 # User documentation
```

## Implementation Order

1. ✅ Task 1: Architecture design (this document)
2. Task 2: Core types and interfaces
3. Task 3: Runtime registry
4. Task 4: SDK adapter
5. Task 5: Session manager
6. Task 6: Actor queue
7. Task 7: Runtime cache
8. Task 8: Thread binding support
9. Task 9: Runtime options
10. Task 10: Identity reconciliation
11. Task 11: Configuration schema
12. Task 12: Policy and authorization
13. Task 13: Spawn system
14. Task 14: Gateway integration
15. Task 15: Agent tool integration
16. Task 16: Server methods
17. Task 17: Startup reconciliation
18. Task 18: Documentation
19. Task 19: CLI commands
20. Task 20: Tests

## References

- [OpenClaw ACP Implementation](https://github.com/anthropics/openclaw)
- [ACP Specification](https://agentclientprotocol.com)
- [acp-go-sdk Documentation](https://pkg.go.dev/github.com/coder/acp-go-sdk)
- [Zed ACP Integration](https://zed.dev)
