package providers

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/smallnest/goclaw/errors"
)

// FailoverProvider 支持故障转移的提供商
type FailoverProvider struct {
	primary         Provider
	fallback        Provider
	circuitBreaker  *CircuitBreaker
	errorClassifier errors.ErrorClassifier
	mu              sync.RWMutex
}

// NewFailoverProvider 创建故障转移提供商
func NewFailoverProvider(primary, fallback Provider, errorClassifier errors.ErrorClassifier) *FailoverProvider {
	return &FailoverProvider{
		primary:         primary,
		fallback:        fallback,
		circuitBreaker:  NewCircuitBreaker(5, 5*time.Minute),
		errorClassifier: errorClassifier,
	}
}

// Chat 聊天（带故障转移）
func (p *FailoverProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
	// 尝试使用主要提供商
	if !p.circuitBreaker.IsOpen() {
		response, err := p.primary.Chat(ctx, messages, tools, options...)
		if err == nil {
			p.circuitBreaker.RecordSuccess()
			return response, nil
		}

		// 检查错误类型
		reason := p.errorClassifier.ClassifyError(err)
		if p.shouldFailover(reason) {
			p.circuitBreaker.RecordFailure()
			// 尝试故障转移
			return p.chatWithFallback(ctx, messages, tools, options...)
		}

		return nil, err
	}

	// 断路器打开，直接使用备用
	return p.chatWithFallback(ctx, messages, tools, options...)
}

// ChatWithTools 聊天（带工具，支持故障转移）
func (p *FailoverProvider) ChatWithTools(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
	return p.Chat(ctx, messages, tools, options...)
}

// chatWithFallback 使用备用提供商聊天
func (p *FailoverProvider) chatWithFallback(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
	p.mu.RLock()
	fallback := p.fallback
	p.mu.RUnlock()

	if fallback == nil {
		return nil, fmt.Errorf("primary provider failed and no fallback available")
	}

	return fallback.Chat(ctx, messages, tools, options...)
}

// shouldFailover 判断是否应该故障转移
func (p *FailoverProvider) shouldFailover(reason errors.FailoverReason) bool {
	switch reason {
	case errors.FailoverReasonAuth, errors.FailoverReasonRateLimit, errors.FailoverReasonBilling:
		return true
	default:
		return false
	}
}

// Close 关闭连接
func (p *FailoverProvider) Close() error {
	var errs []error

	if p.primary != nil {
		if err := p.primary.Close(); err != nil {
			errs = append(errs, fmt.Errorf("primary close error: %w", err))
		}
	}

	if p.fallback != nil {
		if err := p.fallback.Close(); err != nil {
			errs = append(errs, fmt.Errorf("fallback close error: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}

	return nil
}

// SetFallback 设置备用提供商
func (p *FailoverProvider) SetFallback(fallback Provider) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fallback = fallback
}

// GetPrimary 获取主要提供商
func (p *FailoverProvider) GetPrimary() Provider {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.primary
}

// GetFallback 获取备用提供商
func (p *FailoverProvider) GetFallback() Provider {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.fallback
}

// GetCircuitBreaker 获取断路器
func (p *FailoverProvider) GetCircuitBreaker() *CircuitBreaker {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.circuitBreaker
}
