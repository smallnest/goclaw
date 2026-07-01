# GoClaw Agent 架构设计

> 本文档描述了 GoClaw 的核心 Agent 架构，借鉴了 pi-mono 和 openclaw 的设计模式。

## 执行摘要

GoClaw 采用模块化的 Agent 架构，核心组件包括：

- **Agent** - 主代理类，管理消息处理和生命周期
- **Orchestrator** - 执行协调器，管理 LLM 调用循环和工具执行
- **AgentState** - 状态管理，支持消息队列和流式状态
- **ContextBuilder** - 上下文构建器，生成系统提示词
- **ToolRegistry** - 工具注册表，管理所有可用工具
- **SkillsLoader** - 技能加载器，动态加载技能定义

## 1. 架构概览

```
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                           goclaw Agent System                                        │
├─────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                      │
│  ┌──────────────────┐    ┌──────────────────┐    ┌──────────────┐                  │
│  │   Channels       │    │  Agent           │    │  Bus System  │                  │
│  │  (channels/*.go) │───►│  (agent/agent.go)│◄──►│  (bus/*.go)  │                  │
│  └──────────────────┘    └────────┬─────────┘    └──────────────┘                  │
│                                   │                                                  │
│                                   ▼                                                  │
│  ┌──────────────────────────────────────────────────────────────────────────────┐  │
│  │                         Orchestrator                                          │  │
│  │                         (agent/orchestrator.go)                               │  │
│  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐              │  │
│  │  │  AgentState     │  │  ContextBuilder │  │  RetryManager   │              │  │
│  │  │  (状态管理)      │  │  (提示词构建)    │  │  (重试机制)     │              │  │
│  │  └─────────────────┘  └─────────────────┘  └─────────────────┘              │  │
│  └──────────────────────────────────────────┬───────────────────────────────────┘  │
│                                             │                                        │
│  ┌──────────────────────────────────────────▼───────────────────────────────────┐  │
│  │                         Core Components                                       │  │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐            │  │
│  │  │ToolRegistry │ │SkillsLoader │ │ MemoryStore │ │SessionMgr   │            │  │
│  │  │(工具注册表)  │ │(技能加载器)  │ │ (记忆存储)   │ │(会话管理)   │            │  │
│  │  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘            │  │
│  └──────────────────────────────────────────────────────────────────────────────┘  │
│                                                                                      │
│  ┌──────────────────────────────────────────────────────────────────────────────┐  │
│  │                         Provider Layer                                        │  │
│  │                         (providers/*.go)                                      │  │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐           │  │
│  │  │ OpenAI   │ │Anthropic │ │OpenRouter│ │Rotation  │ │ Failover │           │  │
│  │  │ Provider │ │ Provider │ │ Provider │ │ Provider │ │ Manager  │           │  │
│  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘ └──────────┘           │  │
│  └──────────────────────────────────────────────────────────────────────────────┘  │
│                                                                                      │
│  ┌──────────────────────────────────────────────────────────────────────────────┐  │
│  │                         Session Layer                                         │  │
│  │                         (session/*.go)                                        │  │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐                                      │  │
│  │  │ Manager  │ │  Cache   │ │   Tree   │                                      │  │
│  │  │(会话管理) │ │ (缓存)    │ │ (树结构)  │                                      │  │
│  │  └──────────┘ └──────────┘ └──────────┘                                      │  │
│  └──────────────────────────────────────────────────────────────────────────────┘  │
│                                                                                      │
└─────────────────────────────────────────────────────────────────────────────────────┘
```

## 2. 包结构

```
goclaw/
├── agent/                    # Agent 核心逻辑
│   ├── agent.go             # Agent 主类
│   ├── orchestrator.go      # 执行协调器
│   ├── context.go           # 上下文构建器
│   ├── retry.go             # 重试机制
│   ├── types.go             # 类型定义
│   ├── manager.go           # Agent 管理器
│   ├── memory.go            # 记忆存储
│   ├── skills.go            # 技能加载器
│   ├── tool_registry.go     # 工具注册表
│   ├── helper.go            # 辅助函数
│   ├── subagent_registry.go # 子代理注册
│   ├── subagent_announce.go # 子代理通知
│   └── tools/               # 工具实现
│       ├── registry.go      # 工具注册表
│       ├── base.go          # 工具接口
│       ├── filesystem.go    # 文件操作
│       ├── shell.go         # Shell 执行
│       ├── web.go           # Web 工具
│       ├── browser.go       # 浏览器工具
│       ├── memory_tool.go   # 记忆工具
│       ├── skill.go         # 技能工具
│       ├── spawn.go         # 子代理工具
│       └── cron_tool.go     # Cron 工具
├── bus/                      # 消息总线
│   ├── events.go            # 消息事件
│   ├── queue.go             # 消息队列
│   └── streaming.go         # 流式支持
├── channels/                 # 消息通道
│   ├── base.go              # 通道接口
│   ├── telegram.go          # Telegram
│   ├── discord.go           # Discord
│   ├── slack.go             # Slack
│   ├── feishu.go            # 飞书
│   ├── dingtalk.go          # 钉钉
│   ├── weixin.go            # 微信
│   ├── wework.go            # 企业微信
│   ├── teams.go             # Microsoft Teams
│   └── ...                  # 其他通道
├── providers/                # LLM 提供商
│   ├── base.go              # 提供商接口
│   ├── openai.go            # OpenAI
│   ├── anthropic.go         # Anthropic
│   ├── openrouter.go        # OpenRouter
│   ├── requesty.go          # Requesty
│   ├── rotation.go          # 轮换提供商
│   ├── streaming.go         # 流式支持
│   └── circuit.go           # 熔断器
├── session/                  # 会话管理
│   ├── manager.go           # 会话管理器
│   ├── tree.go              # 树结构
│   ├── cache.go             # 缓存
│   └── prune.go             # 剪枝
├── memory/                   # 记忆系统
│   ├── store.go             # 存储接口
│   ├── vector.go            # 向量存储
│   ├── embeddings.go        # 嵌入生成
│   └── qmd/                 # QMD 支持
├── gateway/                  # WebSocket 网关
│   ├── openclaw/            # OpenClaw 协议
│   └── protocol.go          # 协议定义
├── cron/                     # 定时任务
│   ├── service.go           # 调度服务
│   ├── executor.go          # 执行器
│   └── types.go             # 类型定义
├── cli/                      # 命令行界面
│   ├── root.go              # 根命令
│   ├── agent.go             # Agent 命令
│   ├── skills.go            # 技能命令
│   └── commands/            # 子命令
├── config/                   # 配置管理
│   ├── loader.go            # 配置加载
│   └── validator.go         # 配置验证
└── internal/                 # 内部包
    ├── logger/              # 日志
    └── workspace/           # 工作区管理
```

## 3. 核心接口

### 3.1 Agent 类

```go
// Agent 代表主 AI Agent
type Agent struct {
    orchestrator       *Orchestrator
    bus                *bus.MessageBus
    provider           providers.Provider
    sessionMgr         *session.Manager
    tools              *ToolRegistry
    context            *ContextBuilder
    workspace          string
    skillsLoader       *SkillsLoader
    helper             *AgentHelper
    maxHistoryMessages int

    mu        sync.RWMutex
    state     *AgentState
    eventSubs []chan *Event
    running   bool
}

// 主要方法
func (a *Agent) Start(ctx context.Context) error          // 启动 Agent
func (a *Agent) Stop() error                              // 停止 Agent
func (a *Agent) Prompt(ctx context.Context, content string) error  // 发送消息
func (a *Agent) Steer(msg AgentMessage)                   // 中断式消息
func (a *Agent) FollowUp(msg AgentMessage)                // 后续消息
func (a *Agent) Abort()                                   // 中止执行
func (a *Agent) Subscribe() <-chan *Event                 // 订阅事件
```

### 3.2 Orchestrator (协调器)

```go
// Orchestrator 管理 Agent 执行循环
// 基于 pi-mono 的 agent-loop.ts 设计
type Orchestrator struct {
    config     *LoopConfig
    state      *AgentState
    eventChan  chan *Event
    cancelFunc context.CancelFunc
}

// 主要方法
func (o *Orchestrator) Run(ctx context.Context, prompts []AgentMessage) ([]AgentMessage, error)
func (o *Orchestrator) Stop()
func (o *Orchestrator) Subscribe() <-chan *Event
```

### 3.3 AgentState (状态管理)

```go
// AgentState 管理 Agent 的运行状态
type AgentState struct {
    mu              sync.RWMutex
    Messages        []AgentMessage
    SystemPrompt    string
    Model           string
    Provider        string
    SessionKey      string
    Tools           []Tool
    LoadedSkills    []string
    IsStreaming     bool

    // 消息队列
    steeringMessages   []AgentMessage
    steeringMode       MessageQueueMode
    followUpMessages   []AgentMessage
    followUpMode       MessageQueueMode
    pendingToolCalls   map[string]bool
}

// 消息队列模式
type MessageQueueMode string

const (
    MessageQueueModeAppend  MessageQueueMode = "append"  // 追加到末尾
    MessageQueueModeInject  MessageQueueMode = "inject"  // 注入到当前位置
)
```

### 3.4 Tool 接口

```go
// Tool 代表可执行的工具
type Tool interface {
    Name() string
    Description() string
    Parameters() map[string]any
    Execute(ctx context.Context, params map[string]any, onUpdate func(ToolResult)) (ToolResult, error)
}

// ToolResult 工具执行结果
type ToolResult struct {
    Content []ContentBlock
    Details map[string]any
    Error   error
}

// ContentBlock 内容块类型
type ContentBlock interface {
    ContentType() string
}

// 内容块实现
type TextContent struct { Text string }
type ImageContent struct { URL, Data, MimeType string }
type ToolCallContent struct { ID, Name string; Arguments map[string]any }
```

### 3.5 Provider 接口

```go
// Provider LLM 提供商接口
type Provider interface {
    Chat(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error)
    ChatWithTools(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error)
    Close() error
}

// StreamingProvider 流式提供商接口
type StreamingProvider interface {
    Provider
    ChatStream(ctx context.Context, messages []Message, tools []ToolDefinition, handler StreamHandler) error
}
```

## 4. 双循环机制 (Dual Loop)

Orchestrator 采用双循环架构处理不同类型的消息和任务。

**注意**：GoClaw 与 OpenClaw 都采用双循环架构，但恢复逻辑的放置位置不同，详见 [与 OpenClaw 的对比](#与-openclaw-循环架构的对比)。

### 4.1 循环结构

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Orchestrator.runLoop()                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │              Outer Loop (外层循环)                                     │   │
│  │              处理 FollowUp 消息                                        │   │
│  │  ┌────────────────────────────────────────────────────────────────┐  │   │
│  │  │           Inner Loop (内层循环)                                 │  │   │
│  │  │           处理工具调用和 Steering 消息                          │  │   │
│  │  │                                                                 │  │   │
│  │  │   for hasMoreToolCalls || len(pendingMessages) > 0 {           │  │   │
│  │  │       // 1. 检查上下文取消                                       │  │   │
│  │  │       // 2. 检查最大迭代次数                                     │  │   │
│  │  │       // 3. 处理待处理消息 (Steering)                            │  │   │
│  │  │       // 4. 调用 LLM 获取响应                                    │  │   │
│  │  │       // 5. 提取工具调用                                         │  │   │
│  │  │       // 6. 执行工具调用                                         │  │   │
│  │  │       // 7. 检查 Steering 中断                                   │  │   │
│  │  │   }                                                             │  │   │
│  │  │                                                                 │  │   │
│  │  └────────────────────────────────────────────────────────────────┘  │   │
│  │                              │                                       │   │
│  │                              ▼                                       │   │
│  │              检查 FollowUp 消息                                       │   │
│  │              if len(followUp) > 0 { continue }                       │   │
│  │              else { break }                                          │   │
│  │                                                                       │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4.2 内层循环 (Inner Loop)

**职责**：处理工具调用链和 Steering 中断消息

```go
// 内层循环条件
for hasMoreToolCalls || len(pendingMessages) > 0 {
    // 1. 检查上下文取消（超时或停止）
    // 2. 检查最大迭代次数限制
    // 3. 处理待处理消息（Steering 消息注入）
    // 4. 调用 LLM 获取响应
    // 5. 提取工具调用
    // 6. 执行工具调用（可被 Steering 中断）
    // 7. 检查 Steering 消息（中断工具链）
}
```

**关键特性**：
- 当 LLM 返回工具调用时，执行后继续循环
- Steering 消息可以中断工具执行链
- 达到最大迭代次数时，强制输出最终响应

### 4.3 外层循环 (Outer Loop)

**职责**：处理 FollowUp 后续任务

```go
// 外层循环
for {
    // 执行内层循环
    // ...

    // 检查 FollowUp 消息
    followUpMessages := o.fetchFollowUpMessages()
    if len(followUpMessages) > 0 {
        pendingMessages = append(pendingMessages, followUpMessages...)
        continue  // 继续外层循环，处理新任务
    }

    // 无更多消息，退出
    break
}
```

**关键特性**：
- 内层循环完成后，检查是否有 FollowUp 消息
- 如果有 FollowUp，继续新一轮处理
- 实现任务链的自动延续

**注意**：外层循环**仅处理 FollowUp 消息**，不包含其他处理逻辑。重试、故障转移、上下文压缩等逻辑都在内层循环的 LLM 调用阶段处理。

### 4.4 FollowUp 使用场景

FollowUp 是一种**后续任务队列机制**，用于在当前任务完成后自动触发新任务：

```go
// 添加 FollowUp 消息
agent.FollowUp(AgentMessage{
    Role: RoleUser,
    Content: []ContentBlock{TextContent{Text: "继续下一个任务"}},
})
```

| 场景 | 说明 | 示例 |
|------|------|------|
| **任务链** | 完成任务 A 后自动执行任务 B | "分析项目 → 生成报告 → 发送邮件" |
| **异步回调** | 工具执行完成后触发后续处理 | 文件下载完成后通知用户 |
| **多步骤流程** | 第一步完成后自动启动第二步 | 数据收集完成后进行分析 |

### 4.5 各处理逻辑的位置

```
工具执行链
    │
    ├── Tool 1 执行
    │       │
    │       ▼
    │   检查 Steering ──── 有 Steering ────► 中断工具链
    │       │                                      │
    │       无                                     │
    │       │                                      ▼
    │       ▼                              返回已执行结果
    ├── Tool 2 执行                            + Steering 消息
    │       │
    │       ▼
    │   检查 Steering
    │       │
    │       ...
```

**代码实现**：

```go
// executeToolCalls 中检查 Steering
for _, tc := range toolCalls {
    // ... 执行工具 ...

    // 检查 Steering 消息（中断）
    steering := o.fetchSteeringMessages()
    if len(steering) > 0 {
        return results, steering  // 立即返回
    }
}
```

### 4.5 FollowUp 任务链

**FollowUp** 是一种后续任务队列机制，用于在当前任务完成后自动触发新任务。

```go
// 添加 FollowUp 消息
agent.FollowUp(AgentMessage{
    Role: RoleUser,
    Content: []ContentBlock{TextContent{Text: "继续下一个任务"}},
})
```

#### 使用场景

| 场景 | 说明 |
|------|------|
| **任务链** | 完成任务 A 后自动执行任务 B |
| **异步回调** | 工具执行完成后触发后续处理 |
| **多步骤流程** | 第一步完成后自动启动第二步 |
| **定时任务后续** | Cron 任务完成后触发后续分析 |

#### 处理流程

```
任务 A 完成
    │
    ▼
检查 FollowUp 队列
    │
    ├── 有消息 ──► 注入到 pendingMessages ──► 继续外层循环
    │                                              │
    │                                              ▼
    │                                        处理任务 B
    │                                              │
    │                                              ▼
    │                                        检查 FollowUp ...
    │
    └── 无消息 ──► 退出循环，返回结果
```

### 4.6 完整流程示例

```
用户: "帮我分析这个项目，然后生成报告"

┌─────────────────────────────────────────────────────────────────┐
│ 外层循环 - 第 1 轮                                               │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ 内层循环                                                     │ │
│ │   Iteration 1: LLM 调用 → run_shell(ls -la)                 │ │
│ │   Iteration 2: LLM 调用 → read_file(main.go)                │ │
│ │   Iteration 3: LLM 调用 → run_shell(go test)                │ │
│ │   Iteration 4: LLM 调用 → write_file(report.md)             │ │
│ │   Iteration 5: LLM 响应 "报告已生成"                         │ │
│ │   → 无工具调用，内层循环结束                                  │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                          │                                      │
│                          ▼                                      │
│              检查 FollowUp: 无消息                               │
│              → 退出外层循环                                      │
└─────────────────────────────────────────────────────────────────┘

--- 如果有 FollowUp ---

用户发送 FollowUp: "把报告发送到飞书"

┌─────────────────────────────────────────────────────────────────┐
│ 外层循环 - 第 2 轮                                               │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ 内层循环                                                     │ │
│ │   Iteration 6: LLM 调用 → feishu_send(report.md)            │ │
│ │   Iteration 7: LLM 响应 "已发送"                             │ │
│ │   → 无工具调用，内层循环结束                                  │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                          │                                      │
│                          ▼                                      │
│              检查 FollowUp: 无消息                               │
│              → 退出外层循环，任务完成                            │
└─────────────────────────────────────────────────────────────────┘
```

### 4.7 Steering 中断示例

```
用户: "执行长时间任务"
Agent 开始执行...

用户发送 Steering: "立即停止，先处理紧急事件"

┌─────────────────────────────────────────────────────────────────┐
│ 内层循环                                                        │
│   Iteration 1: LLM 调用 → run_shell(sleep 60)                  │
│       │                                                         │
│       ▼                                                         │
│   检查 Steering: 发现紧急消息                                    │
│       │                                                         │
│       ▼                                                         │
│   中断工具链，返回当前结果 + Steering 消息                        │
│       │                                                         │
│       ▼                                                         │
│   注入 Steering 消息到对话                                       │
│       │                                                         │
│       ▼                                                         │
│   Iteration 2: LLM 响应 "收到紧急指令，已停止之前的任务"          │
└─────────────────────────────────────────────────────────────────┘
```

### 4.8 各处理逻辑的位置

| 处理逻辑 | 所在位置 | 说明 |
|----------|----------|------|
| **FollowUp 处理** | 外层循环 | 任务完成后检查后续任务队列 |
| **工具调用执行** | 内层循环 | LLM 返回工具调用后执行 |
| **Steering 中断** | 内层循环 (工具执行后) | 工具执行过程中检查中断消息 |
| **Retry/Failback** | 内层循环 (`streamAssistantResponseWithRetry`) | LLM 调用失败时的重试机制 |
| **Profile Rotation** | 内层循环 (`streamAssistantResponseWithRetry`) | 切换到备用 LLM 配置 |
| **Context Compression** | 内层循环 (`streamAssistantResponseWithRetry`) | 上下文超限时压缩消息历史 |
| **Max Iterations** | 内层循环 | 达到最大迭代次数时强制输出 |

**重要**：外层循环**仅处理 FollowUp 消息**，不包含重试、故障转移等其他处理逻辑。

### 4.9 与 OpenClaw 循环架构的对比

两者都采用双循环架构，但恢复逻辑的放置位置不同：

| 特性 | GoClaw (双循环) | OpenClaw (双循环) |
|------|----------------|-------------------|
| **外层循环** | 处理 FollowUp 后续任务 | 处理重试、故障转移、Profile 轮换、上下文压缩 |
| **内层循环** | 工具调用 + Steering 中断 + 重试/故障转移 | 工具执行 (pi-agent-core 内部) |
| **重试机制** | 在内层循环的 `streamAssistantResponseWithRetry` 中处理 | 在外层循环通过 `continue` 重新调用 `runEmbeddedAttempt` |
| **故障转移** | 在 LLM 调用层处理 | 在外层循环通过 `advanceAuthProfile()` + `continue` 处理 |
| **上下文压缩** | 在 `streamAssistantResponseWithRetry` 内处理 | 在外层循环 `overflowCompactionAttempts++` + `continue` 处理 |
| **迭代计数** | `iteration` 计数工具调用/LLM调用 | `runLoopIterations` 计数重试次数 (外层) |

**OpenClaw 外层循环核心逻辑**：

```typescript
// OpenClaw: 外层循环 - 处理重试、故障转移、压缩
while (true) {
  if (runLoopIterations >= MAX_RUN_LOOP_ITERATIONS) {
    return error; // 超过重试限制
  }
  runLoopIterations += 1;

  // 内层循环通过 runEmbeddedAttempt 调用 pi-agent-core
  const attempt = await runEmbeddedAttempt({ ... });

  if (promptError) {
    // 处理错误，尝试故障转移
    if (await advanceAuthProfile()) {
      continue; // 切换 Profile 后继续外层循环
    }
    throw promptError;
  }

  if (contextOverflowError) {
    // 尝试上下文压缩
    const compactResult = await contextEngine.compact({ ... });
    if (compactResult.compacted) {
      continue; // 压缩后继续外层循环
    }
    // 尝试工具结果截断...
  }

  if (shouldRotate) {
    // Profile 轮换
    const rotated = await advanceAuthProfile();
    if (rotated) {
      continue; // 继续外层循环
    }
  }

  // 成功则返回
  return result;
}
```

**GoClaw 双循环核心逻辑**：

```go
// GoClaw: 外层循环 - 仅处理 FollowUp
for {
    hasMoreToolCalls := true

    // 内层循环 - 包含工具执行和重试/故障转移
    for hasMoreToolCalls || len(pendingMessages) > 0 {
        iteration++
        if iteration > maxIterations { ... }

        // 处理 Steering 消息
        // 调用 LLM (重试机制封装在此函数内)
        assistantMsg, err := o.streamAssistantResponseWithRetry(ctx, state, retryManager)
        // 执行工具
        results, steering := o.executeToolCalls(ctx, toolCalls, state)
        // 检查 Steering 中断
    }
    // 内层循环结束

    // 检查 FollowUp
    followUpMessages := o.fetchFollowUpMessages()
    if len(followUpMessages) > 0 {
        continue // 继续外层循环
    }
    break // 无 FollowUp，退出
}
```

**关键差异总结**：

| 差异点 | GoClaw | OpenClaw |
|--------|--------|----------|
| 外层循环职责 | 仅 FollowUp 消息处理 | 重试、故障转移、Profile 轮换、压缩 |
| 内层循环职责 | 工具执行 + 重试/故障转移 | 工具执行 (pi-agent-core) |
| 迭代计数含义 | 每轮对话的工具调用次数 | 整个任务的重试次数 |
| 架构优势 | 重试逻辑封装在 LLM 调用层，职责清晰 | 恢复逻辑集中在外层，整体流程清晰 |
| 适用场景 | 工具调用密集型任务 | 重试/故障转移频繁场景 |

### 5.1 消息类型

```go
// AgentMessage Agent 内部消息
type AgentMessage struct {
    Role      Role            // user, assistant, system, tool_result
    Content   []ContentBlock  // 内容块
    Timestamp int64           // 时间戳
    Metadata  map[string]any  // 元数据
}

// 角色类型
type Role string

const (
    RoleUser       Role = "user"
    RoleAssistant  Role = "assistant"
    RoleSystem     Role = "system"
    RoleToolResult Role = "tool"
)
```

### 5.2 消息处理流程

```
用户消息 → Channel → MessageBus → Agent.handleInboundMessage()
                                        │
                                        ▼
                               Session 获取/创建
                                        │
                                        ▼
                               历史消息加载
                                        │
                                        ▼
                               Orchestrator.Run()
                                        │
                    ┌───────────────────┴───────────────────┐
                    │                                       │
                    ▼                                       ▼
            ┌───────────────────┐                  ┌───────────────┐
            │    外层循环        │                  │ 检查 FollowUp │
            │ (FollowUp 处理)    │◄─────────────────│   消息队列    │
            └─────────┬─────────┘                  └───────────────┘
                      │                                   ▲
                      ▼                                   │
            ┌───────────────────┐                         │
            │    内层循环        │                         │
            │ (工具调用处理)     │                         │
            │                   │                         │
            │  LLM调用 → 工具执行 │                         │
            │         ↓         │                         │
            │  检查 Steering    │─── 有 ──► 中断 ─────────┤
            │         ↓ 无                               │
            │  继续工具链                                │
            └─────────┬─────────┘                         │
                      │                                   │
                      ▼                                   │
            内层循环结束 ──────────────────────────────────┘
                                        │
                                        ▼
                               Session 更新 → Bus 发布 → Channel 发送
```

## 6. Steering 和 FollowUp 机制

### 6.1 Steering (中断式消息)

Steering 消息用于在 Agent 执行过程中插入紧急消息，立即中断当前操作。

```go
// 添加中断消息
agent.Steer(AgentMessage{
    Role: RoleUser,
    Content: []ContentBlock{TextContent{Text: "紧急停止"}},
})

// 消息队列模式
type MessageQueueMode string

const (
    MessageQueueModeAppend MessageQueueMode = "append" // 追加到末尾
    MessageQueueModeInject MessageQueueMode = "inject" // 注入到当前位置
)

// 设置模式
agent.SetSteeringMode(MessageQueueModeInject)
```

### 6.2 FollowUp (后续消息)

FollowUp 消息在 Agent 完成当前任务后自动处理。

```go
// 添加后续消息
agent.FollowUp(AgentMessage{
    Role: RoleUser,
    Content: []ContentBlock{TextContent{Text: "继续下一个任务"}},
})

// 设置模式
agent.SetFollowUpMode(MessageQueueModeAppend)
```

## 7. 事件系统

### 7.1 事件类型

```go
type EventType string

const (
    EventAgentStart          EventType = "agent_start"
    EventAgentEnd            EventType = "agent_end"
    EventTurnStart           EventType = "turn_start"
    EventTurnEnd             EventType = "turn_end"
    EventMessageStart        EventType = "message_start"
    EventMessageEnd          EventType = "message_end"
    EventToolExecutionStart  EventType = "tool_execution_start"
    EventToolExecutionUpdate EventType = "tool_execution_update"
    EventToolExecutionEnd    EventType = "tool_execution_end"
    EventStreamContent       EventType = "stream_content"
    EventStreamThinking      EventType = "stream_thinking"
    EventStreamFinal         EventType = "stream_final"
    EventStreamDone          EventType = "stream_done"
)
```

### 7.2 事件订阅

```go
// 订阅事件
eventChan := agent.Subscribe()
defer agent.Unsubscribe(eventChan)

for event := range eventChan {
    switch event.Type {
    case EventStreamContent:
        fmt.Print(event.StreamContent)
    case EventToolExecutionStart:
        fmt.Printf("工具执行: %s\n", event.ToolName)
    }
}
```

## 8. 重试机制

### 8.1 重试配置

```go
type RetryConfig struct {
    MaxAttempts      int           // 最大重试次数
    BaseDelay        time.Duration // 基础延迟
    MaxDelay         time.Duration // 最大延迟
    RetryableErrors  []string      // 可重试错误
    ProfileRotation  bool          // 是否启用配置轮换
    ContextCompression bool        // 是否启用上下文压缩
}

type RecoveryAction string

const (
    RecoveryActionNone              RecoveryAction = "none"
    RecoveryActionRotateProfile     RecoveryAction = "rotate_profile"
    RecoveryActionCompressContext   RecoveryAction = "compress_context"
)
```

### 8.2 错误分类

```go
type FailoverReason string

const (
    FailoverReasonUnknown         FailoverReason = "unknown"
    FailoverReasonAuth            FailoverReason = "auth"
    FailoverReasonRateLimit       FailoverReason = "rate_limit"
    FailoverReasonTimeout         FailoverReason = "timeout"
    FailoverReasonNetwork         FailoverReason = "network"
    FailoverReasonBilling         FailoverReason = "billing"
    FailoverReasonContextOverflow FailoverReason = "context_overflow"
)

type ErrorClass string

const (
    ErrorClassTransient ErrorClass = "transient" // 可重试
    ErrorClassPermanent ErrorClass = "permanent" // 不可重试
    ErrorClassUser      ErrorClass = "user"      // 需要用户干预
    ErrorClassFatal     ErrorClass = "fatal"     // 致命错误
)
```

## 9. ContextBuilder (上下文构建器)

### 9.1 提示词模式

```go
type PromptMode string

const (
    PromptModeFull    PromptMode = "full"    // 完整模式（主 Agent）
    PromptModeMinimal PromptMode = "minimal" // 精简模式（子 Agent）
    PromptModeNone    PromptMode = "none"    // 无模式
)
```

### 9.2 系统提示词结构

```
1. 核心身份 + 工具列表
2. 工具调用风格
3. 安全提示
4. 错误处理指导 (仅 full 模式)
5. 技能系统
6. CLI 快速参考 (仅 full 模式)
7. 文档路径 (仅 full 模式)
8. Bootstrap 文件
9. 消息和回复指导 (仅 full 模式)
10. 静默回复规则 (仅 full 模式)
11. 心跳机制 (仅 full 模式)
12. 工作区信息
13. 运行时信息 (仅 full 模式)
```

## 10. 并发模型

### 10.1 Goroutine 架构

```
主 Goroutine
┌──────────────────────────────────────────┐
│ Agent.Start()                             │
│                                           │
│  ┌─────────────────┐  ┌─────────────────┐│
│  │ dispatchEvents  │  │ processMessages ││
│  │ (事件分发)       │  │ (消息处理)       ││
│  └─────────────────┘  └─────────────────┘│
│                                           │
│  工具执行使用独立 goroutine               │
└──────────────────────────────────────────┘
```

### 10.2 并发安全

- AgentState 使用 `sync.RWMutex` 保护
- 事件订阅使用缓冲通道避免阻塞
- 工具执行支持超时和取消

## 11. 设计原则

### 11.1 Go 惯用法

- 使用接口实现可测试性
- Context 传播实现取消
- 正确使用互斥锁保护共享状态
- 基于通道的通信优于回调
- 使用 defer 进行清理

### 11.2 错误处理

- 使用上下文包装错误
- 分类错误以进行适当处理
- 生产代码中不使用 panic
- 使用结构化字段记录错误

### 11.3 可观测性

- 使用 zap 进行结构化日志
- 关键操作的指标
- 状态转换日志
- 工具执行时间跟踪

### 11.4 可扩展性

- 插件式工具系统
- 从目录加载技能
- 新 LLM 的提供商接口
- 模块化架构便于添加

## 12. 与参考实现的比较

### 12.1 采用的 pi-mono 模式

1. **会话管理**：基于树的会话导航
2. **技能系统**：Frontmatter 解析的技能
3. **压缩**：自动上下文压缩
4. **资源加载**：统一的资源发现
5. **扩展系统**：事件驱动扩展
6. **Steering/FollowUp**：消息队列机制

### 12.2 采用的 openclaw 模式

1. **多代理**：子代理生成用于并行任务
2. **消息总线**：解耦通信
3. **提供商轮换**：自动故障转移
4. **熔断器**：防止级联故障
5. **流式**：实时响应流

### 12.3 goclaw 特有优势

1. **Go 性能**：原生 Go 并发
2. **类型安全**：编译时类型检查
3. **单一二进制**：无运行时依赖
4. **总线系统**：清晰的消息流
5. **提供商灵活性**：易于添加新 LLM

## 13. 配置示例

```json
{
  "workspace": {
    "path": ""
  },
  "agents": {
    "defaults": {
      "model": "claude-3-5-sonnet-20241022",
      "max_iterations": 15,
      "temperature": 0.7,
      "max_tokens": 4096
    }
  },
  "providers": {
    "openai": {
      "api_key": "YOUR_API_KEY",
      "base_url": "https://api.openai.com",
      "timeout": 600
    },
    "anthropic": {
      "api_key": "YOUR_API_KEY",
      "timeout": 600
    }
  },
  "tools": {
    "filesystem": {
      "allowed_paths": [],
      "denied_paths": []
    },
    "shell": {
      "enabled": true,
      "denied_cmds": ["rm -rf", "dd", "mkfs"],
      "timeout": 30
    }
  },
  "memory": {
    "backend": "builtin",
    "builtin": {
      "enabled": true,
      "auto_index": true
    }
  }
}
```
