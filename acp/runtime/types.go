package runtime

import "context"

// AcpRuntimePromptMode defines how a prompt is delivered to the ACP agent.
// "prompt" - Standard prompt delivery
// "steer" - Steering/instructional prompt for active turns
type AcpRuntimePromptMode string

const (
	AcpPromptModePrompt AcpRuntimePromptMode = "prompt"
	AcpPromptModeSteer  AcpRuntimePromptMode = "steer"
)

// AcpRuntimeSessionMode defines the lifecycle mode of an ACP session.
// "persistent" - Session remains active after turn completion
// "oneshot" - Session closes after single turn completion
type AcpRuntimeSessionMode string

const (
	AcpSessionModePersistent AcpRuntimeSessionMode = "persistent"
	AcpSessionModeOneshot    AcpRuntimeSessionMode = "oneshot"
)

// AcpRuntimeControl represents control operations supported by ACP runtimes.
type AcpRuntimeControl string

const (
	AcpControlSessionSetMode         AcpRuntimeControl = "session/set_mode"
	AcpControlSessionSetConfigOption AcpRuntimeControl = "session/set_config_option"
	AcpControlSessionStatus          AcpRuntimeControl = "session/status"
)

// AcpRuntimeHandle uniquely identifies an active ACP session.
type AcpRuntimeHandle struct {
	// SessionKey is the goclaw session key for this ACP session
	SessionKey string

	// Backend is the identifier of the runtime backend (e.g., "acpx", "acp-go-sdk")
	Backend string

	// RuntimeSessionName is the session name assigned by the runtime
	RuntimeSessionName string

	// Cwd is the effective working directory for this ACP session
	Cwd string

	// AcpxRecordId is the backend-local record identifier (for acpx backend)
	AcpxRecordId string

	// BackendSessionId is the backend-level ACP session identifier
	BackendSessionId string

	// AgentSessionId is the upstream harness/agent session identifier
	AgentSessionId string
}

// AcpRuntimeEnsureInput contains parameters for creating/ensuring an ACP session.
type AcpRuntimeEnsureInput struct {
	// SessionKey is the goclaw session key
	SessionKey string

	// Agent is the agent ID to use for this session
	Agent string

	// Mode is the session lifecycle mode
	Mode AcpRuntimeSessionMode

	// Cwd is the working directory for the session
	Cwd string

	// Env contains environment variables to pass to the runtime
	Env map[string]string
}

// AcpRuntimeTurnInput contains parameters for executing a turn in an ACP session.
type AcpRuntimeTurnInput struct {
	// Handle identifies the ACP session
	Handle AcpRuntimeHandle

	// Text is the prompt text to send
	Text string

	// Mode is the prompt delivery mode
	Mode AcpRuntimePromptMode

	// RequestID uniquely identifies this turn request
	RequestID string

	// Signal allows canceling the turn
	Signal context.CancelFunc
}

// AcpRuntimeCapabilities describes the capabilities of an ACP runtime.
type AcpRuntimeCapabilities struct {
	// Controls lists the control operations supported by this runtime
	Controls []AcpRuntimeControl

	// ConfigOptionKeys are optional backend-advertised config keys for session/set_config_option.
	// Empty/nil means backend accepts keys but did not advertise a strict list.
	ConfigOptionKeys []string
}

// AcpRuntimeStatus represents the current status of an ACP session.
type AcpRuntimeStatus struct {
	// Summary is a human-readable status summary
	Summary string

	// AcpxRecordId is the backend-local record identifier
	AcpxRecordId string

	// BackendSessionId is the backend-level ACP session identifier
	BackendSessionId string

	// AgentSessionId is the upstream harness session identifier
	AgentSessionId string

	// Details contains additional backend-specific status details
	Details map[string]any
}

// AcpRuntimeDoctorReport is the result of a health check on an ACP runtime.
type AcpRuntimeDoctorReport struct {
	// Ok indicates if the runtime is healthy
	Ok bool

	// Code is an error code if Ok is false
	Code string

	// Message describes the health status
	Message string

	// InstallCommand is a suggested installation command if the runtime is missing
	InstallCommand string

	// Details contains additional diagnostic information
	Details []string
}

// AcpRuntimeEvent represents events streamed during ACP turn execution.
type AcpRuntimeEvent interface {
	isAcpRuntimeEvent()
}

// AcpEventTextDelta is emitted when text content is generated.
type AcpEventTextDelta struct {
	Text   string // Generated text content
	Stream string // "output" or "thought"
}

func (e *AcpEventTextDelta) isAcpRuntimeEvent() {}

// AcpEventStatus is emitted for status updates.
type AcpEventStatus struct {
	Text string // Status message
}

func (e *AcpEventStatus) isAcpRuntimeEvent() {}

// AcpEventToolCall is emitted when a tool is called.
type AcpEventToolCall struct {
	ID        string                 // Tool call ID
	Name      string                 // Tool name/function name
	Arguments map[string]interface{} // Tool arguments
	Text      string                 // Tool call description (legacy)
	Status    string                 // Tool status: "pending", "in_progress", "completed", "failed"
}

func (e *AcpEventToolCall) isAcpRuntimeEvent() {}

// AcpEventDone is emitted when turn execution completes.
type AcpEventDone struct {
	StopReason string // Reason for completion
}

func (e *AcpEventDone) isAcpRuntimeEvent() {}

// AcpEventError is emitted when an error occurs.
type AcpEventError struct {
	Message   string // Error message
	Code      string // Error code
	Retryable bool   // Whether the error is retryable
}

func (e *AcpEventError) isAcpRuntimeEvent() {}

// AcpRuntime is the interface that all ACP runtime backends must implement.
// It provides methods for session lifecycle, turn execution, and control operations.
type AcpRuntime interface {
	// EnsureSession creates or retrieves an ACP session.
	// Returns a handle that identifies the session for subsequent operations.
	EnsureSession(ctx context.Context, input AcpRuntimeEnsureInput) (AcpRuntimeHandle, error)

	// RunTurn executes a single turn in an ACP session.
	// Returns a channel of events that will be closed when the turn completes.
	RunTurn(ctx context.Context, input AcpRuntimeTurnInput) (<-chan AcpRuntimeEvent, error)

	// GetCapabilities returns the capabilities of the runtime.
	// If the runtime doesn't support capability queries, it may return static capabilities.
	GetCapabilities(ctx context.Context, handle *AcpRuntimeHandle) (AcpRuntimeCapabilities, error)

	// GetStatus returns the current status of an ACP session.
	// Optional - runtimes that don't support status queries may return nil.
	GetStatus(ctx context.Context, handle AcpRuntimeHandle) (*AcpRuntimeStatus, error)

	// SetMode changes the runtime mode for an ACP session.
	// Optional - runtimes that don't support mode changes may return an error.
	SetMode(ctx context.Context, handle AcpRuntimeHandle, mode string) error

	// SetConfigOption sets a configuration option on an ACP session.
	// Optional - runtimes that don't support config options may return an error.
	SetConfigOption(ctx context.Context, handle AcpRuntimeHandle, key, value string) error

	// Doctor performs a health check on the runtime backend.
	// Optional - runtimes without health checks may return a static report.
	Doctor(ctx context.Context) (AcpRuntimeDoctorReport, error)

	// Cancel cancels an active turn in an ACP session.
	Cancel(ctx context.Context, handle AcpRuntimeHandle, reason string) error

	// Close closes an ACP session and releases resources.
	Close(ctx context.Context, handle AcpRuntimeHandle, reason string) error
}
