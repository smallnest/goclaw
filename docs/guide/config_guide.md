# Configuration Guide

This guide covers configuring goclaw, including new features like multi-provider failover and WebSocket gateway.

## Basic Configuration

### Minimal Config

```json
{
  "agents": {
    "defaults": {
      "model": "gpt-4",
      "max_iterations": 15,
      "temperature": 0.7
    }
  },
  "providers": {
    "openai": {
      "api_key": "your-api-key"
    }
  },
  "channels": {
    "telegram": {
      "enabled": true,
      "token": "your-bot-token"
    }
  }
}
```

## Provider Configuration

### Single Provider

```json
{
  "providers": {
    "openai": {
      "api_key": "sk-...",
      "base_url": "https://api.openai.com/v1",
      "timeout": 30
    },
    "anthropic": {
      "api_key": "sk-ant-...",
      "base_url": "https://api.anthropic.com",
      "timeout": 30
    },
    "openrouter": {
      "api_key": "sk-or-...",
      "base_url": "https://openrouter.ai/api/v1",
      "timeout": 60,
      "max_retries": 3
    }
  }
}
```

### Multi-Provider Failover

Configure multiple API keys per provider with automatic failover:

```json
{
  "providers": {
    "failover": {
      "enabled": true,
      "strategy": "round_robin",
      "default_cooldown": "5m",
      "circuit_breaker": {
        "failure_threshold": 5,
        "timeout": "5m"
      }
    },
    "profiles": [
      {
        "name": "openai-primary",
        "provider": "openai",
        "api_key": "sk-primary-...",
        "priority": 1
      },
      {
        "name": "openai-backup",
        "provider": "openai",
        "api_key": "sk-backup-...",
        "priority": 2
      },
      {
        "name": "anthropic-fallback",
        "provider": "anthropic",
        "api_key": "sk-ant-...",
        "priority": 3
      }
    ]
  }
}
```

#### Rotation Strategies

- **round_robin**: Cycle through profiles in order
- **least_used**: Use profile with fewest requests
- **random**: Select profile randomly

#### Cooldown Behavior

When a profile fails:
- Auth errors (401, 403): 5 minute cooldown
- Rate limits (429): 5 minute cooldown
- Billing issues (402): 30 minute cooldown

## WebSocket Gateway Configuration

### Basic WebSocket Setup

```json
{
  "gateway": {
    "host": "0.0.0.0",
    "port": 8080,
    "read_timeout": "30s",
    "write_timeout": "30s",
    "websocket": {
      "host": "0.0.0.0",
      "port": 28789,
      "path": "/ws",
      "enable_auth": false
    }
  }
}
```

### WebSocket with Authentication

```json
{
  "gateway": {
    "websocket": {
      "enable_auth": true,
      "auth_token": "your-secure-token-here",
      "ping_interval": "30s",
      "pong_timeout": "60s",
      "read_timeout": "60s",
      "write_timeout": "10s"
    }
  }
}
```

### WebSocket with TLS

```json
{
  "gateway": {
    "websocket": {
      "enable_tls": true,
      "cert_file": "/etc/ssl/certs/goclaw.crt",
      "key_file": "/etc/ssl/private/goclaw.key"
    }
  }
}
```

## Channel Configuration

### Telegram

```json
{
  "channels": {
    "telegram": {
      "enabled": true,
      "token": "123456789:ABCdefGHIjklMNOpqrsTUVwxyz",
      "allowed_ids": ["123456789", "987654321"]
    }
  }
}
```

### WhatsApp

```json
{
  "channels": {
    "whatsapp": {
      "enabled": true,
      "bridge_url": "http://localhost:3000",
      "allowed_ids": ["123456789@c.us", "987654321@c.us"]
    }
  }
}
```

### Feishu

```json
{
  "channels": {
    "feishu": {
      "enabled": true,
      "app_id": "cli_xxxxxxxxx",
      "app_secret": "your-app-secret",
      "encrypt_key": "your-encrypt-key",
      "verification_token": "your-verification-token",
      "webhook_port": 8081,
      "allowed_ids": ["ou_xxxxxxxxx", "oc_xxxxxxxxx"]
    }
  }
}
```

### QQ (Official Bot API)

```json
{
  "channels": {
    "qq": {
      "enabled": true,
      "app_id": "123456789",
      "app_secret": "your-app-secret",
      "allowed_ids": ["123456789", "987654321"]
    }
  }
}
```

### WeWork

```json
{
  "channels": {
    "wework": {
      "enabled": true,
      "corp_id": "your-corp-id",
      "agent_id": "your-agent-id",
      "secret": "your-secret",
      "token": "your-token",
      "encoding_aes_key": "your-aes-key",
      "webhook_port": 8082,
      "allowed_ids": ["user-id", "department-id"]
    }
  }
}
```

## Agent Configuration

### Model Settings

```json
{
  "agents": {
    "defaults": {
      "model": "gpt-4",
      "max_iterations": 15,
      "temperature": 0.7,
      "max_tokens": 4096
    }
  }
}
```

### Model Selection

Models can be specified with prefixes:

- `gpt-4`: Use OpenAI
- `claude-3-opus-20240229`: Use Anthropic
- `openrouter:anthropic/claude-opus-4-5`: Use OpenRouter
- `openai:gpt-4-turbo`: Explicitly use OpenAI

## Tool Configuration

### File System Tool

```json
{
  "tools": {
    "filesystem": {
      "allowed_paths": ["/home/user", "/tmp"],
      "denied_paths": ["/etc", "/root"]
    }
  }
}
```

### Shell Tool

```json
{
  "tools": {
    "shell": {
      "enabled": true,
      "allowed_cmds": ["ls", "cat", "grep", "find"],
      "denied_cmds": ["rm", "dd", "mkfs"],
      "timeout": 30,
      "working_dir": "/home/user",
      "sandbox": {
        "enabled": false
      }
    }
  }
}
```

### Web Tool

```json
{
  "tools": {
    "web": {
      "search_api_key": "your-search-api-key",
      "search_engine": "google",
      "timeout": 30
    }
  }
}
```

### Browser Tool

```json
{
  "tools": {
    "browser": {
      "enabled": true,
      "headless": true,
      "timeout": 60
    }
  }
}
```

## Advanced Configuration

### Environment Variables

Configuration values can be overridden with environment variables:

```bash
export GOCLAW_OPENAI_API_KEY="sk-..."
export GOCLAW_TELEGRAM_TOKEN="123456789:ABC..."
export GOCLAW_WS_AUTH_TOKEN="secure-token"
```

### Configuration File Locations

goclaw searches for config in this order:
1. `~/.goclaw/config.json` (highest priority)
2. `./config.json` (current directory)
3. Path specified with `--config` flag

### Validation

Test your configuration:

```bash
goclaw config validate
```

### View Current Config

```bash
goclaw config show
```

## Common Patterns

### Development vs Production

**Development:**
```json
{
  "agents": {
    "defaults": {
      "model": "gpt-3.5-turbo",
      "temperature": 0.7
    }
  },
  "tools": {
    "shell": {
      "enabled": true
    }
  }
}
```

**Production:**
```json
{
  "agents": {
    "defaults": {
      "model": "gpt-4",
      "temperature": 0.5
    }
  },
  "providers": {
    "failover": {
      "enabled": true
    },
    "profiles": [...]
  },
  "gateway": {
    "websocket": {
      "enable_auth": true,
      "enable_tls": true
    }
  }
}
```

### High Availability Setup

```json
{
  "providers": {
    "failover": {
      "enabled": true,
      "strategy": "least_used",
      "default_cooldown": "5m"
    },
    "profiles": [
      {
        "name": "primary-region-1",
        "provider": "openai",
        "api_key": "${OPENAI_KEY_1}",
        "priority": 1
      },
      {
        "name": "backup-region-2",
        "provider": "openai",
        "api_key": "${OPENAI_KEY_2}",
        "priority": 2
      },
      {
        "name": "fallback-anthropic",
        "provider": "anthropic",
        "api_key": "${ANTHROPIC_KEY}",
        "priority": 3
      }
    ]
  }
}
```

### Cost Optimization

```json
{
  "agents": {
    "defaults": {
      "model": "openrouter:openai/gpt-3.5-turbo",
      "max_tokens": 2048
    }
  },
  "providers": {
    "failover": {
      "enabled": true,
      "strategy": "round_robin"
    },
    "profiles": [
      {
        "name": "openrouter-1",
        "provider": "openrouter",
        "api_key": "${OPENROUTER_KEY_1}",
        "priority": 1
      },
      {
        "name": "openrouter-2",
        "provider": "openrouter",
        "api_key": "${OPENROUTOR_KEY_2}",
        "priority": 2
      }
    ]
  }
}
```

## Troubleshooting Configuration

### Common Issues

1. **Provider not found**
   - Check API key is set
   - Verify provider name matches
   - Check model format

2. **WebSocket connection fails**
   - Verify port is not in use
   - Check firewall settings
   - Ensure correct protocol (ws:// vs wss://)

3. **Failover not working**
   - Ensure `failover.enabled: true`
   - Check profiles array is populated
   - Verify API keys are valid

4. **Channel not receiving messages**
   - Check `enabled: true`
   - Verify webhook URL/port
   - Check allowed_ids configuration

### Debug Mode

Enable debug logging:

```bash
goclaw --log-level debug start
```

### Configuration Reload

Hot reload configuration without restart:

```bash
goclaw config reload
```

## Security Best Practices

1. **Never commit API keys** to version control
2. **Use environment variables** for sensitive data
3. **Enable authentication** for WebSocket in production
4. **Use TLS** for WebSocket connections
5. **Restrict allowed_ids** for channels
6. **Rotate API keys** regularly
7. **Monitor provider usage** and costs
8. **Set appropriate timeouts** for all operations
