# GoClaw Quickstart

GoClaw (🐾 狗爪) 是一个功能强大的 AI Agent 框架，支持多种聊天平台，包括百度如流。

本指南将帮助你快速安装和配置 GoClaw，并将其作为如流机器人使用。

## 前置要求

- Go 1.21 或更高版本
- 一个如流机器人账号

## 快速安装

### 1. 克隆仓库

```bash
git clone https://github.com/smallnest/goclaw.git
cd goclaw
```

### 2. 编译项目

```bash
# 安装依赖
go mod tidy

# 编译二进制文件
go build -o goclaw .
```

或者使用 Makefile：

```bash
make build
```

编译完成后，会在当前目录生成 `goclaw` 可执行文件。

### 3. (可选) 安装到系统路径

```bash
make install
```

这将把 `goclaw` 安装到 `$GOPATH/bin` 或 `~/go/bin` 目录。

## 配置如流机器人

### 获取如流机器人凭证

首先，你需要在如流开放平台创建一个机器人，获取以下信息：

1. **Webhook URL**: 如流推送消息到你服务器的地址
2. **Token**: 机器人验证令牌
3. **AES Key**: 消息加密密钥（如果启用了加密）

### 创建配置文件

GoClaw 按以下顺序查找配置文件：

1. `~/.goclaw/config.json` (用户全局目录，**最高优先级**)
2. `./config.json` (当前目录)

创建配置文件 `config.json`：

```json
{
  "agents": {
    "defaults": {
      "model": {
        "primary": "qianfan/deepseek-v3.2"
      },
      "max_iterations": 15,
      "temperature": 0.7,
      "max_tokens": 4096
    }
  },
  "providers": {
    "qianfan": {
      "api_key": "YOUR_QIANFAN_API_KEY_HERE",
      "base_url": "https://qianfan.baidubce.com/v2",
      "timeout": 600
    }
  },
  "channels": {
    "infoflow": {
      "enabled": true,
      "webhook_url": "https://your-server.com/infoflow",
      "token": "your-infoflow-token",
      "aes_key": "your-aes-key",
      "webhook_port": 8767,
      "allowed_ids": []
    }
  },
  "tools": {
    "filesystem": {
      "allowed_paths": ["/home/user"],
      "denied_paths": ["/etc", "/root"]
    },
    "shell": {
      "enabled": false,
      "timeout": 30
    },
    "web": {
      "timeout": 30
    },
    "browser": {
      "enabled": false,
      "timeout": 30
    }
  }
}
```

### 配置说明

#### 如流通道配置项

| 参数 | 类型 | 必填 | 说明 |
|-----|------|-----|------|
| `enabled` | bool | 是 | 是否启用如流通道 |
| `webhook_url` | string | 是 | Webhook 回调地址（用于发送消息） |
| `token` | string | 是 | 机器人验证令牌 |
| `aes_key` | string | 否 | 消息加密密钥（如流平台提供） |
| `webhook_port` | int | 否 | Webhook 监听端口，默认 8767 |
| `allowed_ids` | []string | 否 | 允许访问的用户/群ID列表，为空则允许所有 |

#### LLM 提供商配置

GoClaw 支持多个 LLM 提供商：

**Qianfan（百度千帆）:**
```json
{
  "providers": {
    "qianfan": {
      "api_key": "bce-v3/...",
      "base_url": "https://qianfan.baidubce.com/v2"
    }
  }
}
```

**OpenAI:**
```json
{
  "providers": {
    "openai": {
      "api_key": "sk-...",
      "base_url": "https://api.openai.com/v1"
    }
  }
}
```

**Anthropic:**
```json
{
  "providers": {
    "anthropic": {
      "api_key": "sk-ant-...",
      "base_url": "https://api.anthropic.com"
    }
  }
}
```

**OpenRouter:**
```json
{
  "providers": {
    "openrouter": {
      "api_key": "sk-or-...",
      "base_url": "https://openrouter.ai/api/v1"
    }
  }
}
```

**Requesty:**
```json
{
  "providers": {
    "requesty": {
      "api_key": "sk-...",
      "base_url": "https://router.requesty.ai/v1"
    }
  }
}
```

#### 模型选择

通过修改 `model` 参数选择不同的模型：

- `qianfan:deepseek-v3.2` - 百度千帆上的 DeepSeek 模型
- `openai:gpt-4` - OpenAI GPT-4
- `openai:gpt-3.5-turbo` - OpenAI GPT-3.5
- `claude-3-opus-20240229` - Anthropic Claude 3 Opus
- `openrouter:anthropic/claude-opus-4-5` - 通过 OpenRouter 使用指定模型
- `requesty:anthropic/claude-sonnet-4-5` - 通过 Requesty 使用指定模型

### 多账号配置

如果你需要配置多个如流机器人账号，可以使用以下格式：

```json
{
  "channels": {
    "infoflow": {
      "enabled": true,
      "accounts": {
        "bot1": {
          "enabled": true,
          "name": "主机器人",
          "webhook_url": "https://server1.com/infoflow",
          "token": "token1",
          "aes_key": "aeskey1",
          "webhook_port": 8767,
          "allowed_ids": ["user1", "group1"]
        },
        "bot2": {
          "enabled": true,
          "name": "备用机器人",
          "webhook_url": "https://server2.com/infoflow",
          "token": "token2",
          "webhook_port": 8768,
          "allowed_ids": ["user2"]
        }
      }
    }
  }
}
```

## 运行和测试

### 启动 GoClaw

```bash
# 使用默认配置文件启动
./goclaw start

# 指定配置文件路径启动
./goclaw start --config /path/to/config.json

# 以调试模式启动
./goclaw start --log-level debug
```

### 验证连接

在如流中发送消息给你的机器人，尝试以下命令：

```
/help
```

机器人会返回帮助信息：

```
Infoflow 机器人帮助:

可用命令:
  /help - 显示此帮助信息
  /status - 查看机器人状态

直接发送消息即可与 AI 助手进行对话。
```

### 查看运行状态

```bash
# 查看所有通道状态
./goclaw channels status

# 查看配置
./goclaw config show

# 查看日志
./goclaw logs -f
```

## 使用场景

### 1. TUI 交互模式

直接在命令行与 AI 助手交互：

```bash
./goclaw tui
```

### 2. 单次执行

```bash
./goclaw agent --message "介绍一下你自己"
```

### 3. 列出可用技能

```bash
./goclaw skills list
```

### 4. 查看会话历史

```bash
./goclaw sessions list
```

## 常见问题

### Q: 如何限制机器人只响应特定用户/群？

A: 在配置文件中设置 `allowed_ids` 参数：

```json
{
  "channels": {
    "infoflow": {
      "allowed_ids": ["user-id-1", "group-id-1"]
    }
  }
}
```

### Q: 机器人不响应消息怎么办？

A: 检查以下几点：

1. 确认 `enabled` 为 `true`
2. 检查 `token` 和 `webhook_url` 是否正确
3. 确认服务器端口 `webhook_port` 可访问
4. 查看日志：`./goclaw logs -f`
5. 检查防火墙设置

### Q: 如何更换 LLM 提供商？

A: 修改配置文件中的 `model` 和 `providers` 部分：

```json
{
  "agents": {
    "defaults": {
      "model": {
        "primary": "qianfan/deepseek-v3.2"
      }
    }
  },
  "providers": {
    "qianfan": {
      "api_key": "bce-v3/...",
      "base_url": "https://qianfan.baidubce.com/v2"
    }
  }
}
```

### Q: 如何启用更多工具（如 Shell、Browser）？

A: 修改配置文件中的 `tools` 部分：

```json
{
  "tools": {
    "shell": {
      "enabled": true,
      "allowed_cmds": ["ls", "cat", "grep"],
      "denied_cmds": ["rm -rf", "dd", "mkfs", "format"],
      "timeout": 30
    },
    "browser": {
      "enabled": true,
      "headless": true,
      "timeout": 60
    }
  }
}
```

**注意**: 启用 Shell 和 Browser 工具可能带来安全风险，请谨慎配置。

### Q: 如何查看机器人状态？

A: 在如流中发送 `/status` 命令，或使用 CLI：

```bash
./goclaw status
```

### Q: webhook_port 无法绑定？

A: 检查端口是否被占用，或更换其他端口：

```bash
# 检查端口占用
lsof -i :8767

# 更换端口
{
  "channels": {
    "infoflow": {
      "webhook_port": 9767
    }
  }
}
```

## 下一步

- 阅读 [配置指南](./config_guide.md) 了解更多高级配置选项
- 查看 [CLI 文档](../cli.md) 了解所有可用命令
- 了解 [技能系统](https://github.com/openclaw/openclaw) 扩展机器人能力

## 获取帮助

如果遇到问题，可以：

1. 查看日志：`./goclaw logs -f`
2. 使用调试模式：`./goclaw start --log-level debug`
3. 访问项目 GitHub: https://github.com/smallnest/goclaw
4. 查看文档: https://github.com/smallnest/goclaw/tree/master/docs
