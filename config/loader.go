package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

var globalConfig *Config

// Load 加载配置文件
func Load(configPath string) (*Config, error) {
	// 创建 viper 实例
	v := viper.New()

	// 设置配置文件路径
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		// 默认配置文件路径
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}

		configDir := filepath.Join(home, ".goclaw")
		v.AddConfigPath(configDir)
		v.AddConfigPath(".")
		v.SetConfigName("config")
		v.SetConfigType("json")
	}

	// 设置环境变量前缀
	v.SetEnvPrefix("GOSKILLS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// 设置默认值
	setDefaults(v)

	// 读取配置文件
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
		// 配置文件不存在，使用默认值和环境变量
	}

	// 解析配置
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	globalConfig = &cfg
	return &cfg, nil
}

// setDefaults 设置默认配置值
func setDefaults(v *viper.Viper) {
	// Agent 默认配置
	v.SetDefault("agents.defaults.model", "openrouter:anthropic/claude-opus-4-5")
	v.SetDefault("agents.defaults.max_iterations", 15)
	v.SetDefault("agents.defaults.temperature", 0.7)
	v.SetDefault("agents.defaults.max_tokens", 4096)

	// Gateway 默认配置
	v.SetDefault("gateway.host", "localhost")
	v.SetDefault("gateway.port", 8080)
	v.SetDefault("gateway.read_timeout", 30)
	v.SetDefault("gateway.write_timeout", 30)

	// 工具默认配置
	v.SetDefault("tools.shell.enabled", true)
	v.SetDefault("tools.shell.timeout", 120)
	v.SetDefault("tools.shell.sandbox.enabled", false)
	v.SetDefault("tools.shell.sandbox.image", "goclaw/sandbox:latest")
	v.SetDefault("tools.shell.sandbox.workdir", "/workspace")
	v.SetDefault("tools.shell.sandbox.remove", true)
	v.SetDefault("tools.shell.sandbox.network", "none")
	v.SetDefault("tools.shell.sandbox.privileged", false)
	v.SetDefault("tools.web.search_engine", "travily")
	v.SetDefault("tools.web.timeout", 10)
	v.SetDefault("tools.browser.enabled", false)
	v.SetDefault("browser.headless", true)
	v.SetDefault("browser.timeout", 30)
}

// Save 保存配置到文件
func Save(cfg *Config, path string) error {
	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// 转换为 JSON（带缩进）
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Get 获取全局配置
func Get() *Config {
	return globalConfig
}

// GetDefaultConfigPath 获取默认配置文件路径
func GetDefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".goclaw", "config.json"), nil
}

// GetWorkspacePath 获取 workspace 目录路径
func GetWorkspacePath(cfg *Config) (string, error) {
	if cfg.Workspace.Path != "" {
		// 使用配置中的自定义路径
		return cfg.Workspace.Path, nil
	}
	// 使用默认路径
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".goclaw", "workspace"), nil
}

// Validate 验证配置 (使用新的验证器)
func Validate(cfg *Config) error {
	validator := NewValidator(true)
	return validator.Validate(cfg)
}

// HasProvider 检查配置中是否有指定的提供商
func HasProvider(cfg *Config, provider string) bool {
	if cfg == nil {
		return false
	}
	switch provider {
	case "openai":
		return cfg.Providers.OpenAI.APIKey != ""
	case "anthropic":
		return cfg.Providers.Anthropic.APIKey != ""
	case "openrouter":
		return cfg.Providers.OpenRouter.APIKey != ""
	default:
		return false
	}
}

// GetGatewayWebSocketURL 获取 Gateway WebSocket URL
func GetGatewayWebSocketURL(cfg *Config) string {
	if cfg == nil {
		return "ws://localhost:28789/ws"
	}

	port := cfg.Gateway.WebSocket.Port
	if port == 0 {
		port = 28789
	}

	host := cfg.Gateway.WebSocket.Host
	if host == "" {
		host = "localhost"
	}

	path := cfg.Gateway.WebSocket.Path
	if path == "" {
		path = "/ws"
	}

	return fmt.Sprintf("ws://%s:%d%s", host, port, path)
}

// GetGatewayHTTPPort 获取 Gateway HTTP 端口
func GetGatewayHTTPPort(cfg *Config) int {
	if cfg == nil {
		return 28789
	}

	port := cfg.Gateway.WebSocket.Port
	if port == 0 {
		port = 28789
	}
	return port
}
