package sdk

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/smallnest/goclaw/acp/runtime"
)

// AcpGoSDKAdapter bridges github.com/coder/acp-go-sdk to goclaw's AcpRuntime interface.
// This adapter starts an ACP agent process and communicates via stdio using JSON-RPC 2.0.
type AcpGoSDKAdapter struct {
	agentPath       string           // Path to the ACP agent executable
	agentArgs       []string         // Arguments to pass to the agent
	env             []string         // Environment variables for the agent process
	timeout         time.Duration    // Timeout for operations
	processRegistry *processRegistry // Registry for tracking running processes
}

// DefaultBackendID is the backend ID used for registration
const DefaultBackendID = "acp-go-sdk"

func init() {
	// Create a default adapter instance that will be used for the backend registration
	defaultAdapter := NewAcpGoSDKAdapter("acp", []string{}, []string{})

	// Register the ACP SDK backend
	err := runtime.RegisterAcpRuntimeBackend(runtime.AcpRuntimeBackend{
		ID:      DefaultBackendID,
		Runtime: defaultAdapter,
		Healthy: func() bool {
			// Check if the acp command is available
			return isAcpAgentAvailable(defaultAdapter.agentPath)
		},
	})
	if err != nil {
		// Log the error but don't fail initialization
		// The backend may be configured later
	}
}

// NewAcpGoSDKAdapter creates a new ACP SDK adapter.
func NewAcpGoSDKAdapter(agentPath string, agentArgs []string, env []string) *AcpGoSDKAdapter {
	return &AcpGoSDKAdapter{
		agentPath:       agentPath,
		agentArgs:       agentArgs,
		env:             env,
		timeout:         30 * time.Second, // Default timeout
		processRegistry: newProcessRegistry(),
	}
}

// SetTimeout sets the timeout for operations.
func (a *AcpGoSDKAdapter) SetTimeout(timeout time.Duration) {
	a.timeout = timeout
}

// SetAgentConfig updates ACP agent launch settings for subsequent sessions.
func (a *AcpGoSDKAdapter) SetAgentConfig(agentPath string, agentArgs []string, env []string) {
	if agentPath != "" {
		a.agentPath = agentPath
	}
	if agentArgs != nil {
		a.agentArgs = append([]string(nil), agentArgs...)
	}
	if env != nil {
		a.env = append([]string(nil), env...)
	}
}

// acpProcess manages a running ACP agent process.
type acpProcess struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	stderr  io.ReadCloser
	session string
	mu      sync.Mutex
	cancel  context.CancelFunc
}

// acpSession represents an active ACP session.
type acpSession struct {
	process     *acpProcess
	sessionID   string
	handle      runtime.AcpRuntimeHandle
	cwd         string
	initialized bool
	mu          sync.Mutex
}

// processRegistry tracks running ACP agent processes by their session ID.
type processRegistry struct {
	processes map[string]*acpProcess
	mu        sync.RWMutex
}

// newProcessRegistry creates a new process registry.
func newProcessRegistry() *processRegistry {
	return &processRegistry{
		processes: make(map[string]*acpProcess),
	}
}

// Register registers a process with the given session ID.
func (r *processRegistry) Register(sessionID string, process *acpProcess) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.processes[sessionID] = process
}

// Get retrieves a process by session ID.
func (r *processRegistry) Get(sessionID string) (*acpProcess, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	process, ok := r.processes[sessionID]
	return process, ok
}

// Unregister removes a process from the registry.
func (r *processRegistry) Unregister(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.processes, sessionID)
}

// EnsureSession creates or retrieves an ACP session.
func (a *AcpGoSDKAdapter) EnsureSession(ctx context.Context, input runtime.AcpRuntimeEnsureInput) (runtime.AcpRuntimeHandle, error) {
	// Create timeout context if not provided
	if a.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.timeout)
		defer cancel()
	}

	// Start the ACP agent process
	process, err := a.startAgentProcess(ctx, input)
	if err != nil {
		return runtime.AcpRuntimeHandle{}, fmt.Errorf("failed to start ACP agent: %w", err)
	}

	// Initialize the session
	sessionID := generateSessionID()
	session := &acpSession{
		process:   process,
		sessionID: sessionID,
		cwd:       input.Cwd,
	}

	if err := a.initializeSession(ctx, session, input); err != nil {
		_ = a.closeProcess(process)
		return runtime.AcpRuntimeHandle{}, fmt.Errorf("failed to initialize ACP session: %w", err)
	}

	session.initialized = true

	// Create and return the handle
	handle := runtime.AcpRuntimeHandle{
		SessionKey:         input.SessionKey,
		Backend:            "acp-go-sdk",
		RuntimeSessionName: sessionID,
		Cwd:                input.Cwd,
		BackendSessionId:   sessionID,
	}

	session.handle = handle

	// Register the process in the registry
	a.processRegistry.Register(sessionID, process)

	return handle, nil
}

// startAgentProcess starts the ACP agent process with stdio pipes.
func (a *AcpGoSDKAdapter) startAgentProcess(ctx context.Context, input runtime.AcpRuntimeEnsureInput) (*acpProcess, error) {
	cmd := exec.CommandContext(ctx, a.agentPath, a.agentArgs...)

	// Set up environment
	cmd.Env = append(os.Environ(), a.env...)
	for k, v := range input.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Set working directory if specified
	if input.Cwd != "" {
		cmd.Dir = input.Cwd
	}

	// Create stdio pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, fmt.Errorf("failed to start agent process: %w", err)
	}

	// Create context for process management
	_, cancel := context.WithCancel(context.Background())

	process := &acpProcess{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		cancel: cancel,
	}

	return process, nil
}

// initializeSession sends the initialize and session/new requests.
func (a *AcpGoSDKAdapter) initializeSession(ctx context.Context, session *acpSession, input runtime.AcpRuntimeEnsureInput) error {
	// Send initialize request
	initReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{
				"roots": map[string]any{
					"list": true,
				},
			},
			"clientInfo": map[string]any{
				"name":    "goclaw",
				"version": "1.0.0",
			},
		},
	}

	initResp, err := a.sendRequest(ctx, session.process, initReq)
	if err != nil {
		return fmt.Errorf("initialize request failed: %w", err)
	}

	if initResp.Error != nil {
		return fmt.Errorf("initialize request error: %s", initResp.Error.Message)
	}

	// Send session/new request
	sessionReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "session/new",
		"params": map[string]any{
			"sessionId": session.sessionID,
		},
	}

	sessionResp, err := a.sendRequest(ctx, session.process, sessionReq)
	if err != nil {
		return fmt.Errorf("session/new request failed: %w", err)
	}

	if sessionResp.Error != nil {
		return fmt.Errorf("session/new request error: %s", sessionResp.Error.Message)
	}

	return nil
}

// RunTurn executes a turn in the ACP session.
func (a *AcpGoSDKAdapter) RunTurn(ctx context.Context, input runtime.AcpRuntimeTurnInput) (<-chan runtime.AcpRuntimeEvent, error) {
	eventChan := make(chan runtime.AcpRuntimeEvent, 10)

	// Get or create session process
	process, err := a.getProcessForHandle(ctx, input.Handle)
	if err != nil {
		close(eventChan)
		return nil, err
	}

	// Send session/prompt request
	promptReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      generateRequestID(),
		"method":  "session/prompt",
		"params": map[string]any{
			"sessionId": input.Handle.BackendSessionId,
			"prompt": map[string]any{
				"text": input.Text,
				"mode": string(input.Mode),
			},
		},
	}

	// Start goroutine to handle turn execution
	go func() {
		defer close(eventChan)

		resp, err := a.sendRequest(ctx, process, promptReq)
		if err != nil {
			eventChan <- &runtime.AcpEventError{
				Message:   fmt.Sprintf("prompt request failed: %v", err),
				Code:      runtime.ErrCodeTurnFailed,
				Retryable: false,
			}
			return
		}

		if resp.Error != nil {
			eventChan <- &runtime.AcpEventError{
				Message:   resp.Error.Message,
				Code:      runtime.ErrCodeTurnFailed,
				Retryable: false,
			}
			return
		}

		// Process the response
		// In a real implementation, this would stream events as they arrive
		// For now, we send a done event
		eventChan <- &runtime.AcpEventDone{
			StopReason: "completed",
		}
	}()

	return eventChan, nil
}

// GetCapabilities returns the capabilities of the ACP runtime.
func (a *AcpGoSDKAdapter) GetCapabilities(ctx context.Context, handle *runtime.AcpRuntimeHandle) (runtime.AcpRuntimeCapabilities, error) {
	capabilities := runtime.AcpRuntimeCapabilities{
		Controls: []runtime.AcpRuntimeControl{
			runtime.AcpControlSessionSetMode,
			runtime.AcpControlSessionSetConfigOption,
			runtime.AcpControlSessionStatus,
		},
		ConfigOptionKeys: []string{},
	}

	return capabilities, nil
}

// GetStatus returns the current status of an ACP session.
func (a *AcpGoSDKAdapter) GetStatus(ctx context.Context, handle runtime.AcpRuntimeHandle) (*runtime.AcpRuntimeStatus, error) {
	process, err := a.getProcessForHandle(ctx, handle)
	if err != nil {
		return nil, err
	}

	statusReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      generateRequestID(),
		"method":  "session/status",
		"params": map[string]any{
			"sessionId": handle.BackendSessionId,
		},
	}

	resp, err := a.sendRequest(ctx, process, statusReq)
	if err != nil {
		return nil, fmt.Errorf("status request failed: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("status request error: %s", resp.Error.Message)
	}

	status := &runtime.AcpRuntimeStatus{
		Summary:          "active",
		BackendSessionId: handle.BackendSessionId,
	}

	// Parse result from json.RawMessage
	if len(resp.Result) > 0 {
		var result map[string]any
		if err := json.Unmarshal(resp.Result, &result); err == nil {
			if summary, ok := result["summary"].(string); ok {
				status.Summary = summary
			}
		}
	}

	return status, nil
}

// SetMode changes the runtime mode for an ACP session.
func (a *AcpGoSDKAdapter) SetMode(ctx context.Context, handle runtime.AcpRuntimeHandle, mode string) error {
	process, err := a.getProcessForHandle(ctx, handle)
	if err != nil {
		return err
	}

	setModeReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      generateRequestID(),
		"method":  "session/set_mode",
		"params": map[string]any{
			"sessionId": handle.BackendSessionId,
			"mode":      mode,
		},
	}

	resp, err := a.sendRequest(ctx, process, setModeReq)
	if err != nil {
		return fmt.Errorf("set_mode request failed: %w", err)
	}

	if resp.Error != nil {
		return fmt.Errorf("set_mode request error: %s", resp.Error.Message)
	}

	return nil
}

// SetConfigOption sets a configuration option on an ACP session.
func (a *AcpGoSDKAdapter) SetConfigOption(ctx context.Context, handle runtime.AcpRuntimeHandle, key, value string) error {
	process, err := a.getProcessForHandle(ctx, handle)
	if err != nil {
		return err
	}

	setConfigReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      generateRequestID(),
		"method":  "session/set_config_option",
		"params": map[string]any{
			"sessionId": handle.BackendSessionId,
			"key":       key,
			"value":     value,
		},
	}

	resp, err := a.sendRequest(ctx, process, setConfigReq)
	if err != nil {
		return fmt.Errorf("set_config_option request failed: %w", err)
	}

	if resp.Error != nil {
		return fmt.Errorf("set_config_option request error: %s", resp.Error.Message)
	}

	return nil
}

// Doctor performs a health check on the ACP runtime.
func (a *AcpGoSDKAdapter) Doctor(ctx context.Context) (runtime.AcpRuntimeDoctorReport, error) {
	report := runtime.AcpRuntimeDoctorReport{
		Ok:      true,
		Message: "ACP runtime is available",
	}

	// Check if agent executable exists
	if _, err := os.Stat(a.agentPath); os.IsNotExist(err) {
		report.Ok = false
		report.Code = runtime.ErrCodeBackendMissing
		report.Message = fmt.Sprintf("ACP agent executable not found: %s", a.agentPath)
		report.InstallCommand = fmt.Sprintf("Install the ACP agent at: %s", a.agentPath)
		return report, nil
	}

	// Check if agent is executable
	if !isExecutable(a.agentPath) {
		report.Ok = false
		report.Code = runtime.ErrCodeBackendUnavailable
		report.Message = fmt.Sprintf("ACP agent is not executable: %s", a.agentPath)
		return report, nil
	}

	return report, nil
}

// Cancel cancels an active turn in an ACP session.
func (a *AcpGoSDKAdapter) Cancel(ctx context.Context, handle runtime.AcpRuntimeHandle, reason string) error {
	process, err := a.getProcessForHandle(ctx, handle)
	if err != nil {
		return err
	}

	cancelReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      generateRequestID(),
		"method":  "session/cancel",
		"params": map[string]any{
			"sessionId": handle.BackendSessionId,
			"reason":    reason,
		},
	}

	resp, err := a.sendRequest(ctx, process, cancelReq)
	if err != nil {
		return fmt.Errorf("cancel request failed: %w", err)
	}

	if resp.Error != nil {
		return fmt.Errorf("cancel request error: %s", resp.Error.Message)
	}

	return nil
}

// Close closes an ACP session and releases resources.
func (a *AcpGoSDKAdapter) Close(ctx context.Context, handle runtime.AcpRuntimeHandle, reason string) error {
	process, err := a.getProcessForHandle(ctx, handle)
	if err != nil {
		return err
	}

	// Unregister the process from the registry
	a.processRegistry.Unregister(handle.BackendSessionId)

	return a.closeProcess(process)
}

// closeProcess closes the agent process and associated resources.
func (a *AcpGoSDKAdapter) closeProcess(process *acpProcess) error {
	process.mu.Lock()
	defer process.mu.Unlock()

	var errs []error

	// Close stdin
	if process.stdin != nil {
		if err := process.stdin.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close stdin: %w", err))
		}
	}

	// Close stdout
	if process.stdout != nil {
		if err := process.stdout.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close stdout: %w", err))
		}
	}

	// Close stderr
	if process.stderr != nil {
		if err := process.stderr.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close stderr: %w", err))
		}
	}

	// Cancel context
	if process.cancel != nil {
		process.cancel()
	}

	// Wait for process to exit
	if process.cmd != nil && process.cmd.Process != nil {
		if err := process.cmd.Wait(); err != nil {
			errs = append(errs, fmt.Errorf("process wait failed: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}

	return nil
}

// jsonRPCRequest represents a JSON-RPC 2.0 request.
type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      any         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// jsonRPCResponse represents a JSON-RPC 2.0 response.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

// jsonRPCError represents a JSON-RPC 2.0 error.
type jsonRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// sendRequest sends a JSON-RPC request and waits for the response.
func (a *AcpGoSDKAdapter) sendRequest(ctx context.Context, process *acpProcess, req map[string]any) (*jsonRPCResponse, error) {
	process.mu.Lock()
	defer process.mu.Unlock()

	// Marshal request
	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send request
	if _, err := fmt.Fprintln(process.stdin, string(reqData)); err != nil {
		return nil, fmt.Errorf("failed to write request: %w", err)
	}

	// Read response
	scanner := bufio.NewScanner(process.stdout)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}
		return nil, fmt.Errorf("unexpected EOF")
	}

	respData := scanner.Text()

	// Parse response
	var resp jsonRPCResponse
	if err := json.Unmarshal([]byte(respData), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp, nil
}

// getProcessForHandle retrieves the process for a given handle.
func (a *AcpGoSDKAdapter) getProcessForHandle(ctx context.Context, handle runtime.AcpRuntimeHandle) (*acpProcess, error) {
	process, ok := a.processRegistry.Get(handle.BackendSessionId)
	if !ok {
		return nil, fmt.Errorf("process not found for session: %s", handle.BackendSessionId)
	}
	return process, nil
}

// generateSessionID generates a unique session ID.
func generateSessionID() string {
	return uuid.New().String()
}

// generateRequestID generates a unique request ID.
func generateRequestID() int {
	return int(time.Now().UnixNano())
}

// isExecutable checks if a file is executable.
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	// Check if file mode has any execute bits
	mode := info.Mode()
	return mode&0111 != 0
}

// isAcpAgentAvailable checks if the ACP agent is available in PATH.
func isAcpAgentAvailable(agentPath string) bool {
	// First check if we can find the command in PATH
	if agentPath == "" || agentPath == "acp" {
		_, err := exec.LookPath("acp")
		return err == nil
	}

	// Check if the specified path exists and is executable
	return isExecutable(agentPath)
}
