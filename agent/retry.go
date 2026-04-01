package agent

import (
	"time"

	"github.com/smallnest/goclaw/errors"
	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

// RetryDecision 重试决策
type RetryDecision struct {
	ShouldRetry     bool
	Delay           time.Duration
	Action          RecoveryAction
	ProfileToUse    string
	CompressContext bool
	Reason          string
}

// RetryManager 重试管理器接口
type RetryManager interface {
	ShouldRetry(err error) bool
	GetDelay() time.Duration
	RecordError(err error) RetryDecision
	RecordSuccess()
	GetState() *RetryState
	Reset()
}

// retryManager 重试管理器实现
type retryManager struct {
	config          *RetryConfig
	classifier      errors.ErrorClassifier
	state           *RetryState
	retryableErrors map[errors.FailoverReason]bool
}

// NewRetryManager 创建重试管理器
func NewRetryManager(cfg *RetryConfig, classifier errors.ErrorClassifier) RetryManager {
	if cfg == nil {
		cfg = DefaultRetryConfig()
	}
	if classifier == nil {
		classifier = errors.NewSimpleErrorClassifier()
	}

	// 构建可重试错误类型集合
	retryableErrors := make(map[errors.FailoverReason]bool)
	for _, errType := range cfg.RetryableErrors {
		switch errType {
		case "auth":
			retryableErrors[errors.FailoverReasonAuth] = true
		case "rate_limit":
			retryableErrors[errors.FailoverReasonRateLimit] = true
		case "timeout":
			retryableErrors[errors.FailoverReasonTimeout] = true
		case "context_overflow":
			retryableErrors[errors.FailoverReasonContextOverflow] = true
		case "billing":
			retryableErrors[errors.FailoverReasonBilling] = true
		}
	}

	return &retryManager{
		config:          cfg,
		classifier:      classifier,
		state:           &RetryState{},
		retryableErrors: retryableErrors,
	}
}

// DefaultRetryConfig 返回默认重试配置
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		Enabled:               true,
		MaxRetries:            3,
		InitialDelay:          2 * time.Second,
		MaxDelay:              60 * time.Second,
		BackoffFactor:         2.0,
		RetryableErrors:       []string{"auth", "rate_limit", "timeout", "context_overflow", "billing"},
		ContextOverflowAction: "compress",
	}
}

// ShouldRetry 检查是否应该重试
func (m *retryManager) ShouldRetry(err error) bool {
	if err == nil {
		return false
	}

	if !m.config.Enabled {
		return false
	}

	if m.state.Attempt >= m.config.MaxRetries {
		return false
	}

	reason := m.classifier.ClassifyError(err)
	return m.retryableErrors[reason]
}

// GetDelay 获取延迟时间
func (m *retryManager) GetDelay() time.Duration {
	if m.state.Attempt == 0 {
		return m.config.InitialDelay
	}

	// 指数退避
	delay := m.config.InitialDelay
	for i := 0; i < m.state.Attempt; i++ {
		delay = time.Duration(float64(delay) * m.config.BackoffFactor)
		if delay > m.config.MaxDelay {
			delay = m.config.MaxDelay
			break
		}
	}

	return delay
}

// RecordError 记录错误并返回重试决策
func (m *retryManager) RecordError(err error) RetryDecision {
	if err == nil {
		return RetryDecision{ShouldRetry: false}
	}

	m.state.Attempt++
	m.state.LastError = err
	m.state.LastErrorReason = m.classifier.ClassifyError(err)

	logger.Warn("LLM call failed, analyzing for retry",
		zap.Int("attempt", m.state.Attempt),
		zap.Int("max_retries", m.config.MaxRetries),
		zap.String("error_reason", string(m.state.LastErrorReason)),
		zap.Error(err))

	// 检查是否超过最大重试次数
	if m.state.Attempt > m.config.MaxRetries {
		logger.Error("Max retries exceeded",
			zap.Int("attempts", m.state.Attempt),
			zap.Int("max_retries", m.config.MaxRetries))
		return RetryDecision{
			ShouldRetry: false,
			Reason:      "max_retries_exceeded",
		}
	}

	// 检查是否可重试的错误
	if !m.retryableErrors[m.state.LastErrorReason] {
		logger.Error("Non-retryable error",
			zap.String("reason", string(m.state.LastErrorReason)))
		return RetryDecision{
			ShouldRetry: false,
			Reason:      "non_retryable_error: " + string(m.state.LastErrorReason),
		}
	}

	// 计算延迟和恢复动作
	decision := m.makeDecision(err)

	logger.Info("Retry decision made",
		zap.Bool("should_retry", decision.ShouldRetry),
		zap.Duration("delay", decision.Delay),
		zap.String("action", string(decision.Action)),
		zap.String("reason", decision.Reason))

	return decision
}

// makeDecision 根据错误类型制定恢复决策
func (m *retryManager) makeDecision(err error) RetryDecision {
	decision := RetryDecision{
		ShouldRetry: true,
		Delay:       m.GetDelay(),
	}

	switch m.state.LastErrorReason {
	case errors.FailoverReasonAuth:
		// 认证错误：轮换 Profile
		decision.Action = RecoveryActionRotateProfile
		decision.Reason = "authentication_error_rotate_profile"
		decision.Delay = 0 // 不需要延迟

	case errors.FailoverReasonRateLimit:
		// 速率限制：退避
		decision.Action = RecoveryActionBackoff
		decision.Reason = "rate_limit_backoff"
		// 使用更长的退避时间
		if decision.Delay < 10*time.Second {
			decision.Delay = 10 * time.Second
		}

	case errors.FailoverReasonTimeout:
		// 超时：轮换 Profile + 退避
		decision.Action = RecoveryActionRotateProfile
		decision.Reason = "timeout_rotate_and_backoff"

	case errors.FailoverReasonContextOverflow:
		// 上下文溢出：压缩上下文
		decision.Action = RecoveryActionCompressContext
		decision.CompressContext = true
		decision.Reason = "context_overflow_compress"
		decision.Delay = 0 // 不需要延迟

	case errors.FailoverReasonBilling:
		// 计费错误：轮换 Profile
		decision.Action = RecoveryActionRotateProfile
		decision.Reason = "billing_error_rotate_profile"
		decision.Delay = 0 // 不需要延迟

	default:
		// 未知错误：简单重试
		decision.Action = RecoveryActionBackoff
		decision.Reason = "unknown_error_backoff"
	}

	m.state.TotalDelay += decision.Delay
	m.state.NextRetryAt = time.Now().Add(decision.Delay)

	return decision
}

// RecordSuccess 记录成功
func (m *retryManager) RecordSuccess() {
	if m.state.Attempt > 0 {
		logger.Info("Retry succeeded",
			zap.Int("attempts", m.state.Attempt),
			zap.Duration("total_delay", m.state.TotalDelay))
	}
	m.Reset()
}

// GetState 获取当前状态
func (m *retryManager) GetState() *RetryState {
	return m.state
}

// Reset 重置状态
func (m *retryManager) Reset() {
	m.state = &RetryState{}
}

// IsRetryableError 检查错误是否可重试（辅助函数）
func IsRetryableError(err error, classifier errors.ErrorClassifier) bool {
	if classifier == nil {
		classifier = errors.NewSimpleErrorClassifier()
	}
	reason := classifier.ClassifyError(err)
	return reason != errors.FailoverReasonUnknown
}
