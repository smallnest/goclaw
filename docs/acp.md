# ACP (Agent Client Protocol) Configuration

## Overview

ACP (Agent Client Protocol) enables goclaw to create thread-bound agent sessions that integrate directly with IDEs and coding environments. This allows agents to perform complex coding tasks with full access to file systems and development tools.

## What is ACP?

ACP is a standardized communication protocol between AI agents and development environments, similar to how LSP (Language Server Protocol) standardized editor-language server communication.

**Key Features:**
- Thread-bound sessions that stay attached to conversations
- Direct IDE integration for coding tasks
- Persistent sessions for multi-step workflows
- Runtime lifecycle management with idle eviction
- Session identity reconciliation across restarts

## Configuration

Add ACP configuration to your `config.json`:

```json
{
  "acp": {
    "enabled": true,
    "backend": "acp-go-sdk",
    "default_agent": "main",
    "max_concurrent_sessions": 5,
    "idle_timeout_ms": 300000,
    "allowed_agents": ["main", "coding"],
    "thread_bindings": {
      "telegram:default": {
        "enabled": true,
        "spawn_enabled": true,
        "idle_timeout_ms": 300000,
        "max_age_ms": 3600000
      }
    }
  }
}
```

## Configuration Options

### `enabled` (boolean)
- Whether ACP is enabled
- Default: `false`

### `backend` (string)
- The ACP runtime backend to use
- Options: `"acp-go-sdk"` (using `github.com/coder/acp-go-sdk`)
- Default: `"acp-go-sdk"`

### `default_agent` (string)
- Default agent ID to use for ACP sessions
- Default: `"main"`

### `max_concurrent_sessions` (int)
- Maximum number of concurrent ACP sessions
- Default: `5` (0 means unlimited)

### `idle_timeout_ms` (int)
- Idle timeout in milliseconds before a session is evicted
- Default: `300000` (5 minutes)

### `allowed_agents` (array of strings)
- List of agent IDs allowed to use ACP
- Empty array means all agents are allowed
- Default: `[]` (all agents)

### `thread_bindings` (object)
- Per-channel thread binding configuration
- Keys are in format `"{channel}:{account_id}"`

## Thread Binding Configuration

Thread bindings attach ACP sessions to channel conversations (threads) for persistent interactions.

### Thread Binding Options

```json
{
  "thread_bindings": {
    "telegram:default": {
      "enabled": true,
      "spawn_enabled": true,
      "idle_timeout_ms": 300000,
      "max_age_ms": 3600000
    }
  }
}
```

#### `enabled` (boolean)
- Whether thread bindings are enabled for this channel
- Default: `true`

#### `spawn_enabled` (boolean)
- Whether spawning new thread-bound ACP sessions is allowed
- Default: `true`

#### `idle_timeout_ms` (int)
- Idle timeout before the thread binding is removed
- Default: `300000` (5 minutes)

#### `max_age_ms` (int)
- Maximum age of a thread binding before it's removed
- Default: `3600000` (1 hour)

## Usage

### Via Agent Tool

Agents can spawn ACP sessions using the `spawn_acp` tool:

```json
{
  "task": "Refactor the authentication code to use OAuth2",
  "mode": "run"
}
```

For persistent, thread-bound sessions:

```json
{
  "task": "Set up a development environment for this project",
  "mode": "session",
  "thread": true
}
```

### Via Gateway API

The goclaw gateway provides WebSocket methods for ACP management:

#### `acp_spawn`
Create a new ACP session.

```json
{
  "jsonrpc": "2.0",
  "id": "1",
  "method": "acp_spawn",
  "params": {
    "task": "Your coding task here",
    "mode": "run",
    "thread": false
  }
}
```

#### `acp_status`
Get the status of an ACP session.

```json
{
  "jsonrpc": "2.0",
  "id": "2",
  "method": "acp_status",
  "params": {
    "session_key": "agent:main:acp:uuid"
  }
}
```

#### `acp_set_mode`
Change the runtime mode for an ACP session.

```json
{
  "jsonrpc": "2.0",
  "id": "3",
  "method": "acp_set_mode",
  "params": {
    "session_key": "agent:main:acp:uuid",
    "runtime_mode": "precise"
  }
}
```

#### `acp_cancel`
Cancel an active ACP turn.

```json
{
  "jsonrpc": "2.0",
  "id": "4",
  "method": "acp_cancel",
  "params": {
    "session_key": "agent:main:acp:uuid",
    "reason": "User cancelled"
  }
}
```

#### `acp_close`
Close an ACP session.

```json
{
  "jsonrpc": "2.0",
  "id": "5",
  "method": "acp_close",
  "params": {
    "session_key": "agent:main:acp:uuid",
    "reason": "Task completed"
  }
}
```

#### `acp_list`
List all active ACP sessions.

```json
{
  "jsonrpc": "2.0",
  "id": "6",
  "method": "acp_list",
  "params": {}
}
```

## Spawn Modes

### Oneshot Mode (`"run"`)
Creates a session that completes a single task and closes:

```json
{
  "task": "Fix the bug in user authentication",
  "mode": "run"
}
```

**Use cases:**
- One-time code fixes
- Single-file refactoring
- Quick debugging tasks

### Persistent Mode (`"session"`)
Creates a session that stays active across multiple turns:

```json
{
  "task": "Set up the project structure",
  "mode": "session",
  "thread": true
}
```

**Use cases:**
- Multi-step development workflows
- Interactive coding sessions
- Long-running refactoring tasks

**Note:** Persistent mode requires `thread: true` to bind the session to a conversation thread.

## Thread-Bound Sessions

When you spawn an ACP session with `thread: true`, it becomes bound to the current conversation thread:

1. **Initial spawn** creates the ACP session and binds it to the thread
2. **Follow-up messages** in the thread are sent to the ACP session
3. **Session lifecycle** is managed by idle timeout and max age settings
4. **Session cleanup** happens automatically when the binding expires

### Thread Binding Lifecycle

```
┌─────────────────────────────────────────────────────────────┐
│ Thread Binding Lifecycle                                    │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  1. Spawn ACP session with thread=true                       │
│     ├─ Creates ACP session                                  │
│     ├─ Creates/attaches to channel thread                   │
│     └─ Binds session to thread                              │
│                                                               │
│  2. Session active                                           │
│     ├─ Messages in thread → ACP session                      │
│     ├─ ACP responses → channel thread                        │
│     └─ Reset idle timeout on each activity                  │
│                                                               │
│  3. Session expires (idle timeout OR max age)               │
│     ├─ Unbinds session from thread                          │
│     ├─ Closes ACP runtime                                    │
│     └─ Cleans up resources                                  │
│                                                               │
└─────────────────────────────────────────────────────────────┘
```

## CLI Commands

### `goclaw acp doctor`
Check ACP backend health and configuration.

```bash
goclaw acp doctor
```

### `goclaw acp list`
List all active ACP sessions.

```bash
goclaw acp list
```

### `goclaw acp status <session-key>`
Show detailed status of an ACP session.

```bash
goclaw acp status agent:main:acp:uuid
```

### `goclaw acp close <session-key>`
Close an ACP session.

```bash
goclaw acp close agent:main:acp:uuid
```

## Session Keys

ACP sessions use scoped session keys:

```
agent:{agentId}:acp:{uuid}
```

For thread-bound sessions:

```
agent:{agentId}:acp:{uuid}:thread:{channel}:{conversationId}
```

## Runtime Backend

goclaw uses the `acp-go-sdk` backend which communicates with ACP agents via stdio using JSON-RPC 2.0.

### Backend Configuration

The backend requires an ACP agent executable. Configure the agent path and arguments:

```json
{
  "acp": {
    "backend": "acp-go-sdk",
    "agent_path": "/path/to/acp-agent",
    "agent_args": ["--verbose"]
  }
}
```

### Protocol Flow

```
goclaw → acp-go-sdk → ACP Agent (stdio)
  │         │             │
  │         │             └─ JSON-RPC 2.0
  │         └─ AgentSideConnection
  └─ AcpRuntime interface
```

## Error Codes

- `ACP_BACKEND_MISSING` - No ACP runtime backend configured
- `ACP_BACKEND_UNAVAILABLE` - Backend health check failed
- `ACP_SESSION_INIT_FAILED` - Session initialization failed
- `ACP_TURN_FAILED` - Turn execution failed
- `ACP_BACKEND_UNSUPPORTED_CONTROL` - Backend doesn't support control operation
- `ACP_SESSION_LIMIT_REACHED` - Maximum concurrent sessions reached
- `ACP_AGENT_UNAUTHORIZED` - Agent not authorized to use ACP
- `ACP_POLICY_DISABLED` - ACP disabled by policy
- `ACP_THREAD_BINDING_DISABLED` - Thread bindings disabled
- `ACP_THREAD_BINDING_SPAWN_DISABLED` - Thread-bound spawning disabled

## Troubleshooting

### ACP sessions not spawning

1. Check ACP is enabled: `acp.enabled = true`
2. Verify backend configuration: `acp.backend`
3. Check agent authorization: `acp.allowed_agents`
4. Verify thread binding policy for channel

### Sessions timing out too quickly

Increase `acp.idle_timeout_ms` or the channel-specific `thread_bindings.{channel}.idle_timeout_ms`.

### Max concurrent sessions reached

Increase `acp.max_concurrent_sessions` or ensure sessions are properly closed.

### Thread bindings not working

1. Verify channel supports thread bindings
2. Check `thread_bindings.{channel}.enabled = true`
3. Ensure `thread_bindings.{channel}.spawn_enabled = true`
4. Verify channel adapter has thread support

## Examples

### Simple Oneshot Task

```json
{
  "task": "Add error handling to the user login function"
}
```

### Persistent Development Session

```json
{
  "task": "Implement the user profile feature",
  "mode": "session",
  "thread": true,
  "cwd": "/path/to/project"
}
```

### With Specific Agent

```json
{
  "task": "Refactor database queries for performance",
  "agent_id": "coding",
  "mode": "run"
}
```

## Security Considerations

1. **Agent Authorization** - Use `allowed_agents` to restrict ACP usage
2. **Channel Policies** - Configure per-channel thread binding policies
3. **Sandbox Enforcement** - ACP agents respect tool boundary policies
4. **Resource Limits** - Set `max_concurrent_sessions` to limit resource usage
5. **Session Isolation** - Each ACP session runs in an isolated runtime

## References

- [ACP Specification](https://agentclientprotocol.com)
- [acp-go-sdk](https://pkg.go.dev/github.com/coder/acp-go-sdk)
- [OpenClaw ACP Implementation](https://github.com/anthropics/openclaw)
