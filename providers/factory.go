package providers

import (
	"fmt"
	"time"

	"github.com/smallnest/goclaw/config"
)

// NewProvider 创建提供商
func NewProvider(cfg *config.Config) (Provider, error) {
	return NewSimpleProvider(cfg)
}

// NewSimpleProvider 创建单一提供商
func NewSimpleProvider(cfg *config.Config) (Provider, error) {
	// 只支持 OpenClaw 风格的配置
	if !cfg.Models.HasProviders() {
		return nil, fmt.Errorf("no LLM provider configured. Please configure models.providers in your config file")
	}
	return NewProviderFromModelsConfig(cfg)
}

// NewProviderFromModelsConfig 从 OpenClaw 风格的 models.providers 配置创建提供商
func NewProviderFromModelsConfig(cfg *config.Config) (Provider, error) {
	resolver := NewProviderResolver(cfg)
	resolved, err := resolver.Resolve(cfg.Agents.Defaults.Model.Effective())
	if err != nil {
		return nil, fmt.Errorf("failed to resolve provider: %w", err)
	}

	// 默认超时 120 秒
	timeout := 120 * time.Second
	maxTokens := resolved.GetMaxTokens(cfg.Agents.Defaults.MaxTokens)

	// 根据 API 类型创建提供商
	switch resolved.API {
	case config.ModelAPIAnthropicMessages:
		return NewAnthropicProviderWithTimeout(resolved.APIKey, resolved.BaseURL, resolved.ModelID, maxTokens, timeout)
	case config.ModelAPIOpenAICompletions:
		return NewOpenAIProviderWithTimeout(resolved.APIKey, resolved.BaseURL, resolved.ModelID, maxTokens, timeout)
	case config.ModelAPIOllama:
		return NewOllamaProviderWithTimeout(resolved.APIKey, resolved.BaseURL, resolved.ModelID, maxTokens, timeout)
	default:
		// 默认使用 OpenAI 兼容的 API
		return NewOpenAIProviderWithTimeout(resolved.APIKey, resolved.BaseURL, resolved.ModelID, maxTokens, timeout)
	}
}
