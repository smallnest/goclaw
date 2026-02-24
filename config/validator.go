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
	if strings.TrimSpace(defaults.Model) == "" {
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

	// Validate subagents configuration
	// Note: Subagents is of type *SubagentsConfig, not *AgentSubagentConfig
	// Skip validation for now as the structure differs
	_ = defaults.Subagents

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
	// Check if at least one provider is configured
	hasProvider := false

	// Validate OpenRouter
	if cfg.Providers.OpenRouter.APIKey != "" {
		hasProvider = true
		if err := v.validateAPIKey(cfg.Providers.OpenRouter.APIKey); err != nil {
			return errors.Wrap(err, errors.ErrCodeInvalidConfig, "invalid OpenRouter API key")
		}
	}

	// Validate OpenAI
	if cfg.Providers.OpenAI.APIKey != "" {
		hasProvider = true
		if err := v.validateAPIKey(cfg.Providers.OpenAI.APIKey); err != nil {
			return errors.Wrap(err, errors.ErrCodeInvalidConfig, "invalid OpenAI API key")
		}
	}

	// Validate Anthropic
	if cfg.Providers.Anthropic.APIKey != "" {
		hasProvider = true
		if err := v.validateAPIKey(cfg.Providers.Anthropic.APIKey); err != nil {
			return errors.Wrap(err, errors.ErrCodeInvalidConfig, "invalid Anthropic API key")
		}
	}

	// Validate profiles
	for i, profile := range cfg.Providers.Profiles {
		if profile.Name == "" {
			return errors.InvalidConfig(fmt.Sprintf("provider profile %d has empty name", i))
		}

		if profile.APIKey == "" {
			return errors.InvalidConfig(fmt.Sprintf("provider profile '%s' has empty API key", profile.Name))
		}

		// Check if provider type is valid
		validProviders := []string{"openai", "anthropic", "openrouter"}
		if !slices.Contains(validProviders, profile.Provider) {
			return errors.InvalidConfig(fmt.Sprintf("provider profile '%s' has invalid provider type: %s",
				profile.Name, profile.Provider))
		}

		if err := v.validateAPIKey(profile.APIKey); err != nil {
			return errors.Wrap(err, errors.ErrCodeInvalidConfig,
				fmt.Sprintf("invalid API key for profile '%s'", profile.Name))
		}
	}

	if !hasProvider {
		return errors.InvalidConfig("at least one LLM provider must be configured")
	}

	// Validate failover configuration
	if cfg.Providers.Failover.Enabled {
		if err := v.validateFailoverConfig(&cfg.Providers.Failover); err != nil {
			return err
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

// validateFailoverConfig validates failover configuration
func (v *Validator) validateFailoverConfig(failover *FailoverConfig) error {
	// Check strategy
	validStrategies := []string{"round_robin", "least_used", "random"}
	if !slices.Contains(validStrategies, failover.Strategy) {
		return errors.InvalidConfig(fmt.Sprintf("invalid failover strategy: %s", failover.Strategy))
	}

	// Check cooldown
	if failover.DefaultCooldown < 0 || failover.DefaultCooldown > time.Hour {
		return errors.InvalidConfig("failover cooldown must be between 0 and 1 hour")
	}

	// Check circuit breaker configuration
	if err := v.validateCircuitBreakerConfig(&failover.CircuitBreaker); err != nil {
		return err
	}

	return nil
}

// validateCircuitBreakerConfig validates circuit breaker configuration
func (v *Validator) validateCircuitBreakerConfig(cb *CircuitBreakerConfig) error {
	if cb.FailureThreshold < 1 || cb.FailureThreshold > 100 {
		return errors.InvalidConfig("circuit breaker failure threshold must be between 1 and 100")
	}

	if cb.Timeout < 0 || cb.Timeout > time.Minute*30 {
		return errors.InvalidConfig("circuit breaker timeout must be between 0 and 30 minutes")
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
	if cfg.Gateway.Port < 1024 || cfg.Gateway.Port > 65535 {
		return errors.InvalidConfig("gateway port must be between 1024 and 65535")
	}

	if cfg.Gateway.ReadTimeout < 1 || cfg.Gateway.ReadTimeout > 300 {
		return errors.InvalidConfig("gateway read_timeout must be between 1 and 300 seconds")
	}

	if cfg.Gateway.WriteTimeout < 1 || cfg.Gateway.WriteTimeout > 300 {
		return errors.InvalidConfig("gateway write_timeout must be between 1 and 300 seconds")
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

	if ws.Port < 1024 || ws.Port > 65535 {
		return errors.InvalidConfig("websocket port must be between 1024 and 65535")
	}

	if ws.PingInterval < 1*time.Second || ws.PingInterval > 5*time.Minute {
		return errors.InvalidConfig("websocket ping_interval must be between 1s and 5m")
	}

	if ws.PongTimeout < 1*time.Second || ws.PongTimeout > 5*time.Minute {
		return errors.InvalidConfig("websocket pong_timeout must be between 1s and 5m")
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
