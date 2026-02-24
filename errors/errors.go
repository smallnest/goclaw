package errors

import (
	"errors"
	"fmt"
	"strings"
)

// ErrorCode represents a unique error code
type ErrorCode string

const (
	// General errors
	ErrCodeUnknown         ErrorCode = "UNKNOWN"
	ErrCodeInvalidInput    ErrorCode = "INVALID_INPUT"
	ErrCodeInvalidConfig   ErrorCode = "INVALID_CONFIG"
	ErrCodeNotFound        ErrorCode = "NOT_FOUND"
	ErrCodeAlreadyExists   ErrorCode = "ALREADY_EXISTS"
	ErrCodePermission      ErrorCode = "PERMISSION_DENIED"
	ErrCodeTimeout         ErrorCode = "TIMEOUT"
	ErrCodeRateLimit       ErrorCode = "RATE_LIMIT"
	ErrCodeAuth            ErrorCode = "AUTHENTICATION_FAILED"
	ErrCodeBilling         ErrorCode = "BILLING_ERROR"
	ErrCodeContextOverflow ErrorCode = "CONTEXT_OVERFLOW"

	// Agent errors
	ErrCodeAgentNotRunning  ErrorCode = "AGENT_NOT_RUNNING"
	ErrCodeAgentStartFailed ErrorCode = "AGENT_START_FAILED"
	ErrCodeAgentStopFailed  ErrorCode = "AGENT_STOP_FAILED"
	ErrCodeToolExecution    ErrorCode = "TOOL_EXECUTION_FAILED"
	ErrCodeToolNotFound     ErrorCode = "TOOL_NOT_FOUND"
	ErrCodeSkillNotFound    ErrorCode = "SKILL_NOT_FOUND"
	ErrCodeSkillLoadFailed  ErrorCode = "SKILL_LOAD_FAILED"

	// Provider errors
	ErrCodeProviderUnavailable ErrorCode = "PROVIDER_UNAVAILABLE"
	ErrCodeProviderTimeout     ErrorCode = "PROVIDER_TIMEOUT"
	ErrCodeProviderError       ErrorCode = "PROVIDER_ERROR"
	ErrCodeProviderResponse    ErrorCode = "PROVIDER_RESPONSE_ERROR"

	// Channel errors
	ErrCodeChannelNotConfigured ErrorCode = "CHANNEL_NOT_CONFIGURED"
	ErrCodeChannelSendFailed    ErrorCode = "CHANNEL_SEND_FAILED"
	ErrCodeChannelReceive       ErrorCode = "CHANNEL_RECEIVE_ERROR"

	// Memory errors
	ErrCodeMemoryNotFound     ErrorCode = "MEMORY_NOT_FOUND"
	ErrCodeMemorySaveFailed   ErrorCode = "MEMORY_SAVE_FAILED"
	ErrCodeMemoryLoadFailed   ErrorCode = "MEMORY_LOAD_FAILED"
	ErrCodeMemorySearchFailed ErrorCode = "MEMORY_SEARCH_FAILED"

	// Session errors
	ErrCodeSessionNotFound  ErrorCode = "SESSION_NOT_FOUND"
	ErrCodeSessionCreate    ErrorCode = "SESSION_CREATE_FAILED"
	ErrCodeSessionSave      ErrorCode = "SESSION_SAVE_FAILED"
	ErrCodeSessionCorrupted ErrorCode = "SESSION_CORRUPTED"
)

// AppError represents a structured application error
type AppError struct {
	Code       ErrorCode
	Message    string
	Err        error
	StackTrace string
	Context    map[string]any
}

// Error implements the error interface
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the wrapped error
func (e *AppError) Unwrap() error {
	return e.Err
}

// New creates a new application error
func New(code ErrorCode, message string) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Context: make(map[string]any),
	}
}

// Wrap wraps an existing error with code and message
func Wrap(err error, code ErrorCode, message string) *AppError {
	if err == nil {
		return nil
	}
	return &AppError{
		Code:    code,
		Message: message,
		Err:     err,
		Context: make(map[string]any),
	}
}

// Wrapf wraps an error with formatted message
func Wrapf(err error, code ErrorCode, format string, args ...any) *AppError {
	if err == nil {
		return nil
	}
	return &AppError{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
		Err:     err,
		Context: make(map[string]any),
	}
}

// WithContext adds context to the error
func (e *AppError) WithContext(key string, value any) *AppError {
	if e.Context == nil {
		e.Context = make(map[string]any)
	}
	e.Context[key] = value
	return e
}

// GetCode returns the error code from an error if it's an AppError
func GetCode(err error) ErrorCode {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return ErrCodeUnknown
}

// GetMessage returns the error message
func GetMessage(err error) string {
	if err == nil {
		return ""
	}
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Message
	}
	return err.Error()
}

// Is checks if error is of specific type
func Is(err error, code ErrorCode) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code == code
	}
	return false
}

// Common error constructors
func InvalidInput(msg string) *AppError {
	return New(ErrCodeInvalidInput, msg)
}

func InvalidConfig(msg string) *AppError {
	return New(ErrCodeInvalidConfig, msg)
}

func NotFound(what string) *AppError {
	return New(ErrCodeNotFound, what+" not found")
}

func AlreadyExists(what string) *AppError {
	return New(ErrCodeAlreadyExists, what+" already exists")
}

func Timeout(operation string) *AppError {
	return New(ErrCodeTimeout, operation+" timed out")
}

func ToolNotFound(name string) *AppError {
	return New(ErrCodeToolNotFound, fmt.Sprintf("tool '%s' not found", name))
}

func ToolExecutionFailed(name string, err error) *AppError {
	return Wrap(err, ErrCodeToolExecution, fmt.Sprintf("tool '%s' execution failed", name))
}

func SkillNotFound(name string) *AppError {
	return New(ErrCodeSkillNotFound, fmt.Sprintf("skill '%s' not found", name))
}

func ProviderUnavailable(provider string) *AppError {
	return New(ErrCodeProviderUnavailable, fmt.Sprintf("provider '%s' is unavailable", provider))
}

func SessionNotFound(id string) *AppError {
	return New(ErrCodeSessionNotFound, fmt.Sprintf("session '%s' not found", id))
}

func MemoryOperationFailed(op string, err error) *AppError {
	return Wrap(err, ErrCodeMemorySearchFailed, fmt.Sprintf("memory %s operation failed", op))
}

// ==============================================================================
// Failover Support
// ==============================================================================

// FailoverReason 失败原因类型
type FailoverReason string

const (
	// FailoverReasonAuth 认证错误
	FailoverReasonAuth FailoverReason = "auth"
	// FailoverReasonRateLimit 速率限制
	FailoverReasonRateLimit FailoverReason = "rate_limit"
	// FailoverReasonTimeout 超时
	FailoverReasonTimeout FailoverReason = "timeout"
	// FailoverReasonBilling 计费错误
	FailoverReasonBilling FailoverReason = "billing"
	// FailoverReasonContextOverflow 上下文溢出
	FailoverReasonContextOverflow FailoverReason = "context_overflow"
	// FailoverReasonUnknown 未知错误
	FailoverReasonUnknown FailoverReason = "unknown"
)

// ErrorClassifier 错误分类器接口
type ErrorClassifier interface {
	ClassifyError(err error) FailoverReason
	IsFailoverError(err error) bool
}

// SimpleErrorClassifier 简单的错误分类器实现
type SimpleErrorClassifier struct {
	authPatterns      []string
	rateLimitPatterns []string
	timeoutPatterns   []string
	billingPatterns   []string
}

// NewSimpleErrorClassifier 创建简单错误分类器
func NewSimpleErrorClassifier() *SimpleErrorClassifier {
	return &SimpleErrorClassifier{
		authPatterns: []string{
			"invalid api key", "incorrect api key", "invalid token",
			"authentication", "re-authenticate", "unauthorized",
			"forbidden", "access denied", "expired", "401", "403",
		},
		rateLimitPatterns: []string{
			"rate limit", "too many requests", "429", "quota exceeded",
			"resource_exhausted", "usage limit", "overloaded",
		},
		timeoutPatterns: []string{
			"timeout", "timed out", "deadline exceeded", "context deadline exceeded",
		},
		billingPatterns: []string{
			"402", "payment required", "insufficient credits", "billing",
		},
	}
}

// ClassifyError 分类错误
func (c *SimpleErrorClassifier) ClassifyError(err error) FailoverReason {
	if err == nil {
		return FailoverReasonUnknown
	}

	errMsg := strings.ToLower(err.Error())

	if c.matchesAny(errMsg, c.authPatterns) {
		return FailoverReasonAuth
	}
	if c.matchesAny(errMsg, c.rateLimitPatterns) {
		return FailoverReasonRateLimit
	}
	if c.matchesAny(errMsg, c.timeoutPatterns) {
		return FailoverReasonTimeout
	}
	if c.matchesAny(errMsg, c.billingPatterns) {
		return FailoverReasonBilling
	}

	return FailoverReasonUnknown
}

// IsFailoverError 检查是否为可回退的错误
func (c *SimpleErrorClassifier) IsFailoverError(err error) bool {
	if err == nil {
		return false
	}
	reason := c.ClassifyError(err)
	return reason != FailoverReasonUnknown
}

// matchesAny 检查错误消息是否匹配任何模式
func (c *SimpleErrorClassifier) matchesAny(errMsg string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}
	return false
}
