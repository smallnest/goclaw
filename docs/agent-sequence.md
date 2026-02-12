# Agent Execution Flow

This document describes the sequence of operations for processing a user request.

## Request Processing Sequence

```
┌───────────────┐
│ User Message   │
│ (cli/discord)│
└───────┬───────┘
        │
        ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Agent Loop                          │
│                 (loop.go)                             │
│                                                         │
│  1. Receive InboundMessage                                │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ - Get or create session                      │   │
│  │ - Add user message to history              │   │
│  │ - Check for attached media                        │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                         │
│  2. Build Context                                        │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ - Load bootstrap files (IDENTITY.md, etc.)     │   │
│  │ - Load available skills                        │   │
│  │ - Load memory (today + long-term)                │   │
│  │ - Build system prompt                           │   │
│  │ - Format message history                      │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                         │
│  3. Call Provider (with retry/failover)                │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ Provider.Chat(messages, tools)             │   │
│  │                                              │   │
│  │ ┌────────────────────────────────────────┐       │   │
│  │ │  Provider Rotation               │       │   │
│  │ │  ├─ OpenAI                    │       │   │
│  │ │  ├─ Anthropic                  │       │   │
│  │ │  ├─ OpenRouter                 │       │   │
│  │ │  └─ Circuit Breaker            │       │   │
│  │ └────────────────────────────────────────┘       │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                         │
│  4. Process Response                                    │
│  ┌──────────────────────────────────────────────────────┐   │
│  │                                                  │   │
│  │ Response has Tool Calls?                             │   │
│  │       │                                           │   │
│  │       ├─ Yes                                     │   │
│  │       │        │                                  │   │
│  │       │        ▼                                  │   │
│  │       │  5. Execute Tools                        │   │
│  │       │  ┌────────────────────────────┐         │   │
│  │       │  │ Tool Registry           │         │   │
│  │       │  │ - Get tool by name    │         │   │
│  │       │  │ - Validate params       │         │   │
│  │       │  │ - Execute              │         │   │
│  │       │  └──────────┬─────────────┘         │   │
│  │       │             │                            │   │
│  │       │             ▼                            │   │
│  │       │  6. Tool Results                     │   │
│  │       │  ┌────────────────────────────┐         │   │
│  │       │  │ Add tool response to    │         │   │
│  │       │  │ conversation history     │         │   │
│  │       │  └────────────────────────────┘         │   │
│  │       │                                        │   │
│  │       │  7. Continue (repeat from step 3)        │   │
│  │       │                                        │   │
│  │       └─ No (has content)                      │   │
│  │              │                                  │   │
│  │              ▼                                  │   │
│  │       8. Filter & Format Response                  │   │
│  │       ┌────────────────────────────┐            │   │
│  │       │ - Remove rejection messages│            │   │
│  │       │ - Format output          │            │   │
│  │       │ - Add media if present   │            │   │
│  │       └──────────┬─────────────┘            │   │
│  │                  │                            │   │
│  └──────────────────┼────────────────────────────┘   │
│                     │                              │
│                     ▼                              │
│  9. Send to Bus                               │
│  ┌──────────────────────────────────────────────┐   │
│  │ bus.PublishOutbound(OutboundMessage) │   │
│  └──────────────────────────────────────────────┘   │
│                                                     │
└─────────────────────────────────────────────────────────────┘
          │
          ▼
┌──────────────────┐
│ User/Channel    │
│ Receive         │
└──────────────────┘
```

## Error Handling Flow

```
Tool Execution Error
        │
        ▼
┌─────────────────────────────┐
│ Error Classifier            │
│ - Identify error type        │
│ - Classify severity        │
└───────────┬─────────────┘
            │
            ▼
    ┌───────────────┐
    Is Retryable?    │
    └───┬─────┬───┘
        │     │
        │ No  │ Yes
        │     │
        ▼     ▼
   ┌────┴────┐
   │ Format   │Retry
   │ Error    │Manager
   │ for User │- Check
   └────┬────┘attempt
        │count
        │    │
        │    ▼
        │  ┌──────────────┐
        │  │ Calculate   │
        │  │ Delay       │
        │  │ (exponential│
        │  │ backoff)    │
        │  └──────┬─────┘
        │         │
        │         ▼
        │  ┌──────────────────┐
        │  │ Wait or Fail   │
        │  └──────────────────┘
        │         │
        ▼         ▼
   Return to LLM with tool result
```

## Skill Loading Sequence

```
┌──────────────────┐
│ Agent Starts    │
└───────┬───────┘
        │
        ▼
┌─────────────────────────────────┐
│ Skills Loader                │
├─────────────────────────────────┤
│ 1. Scan directories        │
│  - workspace/skills/        │
│  - ~/.goclaw/skills/       │
│  - exe-relative/skills/    │
│                             │
│ 2. For each directory:       │
│  - Read SKILL.md           │
│  - Parse YAML frontmatter    │
│  - Extract metadata         │
│  - Check dependencies       │
│  - Store in skills map     │
│                             │
│ 3. Always skills:           │
│  - Auto-inject into prompt  │
│                             │
│ 4. On-demand skills:        │
│  - Available via use_skill  │
│  - Injected when selected  │
└─────────────────────────────────┘
        │
        ▼
┌──────────────────┐
│ Build System    │
│ Prompt with:   │
│ - Tool list    │
│ - Skill names  │
│ - Memory       │
│ - Bootstrap   │
└──────────────────┘
```

## Sub-Agent Spawning

```
┌──────────────────────────────┐
│ Main Agent                │
│ (decides to delegate)      │
└──────────┬───────────────┘
           │
           ▼
┌─────────────────────────────────────┐
│ Subagent Manager               │
│ 1. Generate unique ID            │
│ 2. Create task message          │
│ 3. Publish to system channel    │
└───────────┬─────────────────────┘
            │
            ▼
┌─────────────────────────────────────┐
│ Message Bus                       │
│ Forwards to agent loop handler  │
└───────────┬─────────────────────┘
            │
            ▼
┌─────────────────────────────────────┐
│ New Agent Instance (goroutine) │
│ - Own session/context             │
│ - Runs independently            │
│ - Reports back when done         │
└───────────┬─────────────────────┘
            │
            ▼ (summary)
┌─────────────────────────────────────┐
│ Original Session                │
│ Receives compiled result         │
└─────────────────────────────────────┘
```
