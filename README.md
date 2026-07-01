# goclaw (🐾 狗爪)

Go 语言版本的 OpenClaw - 一个功能强大的 AI Agent 框架。

[![License](https://img.shields.io/:license-MIT-blue.svg)](https://opensource.org/licenses/MIT) [![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat-square)](https://pkg.go.dev/github.com/smallnest/goclaw) [![github actions](https://github.com/smallnest/goclaw/actions/workflows/go.yaml/badge.svg)](https://github.com/smallnest/goclaw/actions) [![Go Report Card](https://goreportcard.com/badge/github.com/smallnest/goclaw)](https://goreportcard.com/report/github.com/smallnest/goclaw) [![Coverage Status](https://coveralls.io/repos/github/smallnest/goclaw/badge.svg?branch=master)](https://coveralls.io/github/smallnest/goclaw?branch=master)

![](docs/goclaw.png)

## 功能特性

- 🛠️ **完整的工具系统**：FileSystem、Shell、Web、Browser，支持 Docker 沙箱与权限控制
- 📚 **技能系统 (Skills)**：兼容 [OpenClaw](https://github.com/openclaw/openclaw) 和 [AgentSkills](https://agentskills.io) 规范，支持自动发现与环境准入控制 (Gating)
- 💾 **持久化会话**：基于 JSONL 的会话存储，支持完整的工具调用链 (Tool Calls) 记录与恢复
- 📢 **多渠道支持**：Telegram、WhatsApp、飞书 (Feishu)、QQ、企业微信 (WeWork)、钉钉 (DingTalk)、百度如流 (Infoflow)、Gotify、Slack、Discord、Google Chat、Microsoft Teams、微信 (Weixin)
- 🔧 **灵活配置**：支持 YAML/JSON 配置，热加载，环境变量支持
- 🎯 **多 LLM 提供商**：OpenAI、Qianfan（百度千帆，OpenAI-compatible）、Anthropic、OpenRouter、Requesty，支持故障转移
- 🌐 **WebSocket Gateway**：内置网关服务，支持实时通信
- ⏰ **Cron 调度**：内置定时任务调度器
- 🖥️ **Browser 自动化**：基于 Chrome DevTools Protocol 的浏览器控制
- 🧠 **记忆系统**：支持内置向量数据库和 QMD (Quick Markdown Database)
- 👥 **多账号支持**：每个通道支持配置多个账号实例
- 🪟 **跨平台**：支持 Linux、macOS、Windows

## 技能系统 (New!)

goclaw 引入了先进的技能系统，允许用户通过编写 Markdown 文档 (`SKILL.md`) 来扩展 Agent 的能力。

### 特性
*   **Prompt-Driven**: 技能本质上是注入到 System Prompt 中的指令集，指导 LLM 使用现有工具 (exec, read_file 等) 完成任务。
*   **OpenClaw 兼容**: 完全兼容 OpenClaw 的技能生态。您可以直接将 `openclaw/skills` 目录下的技能复制过来使用。
*   **自动准入 (Gating)**: 智能检测系统环境。例如，只有当系统安装了 `curl` 时，`weather` 技能才会生效；只有安装了 `git` 时，`git-helper` 才会加载。

### 使用方法

#### 配置文件加载优先级

goclaw 按以下顺序查找配置文件（找到第一个即使用）：

1. `~/.goclaw/config.json` (用户全局目录，**最高优先级**)
2. `./config.json` (当前目录)
3. 环境变量 `GOSKILLS_*` 前缀

可通过 `--config` 参数指定配置文件路径覆盖默认行为。支持 YAML 和 JSON 格式。

#### Skills 加载顺序

技能按以下顺序加载，**同名技能后面的会覆盖前面的**：

| 顺序 | 路径 | 说明 |
|-----|------|------|
| 1 | `~/.goclaw/skills/` | 用户全局目录（最低优先级） |
| 2 | `${WORKSPACE}/skills/` | 工作区目录 |
| 3 | `./skills/` (当前目录) | **最后加载，优先级最高** |

默认 `WORKSPACE` 为 `~/.goclaw/workspace`。

1.  **列出可用技能**
    ```bash
    ./goclaw skills list
    ```

2.  **安装技能**
    将技能文件夹放入以下任一位置：
    *   `~/.goclaw/skills/` (用户全局目录)
    *   `${WORKSPACE}/skills/` (工作区目录)
    *   `./skills/` (当前目录，**最高优先级，后加载会覆盖前面的**)

3.  **编写技能**
    创建一个目录 `my-skill`，并在其中创建 `SKILL.md`：
    ```yaml
    ---
    name: my-skill
    description: A custom skill description.
    metadata:
      openclaw:
        requires:
          bins: ["python3"] # 仅当 python3 存在时加载
    ---
    # My Skill Instructions
    When the user asks for X, use `exec` to run `python3 script.py`.
    ```

## 项目结构

```
goclaw/
├── agent/              # Agent 核心逻辑
│   ├── loop.go         # Agent 循环
│   ├── context.go      # 上下文构建器
│   ├── memory.go       # 记忆系统
│   ├── skills.go       # 技能加载器
│   ├── subagent.go     # 子代理管理器
│   └── tools/          # 工具系统
│       ├── filesystem.go   # 文件系统工具
│       ├── shell.go        # Shell 工具
│       ├── web.go          # Web 工具
│       ├── browser.go      # 浏览器工具
│       └── message.go      # 消息工具
├── channels/           # 消息通道
│   ├── base.go         # 通道接口
│   ├── manager.go      # 通道管理器
│   ├── telegram.go     # Telegram 实现
│   ├── whatsapp.go     # WhatsApp 实现
│   ├── feishu.go       # 飞书实现
│   ├── qq.go           # QQ 实现
│   ├── wework.go       # 企业微信实现
│   ├── dingtalk.go     # 钉钉实现
│   ├── infoflow.go     # 百度如流实现
│   ├── gotify.go       # Gotify 实现
│   ├── slack.go        # Slack 实现
│   ├── discord.go      # Discord 实现
│   ├── googlechat.go   # Google Chat 实现
│   ├── teams.go        # Microsoft Teams 实现
│   └── weixin.go       # 微信实现
├── bus/                # 消息总线
│   ├── events.go       # 消息事件
│   └── queue.go        # 消息队列
├── config/             # 配置管理
│   ├── schema.go       # 配置结构
│   └── loader.go       # 配置加载器
├── providers/          # LLM 提供商
│   ├── base.go         # 提供商接口
│   ├── factory.go      # 提供商工厂
│   ├── openai.go       # OpenAI 实现
│   ├── anthropic.go    # Anthropic 实现
│   ├── openrouter.go   # OpenRouter 实现
│   └── requesty.go     # Requesty 实现
├── gateway/            # WebSocket 网关
│   ├── server.go       # 网关服务器
│   ├── handler.go      # 消息处理器
│   └── protocol.go     # 协议定义
├── cron/               # 定时任务调度
│   ├── scheduler.go    # 调度器
│   └── cron.go         # Cron 任务
├── session/            # 会话管理
│   └── manager.go      # 会话管理器
├── cli/                # 命令行界面
│   ├── root.go         # 根命令
│   ├── agent.go        # Agent 命令
│   ├── agents.go       # Agents 管理命令
│   ├── sessions.go     # 会话命令
│   ├── cron_cli.go     # Cron 命令
│   ├── approvals.go    # 审批命令
│   ├── system.go       # 系统命令
│   └── commands/       # 子命令
│       ├── tui.go      # TUI 命令
│       ├── gateway.go  # Gateway 命令
│       ├── browser.go  # Browser 命令
│       ├── health.go   # 健康检查
│       ├── status.go   # 状态查询
│       ├── memory.go   # 记忆管理
│       └── logs.go     # 日志查询
├── internal/           # 内部包
│   ├── logger/         # 日志
│   └── utils/          # 工具函数
├── docs/               # 文档
│   ├── cli.md          # CLI 详细文档
│   └── INTRODUCTION.md # 项目介绍
└── main.go             # 主入口
```

## 快速开始

### 安装

```bash
# 克隆仓库
git clone https://github.com/smallnest/goclaw.git
cd goclaw

# 安装依赖
go mod tidy

# 仅编译 Go 程序
go build -o goclaw .

# 完整构建（包含 UI，推荐）
make build-full

# 或直接运行
go run main.go start
```

本地可以访问 dashboard:
```
http://localhost:28789/dashboard/
```


### 配置

goclaw 按以下顺序查找配置文件（找到第一个即使用）：

1. `~/.goclaw/config.json` (用户全局目录，**最高优先级**)
2. `./config.json` (当前目录)
3. 环境变量 `GOSKILLS_*` 前缀

可通过 `--config` 参数指定配置文件路径覆盖默认行为。支持 YAML 和 JSON 格式。

创建 `config.json` (参考 `internal/config.example.json`):

```json
{
  "workspace": {
    "path": ""
  },
  "agents": {
    "defaults": {
      "model": {
        "primary": "qianfan/kimi-k2.5"
      },
      "max_iterations": 15,
      "temperature": 0.7,
      "max_tokens": 4096
    }
  },
  "models": {
    "mode": "merge",
    "providers": {
      "qianfan": {
        "baseUrl": "https://qianfan.baidubce.com/v2",
        "apiKey": "${QIANFAN_API_KEY}",
        "api": "openai-completions",
        "models": [
          {
            "id": "kimi-k2.5",
            "name": "Kimi K2.5",
            "contextWindow": 131072,
            "maxTokens": 8192,
            "input": ["text", "image"]
          }
        ]
      },
      "openai": {
        "baseUrl": "https://api.openai.com/v1",
        "apiKey": "${OPENAI_API_KEY}",
        "api": "openai-completions",
        "models": [
          {
            "id": "gpt-4o",
            "name": "GPT-4o",
            "contextWindow": 128000,
            "maxTokens": 16384,
            "input": ["text", "image"]
          }
        ]
      }
    }
  },
  "channels": {
    "telegram": {
      "enabled": false,
      "token": "your-telegram-bot-token",
      "allowed_ids": []
    },
    "feishu": {
      "enabled": false,
      "app_id": "",
      "app_secret": "",
      "domain": "feishu",
      "group_policy": "open"
    },
    "dingtalk": {
      "enabled": false,
      "client_id": "",
      "secret": "",
      "allowed_ids": []
    }
  },
  "tools": {
    "filesystem": {
      "allowed_paths": [],
      "denied_paths": []
    },
    "shell": {
      "enabled": true,
      "allowed_cmds": [],
      "denied_cmds": ["rm -rf", "dd", "mkfs", "format"],
      "timeout": 30,
      "working_dir": ""
    },
    "browser": {
      "enabled": true,
      "headless": true,
      "timeout": 30,
      "relay_url": "ws://127.0.0.1:18789",
      "relay_mode": "auto"
    }
  },
  "memory": {
    "backend": "builtin",
    "builtin": {
      "enabled": true,
      "database_path": "",
      "auto_index": true
    }
  }
}
```

### 运行

```bash
# 启动 Agent 服务
./goclaw start

# 交互式 TUI 模式
./goclaw tui

# 单次执行 Agent
./goclaw agent --message "你好，介绍一下你自己"

# 查看配置
./goclaw config show

# 查看帮助
./goclaw --help
```

### 启动 Dashboard

```bash
# 启动 WebSocket Gateway（内置 Dashboard UI）
./goclaw gateway run

# 访问 Dashboard
# 本地访问：http://localhost:28789/dashboard/
# 远程访问：http://your-host:28789/dashboard/?token=YOUR_TOKEN
```

Dashboard 功能：
- 实时聊天界面
- 会话管理
- Channel 状态监控
- Cron 任务管理

> **注意**：如果监听非本地地址（非 127.0.0.1/localhost），远程访问需要配置 `auth_token`：
> ```json
> {
>   "gateway": {
>     "websocket": {
>       "host": "0.0.0.0",
>       "port": 28789,
>       "enable_auth": true,
>       "auth_token": "your-secret-token"
>     }
>   }
> }
> ```

### Desktop App (Tauri)

桌面版现在是一个本地壳程序：

- 启动时先拉起 sidecar `goclaw start`
- 等待 `127.0.0.1:28789` 就绪
- 自动在同一个窗口打开 `http://127.0.0.1:28789/dashboard/`
- 如果启动失败，会停留在本地启动页，并提供重启后端、打开配置文件的按钮

常用命令：

```bash
# 准备桌面版需要的 sidecar 和内嵌 dashboard 资源
make prepare-tauri-sidecar

# 开发模式运行桌面版
make dev-tauri

# 构建当前平台桌面版
make build-tauri-current
```

测试步骤：

```bash
# 1. 验证前端 dashboard 能正常构建
cd ui && npm run build

# 2. 验证 Tauri Rust 端能正常编译
cd ../src-tauri && cargo check

# 3. 验证桌面版 sidecar 资源准备完成
cd .. && make prepare-tauri-sidecar

# 4. 实际启动桌面版
make dev-tauri
```

启动后重点检查：

1. 首屏先出现本地启动页，而不是旧的 React 控制台壳。
2. 几秒内自动跳转到 `http://127.0.0.1:28789/dashboard/`。
3. Dashboard 中的 RPC 和 WebSocket 都能正常工作。
4. 关闭应用后，28789 端口不应残留由本应用启动的 sidecar 进程。
5. 如果故意把配置改坏，应用应停留在启动页，并能通过 “Restart Backend” 和 “Open Config” 恢复。

macOS 发布与公证流程见 [docs/release-macos.md](/Users/chaoyuepan/ai/goclaw/docs/release-macos.md)。

### 使用示例

```bash
# 查看所有可用命令
./goclaw --help

# 列出所有技能
./goclaw skills list

# 列出所有会话
./goclaw sessions list

# 查看 Gateway 状态
./goclaw gateway status

# 查看 Cron 任务
./goclaw cron list

# 健康检查
./goclaw health
```

## CLI 命令参考

goclaw 提供了丰富的命令行工具，主要命令包括：

### 基本命令

| 命令 | 描述 |
|-----|------|
| `goclaw start` | 启动 Agent 服务 |
| `goclaw tui` | 启动交互式终端界面 |
| `goclaw agent --message <msg>` | 单次执行 Agent |
| `goclaw config show` | 显示当前配置 |

### Agent 管理

| 命令 | 描述 |
|-----|------|
| `goclaw agents list` | 列出所有 agents |
| `goclaw agents add` | 添加新 agent |
| `goclaw agents delete <name>` | 删除 agent |

### Channel 管理

| 命令 | 描述 |
|-----|------|
| `goclaw channels list` | 列出所有 channels |
| `goclaw channels status` | 检查 channel 状态 |
| `goclaw channels weixin login [account-id]` | 微信扫码登录 |
| `goclaw channels weixin logout [account-id]` | 微信登出 |
| `goclaw channels weixin status [account-id]` | 查看微信登录状态 |

### Gateway 管理

| 命令 | 描述 |
|-----|------|
| `goclaw gateway run` | 运行 WebSocket Gateway |
| `goclaw gateway install` | 安装为系统服务 |
| `goclaw gateway status` | 查看 Gateway 状态 |

### Cron 定时任务

| 命令 | 描述 |
|-----|------|
| `goclaw cron list` | 列出所有定时任务 |
| `goclaw cron add` | 添加定时任务 |
| `goclaw cron edit <id>` | 编辑定时任务 |
| `goclaw cron run <id>` | 立即运行任务 |

### Browser 自动化

| 命令 | 描述 |
|-----|------|
| `goclaw browser status` | 查看浏览器状态 |
| `goclaw browser open <url>` | 打开 URL |
| `goclaw browser screenshot` | 截图 |
| `goclaw browser click <selector>` | 点击元素 |

### 其他命令

| 命令 | 描述 |
|-----|------|
| `goclaw skills list` | 列出所有技能 |
| `goclaw sessions list` | 列出所有会话 |
| `goclaw memory status` | 查看记忆状态 |
| `goclaw logs` | 查看日志 |
| `goclaw health` | 健康检查 |
| `goclaw status` | 状态查看 |

详细的 CLI 文档请参考 [docs/cli.md](docs/cli.md)

## 架构概述

goclaw 采用模块化架构设计，主要组件包括：

![](docs/architecture.png)

### 核心组件

1. **Agent Loop** - 主循环，处理消息、调用工具、生成响应
2. **Message Bus** - 消息总线，连接各组件
3. **Channel Manager** - 通道管理器，管理多个消息通道
4. **Gateway** - WebSocket 网关，提供实时通信接口
5. **Tool Registry** - 工具注册表，管理所有可用工具
6. **Skills Loader** - 技能加载器，动态加载技能
7. **Session Manager** - 会话管理器，管理用户会话
8. **Cron Scheduler** - 定时任务调度器

### 通信流程

```
用户消息 → Channel → Message Bus → Agent Loop → LLM Provider
                                                     ↓
                                            Tool Registry → 工具执行
                                                     ↓
Agent Loop ← Message Bus ← Channel ← 响应消息
```

## 开发

### 添加新工具

在 `agent/tools/` 目录下创建新工具文件，实现 `Tool` 接口：

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() map[string]interface{}
    Execute(ctx context.Context, params map[string]interface{}) (string, error)
}
```

然后在 `cli/root.go` 或相关启动文件中注册工具。

### 添加新通道

在 `channels/` 目录下创建新通道，实现 `BaseChannel` 接口：

```go
type BaseChannel interface {
    Name() string
    Start(ctx context.Context) error
    Send(msg OutboundMessage) error
    IsAllowed(senderID string) bool
}
```

### 添加新 CLI 命令

1. 在 `cli/` 目录下创建新文件或添加到 `cli/commands/` 目录
2. 使用 `cobra` 创建命令
3. 在 `cli/root.go` 的 `init()` 函数中注册命令

### 环境变量

goclaw 支持以下环境变量（前缀 `GOSKILLS_`）：

| 变量 | 描述 |
|-----|------|
| `GOSKILLS_CONFIG_PATH` | 配置文件路径 |
| `GOSKILLS_WORKSPACE` | 工作区目录 (默认: `~/.goclaw/workspace`) |
| `ANTHROPIC_API_KEY` | Anthropic API Key |
| `OPENAI_API_KEY` | OpenAI API Key |
| `GOSKILLS_GATEWAY_URL` | Gateway WebSocket URL |
| `GOSKILLS_GATEWAY_TOKEN` | Gateway 认证 Token |

配置项可通过环境变量覆盖，例如：
- `GOSKILLS_AGENTS_DEFAULTS_MODEL` - 覆盖默认模型
- `GOSKILLS_TOOLS_SHELL_TIMEOUT` - 覆盖 Shell 工具超时时间

## 常见问题

### Q: 如何切换不同的 LLM 提供商？

A: 在 `models.providers` 中配置提供商，然后在 `agents.defaults.model.primary` 中指定使用哪个模型（格式：`provider/model`，冒号也继续受支持）：

```json
{
  "agents": {
    "defaults": {
      "model": {
        "primary": "qianfan/kimi-k2.5"
      }
    }
  },
  "models": {
    "providers": {
      "qianfan": {
        "baseUrl": "https://qianfan.baidubce.com/v2",
        "apiKey": "${QIANFAN_API_KEY}",
        "api": "openai-completions",
        "models": [{"id": "kimi-k2.5", "name": "Kimi K2.5"}]
      },
      "openai": {
        "baseUrl": "https://api.openai.com/v1",
        "apiKey": "${OPENAI_API_KEY}",
        "api": "openai-completions",
        "models": [{"id": "gpt-4o", "name": "GPT-4o"}]
      },
      "anthropic": {
        "baseUrl": "https://api.anthropic.com",
        "apiKey": "${ANTHROPIC_API_KEY}",
        "api": "anthropic-messages",
        "models": [{"id": "claude-sonnet-4-20250514", "name": "Claude Sonnet 4"}]
      }
    }
  }
}
```

支持的模型格式：
- `qianfan/kimi-k2.5` - Qianfan（百度千帆）
- `openai/gpt-4o` - OpenAI
- `anthropic/claude-sonnet-4-20250514` - Anthropic
- `openrouter/anthropic/claude-sonnet-4` - OpenRouter
- `requesty/anthropic/claude-sonnet-4-5` - Requesty

### Q: 工具调用失败怎么办？

A: 检查工具配置，确保 `enabled: true`，且没有权限限制。查看日志获取详细错误信息：

```bash
./goclaw logs -f
```

### Q: 如何限制 Shell 工具的权限？

A: 在配置中设置 `denied_cmds` 列表，添加危险的命令。也可以启用 Docker 沙箱：

```json
{
  "tools": {
    "shell": {
      "denied_cmds": ["rm -rf", "dd", "mkfs", "format", ":(){ :|:& };:"],
      "sandbox": {
        "enabled": true,
        "image": "golang:alpine",
        "remove": true
      }
    }
  }
}
```

### Q: 如何配置多个 LLM 提供商实现故障转移？

A: 在 `models.providers` 中配置多个提供商，Agent 会自动使用配置的模型：

```json
{
  "models": {
    "providers": {
      "qianfan": {
        "baseUrl": "https://qianfan.baidubce.com/v2",
        "apiKey": "${QIANFAN_API_KEY}",
        "api": "openai-completions",
        "models": [
          {"id": "kimi-k2.5", "name": "Kimi K2.5", "contextWindow": 131072, "maxTokens": 8192}
        ]
      },
      "openai": {
        "baseUrl": "https://api.openai.com/v1",
        "apiKey": "${OPENAI_API_KEY}",
        "api": "openai-completions",
        "models": [
          {"id": "gpt-4o", "name": "GPT-4o", "contextWindow": 128000, "maxTokens": 16384}
        ]
      },
      "anthropic": {
        "baseUrl": "https://api.anthropic.com",
        "apiKey": "${ANTHROPIC_API_KEY}",
        "api": "anthropic-messages",
        "models": [
          {"id": "claude-sonnet-4-20250514", "name": "Claude Sonnet 4", "contextWindow": 200000, "maxTokens": 8192}
        ]
      }
    }
  }
}
```

然后通过修改 `agents.defaults.model.primary` 切换不同的提供商，例如：
- `qianfan/kimi-k2.5` - 使用千帆
- `openai/gpt-4o` - 使用 OpenAI
- `anthropic/claude-sonnet-4-20250514` - 使用 Anthropic

### Q: Browser 工具需要什么依赖？

A: Browser 工具使用 Chrome DevTools Protocol，需要安装 Chrome 或 Chromium 浏览器：

```bash
# Ubuntu/Debian
sudo apt-get install chromium-browser

# macOS
brew install chromium

# 确保 Chrome/Chromium 在 PATH 中
which chromium
```

### Q: 如何调试 Agent 行为？

A: 使用 `--thinking` 参数查看思考过程，或查看日志：

```bash
./goclaw agent --message "测试" --thinking
./goclaw logs -f
```

### Q: 如何配置多个相同通道的账号？

A: 使用 `accounts` 字段配置多个账号实例：

```json
{
  "channels": {
    "telegram": {
      "accounts": {
        "bot1": {
          "enabled": true,
          "token": "bot1-token",
          "allowed_ids": ["user1"]
        },
        "bot2": {
          "enabled": true,
          "token": "bot2-token",
          "allowed_ids": ["user2"]
        }
      }
    }
  }
}
```

### Q: 如何配置微信 (Weixin) 通道？

A: 微信通道基于腾讯 OpenClaw-weixin 插件协议，需要先扫码登录：

```bash
# 1. 登录微信账号
./goclaw channels weixin login my-weixin

# 2. 查看登录状态
./goclaw channels weixin status my-weixin
```

配置示例：

```json
{
  "channels": {
    "weixin": {
      "enabled": true,
      "base_url": "https://ilinkai.weixin.qq.com",
      "cdn_base_url": "https://novac2c.cdn.weixin.qq.com/c2c",
      "allowed_ids": [],
      "accounts": {
        "my-weixin": {
          "enabled": true,
          "name": "我的微信",
          "allowed_ids": []
        }
      }
    }
  }
}
```

微信通道特性：
- 支持扫码登录获取 token
- 支持文本、图片、语音、视频、文件消息
- 支持消息收发和媒体文件传输
- 使用 AES-128-ECB 加密保护媒体文件
- Token 存储在 `~/.goclaw/weixin/accounts/<account_id>.json`

### Q: 记忆系统如何使用？

A: goclaw 支持两种记忆后端：

1. **内置向量数据库** (`builtin`)：
```json
{
  "memory": {
    "backend": "builtin",
    "builtin": {
      "enabled": true,
      "database_path": "",
      "auto_index": true
    }
  }
}
```

2. **QMD (Quick Markdown Database)**：
```json
{
  "memory": {
    "backend": "qmd",
    "qmd": {
      "command": "qmd",
      "enabled": true,
      "paths": [
        {
          "name": "notes",
          "path": "~/notes",
          "pattern": "**/*.md"
        }
      ]
    }
  }
}
```

## 相关文档

- [CLI 详细文档](docs/cli.md) - 完整的命令行参考
- [项目介绍](docs/INTRODUCTION.md) - 深入了解项目设计
- [OpenClaw 文档](https://docs.openclaw.ai) - 原始项目文档
- [AgentSkills 规范](https://agentskills.io) - 技能系统规范

## 许可证

MIT

---

Made with ❤️ by [smallnest](https://github.com/smallnest)
