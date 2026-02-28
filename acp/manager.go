package acp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/smallnest/goclaw/acp/runtime"
	_ "github.com/smallnest/goclaw/acp/sdk" // Blank import to trigger backend registration
	"github.com/smallnest/goclaw/channels"
	"github.com/smallnest/goclaw/config"
)

// Global manager singleton
var (
	globalManager                *Manager
	globalManagerMu              sync.RWMutex
	globalThreadBindingService   *channels.ThreadBindingService
	globalThreadBindingServiceMu sync.RWMutex
)

// GetGlobalManager returns the global ACP manager instance.
// If no global manager exists, returns nil.
func GetGlobalManager() *Manager {
	globalManagerMu.RLock()
	defer globalManagerMu.RUnlock()
	return globalManager
}

// SetGlobalManager sets the global ACP manager instance.
// This should be called once during application initialization.
func SetGlobalManager(manager *Manager) {
	globalManagerMu.Lock()
	defer globalManagerMu.Unlock()
	globalManager = manager
}

// GetOrCreateGlobalManager gets the existing global manager or creates a new one.
func GetOrCreateGlobalManager(cfg *config.Config) *Manager {
	globalManagerMu.RLock()
	mgr := globalManager
	globalManagerMu.RUnlock()

	if mgr != nil {
		configureRuntimeFromConfig(cfg)
		return mgr
	}

	globalManagerMu.Lock()
	defer globalManagerMu.Unlock()

	// Double-check after acquiring write lock
	if globalManager != nil {
		configureRuntimeFromConfig(cfg)
		return globalManager
	}

	globalManager = NewManager(cfg)
	configureRuntimeFromConfig(cfg)
	return globalManager
}

type runtimeAgentConfigurable interface {
	SetAgentConfig(agentPath string, agentArgs []string, env []string)
}

func configureRuntimeFromConfig(cfg *config.Config) {
	if cfg == nil {
		return
	}

	backendID := ResolveAcpBackend(cfg, "")
	if backendID == "" {
		backendID = "acp-go-sdk"
	}

	backend := runtime.GetAcpRuntimeBackend(backendID)
	if backend == nil || backend.Runtime == nil {
		return
	}

	if configurable, ok := backend.Runtime.(runtimeAgentConfigurable); ok {
		configurable.SetAgentConfig(cfg.ACP.AgentPath, cfg.ACP.AgentArgs, cfg.ACP.AgentEnv)
	}
}

// SetGlobalThreadBindingService sets the global thread binding service.
func SetGlobalThreadBindingService(service *channels.ThreadBindingService) {
	globalThreadBindingServiceMu.Lock()
	defer globalThreadBindingServiceMu.Unlock()
	globalThreadBindingService = service
}

// GetGlobalThreadBindingService returns the global thread binding service.
func GetGlobalThreadBindingService() *channels.ThreadBindingService {
	globalThreadBindingServiceMu.RLock()
	defer globalThreadBindingServiceMu.RUnlock()
	return globalThreadBindingService
}

func unbindThreadBindingsForSession(sessionKey string) {
	service := GetGlobalThreadBindingService()
	if service == nil || sessionKey == "" {
		return
	}

	for _, binding := range service.GetBySession(sessionKey) {
		if binding == nil {
			continue
		}
		_ = service.Unbind(binding.ID)
	}
}

// Manager manages ACP session lifecycle.
type Manager struct {
	actorQueue          *ActorQueue
	runtimeCache        *RuntimeCache
	activeTurnBySession map[string]*ActiveTurnState
	turnLatencyStats    *TurnLatencyStats
	errorCountsByCode   map[string]int
	mu                  sync.RWMutex
	sessionLimitMu      sync.Mutex
	pendingSessionInits int

	// Dependencies
	cfg *config.Config
}

// ActorQueue serializes operations per session.
type ActorQueue struct {
	mu           sync.Mutex
	queues       map[string]*chan struct{}
	pendingByKey map[string]int
	pendingCount int // Track total pending operations
}

// NewActorQueue creates a new actor queue.
func NewActorQueue() *ActorQueue {
	return &ActorQueue{
		queues:       make(map[string]*chan struct{}),
		pendingByKey: make(map[string]int),
	}
}

// Run executes a function with session-level serialization.
func (q *ActorQueue) Run(sessionKey string, fn func() error) error {
	q.mu.Lock()

	// Get or create queue for this session
	queue, exists := q.queues[sessionKey]
	if !exists {
		ch := make(chan struct{}, 1)
		ch <- struct{}{} // Initially available
		queue = &ch
		q.queues[sessionKey] = queue
	}

	// Track pending operation
	q.pendingByKey[sessionKey]++
	q.pendingCount++
	q.mu.Unlock()

	// Wait for turn
	<-(*queue)
	defer func() {
		(*queue) <- struct{}{}
		// Decrease pending count when done
		q.mu.Lock()
		if q.pendingByKey[sessionKey] > 0 {
			q.pendingByKey[sessionKey]--
		}
		if q.pendingByKey[sessionKey] == 0 {
			delete(q.pendingByKey, sessionKey)
			delete(q.queues, sessionKey)
		}
		q.pendingCount--
		q.mu.Unlock()
	}()

	return fn()
}

// GetTotalPendingCount returns the total number of pending operations.
func (q *ActorQueue) GetTotalPendingCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.pendingCount
}

// GetTailMapForTesting returns the queues map for testing.
func (q *ActorQueue) GetTailMapForTesting() map[string]*chan struct{} {
	q.mu.Lock()
	defer q.mu.Unlock()

	return q.queues
}

// ActiveTurnState tracks an active turn execution.
type ActiveTurnState struct {
	runtime         runtime.AcpRuntime
	handle          runtime.AcpRuntimeHandle
	abortController context.CancelFunc
	cancelDone      chan struct{}
	cancelErr       error
}

// TurnLatencyStats tracks turn execution statistics.
type TurnLatencyStats struct {
	completed int
	failed    int
	totalMs   int64
	maxMs     int64
	mu        sync.RWMutex
}

// RecordCompletion records a completed turn.
func (s *TurnLatencyStats) RecordCompletion(startedAt time.Time, err error) {
	duration := time.Since(startedAt)
	durationMs := duration.Milliseconds()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.totalMs += durationMs
	if durationMs > s.maxMs {
		s.maxMs = durationMs
	}

	if err != nil {
		s.failed++
	} else {
		s.completed++
	}
}

// GetAverageLatency returns the average latency in milliseconds.
func (s *TurnLatencyStats) GetAverageLatency() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := s.completed + s.failed
	if total == 0 {
		return 0
	}
	return s.totalMs / int64(total)
}

// GetStats returns the current statistics.
func (s *TurnLatencyStats) GetStats() (completed, failed int, totalMs, maxMs int64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.completed, s.failed, s.totalMs, s.maxMs
}

// NewManager creates a new ACP session manager.
func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		actorQueue:          NewActorQueue(),
		runtimeCache:        NewRuntimeCache(),
		activeTurnBySession: make(map[string]*ActiveTurnState),
		turnLatencyStats:    &TurnLatencyStats{},
		errorCountsByCode:   make(map[string]int),
		cfg:                 cfg,
	}
}

// SessionResolution represents the result of resolving a session.
type SessionResolution struct {
	Kind       string // "none", "ready", "stale"
	SessionKey string
	Meta       *SessionAcpMeta
	Error      error
}

// SessionAcpMeta contains ACP session metadata.
type SessionAcpMeta struct {
	Backend            string                        `json:"backend"`
	Agent              string                        `json:"agent"`
	RuntimeSessionName string                        `json:"runtime_session_name"`
	Identity           *SessionIdentity              `json:"identity,omitempty"`
	Mode               runtime.AcpRuntimeSessionMode `json:"mode"`
	RuntimeOptions     map[string]any                `json:"runtime_options,omitempty"`
	Cwd                string                        `json:"cwd"`
	State              string                        `json:"state"` // "idle", "running", "error"
	LastError          string                        `json:"last_error,omitempty"`
	LastActivityAt     int64                         `json:"last_activity_at"`
}

// AcpSessionStatus represents the status of an ACP session.
type AcpSessionStatus struct {
	SessionKey     string                         `json:"session_key"`
	Backend        string                         `json:"backend"`
	Agent          string                         `json:"agent"`
	Identity       *SessionIdentity               `json:"identity,omitempty"`
	State          string                         `json:"state"`
	Mode           runtime.AcpRuntimeSessionMode  `json:"mode"`
	RuntimeOptions map[string]any                 `json:"runtime_options"`
	Capabilities   runtime.AcpRuntimeCapabilities `json:"capabilities"`
	RuntimeStatus  *runtime.AcpRuntimeStatus      `json:"runtime_status,omitempty"`
	LastActivityAt int64                          `json:"last_activity_at"`
	LastError      string                         `json:"last_error,omitempty"`
}

// ManagerObservabilitySnapshot represents observability data.
type ManagerObservabilitySnapshot struct {
	RuntimeCache RuntimeCacheSnapshot `json:"runtime_cache"`
	Turns        TurnsSnapshot        `json:"turns"`
	ErrorsByCode map[string]int       `json:"errors_by_code"`
}

// RuntimeCacheSnapshot represents runtime cache statistics.
type RuntimeCacheSnapshot struct {
	ActiveSessions int    `json:"active_sessions"`
	IdleTtlMs      int64  `json:"idle_ttl_ms"`
	EvictedTotal   int    `json:"evicted_total"`
	LastEvictedAt  *int64 `json:"last_evicted_at,omitempty"`
}

// TurnsSnapshot represents turn execution statistics.
type TurnsSnapshot struct {
	Active           int   `json:"active"`
	QueueDepth       int   `json:"queue_depth"`
	Completed        int   `json:"completed"`
	Failed           int   `json:"failed"`
	AverageLatencyMs int64 `json:"average_latency_ms"`
	MaxLatencyMs     int64 `json:"max_latency_ms"`
}

// ResolveSession resolves a session to determine if it has ACP capabilities.
func (m *Manager) ResolveSession(sessionKey string) SessionResolution {
	sessionKey = normalizeSessionKey(sessionKey)
	if sessionKey == "" {
		return SessionResolution{
			Kind:       "none",
			SessionKey: "",
		}
	}

	// Check if session exists in runtime cache
	cached := m.runtimeCache.Get(sessionKey)
	if cached == nil {
		return SessionResolution{
			Kind:       "none",
			SessionKey: sessionKey,
		}
	}

	// Session exists in cache - it's ACP-enabled
	return SessionResolution{
		Kind:       "ready",
		SessionKey: sessionKey,
		Meta: &SessionAcpMeta{
			Backend:            cached.backend,
			RuntimeSessionName: cached.handle.RuntimeSessionName,
			Mode:               cached.mode,
			Cwd:                cached.cwd,
			State:              "idle",
			LastActivityAt:     time.Now().UnixMilli(),
		},
	}
}

// GetObservabilitySnapshot returns observability data.
func (m *Manager) GetObservabilitySnapshot() ManagerObservabilitySnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	completed, failed, totalMs, maxMs := m.turnLatencyStats.GetStats()
	averageLatency := int64(0)
	total := completed + failed
	if total > 0 {
		averageLatency = totalMs / int64(total)
	}

	return ManagerObservabilitySnapshot{
		RuntimeCache: m.runtimeCache.GetSnapshot(),
		Turns: TurnsSnapshot{
			Active:           len(m.activeTurnBySession),
			QueueDepth:       m.actorQueue.GetTotalPendingCount(),
			Completed:        int(completed),
			Failed:           int(failed),
			AverageLatencyMs: averageLatency,
			MaxLatencyMs:     maxMs,
		},
		ErrorsByCode: m.GetErrorCounts(),
	}
}

// InitializeSession initializes a new ACP session.
type InitializeSessionInput struct {
	Cfg        *config.Config
	SessionKey string
	Agent      string
	Mode       runtime.AcpRuntimeSessionMode
	Cwd        string
	BackendID  string
}

func (m *Manager) InitializeSession(ctx context.Context, input InitializeSessionInput) (*runtime.AcpRuntimeHandle, *SessionAcpMeta, error) {
	sessionKey := normalizeSessionKey(input.SessionKey)
	if sessionKey == "" {
		return nil, nil, runtime.NewSessionInitError("ACP session key is required", nil)
	}

	agent := normalizeAgentID(input.Agent)

	// Evict idle runtime handles
	_ = m.evictIdleRuntimeHandles(input.Cfg)

	// Run with actor queue for session serialization
	var resultHandle *runtime.AcpRuntimeHandle
	var resultMeta *SessionAcpMeta
	var resultErr error

	err := m.actorQueue.Run(sessionKey, func() error {
		if existing := m.runtimeCache.Get(sessionKey); existing != nil {
			meta := &SessionAcpMeta{
				Backend:            existing.backend,
				Agent:              existing.agent,
				RuntimeSessionName: existing.handle.RuntimeSessionName,
				Mode:               existing.mode,
				Cwd:                existing.cwd,
				State:              "idle",
				LastActivityAt:     time.Now().UnixMilli(),
			}
			handle := existing.handle
			resultHandle = &handle
			resultMeta = meta
			return nil
		}

		maxSessions := ResolveAcpMaxConcurrentSessions(input.Cfg)
		if maxSessions > 0 {
			release, acquireErr := m.acquireSessionInitSlot(maxSessions)
			if acquireErr != nil {
				resultErr = acquireErr
				return acquireErr
			}
			defer release()
		}

		// Get runtime backend
		backendID := input.BackendID
		if backendID == "" {
			backendID = input.Cfg.ACP.Backend
		}
		backend, err := runtime.RequireAcpRuntimeBackend(backendID)
		if err != nil {
			resultErr = err
			return err
		}

		rt := backend.Runtime

		// Ensure session
		ensureInput := runtime.AcpRuntimeEnsureInput{
			SessionKey: sessionKey,
			Agent:      agent,
			Mode:       input.Mode,
			Cwd:        input.Cwd,
		}

		handle, err := rt.EnsureSession(ctx, ensureInput)
		if err != nil {
			resultErr = runtime.NewSessionInitError("Could not initialize ACP session runtime", err)
			return resultErr
		}

		// Create metadata
		meta := &SessionAcpMeta{
			Backend:            handle.Backend,
			Agent:              agent,
			RuntimeSessionName: handle.RuntimeSessionName,
			Identity: &SessionIdentity{
				State:         "pending",
				Source:        "ensure",
				LastUpdatedAt: time.Now().UnixMilli(),
			},
			Mode:           input.Mode,
			Cwd:            handle.Cwd,
			State:          "idle",
			LastActivityAt: time.Now().UnixMilli(),
		}

		// Cache runtime state
		m.runtimeCache.Set(sessionKey, &CachedRuntimeState{
			runtime: rt,
			handle:  handle,
			backend: handle.Backend,
			agent:   agent,
			mode:    input.Mode,
			cwd:     handle.Cwd,
		})

		resultHandle = &handle
		resultMeta = meta
		return nil
	})

	if err != nil {
		return nil, nil, resultErr
	}

	return resultHandle, resultMeta, nil
}

func (m *Manager) acquireSessionInitSlot(maxSessions int) (func(), error) {
	m.sessionLimitMu.Lock()
	defer m.sessionLimitMu.Unlock()

	active := m.runtimeCache.Size() + m.pendingSessionInits
	if active >= maxSessions {
		return nil, runtime.NewSessionLimitError(active, maxSessions)
	}

	m.pendingSessionInits++
	released := false
	return func() {
		m.sessionLimitMu.Lock()
		defer m.sessionLimitMu.Unlock()
		if released {
			return
		}
		released = true
		if m.pendingSessionInits > 0 {
			m.pendingSessionInits--
		}
	}, nil
}

// GetSessionStatus returns the status of an ACP session.
type GetSessionStatusInput struct {
	Cfg        *config.Config
	SessionKey string
}

func (m *Manager) GetSessionStatus(ctx context.Context, input GetSessionStatusInput) (*AcpSessionStatus, error) {
	sessionKey := normalizeSessionKey(input.SessionKey)
	if sessionKey == "" {
		return nil, runtime.NewSessionInitError("ACP session key is required", nil)
	}

	// Evict idle runtime handles
	_ = m.evictIdleRuntimeHandles(input.Cfg)

	var resultStatus *AcpSessionStatus
	var resultErr error

	err := m.actorQueue.Run(sessionKey, func() error {
		// Resolve session
		resolution := m.ResolveSession(sessionKey)
		if resolution.Kind == "none" {
			resultErr = runtime.NewSessionInitError(fmt.Sprintf("Session is not ACP-enabled: %s", sessionKey), nil)
			return resultErr
		}

		// Get or ensure runtime handle
		cached := m.runtimeCache.Get(sessionKey)
		if cached == nil {
			resultErr = runtime.NewSessionInitError(fmt.Sprintf("Session not found: %s", sessionKey), nil)
			return resultErr
		}

		rt := cached.runtime
		handle := cached.handle

		// Get capabilities
		capabilities, _ := rt.GetCapabilities(ctx, &handle)

		// Get status
		var runtimeStatus *runtime.AcpRuntimeStatus
		// Check if runtime supports GetStatus by trying it
		status, err := rt.GetStatus(ctx, handle)
		if err == nil && status != nil {
			runtimeStatus = status
		}

		// Build status response
		resultStatus = &AcpSessionStatus{
			SessionKey:     sessionKey,
			Backend:        handle.Backend,
			Agent:          cached.agent,
			State:          "idle",
			Mode:           cached.mode,
			RuntimeOptions: make(map[string]any),
			Capabilities:   capabilities,
			RuntimeStatus:  runtimeStatus,
			LastActivityAt: time.Now().UnixMilli(),
		}

		return nil
	})

	if err != nil {
		return nil, resultErr
	}

	return resultStatus, nil
}

// SetSessionRuntimeMode sets the runtime mode for an ACP session.
type SetSessionRuntimeModeInput struct {
	Cfg         *config.Config
	SessionKey  string
	RuntimeMode string
}

func (m *Manager) SetSessionRuntimeMode(ctx context.Context, input SetSessionRuntimeModeInput) (map[string]any, error) {
	sessionKey := normalizeSessionKey(input.SessionKey)
	if sessionKey == "" {
		return nil, runtime.NewSessionInitError("ACP session key is required", nil)
	}

	var resultOptions map[string]any
	var resultErr error

	err := m.actorQueue.Run(sessionKey, func() error {
		cached := m.runtimeCache.Get(sessionKey)
		if cached == nil {
			resultErr = runtime.NewSessionInitError(fmt.Sprintf("Session not found: %s", sessionKey), nil)
			return resultErr
		}

		actorKey := normalizeActorKey(sessionKey)
		m.mu.RLock()
		_, hasActiveTurn := m.activeTurnBySession[actorKey]
		m.mu.RUnlock()
		if hasActiveTurn {
			resultErr = runtime.NewTurnError(fmt.Sprintf("ACP turn already active for session: %s", sessionKey), nil)
			return resultErr
		}

		rt := cached.runtime
		handle := cached.handle

		// Try SetMode - it may not be supported by all backends
		if err := rt.SetMode(ctx, handle, input.RuntimeMode); err != nil {
			// Check if this is an unsupported operation
			if runtime.GetAcpErrorCode(err) == runtime.ErrCodeBackendUnsupportedControl {
				resultErr = runtime.NewUnsupportedControlError(handle.Backend, runtime.AcpControlSessionSetMode)
			} else {
				resultErr = runtime.NewTurnError("Could not update ACP runtime mode", err)
			}
			return resultErr
		}

		resultOptions = map[string]any{
			"runtimeMode": input.RuntimeMode,
		}
		return nil
	})

	if err != nil {
		return nil, resultErr
	}

	return resultOptions, nil
}

// SetSessionConfigOption sets a config option on an ACP session.
type SetSessionConfigOptionInput struct {
	Cfg        *config.Config
	SessionKey string
	Key        string
	Value      string
}

func (m *Manager) SetSessionConfigOption(ctx context.Context, input SetSessionConfigOptionInput) (map[string]any, error) {
	sessionKey := normalizeSessionKey(input.SessionKey)
	if sessionKey == "" {
		return nil, runtime.NewSessionInitError("ACP session key is required", nil)
	}

	var resultOptions map[string]any
	var resultErr error

	err := m.actorQueue.Run(sessionKey, func() error {
		cached := m.runtimeCache.Get(sessionKey)
		if cached == nil {
			resultErr = runtime.NewSessionInitError(fmt.Sprintf("Session not found: %s", sessionKey), nil)
			return resultErr
		}

		rt := cached.runtime
		handle := cached.handle

		// Try SetConfigOption - it may not be supported by all backends
		if err := rt.SetConfigOption(ctx, handle, input.Key, input.Value); err != nil {
			// Check if this is an unsupported operation
			if runtime.GetAcpErrorCode(err) == runtime.ErrCodeBackendUnsupportedControl {
				resultErr = runtime.NewUnsupportedControlError(handle.Backend, runtime.AcpControlSessionSetConfigOption)
			} else {
				resultErr = runtime.NewTurnError("Could not update ACP config option", err)
			}
			return resultErr
		}

		resultOptions = map[string]any{
			input.Key: input.Value,
		}
		return nil
	})

	if err != nil {
		return nil, resultErr
	}

	return resultOptions, nil
}

// CancelSession cancels an active turn in an ACP session.
type CancelSessionInput struct {
	Cfg        *config.Config
	SessionKey string
	Reason     string
}

func (m *Manager) CancelSession(ctx context.Context, input CancelSessionInput) error {
	sessionKey := normalizeSessionKey(input.SessionKey)
	if sessionKey == "" {
		return runtime.NewSessionInitError("ACP session key is required", nil)
	}

	_ = m.evictIdleRuntimeHandles(input.Cfg)

	actorKey := normalizeActorKey(sessionKey)
	var cancelErr error

	// Use actor queue to serialize with turn execution
	err := m.actorQueue.Run(sessionKey, func() error {
		m.mu.Lock()
		activeTurn, exists := m.activeTurnBySession[actorKey]
		if exists {
			activeTurn.abortController()
			if activeTurn.cancelDone == nil {
				activeTurn.cancelDone = make(chan struct{})
				rt := activeTurn.runtime
				handle := activeTurn.handle
				done := activeTurn.cancelDone
				go func() {
					err := rt.Cancel(ctx, handle, input.Reason)
					m.mu.Lock()
					activeTurn.cancelErr = err
					close(done)
					m.mu.Unlock()
				}()
			}
			done := activeTurn.cancelDone
			m.mu.Unlock()

			<-done
			m.mu.RLock()
			cancelErr = activeTurn.cancelErr
			m.mu.RUnlock()
			return cancelErr
		}
		m.mu.Unlock()

		return nil
	})

	return err
}

// RunTrackedTurnInput contains parameters for running a tracked turn.
type RunTrackedTurnInput struct {
	Cfg        *config.Config
	SessionKey string
	Text       string
	Mode       runtime.AcpRuntimePromptMode
	RequestID  string
}

// RunTrackedTurnResult contains the result of running a tracked turn.
type RunTrackedTurnResult struct {
	EventChan <-chan runtime.AcpRuntimeEvent
	RequestID string
}

// RunTrackedTurn runs a turn with proper tracking for cancellation.
func (m *Manager) RunTrackedTurn(ctx context.Context, input RunTrackedTurnInput) (*RunTrackedTurnResult, error) {
	sessionKey := normalizeSessionKey(input.SessionKey)
	if sessionKey == "" {
		return nil, runtime.NewSessionInitError("ACP session key is required", nil)
	}

	var result *RunTrackedTurnResult
	var resultErr error

	err := m.actorQueue.Run(sessionKey, func() error {
		cached := m.runtimeCache.Get(sessionKey)
		if cached == nil {
			resultErr = runtime.NewSessionInitError(fmt.Sprintf("Session not found: %s", sessionKey), nil)
			return resultErr
		}

		actorKey := normalizeActorKey(sessionKey)
		m.mu.RLock()
		_, hasActiveTurn := m.activeTurnBySession[actorKey]
		m.mu.RUnlock()
		if hasActiveTurn {
			resultErr = runtime.NewTurnError(fmt.Sprintf("ACP turn already active for session: %s", sessionKey), nil)
			return resultErr
		}

		rt := cached.runtime
		handle := cached.handle

		// Create cancellable context
		cancelCtx, abortController := context.WithCancel(ctx)

		// Prepare turn input
		turnInput := runtime.AcpRuntimeTurnInput{
			Handle:    handle,
			Text:      input.Text,
			Mode:      input.Mode,
			RequestID: input.RequestID,
			Signal:    abortController,
		}

		// Run the turn
		rawEventChan, err := rt.RunTurn(cancelCtx, turnInput)
		if err != nil {
			resultErr = err
			return err
		}

		// Track the active turn
		activeTurn := &ActiveTurnState{
			runtime:         rt,
			handle:          handle,
			abortController: abortController,
			cancelDone:      nil,
		}

		m.mu.Lock()
		m.activeTurnBySession[actorKey] = activeTurn
		m.mu.Unlock()

		startedAt := time.Now()
		trackedChan := make(chan runtime.AcpRuntimeEvent, 16)

		// Clean up tracking when turn completes while preserving events for caller.
		go func() {
			var turnErr error
			for event := range rawEventChan {
				if event != nil {
					if eventErr, ok := event.(*runtime.AcpEventError); ok {
						turnErr = runtime.NewTurnError(eventErr.Message, nil)
					}
					trackedChan <- event
				}
			}

			close(trackedChan)

			m.mu.Lock()
			delete(m.activeTurnBySession, actorKey)
			if turnErr != nil {
				if code := runtime.GetAcpErrorCode(turnErr); code != "" {
					m.errorCountsByCode[code]++
				}
			}
			m.mu.Unlock()

			m.turnLatencyStats.RecordCompletion(startedAt, turnErr)
		}()

		result = &RunTrackedTurnResult{
			EventChan: trackedChan,
			RequestID: input.RequestID,
		}
		return nil
	})

	if err != nil {
		return nil, resultErr
	}

	return result, nil
}

// CloseSession closes an ACP session.
type CloseSessionInput struct {
	Cfg               *config.Config
	SessionKey        string
	Reason            string
	RequireAcpSession bool
	ClearMeta         bool
}

type CloseSessionResult struct {
	RuntimeClosed bool
	RuntimeNotice string
	MetaCleared   bool
}

func (m *Manager) CloseSession(ctx context.Context, input CloseSessionInput) (*CloseSessionResult, error) {
	sessionKey := normalizeSessionKey(input.SessionKey)
	if sessionKey == "" {
		return nil, runtime.NewSessionInitError("ACP session key is required", nil)
	}

	_ = m.evictIdleRuntimeHandles(input.Cfg)

	var result *CloseSessionResult
	var resultErr error

	err := m.actorQueue.Run(sessionKey, func() error {
		actorKey := normalizeActorKey(sessionKey)

		// Ensure any active turn is canceled before closing runtime handle.
		m.mu.Lock()
		activeTurn, hasActiveTurn := m.activeTurnBySession[actorKey]
		if hasActiveTurn {
			activeTurn.abortController()
			if activeTurn.cancelDone == nil {
				activeTurn.cancelDone = make(chan struct{})
				rt := activeTurn.runtime
				handle := activeTurn.handle
				done := activeTurn.cancelDone
				go func() {
					err := rt.Cancel(ctx, handle, input.Reason)
					m.mu.Lock()
					activeTurn.cancelErr = err
					close(done)
					m.mu.Unlock()
				}()
			}
		}
		var cancelDone chan struct{}
		if hasActiveTurn {
			cancelDone = activeTurn.cancelDone
		}
		m.mu.Unlock()

		if cancelDone != nil {
			<-cancelDone
			m.mu.RLock()
			cancelErr := activeTurn.cancelErr
			m.mu.RUnlock()
			if cancelErr != nil {
				resultErr = cancelErr
				return cancelErr
			}
		}

		resolution := m.ResolveSession(sessionKey)
		if resolution.Kind == "none" {
			if input.RequireAcpSession {
				return runtime.NewSessionInitError(fmt.Sprintf("Session is not ACP-enabled: %s", sessionKey), nil)
			}
			unbindThreadBindingsForSession(sessionKey)
			result = &CloseSessionResult{
				RuntimeClosed: false,
				MetaCleared:   false,
			}
			return nil
		}

		cached := m.runtimeCache.Get(sessionKey)
		if cached == nil {
			if input.RequireAcpSession {
				return runtime.NewSessionInitError(fmt.Sprintf("Session not found: %s", sessionKey), nil)
			}
			unbindThreadBindingsForSession(sessionKey)
			result = &CloseSessionResult{
				RuntimeClosed: false,
				MetaCleared:   false,
			}
			return nil
		}

		rt := cached.runtime
		handle := cached.handle

		runtimeClosed := false
		runtimeNotice := ""

		if err := rt.Close(ctx, handle, input.Reason); err != nil {
			if !input.AllowBackendUnavailable() {
				resultErr = err
				return err
			}
			runtimeNotice = err.Error()
		} else {
			runtimeClosed = true
		}

		m.runtimeCache.Clear(sessionKey)
		unbindThreadBindingsForSession(sessionKey)

		metaCleared := false
		if input.ClearMeta {
			// In a real implementation, this would clear session metadata from storage
			metaCleared = true
		}

		result = &CloseSessionResult{
			RuntimeClosed: runtimeClosed,
			RuntimeNotice: runtimeNotice,
			MetaCleared:   metaCleared,
		}

		return nil
	})

	if err != nil {
		return nil, resultErr
	}

	return result, nil
}

// RecordError records an error by code for observability.
func (m *Manager) RecordError(code string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errorCountsByCode[code]++
}

// GetErrorCounts returns a copy of error counts by code.
func (m *Manager) GetErrorCounts() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	counts := make(map[string]int, len(m.errorCountsByCode))
	for k, v := range m.errorCountsByCode {
		counts[k] = v
	}
	return counts
}

// evictIdleRuntimeHandles evicts idle runtime handles based on TTL.
func (m *Manager) evictIdleRuntimeHandles(cfg *config.Config) int {
	idleTTL := resolveRuntimeIdleTTL(cfg)
	if idleTTL <= 0 || m.runtimeCache.Size() == 0 {
		return 0
	}

	candidates := m.runtimeCache.CollectIdleCandidates(idleTTL, time.Now())
	evicted := 0

	for _, candidate := range candidates {
		actorKey := normalizeActorKey(candidate.SessionKey)

		m.mu.Lock()
		_, hasActiveTurn := m.activeTurnBySession[actorKey]
		m.mu.Unlock()

		if hasActiveTurn {
			continue
		}

		// Run eviction in actor queue
		_ = m.actorQueue.Run(candidate.SessionKey, func() error {
			cached := m.runtimeCache.Get(candidate.SessionKey)
			if cached == nil {
				return nil
			}

			// Verify still idle
			if time.Since(candidate.LastTouchedAt) < idleTTL {
				return nil
			}

			m.runtimeCache.Clear(candidate.SessionKey)

			// Close runtime
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			_ = cached.runtime.Close(ctx, cached.handle, "idle-evicted")

			// Track eviction statistics
			m.runtimeCache.IncrementEvicted()

			evicted++
			return nil
		})
	}

	return evicted
}

// normalizeSessionKey normalizes a session key.
func normalizeSessionKey(key string) string {
	if key == "" {
		return ""
	}
	// Simple trim - in a real implementation, this would be more sophisticated
	return key
}

// normalizeAgentID normalizes an agent ID.
func normalizeAgentID(id string) string {
	if id == "" {
		return "main"
	}
	return id
}

// normalizeActorKey normalizes a session key for use as an actor key.
func normalizeActorKey(sessionKey string) string {
	return sessionKey
}

// resolveRuntimeIdleTTL returns the idle TTL from config.
func resolveRuntimeIdleTTL(cfg *config.Config) time.Duration {
	if cfg == nil || cfg.ACP.IdleTimeoutMs == 0 {
		return 5 * time.Minute // Default 5 minutes
	}
	return time.Duration(cfg.ACP.IdleTimeoutMs) * time.Millisecond
}

// AllowBackendUnavailable checks if backend unavailable should be allowed.
func (input *CloseSessionInput) AllowBackendUnavailable() bool {
	// This would be determined by input parameters
	return false
}
