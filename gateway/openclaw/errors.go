package openclaw

// ErrorCode 错误码
type ErrorCode string

const (
	// 通用错误
	ErrorInvalidRequest ErrorCode = "INVALID_REQUEST"
	ErrorParseError     ErrorCode = "PARSE_ERROR"
	ErrorMethodNotFound ErrorCode = "METHOD_NOT_FOUND"
	ErrorInvalidParams  ErrorCode = "INVALID_PARAMS"
	ErrorInternalError  ErrorCode = "INTERNAL_ERROR"
	ErrorNotImplemented ErrorCode = "NOT_IMPLEMENTED"
	ErrorUnavailable    ErrorCode = "UNAVAILABLE"

	// 认证相关
	ErrorUnauthorized     ErrorCode = "UNAUTHORIZED"
	ErrorNotPaired        ErrorCode = "NOT_PAIRED"
	ErrorInvalidToken     ErrorCode = "INVALID_TOKEN"
	ErrorTokenExpired     ErrorCode = "TOKEN_EXPIRED"
	ErrorInvalidSignature ErrorCode = "INVALID_SIGNATURE"

	// 连接相关
	ErrorProtocolMismatch ErrorCode = "PROTOCOL_MISMATCH"
	ErrorConnectionClosed ErrorCode = "CONNECTION_CLOSED"
	ErrorRateLimited      ErrorCode = "RATE_LIMITED"

	// 资源相关
	ErrorNotFound      ErrorCode = "NOT_FOUND"
	ErrorAlreadyExists ErrorCode = "ALREADY_EXISTS"
	ErrorConflict      ErrorCode = "CONFLICT"

	// 执行相关
	ErrorExecutionFailed ErrorCode = "EXECUTION_FAILED"
	ErrorAborted         ErrorCode = "ABORTED"
	ErrorTimeout         ErrorCode = "TIMEOUT"

	// 配置相关
	ErrorInvalidConfig  ErrorCode = "INVALID_CONFIG"
	ErrorConfigRequired ErrorCode = "CONFIG_REQUIRED"

	// Node 相关
	ErrorNodeOffline     ErrorCode = "NODE_OFFLINE"
	ErrorNodeUnreachable ErrorCode = "NODE_UNREACHABLE"
	ErrorInvokeFailed    ErrorCode = "INVOKE_FAILED"

	// Session 相关
	ErrorSessionNotFound ErrorCode = "SESSION_NOT_FOUND"
	ErrorSessionExpired  ErrorCode = "SESSION_EXPIRED"
	ErrorSessionLocked   ErrorCode = "SESSION_LOCKED"

	// Cron 相关
	ErrorCronNotFound    ErrorCode = "CRON_NOT_FOUND"
	ErrorInvalidSchedule ErrorCode = "INVALID_SCHEDULE"

	// Channel 相关
	ErrorChannelNotFound ErrorCode = "CHANNEL_NOT_FOUND"
	ErrorChannelDisabled ErrorCode = "CHANNEL_DISABLED"
)

// ErrorSeverity 错误严重程度
type ErrorSeverity string

const (
	SeverityFatal   ErrorSeverity = "fatal"
	SeverityError   ErrorSeverity = "error"
	SeverityWarning ErrorSeverity = "warning"
	SeverityInfo    ErrorSeverity = "info"
)

// ErrorInfo 错误信息（用于响应）
type ErrorInfo struct {
	Code       ErrorCode     `json:"code"`
	Message    string        `json:"message"`
	Details    interface{}   `json:"details,omitempty"`
	Severity   ErrorSeverity `json:"severity,omitempty"`
	Retryable  bool          `json:"retryable,omitempty"`
	RetryAfter int64         `json:"retryAfterMs,omitempty"`
}

// NewErrorInfo 创建错误信息
func NewErrorInfo(code ErrorCode, message string) *ErrorInfo {
	return &ErrorInfo{
		Code:      code,
		Message:   message,
		Severity:  SeverityError,
		Retryable: isRetryable(code),
	}
}

// NewErrorInfoWithDetails 创建带详情的错误信息
func NewErrorInfoWithDetails(code ErrorCode, message string, details interface{}) *ErrorInfo {
	return &ErrorInfo{
		Code:      code,
		Message:   message,
		Details:   details,
		Severity:  SeverityError,
		Retryable: isRetryable(code),
	}
}

// isRetryable 判断错误是否可重试
func isRetryable(code ErrorCode) bool {
	switch code {
	case ErrorUnavailable, ErrorNodeOffline, ErrorNodeUnreachable,
		ErrorRateLimited, ErrorTimeout:
		return true
	default:
		return false
	}
}

// Common error messages
const (
	ErrMessageInvalidRequest  = "Invalid request format"
	ErrMessageParseError      = "Failed to parse request"
	ErrMessageMethodNotFound  = "Method not found or not supported"
	ErrMessageInvalidParams   = "Invalid parameters"
	ErrMessageInternalError   = "Internal server error"
	ErrMessageUnauthorized    = "Authentication required"
	ErrMessageNotPaired       = "Device/node not paired"
	ErrMessageNotFound        = "Resource not found"
	ErrMessageAlreadyExists   = "Resource already exists"
	ErrMessageSessionNotFound = "Session not found"
	ErrMessageCronNotFound    = "Cron job not found"
	ErrMessageInvalidSchedule = "Invalid schedule expression"
	ErrMessageChannelNotFound = "Channel not found"
	ErrMessageNodeOffline     = "Node is offline"
	ErrMessageExecutionFailed = "Execution failed"
	ErrMessageAborted         = "Operation aborted"
	ErrMessageTimeout         = "Operation timeout"
)
