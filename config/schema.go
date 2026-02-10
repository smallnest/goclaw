package config

import (
	"time"
)

// Config 是主配置结构
type Config struct {
	Agents    AgentsConfig    `mapstructure:"agents" json:"agents"`
	Channels  ChannelsConfig  `mapstructure:"channels" json:"channels"`
	Providers ProvidersConfig `mapstructure:"providers" json:"providers"`
	Gateway   GatewayConfig   `mapstructure:"gateway" json:"gateway"`
	Tools     ToolsConfig     `mapstructure:"tools" json:"tools"`
	Approvals ApprovalsConfig `mapstructure:"approvals" json:"approvals"`
}

// AgentsConfig Agent 配置
type AgentsConfig struct {
	Defaults AgentDefaults `mapstructure:"defaults" json:"defaults"`
}

// AgentDefaults Agent 默认配置
type AgentDefaults struct {
	Model         string  `mapstructure:"model" json:"model"`
	MaxIterations int     `mapstructure:"max_iterations" json:"max_iterations"`
	Temperature   float64 `mapstructure:"temperature" json:"temperature"`
	MaxTokens     int     `mapstructure:"max_tokens" json:"max_tokens"`
}

// ChannelsConfig 通道配置
type ChannelsConfig struct {
	Telegram TelegramChannelConfig `mapstructure:"telegram" json:"telegram"`
	WhatsApp WhatsAppChannelConfig `mapstructure:"whatsapp" json:"whatsapp"`
	Feishu   FeishuChannelConfig   `mapstructure:"feishu" json:"feishu"`
	QQ       QQChannelConfig       `mapstructure:"qq" json:"qq"`
	WeWork   WeWorkChannelConfig   `mapstructure:"wework" json:"wework"`
}

// TelegramChannelConfig Telegram 通道配置
type TelegramChannelConfig struct {
	Enabled    bool     `mapstructure:"enabled" json:"enabled"`
	Token       string   `mapstructure:"token" json:"token"`
	AllowedIDs []string `mapstructure:"allowed_ids" json:"allowed_ids"`
}

// WhatsAppChannelConfig WhatsApp 通道配置
type WhatsAppChannelConfig struct {
	Enabled    bool     `mapstructure:"enabled" json:"enabled"`
	BridgeURL  string   `mapstructure:"bridge_url" json:"bridge_url"`
	AllowedIDs []string `mapstructure:"allowed_ids" json:"allowed_ids"`
}

// FeishuChannelConfig 飞书通道配置
type FeishuChannelConfig struct {
	Enabled           bool     `mapstructure:"enabled" json:"enabled"`
	AppID             string   `mapstructure:"app_id" json:"app_id"`
	AppSecret         string   `mapstructure:"app_secret" json:"app_secret"`
	EncryptKey        string   `mapstructure:"encrypt_key" json:"encrypt_key"`
	VerificationToken string   `mapstructure:"verification_token" json:"verification_token"`
	WebhookPort       int      `mapstructure:"webhook_port" json:"webhook_port"`
	AllowedIDs        []string `mapstructure:"allowed_ids" json:"allowed_ids"`
}

// QQChannelConfig QQ 通道配置 (QQ 开放平台官方 Bot API)
type QQChannelConfig struct {
	Enabled    bool     `mapstructure:"enabled" json:"enabled"`
	AppID      string   `mapstructure:"app_id" json:"app_id"`           // QQ 机器人 AppID
	AppSecret  string   `mapstructure:"app_secret" json:"app_secret"`       // AppSecret (ClientSecret)
	AllowedIDs []string `mapstructure:"allowed_ids" json:"allowed_ids"`    // 允许的用户/群ID列表
}

// WeWorkChannelConfig 企业微信通道配置
type WeWorkChannelConfig struct {
	Enabled        bool     `mapstructure:"enabled" json:"enabled"`
	CorpID         string   `mapstructure:"corp_id" json:"corp_id"`
	AgentID        string   `mapstructure:"agent_id" json:"agent_id"`
	Secret         string   `mapstructure:"secret" json:"secret"`
	Token          string   `mapstructure:"token" json:"token"`
	EncodingAESKey string   `mapstructure:"encoding_aes_key" json:"encoding_aes_key"`
	WebhookPort    int      `mapstructure:"webhook_port" json:"webhook_port"`
	AllowedIDs     []string `mapstructure:"allowed_ids" json:"allowed_ids"`
}

// ProvidersConfig LLM 提供商配置
type ProvidersConfig struct {
	OpenRouter OpenRouterProviderConfig `mapstructure:"openrouter" json:"openrouter"`
	OpenAI     OpenAIProviderConfig     `mapstructure:"openai" json:"openai"`
	Anthropic  AnthropicProviderConfig  `mapstructure:"anthropic" json:"anthropic"`
	Profiles   []ProviderProfileConfig  `mapstructure:"profiles" json:"profiles"`
	Failover   FailoverConfig           `mapstructure:"failover" json:"failover"`
}

// ProviderProfileConfig 提供商配置
type ProviderProfileConfig struct {
	Name     string `mapstructure:"name" json:"name"`
	Provider string `mapstructure:"provider" json:"provider"` // openai, anthropic, openrouter
	APIKey   string `mapstructure:"api_key" json:"api_key"`
	BaseURL  string `mapstructure:"base_url" json:"base_url"`
	Priority int    `mapstructure:"priority" json:"priority"`
}

// FailoverConfig 故障转移配置
type FailoverConfig struct {
	Enabled         bool          `mapstructure:"enabled" json:"enabled"`
	Strategy        string        `mapstructure:"strategy" json:"strategy"` // round_robin, least_used, random
	DefaultCooldown time.Duration `mapstructure:"default_cooldown" json:"default_cooldown"`
	CircuitBreaker  CircuitBreakerConfig `mapstructure:"circuit_breaker" json:"circuit_breaker"`
}

// CircuitBreakerConfig 断路器配置
type CircuitBreakerConfig struct {
	FailureThreshold int           `mapstructure:"failure_threshold" json:"failure_threshold"`
	Timeout          time.Duration `mapstructure:"timeout" json:"timeout"`
}

// OpenRouterProviderConfig OpenRouter 配置
type OpenRouterProviderConfig struct {
	APIKey     string  `mapstructure:"api_key" json:"api_key"`
	BaseURL    string  `mapstructure:"base_url" json:"base_url"`
	Timeout    int     `mapstructure:"timeout" json:"timeout"`
	MaxRetries int     `mapstructure:"max_retries" json:"max_retries"`
}

// OpenAIProviderConfig OpenAI 配置
type OpenAIProviderConfig struct {
	APIKey  string `mapstructure:"api_key" json:"api_key"`
	BaseURL string `mapstructure:"base_url" json:"base_url"`
	Timeout int    `mapstructure:"timeout" json:"timeout"`
}

// AnthropicProviderConfig Anthropic 配置
type AnthropicProviderConfig struct {
	APIKey  string `mapstructure:"api_key" json:"api_key"`
	BaseURL string `mapstructure:"base_url" json:"base_url"`
	Timeout int    `mapstructure:"timeout" json:"timeout"`
}

// GatewayConfig 网关配置
type GatewayConfig struct {
	Host         string            `mapstructure:"host" json:"host"`
	Port         int               `mapstructure:"port" json:"port"`
	ReadTimeout  time.Duration     `mapstructure:"read_timeout" json:"read_timeout"`
	WriteTimeout time.Duration     `mapstructure:"write_timeout" json:"write_timeout"`
	WebSocket    WebSocketConfig   `mapstructure:"websocket" json:"websocket"`
}

// WebSocketConfig WebSocket 配置
type WebSocketConfig struct {
	Host         string        `mapstructure:"host" json:"host"`
	Port         int           `mapstructure:"port" json:"port"`
	Path         string        `mapstructure:"path" json:"path"`
	EnableAuth   bool          `mapstructure:"enable_auth" json:"enable_auth"`
	AuthToken    string        `mapstructure:"auth_token" json:"auth_token"`
	PingInterval time.Duration `mapstructure:"ping_interval" json:"ping_interval"`
	PongTimeout  time.Duration `mapstructure:"pong_timeout" json:"pong_timeout"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout" json:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout" json:"write_timeout"`
}

// ToolsConfig 工具配置
type ToolsConfig struct {
	FileSystem FileSystemToolConfig `mapstructure:"filesystem" json:"filesystem"`
	Shell      ShellToolConfig      `mapstructure:"shell" json:"shell"`
	Web        WebToolConfig        `mapstructure:"web" json:"web"`
	Browser    BrowserToolConfig    `mapstructure:"browser" json:"browser"`
}

// FileSystemToolConfig 文件系统工具配置
type FileSystemToolConfig struct {
	AllowedPaths []string `mapstructure:"allowed_paths" json:"allowed_paths"`
	DeniedPaths  []string `mapstructure:"denied_paths" json:"denied_paths"`
}

// ShellToolConfig Shell 工具配置
type ShellToolConfig struct {
	Enabled       bool           `mapstructure:"enabled" json:"enabled"`
	AllowedCmds   []string       `mapstructure:"allowed_cmds" json:"allowed_cmds"`
	DeniedCmds    []string       `mapstructure:"denied_cmds" json:"denied_cmds"`
	Timeout       int            `mapstructure:"timeout" json:"timeout"`
	WorkingDir    string         `mapstructure:"working_dir" json:"working_dir"`
	Sandbox       SandboxConfig  `mapstructure:"sandbox" json:"sandbox"`
}

// SandboxConfig Docker 沙箱配置
type SandboxConfig struct {
	Enabled    bool   `mapstructure:"enabled" json:"enabled"`
	Image      string `mapstructure:"image" json:"image"`
	Workdir    string `mapstructure:"workdir" json:"workdir"`
	Remove     bool   `mapstructure:"remove" json:"remove"`
	Network    string `mapstructure:"network" json:"network"`
	Privileged bool   `mapstructure:"privileged" json:"privileged"`
}

// WebToolConfig Web 工具配置
type WebToolConfig struct {
	SearchAPIKey string `mapstructure:"search_api_key" json:"search_api_key"`
	SearchEngine string `mapstructure:"search_engine" json:"search_engine"`
	Timeout      int    `mapstructure:"timeout" json:"timeout"`
}

// BrowserToolConfig 浏览器工具配置
type BrowserToolConfig struct {
	Enabled  bool `mapstructure:"enabled" json:"enabled"`
	Headless bool `mapstructure:"headless" json:"headless"`
	Timeout  int  `mapstructure:"timeout" json:"timeout"`
}

// ApprovalsConfig 审批配置
type ApprovalsConfig struct {
	Behavior  string   `mapstructure:"behavior" json:"behavior"`   // auto, manual, prompt
	Allowlist []string `mapstructure:"allowlist" json:"allowlist"` // 工具允许列表
}
