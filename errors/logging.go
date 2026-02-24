package errors

import (
	"errors"
	"fmt"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

// ErrorHandler provides centralized error handling
type ErrorHandler struct {
	logger *zap.Logger
}

// NewErrorHandler creates a new error handler
func NewErrorHandler() *ErrorHandler {
	return &ErrorHandler{
		logger: logger.L(),
	}
}

// Handle handles an error with appropriate logging based on severity
func (h *ErrorHandler) Handle(err error) {
	if err == nil {
		return
	}

	// Get error details
	code := GetCode(err)
	msg := GetMessage(err)

	// Log based on error code severity
	switch code {
	case ErrCodeInvalidInput, ErrCodeNotFound, ErrCodeAlreadyExists:
		// User errors - log at debug level
		h.logger.Debug("User error", zap.String("code", string(code)), zap.String("message", msg))
	case ErrCodeTimeout, ErrCodeRateLimit:
		// Temporary errors - log at info level
		h.logger.Info("Temporary error", zap.String("code", string(code)), zap.String("message", msg))
	case ErrCodeAuth, ErrCodeBilling, ErrCodePermission:
		// Security/billing errors - log at warn level
		h.logger.Warn("Security error", zap.String("code", string(code)), zap.String("message", msg))
	case ErrCodeToolExecution, ErrCodeProviderError, ErrCodeMemorySearchFailed:
		// Operation errors - log at error level
		h.logWithError("Operation failed", err)
	case ErrCodeAgentStartFailed, ErrCodeSessionCorrupted:
		// Critical errors - log at error level with stack trace
		h.logWithStack("Critical error", err)
	default:
		// Unknown errors - log at error level
		h.logWithError("Unknown error", err)
	}
}

// Handlef handles an error with formatted message
func (h *ErrorHandler) Handlef(err error, format string, args ...any) {
	if err == nil {
		return
	}
	msg := fmt.Sprintf(format, args...)
	h.logger.Error(msg,
		zap.String("error_code", string(GetCode(err))),
		zap.String("error_message", GetMessage(err)),
		zap.Error(err))
}

// HandleWithFields handles an error with additional fields
func (h *ErrorHandler) HandleWithFields(err error, fields ...zap.Field) {
	if err == nil {
		return
	}
	allFields := append([]zap.Field{
		zap.String("error_code", string(GetCode(err))),
		zap.String("error_message", GetMessage(err)),
	}, fields...)
	h.logger.Error("Error occurred", allFields...)
}

// Recover handles panics and converts to errors
func (h *ErrorHandler) Recover(operation string) error {
	if r := recover(); r != nil {
		stack := debug.Stack()
		err := New(ErrCodeUnknown, fmt.Sprintf("panic in %s", operation))

		h.logger.Error("Panic recovered",
			zap.String("operation", operation),
			zap.Any("recover", r),
			zap.String("stack", string(stack)))

		return err
	}
	return nil
}

// Validate checks if error is nil, returns wrapped error if not
func (h *ErrorHandler) Validate(err error, code ErrorCode, message string) error {
	if err == nil {
		return nil
	}
	return Wrap(err, code, message)
}

// logWithError logs error with full details
func (h *ErrorHandler) logWithError(message string, err error) {
	var appErr *AppError
	if errors.As(err, &appErr) && appErr.Err != nil {
		h.logger.Error(message,
			zap.String("error_code", string(appErr.Code)),
			zap.String("error_message", appErr.Message),
			zap.Error(appErr.Err),
			zap.Any("context", appErr.Context))
	} else {
		h.logger.Error(message,
			zap.String("error_code", string(GetCode(err))),
			zap.String("error_message", GetMessage(err)),
			zap.Error(err))
	}
}

// logWithStack logs error with stack trace
func (h *ErrorHandler) logWithStack(message string, err error) {
	stack := debug.Stack()
	var appErr *AppError
	if errors.As(err, &appErr) && appErr.Err != nil {
		h.logger.Error(message,
			zap.String("error_code", string(appErr.Code)),
			zap.String("error_message", appErr.Message),
			zap.Error(appErr.Err),
			zap.String("stack", string(stack)),
			zap.Any("context", appErr.Context))
	} else {
		h.logger.Error(message,
			zap.String("error_code", string(GetCode(err))),
			zap.String("error_message", GetMessage(err)),
			zap.Error(err),
			zap.String("stack", string(stack)))
	}
}

// IsRetryable checks if an error is retryable
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	code := GetCode(err)
	retryableCodes := []ErrorCode{
		ErrCodeTimeout,
		ErrCodeRateLimit,
		ErrCodeProviderUnavailable,
		ErrCodeProviderTimeout,
	}

	if slices.Contains(retryableCodes, code) {
		return true
	}

	// Also check error message for network-related issues
	msg := strings.ToLower(err.Error())
	networkKeywords := []string{
		"connection refused",
		"connection reset",
		"timeout",
		"temporary failure",
		"network",
	}

	for _, keyword := range networkKeywords {
		if strings.Contains(msg, keyword) {
			return true
		}
	}

	return false
}

// IsFatal checks if an error is fatal (should stop operation)
func IsFatal(err error) bool {
	if err == nil {
		return false
	}

	code := GetCode(err)
	fatalCodes := []ErrorCode{
		ErrCodeInvalidConfig,
		ErrCodePermission,
		ErrCodeBilling,
		ErrCodeAuth,
		ErrCodeSessionCorrupted,
	}

	if slices.Contains(fatalCodes, code) {
		return true
	}

	return false
}

// GetUserMessage returns a user-friendly error message
func GetUserMessage(err error) string {
	if err == nil {
		return ""
	}

	code := GetCode(err)
	msg := GetMessage(err)

	// Map error codes to user-friendly messages
	messages := map[ErrorCode]string{
		ErrCodeInvalidInput:        "The input provided is invalid. Please check and try again.",
		ErrCodeInvalidConfig:       "Configuration error. Please check your settings.",
		ErrCodeNotFound:            "The requested resource was not found.",
		ErrCodeAlreadyExists:       "The resource already exists.",
		ErrCodePermission:          "You don't have permission to perform this action.",
		ErrCodeTimeout:             "The operation timed out. Please try again.",
		ErrCodeRateLimit:           "Too many requests. Please wait and try again.",
		ErrCodeAuth:                "Authentication failed. Please check your credentials.",
		ErrCodeBilling:             "Billing error. Please check your account.",
		ErrCodeToolExecution:       "Failed to execute the requested operation.",
		ErrCodeToolNotFound:        "The requested tool is not available.",
		ErrCodeSkillNotFound:       "The requested skill is not available.",
		ErrCodeProviderUnavailable: "The AI service is currently unavailable.",
		ErrCodeSessionNotFound:     "Session not found. Please start a new conversation.",
		ErrCodeMemorySearchFailed:  "Failed to search memory. The operation will be retried.",
	}

	if userMsg, ok := messages[code]; ok {
		return userMsg
	}

	// Default message
	if msg != "" {
		return msg
	}

	return "An unexpected error occurred. Please try again."
}
