package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/smallnest/dogclaw/goclaw/internal/logger"
	"go.uber.org/zap"
)

// RetryConfig 重试配置
type RetryConfig struct {
	MaxAttempts      int
	BaseDelay        time.Duration
	MaxDelay         time.Duration
	EnableAuthRotate bool
	EnableCompact    bool
}

// DefaultRetryConfig 默认重试配置
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts:      3,
		BaseDelay:        1 * time.Second,
		MaxDelay:         10 * time.Second,
		EnableAuthRotate: true,
		EnableCompact:    true,
	}
}

// RetryAttempt 重试尝试
type RetryAttempt struct {
	AttemptNumber int
	Reason        FailoverReason
	Error         error
	Delay         time.Duration
}

// RetryPolicy 重试策略接口
type RetryPolicy interface {
	ShouldRetry(attempt int, err error) (bool, FailoverReason)
	GetDelay(attempt int, reason FailoverReason) time.Duration
}

// DefaultRetryPolicy 默认重试策略
type DefaultRetryPolicy struct {
	config *RetryConfig
}

// NewDefaultRetryPolicy 创建默认重试策略
func NewDefaultRetryPolicy(config *RetryConfig) *DefaultRetryPolicy {
	if config == nil {
		config = DefaultRetryConfig()
	}
	return &DefaultRetryPolicy{
		config: config,
	}
}

// ShouldRetry 判断是否应该重试
func (p *DefaultRetryPolicy) ShouldRetry(attempt int, err error) (bool, FailoverReason) {
	if attempt >= p.config.MaxAttempts {
		return false, FailoverReasonUnknown
	}

	classifier := NewErrorClassifier()
	reason := classifier.ClassifyError(err)

	// 可回退的错误类型允许重试
	switch reason {
	case FailoverReasonAuth, FailoverReasonRateLimit, FailoverReasonTimeout:
		return true, reason
	case FailoverReasonBilling:
		return false, reason // 计费错误不重试
	default:
		return false, reason
	}
}

// GetDelay 获取重试延迟
func (p *DefaultRetryPolicy) GetDelay(attempt int, reason FailoverReason) time.Duration {
	// 限流错误使用更长的延迟
	if reason == FailoverReasonRateLimit {
		delay := time.Duration(1<<uint(attempt)) * p.config.BaseDelay
		if delay > p.config.MaxDelay {
			return p.config.MaxDelay
		}
		return delay
	}

	// 其他错误使用指数退避
	delay := time.Duration(1<<uint(attempt)) * p.config.BaseDelay
	if delay > p.config.MaxDelay {
		return p.config.MaxDelay
	}
	return delay
}

// RetryManager 重试管理器
type RetryManager struct {
	policy     RetryPolicy
	classifier *ErrorClassifier
	attempts   []RetryAttempt
}

// NewRetryManager 创建重试管理器
func NewRetryManager(policy RetryPolicy) *RetryManager {
	if policy == nil {
		policy = NewDefaultRetryPolicy(nil)
	}
	return &RetryManager{
		policy:     policy,
		classifier: NewErrorClassifier(),
		attempts:   make([]RetryAttempt, 0),
	}
}

// ExecuteWithRetry 使用重试策略执行函数
func (rm *RetryManager) ExecuteWithRetry(
	ctx context.Context,
	fn func() error,
	onRetry func(attempt int, err error, delay time.Duration),
) error {
	attempt := 0

	for {
		attempt++
		err := fn()

		if err == nil {
			// 成功
			logger.Info("Operation succeeded", zap.Int("attempt", attempt))
			return nil
		}

		// 检查上下文是否已取消
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// 检查是否应该重试
		shouldRetry, reason := rm.policy.ShouldRetry(attempt, err)

		// 记录尝试
		rm.attempts = append(rm.attempts, RetryAttempt{
			AttemptNumber: attempt,
			Reason:        reason,
			Error:         err,
		})

		if !shouldRetry {
			logger.Warn("Operation failed after all retries",
				zap.Int("attempts", attempt),
				zap.Error(err))
			return fmt.Errorf("operation failed after %d attempts: %w", attempt, err)
		}

		// 计算延迟
		delay := rm.policy.GetDelay(attempt, reason)
		rm.attempts[len(rm.attempts)-1].Delay = delay

		logger.Warn("Retrying operation",
			zap.Int("attempt", attempt),
			zap.String("reason", string(reason)),
			zap.Duration("delay", delay),
			zap.Error(err))

		// 执行回调
		if onRetry != nil {
			onRetry(attempt, err, delay)
		}

		// 等待
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			continue
		}
	}
}

// GetAttempts 获取所有尝试记录
func (rm *RetryManager) GetAttempts() []RetryAttempt {
	return rm.attempts
}

// GetTotalAttempts 获取总尝试次数
func (rm *RetryManager) GetTotalAttempts() int {
	return len(rm.attempts)
}

// CompactStrategy 上下文压缩策略
type CompactStrategy struct {
	MaxHistorySize   int
	MaxHistoryTurns  int
	CompactThreshold int
}

// DefaultCompactStrategy 默认压缩策略
func DefaultCompactStrategy() *CompactStrategy {
	return &CompactStrategy{
		MaxHistorySize:   10000, // 最多保留的字符数
		MaxHistoryTurns:  20,    // 最多保留的轮次数
		CompactThreshold: 30,    // 历史消息数超过此值时压缩
	}
}

// CompactContext 压缩上下文
func CompactContext(messages []sessionMessage) []sessionMessage {
	strategy := DefaultCompactStrategy()

	if len(messages) <= strategy.CompactThreshold {
		return messages
	}

	logger.Info("Compacting session context",
		zap.Int("original_count", len(messages)),
		zap.Int("max_turns", strategy.MaxHistoryTurns))

	// 保留最近的 N 轮对话
	turnCount := 0
	var result []sessionMessage
	var pending []sessionMessage // 存储一个轮次的完整消息

	// 从后向前遍历，保留最近的轮次
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]

		switch msg.Role {
		case "system":
			// 系统消息始终保留
			result = append([]sessionMessage{msg}, result...)

		case "user":
			// 新的用户消息开始一个新的轮次
			if turnCount > 0 && len(pending) > 0 {
				// 将上一轮的消息添加到结果中（按原始顺序）
				for j := len(pending) - 1; j >= 0; j-- {
					result = append([]sessionMessage{pending[j]}, result...)
				}
			}
			pending = nil
			pending = append(pending, msg)
			turnCount++

			// 检查是否达到最大轮次
			if turnCount > strategy.MaxHistoryTurns {
				break
			}

		case "assistant", "tool":
			// 将助手和工具消息添加到当前轮次
			pending = append(pending, msg)
		}
	}

	// 添加最后一轮的消息
	if len(pending) > 0 {
		for j := len(pending) - 1; j >= 0; j-- {
			result = append([]sessionMessage{pending[j]}, result...)
		}
	}

	logger.Info("Context compacted",
		zap.Int("original_count", len(messages)),
		zap.Int("compacted_count", len(result)),
		zap.Int("turns_kept", turnCount))

	return result
}

// sessionMessage 简化的消息类型用于压缩
type sessionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ConvertToSessionMessages 转换消息格式
func ConvertToSessionMessages(messages []Message) []sessionMessage {
	result := make([]sessionMessage, len(messages))
	for i, msg := range messages {
		result[i] = sessionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}
	return result
}
