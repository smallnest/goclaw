package runtime

import "fmt"

// AcpRuntimeError represents errors that occur during ACP runtime operations.
type AcpRuntimeError struct {
	Code    string // Machine-readable error code
	Message string // Human-readable error message
	Err     error  // Underlying error, if any
}

// Error implements the error interface.
func (e *AcpRuntimeError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying error.
func (e *AcpRuntimeError) Unwrap() error {
	return e.Err
}

// ACP runtime error codes.
const (
	// ErrCodeBackendMissing indicates no ACP runtime backend is configured.
	ErrCodeBackendMissing = "ACP_BACKEND_MISSING"

	// ErrCodeBackendUnavailable indicates the ACP runtime backend is unhealthy.
	ErrCodeBackendUnavailable = "ACP_BACKEND_UNAVAILABLE"

	// ErrCodeSessionInitFailed indicates ACP session initialization failed.
	ErrCodeSessionInitFailed = "ACP_SESSION_INIT_FAILED"

	// ErrCodeTurnFailed indicates an ACP turn execution failed.
	ErrCodeTurnFailed = "ACP_TURN_FAILED"

	// ErrCodeBackendUnsupportedControl indicates the backend doesn't support a control operation.
	ErrCodeBackendUnsupportedControl = "ACP_BACKEND_UNSUPPORTED_CONTROL"

	// ErrCodeSessionLimitReached indicates the maximum concurrent sessions limit was reached.
	ErrCodeSessionLimitReached = "ACP_SESSION_LIMIT_REACHED"

	// ErrCodeSessionNotFound indicates the ACP session doesn't exist.
	ErrCodeSessionNotFound = "ACP_SESSION_NOT_FOUND"

	// ErrCodeInvalidSessionKey indicates the session key is invalid.
	ErrCodeInvalidSessionKey = "ACP_INVALID_SESSION_KEY"

	// ErrCodeAgentUnauthorized indicates the agent is not authorized to use ACP.
	ErrCodeAgentUnauthorized = "ACP_AGENT_UNAUTHORIZED"

	// ErrCodePolicyDisabled indicates ACP is disabled by policy.
	ErrCodePolicyDisabled = "ACP_POLICY_DISABLED"

	// ErrCodeThreadBindingDisabled indicates thread bindings are disabled.
	ErrCodeThreadBindingDisabled = "ACP_THREAD_BINDING_DISABLED"

	// ErrCodeThreadBindingSpawnDisabled indicates thread-bound spawning is disabled.
	ErrCodeThreadBindingSpawnDisabled = "ACP_THREAD_BINDING_SPAWN_DISABLED"

	// ErrCodeThreadBindingFailed indicates thread binding creation failed.
	ErrCodeThreadBindingFailed = "ACP_THREAD_BINDING_FAILED"
)

// NewAcpRuntimeError creates a new AcpRuntimeError.
func NewAcpRuntimeError(code, message string, err error) *AcpRuntimeError {
	return &AcpRuntimeError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// NewBackendMissingError creates an error for missing backend.
func NewBackendMissingError(backend string) *AcpRuntimeError {
	return &AcpRuntimeError{
		Code:    ErrCodeBackendMissing,
		Message: fmt.Sprintf("ACP runtime backend '%s' is not configured", backend),
	}
}

// NewBackendUnavailableError creates an error for unavailable backend.
func NewBackendUnavailableError(backend string) *AcpRuntimeError {
	return &AcpRuntimeError{
		Code:    ErrCodeBackendUnavailable,
		Message: fmt.Sprintf("ACP runtime backend '%s' is currently unavailable", backend),
	}
}

// NewSessionInitError creates an error for session initialization failure.
func NewSessionInitError(message string, err error) *AcpRuntimeError {
	return &AcpRuntimeError{
		Code:    ErrCodeSessionInitFailed,
		Message: message,
		Err:     err,
	}
}

// NewTurnError creates an error for turn execution failure.
func NewTurnError(message string, err error) *AcpRuntimeError {
	return &AcpRuntimeError{
		Code:    ErrCodeTurnFailed,
		Message: message,
		Err:     err,
	}
}

// NewUnsupportedControlError creates an error for unsupported control operations.
func NewUnsupportedControlError(backend string, control AcpRuntimeControl) *AcpRuntimeError {
	return &AcpRuntimeError{
		Code:    ErrCodeBackendUnsupportedControl,
		Message: fmt.Sprintf("ACP runtime backend '%s' does not support control '%s'", backend, control),
	}
}

// NewSessionLimitError creates an error for session limit reached.
func NewSessionLimitError(current, max int) *AcpRuntimeError {
	return &AcpRuntimeError{
		Code:    ErrCodeSessionLimitReached,
		Message: fmt.Sprintf("ACP max concurrent sessions reached (%d/%d)", current, max),
	}
}

// IsAcpRuntimeError checks if an error is an AcpRuntimeError.
func IsAcpRuntimeError(err error) bool {
	_, ok := err.(*AcpRuntimeError)
	return ok
}

// GetAcpErrorCode extracts the error code from an error.
// Returns empty string if not an AcpRuntimeError.
func GetAcpErrorCode(err error) string {
	if acpErr, ok := err.(*AcpRuntimeError); ok {
		return acpErr.Code
	}
	return ""
}
