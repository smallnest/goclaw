package sdk

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
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
	_ = runtime.RegisterAcpRuntimeBackend(runtime.AcpRuntimeBackend{
		ID:      DefaultBackendID,
		Runtime: defaultAdapter,
		Healthy: func() bool {
			// Check if the acp command is available
			return isAcpAgentAvailable(defaultAdapter.agentPath)
		},
	})
	// Error is intentionally ignored - the backend may be configured later
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
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
	mu     sync.Mutex
	cancel context.CancelFunc
}

// acpSession represents an active ACP session.
type acpSession struct {
	process     *acpProcess
	sessionID   string
	handle      runtime.AcpRuntimeHandle
	cwd         string
	initialized bool
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

		// Send the request
		if err := a.sendRequestNoResponse(process, promptReq); err != nil {
			eventChan <- &runtime.AcpEventError{
				Message:   fmt.Sprintf("prompt request failed: %v", err),
				Code:      runtime.ErrCodeTurnFailed,
				Retryable: false,
			}
			return
		}

		// Read streaming responses from stdout
		scanner := bufio.NewScanner(process.stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			// Parse the notification/response
			events := a.parseACPResponse(line)
			for _, event := range events {
				select {
				case eventChan <- event:
				case <-ctx.Done():
					eventChan <- &runtime.AcpEventError{
						Message:   "turn execution canceled",
						Code:      runtime.ErrCodeTurnCanceled,
						Retryable: false,
					}
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			eventChan <- &runtime.AcpEventError{
				Message:   fmt.Sprintf("error reading responses: %v", err),
				Code:      runtime.ErrCodeTurnFailed,
				Retryable: false,
			}
			return
		}

		// Send completion event
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

// sendRequestNoResponse sends a JSON-RPC request without waiting for response.
func (a *AcpGoSDKAdapter) sendRequestNoResponse(process *acpProcess, req map[string]any) error {
	process.mu.Lock()
	defer process.mu.Unlock()

	// Marshal request
	reqData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send request
	if _, err := fmt.Fprintln(process.stdin, string(reqData)); err != nil {
		return fmt.Errorf("failed to write request: %w", err)
	}

	return nil
}

// parseACPResponse parses a JSON-RPC response/notification from ACP agent
// and returns one or more ACP runtime events.
func (a *AcpGoSDKAdapter) parseACPResponse(line string) []runtime.AcpRuntimeEvent {
	var events []runtime.AcpRuntimeEvent

	// Parse as JSON-RPC message
	var rawMsg json.RawMessage
	if err := json.Unmarshal([]byte(line), &rawMsg); err != nil {
		// Not valid JSON, skip
		return events
	}

	// Try to parse as jsonRPCResponse
	var resp struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      any             `json:"id"`
		Result  json.RawMessage `json:"result,omitempty"`
		Error   *jsonRPCError   `json:"error,omitempty"`
	}
	if err := json.Unmarshal(rawMsg, &resp); err == nil && resp.JSONRPC == "2.0" {
		// Handle response
		if resp.Error != nil {
			events = append(events, &runtime.AcpEventError{
				Message:   resp.Error.Message,
				Code:      runtime.ErrCodeTurnFailed,
				Retryable: false,
			})
		} else if len(resp.Result) > 0 {
			// Parse result - could be text content or structured data
			var resultStr string
			if err := json.Unmarshal(resp.Result, &resultStr); err == nil {
				events = append(events, &runtime.AcpEventTextDelta{
					Text:   resultStr,
					Stream: "output",
				})
			} else {
				// Try to parse as object with content field
				var resultObj map[string]any
				if err := json.Unmarshal(resp.Result, &resultObj); err == nil {
					events = append(events, a.parseResultObject(resultObj)...)
				}
			}
		}
		return events
	}

	// Try to parse as jsonRPCNotification (session/update)
	var notif struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  struct {
			SessionID string `json:"sessionId"`
			Update    struct {
				SessionUpdate string `json:"sessionUpdate"`
				Text          string `json:"text"`
				Stream        string `json:"stream"`

				// Tool call fields
				ToolCallID string `json:"toolCallId"`
				Title      string `json:"title"`
				Kind       string `json:"kind"`
				Status     string `json:"status"`

				// Agent message/thought fields
				Content string `json:"content"`
			} `json:"update"`
		} `json:"params"`
	}
	if err := json.Unmarshal(rawMsg, &notif); err == nil && notif.JSONRPC == "2.0" && notif.Method == "session/update" {
		update := notif.Params.Update

		switch update.SessionUpdate {
		case "agent_message_chunk", "user_message_chunk":
			events = append(events, &runtime.AcpEventTextDelta{
				Text:   update.Content,
				Stream: "output",
			})
		case "agent_thought_chunk":
			events = append(events, &runtime.AcpEventTextDelta{
				Text:   update.Content,
				Stream: "thought",
			})
		case "tool_call":
			// Extract tool call details from title or content
			toolName, args := a.parseToolCallString(update.Content)
			if toolName == "" {
				toolName, args = a.parseToolCallString(update.Title)
			}

			// Map ACP tool names to goclaw tool names
			mappedName := a.mapACPToolToGoclaw(toolName)
			if mappedName != "" {
				toolName = mappedName
			}

			events = append(events, &runtime.AcpEventToolCall{
				ID:        update.ToolCallID,
				Name:      toolName,
				Arguments: args,
				Text:      update.Title,
				Status:    update.Status,
			})
		case "tool_call_update":
			// Tool status update - could include result
			events = append(events, &runtime.AcpEventToolCall{
				ID:     update.ToolCallID,
				Status: update.Status,
				Text:   update.Content,
			})
		default:
			// Unknown update type, try to send as text
			if update.Content != "" {
				events = append(events, &runtime.AcpEventTextDelta{
					Text:   update.Content,
					Stream: "output",
				})
			}
		}
	}

	return events
}

// parseResultObject parses a result object and returns events.
func (a *AcpGoSDKAdapter) parseResultObject(obj map[string]any) []runtime.AcpRuntimeEvent {
	var events []runtime.AcpRuntimeEvent

	// Check for content field
	if content, ok := obj["content"].(string); ok {
		events = append(events, &runtime.AcpEventTextDelta{
			Text:   content,
			Stream: "output",
		})
	}

	// Check for tool_calls field (OpenAI-style format)
	if toolCalls, ok := obj["tool_calls"].([]any); ok {
		for _, tc := range toolCalls {
			if tcMap, ok := tc.(map[string]any); ok {
				if function, ok := tcMap["function"].(map[string]any); ok {
					name, _ := function["name"].(string)
					argsStr, _ := function["arguments"].(string)

					var args map[string]any
					if argsStr != "" {
						_ = json.Unmarshal([]byte(argsStr), &args)
					}

					tcID, _ := tcMap["id"].(string)
					events = append(events, &runtime.AcpEventToolCall{
						ID:        tcID,
						Name:      name,
						Arguments: args,
						Text:      fmt.Sprintf("%s(%s)", name, argsStr),
						Status:    "pending",
					})
				}
			}
		}
	}

	return events
}

// parseToolCallString parses a tool call string and extracts name and arguments.
// Supports formats like: "tool_name(arg1=value1, arg2=value2)" or JSON arguments.
func (a *AcpGoSDKAdapter) parseToolCallString(s string) (string, map[string]any) {
	if s == "" {
		return "", nil
	}

	// Try to parse as JSON object first
	var args map[string]any
	if err := json.Unmarshal([]byte(s), &args); err == nil {
		// If it's a simple object, we need the tool name from context
		// This will be handled by the caller
		return "", args
	}

	// Try to extract tool name from format like "tool_name(...)"
	leftParen := strings.Index(s, "(")
	rightParen := strings.LastIndex(s, ")")
	if leftParen > 0 && rightParen > leftParen {
		name := strings.TrimSpace(s[:leftParen])
		argsStr := s[leftParen+1 : rightParen]

		// Parse arguments
		args = make(map[string]any)
		if argsStr != "" {
			// Split by comma (simple parsing)
			parts := strings.Split(argsStr, ",")
			for _, part := range parts {
				kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
				if len(kv) == 2 {
					args[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
				}
			}
		}

		return name, args
	}

	return "", nil
}

// mapACPToolToGoclaw maps ACP tool names to goclaw tool names.
// ACP agents may use different tool names than goclaw's internal tools.
func (a *AcpGoSDKAdapter) mapACPToolToGoclaw(acpToolName string) string {
	// Mapping of common ACP tool names to goclaw tool names
	switch acpToolName {
	case "exec", "execute", "run", "shell", "bash", "sh":
		return "run_shell"
	case "read_file", "read", "cat":
		return "read_file"
	case "write_file", "write":
		return "write_file"
	case "edit_file", "edit":
		return "edit_file"
	case "search", "find":
		return "search"
	case "http_request", "fetch", "curl":
		return "http_request"
	case "browser", "browse":
		return "browser"
	default:
		// If no mapping found, return the original name
		// The tool may already be a goclaw tool name
		return acpToolName
	}
}
