# Ollama 本地大模型配置指南

## 概述

本项目已支持调用本地 Ollama 运行的大模型。Ollama 是一个简单易用的本地大模型运行工具，可以在本地计算机上运行各种开源大语言模型。

## 前置条件

### 1. 安装 Ollama

访问 [Ollama 官网](https://ollama.ai/) 下载并安装 Ollama。

### 2. 下载模型

启动 Ollama 后，使用以下命令下载你需要的模型：

```bash
# 下载 Llama 3.2 模型
ollama pull llama3.2

# 下载 Qwen 2.5 模型
ollama pull qwen2.5

# 下载 Mistral 模型
ollama pull mistral

# 查看更多可用模型
ollama list
```

### 3. 启动 Ollama 服务

Ollama 安装后会自动运行一个本地服务，默认地址为 `http://localhost:11434`。

你可以通过以下命令验证服务是否正常运行：

```bash
curl http://localhost:11434/api/tags
```

## 配置方法

在你的 `config.json` 文件中添加 Ollama 提供商配置：

```json
{
  "agents": {
    "defaults": {
      "model": {
        "primary": "ollama:llama3.2"
      },
      "max_iterations": 20,
      "temperature": 0.7,
      "max_tokens": 8192
    }
  },
  "models": {
    "mode": "merge",
    "providers": {
      "ollama": {
        "baseUrl": "http://localhost:11434",
        "api": "ollama",
        "models": [
          {
            "id": "llama3.2",
            "name": "Llama 3.2",
            "contextWindow": 131072,
            "maxTokens": 8192,
            "input": ["text"]
          },
          {
            "id": "qwen2.5",
            "name": "Qwen 2.5",
            "contextWindow": 131072,
            "maxTokens": 8192,
            "input": ["text"]
          },
          {
            "id": "mistral",
            "name": "Mistral",
            "contextWindow": 32768,
            "maxTokens": 8192,
            "input": ["text"]
          }
        ]
      }
    }
  }
}
```

## 配置说明

### 必填字段

- `baseUrl`: Ollama 服务地址，默认为 `http://localhost:11434`
- `api`: 必须设置为 `"ollama"`
- `models`: 模型列表，每个模型需要配置：
  - `id`: 模型 ID（与 Ollama 中的模型名称一致）
  - `name`: 模型显示名称
  - `contextWindow`: 上下文窗口大小
  - `maxTokens`: 最大输出 token 数
  - `input`: 支持的输入类型（`text` 或 `image`）

### 使用不同模型

在 `agents.defaults.model.primary` 中指定要使用的模型：

```json
"primary": "ollama:llama3.2"  // 使用 Llama 3.2
"primary": "ollama:qwen2.5"   // 使用 Qwen 2.5
"primary": "ollama:mistral"   // 使用 Mistral
```

## 常见问题

### 1. 连接失败

如果提示无法连接到 Ollama 服务，请检查：

- Ollama 是否已启动运行
- 服务地址是否正确（默认 `http://localhost:11434`）
- 防火墙是否阻止了连接

### 2. 模型不存在

如果提示模型不存在，请确保已使用 `ollama pull` 下载该模型：

```bash
ollama pull llama3.2
```

### 3. 性能优化

对于本地部署，建议：

- 确保有足够的内存（至少 8GB，推荐 16GB 以上）
- 使用 GPU 加速（如果可用）
- 根据硬件配置选择合适的模型大小

## 验证配置

启动应用后，你可以通过日志确认 Ollama 提供商是否正确加载：

```
INFO Creating Ollama provider model=llama3.2 url=http://localhost:11434 maxTokens=8192
INFO Ollama service is available url=http://localhost:11434
```

## 高级配置

### 自定义 Ollama 地址

如果你的 Ollama 服务运行在其他地址或端口，可以修改 `baseUrl`：

```json
"ollama": {
  "baseUrl": "http://192.168.1.100:11434",
  "api": "ollama",
  ...
}
```

### 多模型配置

你可以在一个提供商中配置多个模型，并在不同的 agent 中使用不同的模型：

```json
"agents": {
  "defaults": {
    "model": {
      "primary": "ollama:llama3.2"
    }
  },
  "agents": {
    "coder": {
      "model": {
        "primary": "ollama:qwen2.5-coder"
      }
    }
  }
}
```

## 技术支持

如遇到问题，请查看应用日志或访问 [Ollama 官方文档](https://ollama.ai/docs) 获取帮助。
