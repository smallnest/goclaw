package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/smallnest/goclaw/errors"
)

// Validator provides configuration validation
type Validator struct {
	strictMode bool
}

// NewValidator creates a new configuration validator
func NewValidator(strict bool) *Validator {
	return &Validator{
		strictMode: strict,
	}
}

// Validate performs comprehensive configuration validation
func (v *Validator) Validate(cfg *Config) error {
	if cfg == nil {
		return errors.InvalidConfig("configuration cannot be nil")
	}

	// Validate in order of dependency
	validators := []func(*Config) error{
		v.validateWorkspace,
		v.validateAgents,
		v.validateProviders,
		v.validateChannels,
		v.validateTools,
		v.validateGateway,
		v.validateMemory,
	}

	for _, validator := range validators {
		if err := validator(cfg); err != nil {
			return err
		}
	}

	return nil
}

// validateWorkspace validates workspace configuration
func (v *Validator) validateWorkspace(cfg *Config) error {
	// Check workspace path
	if cfg.Workspace.Path != "" {
		// Check if path is absolute
		if !filepath.IsAbs(cfg.Workspace.Path) {
			return errors.InvalidConfig("workspace path must be absolute")
		}

		// Check if directory exists or can be created
		if err := os.MkdirAll(cfg.Workspace.Path, 0755); err != nil {
			return errors.Wrap(err, errors.ErrCodeInvalidConfig,
				"cannot create workspace directory")
		}
	}

	return nil
}

// validateAgents validates agent configuration
func (v *Validator) validateAgents(cfg *Config) error {
	// Check default configuration
	if err := v.validateAgentDefaults(&cfg.Agents.Defaults); err != nil {
		return errors.Wrap(err, errors.ErrCodeInvalidConfig, "invalid agent defaults")
	}

	// Check individual agents
	agentIDs := make(map[string]bool)
	for i, agent := range cfg.Agents.List {
		if agent.ID == "" {
			return errors.InvalidConfig(fmt.Sprintf("agent at index %d has empty ID", i))
		}

		// Check for duplicate IDs
		if agentIDs[agent.ID] {
			return errors.InvalidConfig(fmt.Sprintf("duplicate agent ID: %s", agent.ID))
		}
		agentIDs[agent.ID] = true

		// Validate agent configuration
		if err := v.validateAgentConfig(&agent); err != nil {
			return errors.Wrap(err, errors.ErrCodeInvalidConfig,
				fmt.Sprintf("invalid agent '%s'", agent.ID))
		}
	}

	// Check bindings
	for _, binding := range cfg.Bindings {
		if !agentIDs[binding.AgentID] {
			return errors.InvalidConfig(fmt.Sprintf("binding references non-existent agent: %s",
				binding.AgentID))
		}
	}

	return nil
}

// validateAgentDefaults validates default agent configuration
func (v *Validator) validateAgentDefaults(defaults *AgentDefaults) error {
	// Check model
	if strings.TrimSpace(defaults.Model.Effective()) == "" {
		return errors.InvalidConfig("default agent model cannot be empty")
	}

	// Check max iterations
	if defaults.MaxIterations < 1 || defaults.MaxIterations > 100 {
		return errors.InvalidConfig("max_iterations must be between 1 and 100")
	}

	// Check temperature
	if defaults.Temperature < 0 || defaults.Temperature > 2 {
		return errors.InvalidConfig("temperature must be between 0 and 2")
	}

	// Check max tokens
	if defaults.MaxTokens < 1 || defaults.MaxTokens > 128000 {
		return errors.InvalidConfig("max_tokens must be between 1 and 128000")
	}

	// Validate retry configuration
	if defaults.Retry != nil {
		if err := v.validateRetryConfig(defaults.Retry); err != nil {
			return err
		}
	}

	// Validate subagents configuration
	if defaults.Subagents != nil {
		if err := v.validateSubagentsDefaults(defaults.Subagents); err != nil {
			return err
		}
	}

	return nil
}

// validateRetryConfig validates retry configuration
func (v *Validator) validateRetryConfig(retry *RetryConfig) error {
	if retry.MaxRetries < 0 || retry.MaxRetries > 10 {
		return errors.InvalidConfig("retry.max_retries must be between 0 and 10")
	}

	if retry.InitialDelay < 0 || retry.InitialDelay > 60*time.Second {
		return errors.InvalidConfig("retry.initial_delay must be between 0 and 60 seconds")
	}

	if retry.MaxDelay < 0 || retry.MaxDelay > 300*time.Second {
		return errors.InvalidConfig("retry.max_delay must be between 0 and 300 seconds")
	}

	if retry.BackoffFactor < 1.0 || retry.BackoffFactor > 10.0 {
		return errors.InvalidConfig("retry.backoff_factor must be between 1.0 and 10.0")
	}

	return nil
}

// validateSubagentsDefaults validates subagents configuration in defaults
func (v *Validator) validateSubagentsDefaults(subagents *SubagentsConfig) error {
	if subagents.MaxConcurrent < 1 || subagents.MaxConcurrent > 10 {
		return errors.InvalidConfig("subagents.max_concurrent must be between 1 and 10")
	}

	if subagents.ArchiveAfterMinutes < 1 || subagents.ArchiveAfterMinutes > 1440 {
		return errors.InvalidConfig("subagents.archive_after_minutes must be between 1 and 1440 (24 hours)")
	}

	if subagents.TimeoutSeconds < 60 || subagents.TimeoutSeconds > 3600 {
		return errors.InvalidConfig("subagents.timeout_seconds must be between 60 and 3600")
	}

	return nil
}

// validateAgentConfig validates individual agent configuration
func (v *Validator) validateAgentConfig(agent *AgentConfig) error {
	// Check model
	if strings.TrimSpace(agent.Model) == "" {
		return errors.InvalidConfig("agent model cannot be empty")
	}

	// Validate subagents configuration
	if agent.Subagents != nil {
		if err := v.validateSubagentsConfig(agent.Subagents); err != nil {
			return err
		}
	}

	return nil
}

// validateSubagentsConfig validates subagent configuration
func (v *Validator) validateSubagentsConfig(subagents *AgentSubagentConfig) error {
	// Check timeout
	if subagents.TimeoutSeconds < 1 || subagents.TimeoutSeconds > 3600 {
		return errors.InvalidConfig("subagent timeout must be between 1 and 3600 seconds")
	}

	// Check allowed tools and denied tools don't overlap
	for _, allowed := range subagents.AllowTools {
		if slices.Contains(subagents.DenyTools, allowed) {
			return errors.InvalidConfig(fmt.Sprintf(
				"tool '%s' is both allowed and denied", allowed))
		}
	}

	return nil
}

// validateProviders validates LLM provider configuration
func (v *Validator) validateProviders(cfg *Config) error {
	// Check if at least one provider is configured in models.providers
	if !cfg.Models.HasProviders() {
		return errors.InvalidConfig("at least one LLM provider must be configured in models.providers")
	}

	// Validate models.mode
	if cfg.Models.Mode != "" && cfg.Models.Mode != "merge" && cfg.Models.Mode != "replace" {
		return errors.InvalidConfig("models.mode must be 'merge' or 'replace'")
	}

	// Validate each provider in models.providers
	for providerName, provider := range cfg.Models.Providers {
		if provider.BaseURL == "" {
			return errors.InvalidConfig(fmt.Sprintf("provider '%s' has empty baseUrl", providerName))
		}

		// API key can be an environment variable reference, so empty is allowed
		if provider.APIKey != "" {
			if err := v.validateAPIKey(provider.APIKey); err != nil {
				return errors.Wrap(err, errors.ErrCodeInvalidConfig,
					fmt.Sprintf("invalid API key for provider '%s'", providerName))
			}
		}

		// Validate models
		for i, model := range provider.Models {
			if model.ID == "" {
				return errors.InvalidConfig(fmt.Sprintf("provider '%s' model at index %d has empty id", providerName, i))
			}
			if model.Name == "" {
				return errors.InvalidConfig(fmt.Sprintf("provider '%s' model '%s' has empty name", providerName, model.ID))
			}
		}
	}

	return nil
}

// validateAPIKey validates API key format
func (v *Validator) validateAPIKey(key string) error {
	key = strings.TrimSpace(key)

	if len(key) < 10 {
		return errors.InvalidInput("API key too short (minimum 10 characters)")
	}

	if strings.Contains(key, " ") {
		return errors.InvalidInput("API key cannot contain spaces")
	}

	return nil
}

// validateChannels validates channel configuration
func (v *Validator) validateChannels(cfg *Config) error {
	validators := []func(*ChannelsConfig) error{
		v.validateTelegram,
		v.validateWhatsApp,
		v.validateFeishu,
		v.validateQQ,
		v.validateWeWork,
		v.validateDingTalk,
		v.validateInfoflow,
	}

	for _, validator := range validators {
		if err := validator(&cfg.Channels); err != nil {
			return err
		}
	}

	return nil
}

// validateTelegram validates Telegram channel configuration
func (v *Validator) validateTelegram(channels *ChannelsConfig) error {
	if !channels.Telegram.Enabled {
		return nil
	}

	if channels.Telegram.Token == "" {
		return errors.InvalidConfig("telegram token is required when enabled")
	}

	return nil
}

// validateWhatsApp validates WhatsApp channel configuration
func (v *Validator) validateWhatsApp(channels *ChannelsConfig) error {
	if !channels.WhatsApp.Enabled {
		return nil
	}

	if channels.WhatsApp.BridgeURL == "" {
		return errors.InvalidConfig("whatsapp bridge_url is required when enabled")
	}

	if _, err := url.Parse(channels.WhatsApp.BridgeURL); err != nil {
		return errors.Wrap(err, errors.ErrCodeInvalidConfig, "invalid whatsapp bridge_url")
	}

	return nil
}

// validateFeishu validates Feishu channel configuration
func (v *Validator) validateFeishu(channels *ChannelsConfig) error {
	if !channels.Feishu.Enabled {
		return nil
	}

	if channels.Feishu.AppID == "" {
		return errors.InvalidConfig("feishu app_id is required when enabled")
	}

	if channels.Feishu.AppSecret == "" {
		return errors.InvalidConfig("feishu app_secret is required when enabled")
	}

	// verification_token is optional (for webhook mode)
	// webhook_port is optional (defaults to 8765 if not set)
	if channels.Feishu.WebhookPort != 0 {
		if channels.Feishu.WebhookPort < 1024 || channels.Feishu.WebhookPort > 65535 {
			return errors.InvalidConfig("feishu webhook_port must be between 1024 and 65535")
		}
	}

	return nil
}

// validateQQ validates QQ channel configuration
func (v *Validator) validateQQ(channels *ChannelsConfig) error {
	if !channels.QQ.Enabled {
		return nil
	}

	if channels.QQ.AppID == "" {
		return errors.InvalidConfig("qq app_id is required when enabled")
	}

	if channels.QQ.AppSecret == "" {
		return errors.InvalidConfig("qq app_secret is required when enabled")
	}

	return nil
}

// validateWeWork validates WeWork channel configuration
func (v *Validator) validateWeWork(channels *ChannelsConfig) error {
	if !channels.WeWork.Enabled {
		return nil
	}

	if channels.WeWork.CorpID == "" {
		return errors.InvalidConfig("wework corp_id is required when enabled")
	}

	if channels.WeWork.Secret == "" {
		return errors.InvalidConfig("wework secret is required when enabled")
	}

	if channels.WeWork.AgentID == "" {
		return errors.InvalidConfig("wework agent_id is required when enabled")
	}

	if channels.WeWork.WebhookPort < 1024 || channels.WeWork.WebhookPort > 65535 {
		return errors.InvalidConfig("wework webhook_port must be between 1024 and 65535")
	}

	return nil
}

// validateDingTalk validates DingTalk channel configuration
func (v *Validator) validateDingTalk(channels *ChannelsConfig) error {
	if !channels.DingTalk.Enabled {
		return nil
	}

	if channels.DingTalk.ClientID == "" {
		return errors.InvalidConfig("dingtalk client_id is required when enabled")
	}

	if channels.DingTalk.ClientSecret == "" {
		return errors.InvalidConfig("dingtalk client_secret is required when enabled")
	}

	return nil
}

// validateInfoflow validates Infoflow channel configuration
func (v *Validator) validateInfoflow(channels *ChannelsConfig) error {
	if !channels.Infoflow.Enabled {
		return nil
	}

	if channels.Infoflow.WebhookURL == "" {
		return errors.InvalidConfig("infoflow webhook_url is required when enabled")
	}

	if _, err := url.Parse(channels.Infoflow.WebhookURL); err != nil {
		return errors.Wrap(err, errors.ErrCodeInvalidConfig, "invalid infoflow webhook_url")
	}

	if channels.Infoflow.WebhookPort < 1024 || channels.Infoflow.WebhookPort > 65535 {
		return errors.InvalidConfig("infoflow webhook_port must be between 1024 and 65535")
	}

	return nil
}

// validateTools validates tool configuration
func (v *Validator) validateTools(cfg *Config) error {
	if err := v.validateShellTool(&cfg.Tools.Shell); err != nil {
		return err
	}

	if err := v.validateWebTool(&cfg.Tools.Web); err != nil {
		return err
	}

	if err := v.validateBrowserTool(&cfg.Tools.Browser); err != nil {
		return err
	}

	return nil
}

// validateShellTool validates shell tool configuration
func (v *Validator) validateShellTool(shell *ShellToolConfig) error {
	if !shell.Enabled {
		return nil
	}

	// Check timeout
	if shell.Timeout < 1 || shell.Timeout > 3600 {
		return errors.InvalidConfig("shell timeout must be between 1 and 3600 seconds")
	}

	// Check for dangerous commands
	dangerousCmds := []string{"rm -rf", "dd", "mkfs"}
	for _, dangerous := range dangerousCmds {
		found := false
		for _, denied := range shell.DeniedCmds {
			if strings.Contains(denied, dangerous) {
				found = true
				break
			}
		}
		if !found {
			return errors.InvalidConfig(fmt.Sprintf(
				"dangerous command '%s' should be in denied_cmds list", dangerous))
		}
	}

	// Validate sandbox configuration
	if shell.Sandbox.Enabled {
		if shell.Sandbox.Image == "" {
			return errors.InvalidConfig("sandbox image is required when enabled")
		}
	}

	return nil
}

// validateWebTool validates web tool configuration
func (v *Validator) validateWebTool(web *WebToolConfig) error {
	// Timeout of 0 means not set, skip validation
	if web.Timeout == 0 {
		return nil
	}

	// Check timeout
	if web.Timeout < 1 || web.Timeout > 300 {
		return errors.InvalidConfig("web timeout must be between 1 and 300 seconds")
	}

	return nil
}

// validateBrowserTool validates browser tool configuration
func (v *Validator) validateBrowserTool(browser *BrowserToolConfig) error {
	if !browser.Enabled {
		return nil
	}

	if browser.Timeout < 1 || browser.Timeout > 600 {
		return errors.InvalidConfig("browser timeout must be between 1 and 600 seconds")
	}

	return nil
}

// validateGateway validates gateway configuration
func (v *Validator) validateGateway(cfg *Config) error {
	// Port 0 means not configured, skip validation
	if cfg.Gateway.Port != 0 && (cfg.Gateway.Port < 1024 || cfg.Gateway.Port > 65535) {
		return errors.InvalidConfig("gateway port must be between 1024 and 65535")
	}

	if cfg.Gateway.ReadTimeout < 0 || cfg.Gateway.ReadTimeout > 300*time.Second {
		return errors.InvalidConfig("gateway read_timeout must be between 0 and 300 seconds")
	}

	if cfg.Gateway.WriteTimeout < 0 || cfg.Gateway.WriteTimeout > 300*time.Second {
		return errors.InvalidConfig("gateway write_timeout must be between 0 and 300 seconds")
	}

	// Validate WebSocket configuration
	if err := v.validateWebSocketConfig(&cfg.Gateway.WebSocket); err != nil {
		return err
	}

	return nil
}

// validateWebSocketConfig validates WebSocket configuration
func (v *Validator) validateWebSocketConfig(ws *WebSocketConfig) error {
	// Only validate WebSocket config if host is set (WebSocket is optional)
	if ws.Host == "" {
		return nil
	}

	// Port 0 means use default, skip validation
	if ws.Port != 0 && (ws.Port < 1024 || ws.Port > 65535) {
		return errors.InvalidConfig("websocket port must be between 1024 and 65535")
	}

	if ws.PingInterval < 0 || ws.PingInterval > 5*time.Minute {
		return errors.InvalidConfig("websocket ping_interval must be between 0 and 5m")
	}

	if ws.PongTimeout < 0 || ws.PongTimeout > 5*time.Minute {
		return errors.InvalidConfig("websocket pong_timeout must be between 0 and 5m")
	}

	// Validate auth token if auth is enabled
	if ws.EnableAuth && strings.TrimSpace(ws.AuthToken) == "" {
		return errors.InvalidConfig("websocket auth_token is required when enable_auth is true")
	}

	return nil
}

// validateMemory validates memory configuration
func (v *Validator) validateMemory(cfg *Config) error {
	if cfg.Memory.Backend == "" {
		return errors.InvalidConfig("memory backend cannot be empty")
	}

	validBackends := []string{"builtin", "qmd"}
	if !slices.Contains(validBackends, cfg.Memory.Backend) {
		return errors.InvalidConfig(fmt.Sprintf("invalid memory backend: %s", cfg.Memory.Backend))
	}

	return nil
}
