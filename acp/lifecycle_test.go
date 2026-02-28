package acp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	acpruntime "github.com/smallnest/goclaw/acp/runtime"
	"github.com/smallnest/goclaw/channels"
	"github.com/smallnest/goclaw/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testRuntime struct {
	backendID   string
	runTurnErr  error
	blockOnCtx  bool
	turnStarted chan struct{}
	mu          sync.Mutex
	cancelCalls int
	closeCalls  int
}

func newTestRuntime(backendID string) *testRuntime {
	return &testRuntime{
		backendID:   backendID,
		turnStarted: make(chan struct{}, 1),
	}
}

func (r *testRuntime) EnsureSession(ctx context.Context, input acpruntime.AcpRuntimeEnsureInput) (acpruntime.AcpRuntimeHandle, error) {
	return acpruntime.AcpRuntimeHandle{
		SessionKey:         input.SessionKey,
		Backend:            r.backendID,
		RuntimeSessionName: "test-session",
		Cwd:                input.Cwd,
		BackendSessionId:   "backend-test-session",
	}, nil
}

func (r *testRuntime) RunTurn(ctx context.Context, input acpruntime.AcpRuntimeTurnInput) (<-chan acpruntime.AcpRuntimeEvent, error) {
	if r.runTurnErr != nil {
		return nil, r.runTurnErr
	}

	ch := make(chan acpruntime.AcpRuntimeEvent, 1)
	r.turnStarted <- struct{}{}
	go func() {
		defer close(ch)
		if r.blockOnCtx {
			<-ctx.Done()
		}
		ch <- &acpruntime.AcpEventDone{StopReason: "completed"}
	}()
	return ch, nil
}

func (r *testRuntime) GetCapabilities(ctx context.Context, handle *acpruntime.AcpRuntimeHandle) (acpruntime.AcpRuntimeCapabilities, error) {
	return acpruntime.AcpRuntimeCapabilities{}, nil
}

func (r *testRuntime) GetStatus(ctx context.Context, handle acpruntime.AcpRuntimeHandle) (*acpruntime.AcpRuntimeStatus, error) {
	return &acpruntime.AcpRuntimeStatus{Summary: "ok"}, nil
}

func (r *testRuntime) SetMode(ctx context.Context, handle acpruntime.AcpRuntimeHandle, mode string) error {
	return nil
}

func (r *testRuntime) SetConfigOption(ctx context.Context, handle acpruntime.AcpRuntimeHandle, key, value string) error {
	return nil
}

func (r *testRuntime) Doctor(ctx context.Context) (acpruntime.AcpRuntimeDoctorReport, error) {
	return acpruntime.AcpRuntimeDoctorReport{Ok: true, Message: "ok"}, nil
}

func (r *testRuntime) Cancel(ctx context.Context, handle acpruntime.AcpRuntimeHandle, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cancelCalls++
	return nil
}

func (r *testRuntime) Close(ctx context.Context, handle acpruntime.AcpRuntimeHandle, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closeCalls++
	return nil
}

func (r *testRuntime) getCancelCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cancelCalls
}

func (r *testRuntime) getCloseCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closeCalls
}

func TestCancelSessionCancelsTrackedTurn(t *testing.T) {
	backendID := "test-cancel-backend"
	rt := newTestRuntime(backendID)
	rt.blockOnCtx = true

	require.NoError(t, acpruntime.RegisterAcpRuntimeBackend(acpruntime.AcpRuntimeBackend{
		ID:      backendID,
		Runtime: rt,
		Healthy: func() bool { return true },
	}))
	t.Cleanup(func() {
		acpruntime.UnregisterAcpRuntimeBackend(backendID)
	})

	globalManagerMu.Lock()
	globalManager = nil
	globalManagerMu.Unlock()

	cfg := &config.Config{}
	cfg.ACP.Enabled = true
	cfg.ACP.Backend = backendID
	cfg.ACP.DefaultAgent = "main"

	mgr := GetOrCreateGlobalManager(cfg)
	_, _, err := mgr.InitializeSession(context.Background(), InitializeSessionInput{
		Cfg:        cfg,
		SessionKey: "agent:main:acp:test-cancel",
		Agent:      "main",
		Mode:       acpruntime.AcpSessionModePersistent,
	})
	require.NoError(t, err)

	turnResult, err := mgr.RunTrackedTurn(context.Background(), RunTrackedTurnInput{
		Cfg:        cfg,
		SessionKey: "agent:main:acp:test-cancel",
		Text:       "test",
		Mode:       acpruntime.AcpPromptModePrompt,
		RequestID:  "req-cancel",
	})
	require.NoError(t, err)
	require.NotNil(t, turnResult)

	select {
	case <-rt.turnStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("turn did not start")
	}

	var wg sync.WaitGroup
	wg.Add(2)
	cancelErrs := make([]error, 2)
	for i := 0; i < 2; i++ {
		i := i
		go func() {
			defer wg.Done()
			cancelErrs[i] = mgr.CancelSession(context.Background(), CancelSessionInput{
				Cfg:        cfg,
				SessionKey: "agent:main:acp:test-cancel",
				Reason:     "test-cancel",
			})
		}()
	}
	wg.Wait()

	assert.NoError(t, cancelErrs[0])
	assert.NoError(t, cancelErrs[1])
	assert.Equal(t, 1, rt.getCancelCalls())

	assert.Eventually(t, func() bool {
		mgr.mu.RLock()
		defer mgr.mu.RUnlock()
		_, exists := mgr.activeTurnBySession["agent:main:acp:test-cancel"]
		return !exists
	}, 2*time.Second, 20*time.Millisecond)

	// Drain for completeness.
	for range turnResult.EventChan {
	}
}

func TestSpawnFailureCleansUpSession(t *testing.T) {
	backendID := "test-spawn-cleanup-backend"
	rt := newTestRuntime(backendID)
	rt.runTurnErr = errors.New("turn failed")

	require.NoError(t, acpruntime.RegisterAcpRuntimeBackend(acpruntime.AcpRuntimeBackend{
		ID:      backendID,
		Runtime: rt,
		Healthy: func() bool { return true },
	}))
	t.Cleanup(func() {
		acpruntime.UnregisterAcpRuntimeBackend(backendID)
	})

	globalManagerMu.Lock()
	globalManager = nil
	globalManagerMu.Unlock()

	cfg := &config.Config{}
	cfg.ACP.Enabled = true
	cfg.ACP.Backend = backendID
	cfg.ACP.DefaultAgent = "main"

	result, err := SpawnAcpDirect(context.Background(), cfg, SpawnAcpParams{
		Task:   "failing task",
		Mode:   SpawnModeRun,
		Thread: false,
	}, SpawnAcpContext{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "error", result.Status)
	assert.Contains(t, result.Error, "failed to run ACP turn")

	assert.Eventually(t, func() bool {
		return rt.getCloseCalls() > 0
	}, 2*time.Second, 20*time.Millisecond, fmt.Sprintf("expected close to be called, got %d", rt.getCloseCalls()))
}

func TestCloseSessionCancelsActiveTurn(t *testing.T) {
	backendID := "test-close-cancel-backend"
	rt := newTestRuntime(backendID)
	rt.blockOnCtx = true

	require.NoError(t, acpruntime.RegisterAcpRuntimeBackend(acpruntime.AcpRuntimeBackend{
		ID:      backendID,
		Runtime: rt,
		Healthy: func() bool { return true },
	}))
	t.Cleanup(func() {
		acpruntime.UnregisterAcpRuntimeBackend(backendID)
	})

	globalManagerMu.Lock()
	globalManager = nil
	globalManagerMu.Unlock()

	cfg := &config.Config{}
	cfg.ACP.Enabled = true
	cfg.ACP.Backend = backendID
	cfg.ACP.DefaultAgent = "main"

	mgr := GetOrCreateGlobalManager(cfg)
	_, _, err := mgr.InitializeSession(context.Background(), InitializeSessionInput{
		Cfg:        cfg,
		SessionKey: "agent:main:acp:test-close",
		Agent:      "main",
		Mode:       acpruntime.AcpSessionModePersistent,
	})
	require.NoError(t, err)

	turnResult, err := mgr.RunTrackedTurn(context.Background(), RunTrackedTurnInput{
		Cfg:        cfg,
		SessionKey: "agent:main:acp:test-close",
		Text:       "test",
		Mode:       acpruntime.AcpPromptModePrompt,
		RequestID:  "req-close",
	})
	require.NoError(t, err)
	require.NotNil(t, turnResult)

	select {
	case <-rt.turnStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("turn did not start")
	}

	closeResult, err := mgr.CloseSession(context.Background(), CloseSessionInput{
		Cfg:        cfg,
		SessionKey: "agent:main:acp:test-close",
		Reason:     "close-test",
	})
	require.NoError(t, err)
	require.NotNil(t, closeResult)
	assert.True(t, closeResult.RuntimeClosed)
	assert.Equal(t, 1, rt.getCancelCalls())
	assert.Equal(t, 1, rt.getCloseCalls())

	// Drain for completeness.
	for range turnResult.EventChan {
	}
}

func TestCloseSessionUnbindsThreadBindings(t *testing.T) {
	backendID := "test-close-unbind-backend"
	rt := newTestRuntime(backendID)

	require.NoError(t, acpruntime.RegisterAcpRuntimeBackend(acpruntime.AcpRuntimeBackend{
		ID:      backendID,
		Runtime: rt,
		Healthy: func() bool { return true },
	}))
	t.Cleanup(func() {
		acpruntime.UnregisterAcpRuntimeBackend(backendID)
	})

	globalManagerMu.Lock()
	globalManager = nil
	globalManagerMu.Unlock()

	bindingService := channels.NewThreadBindingService(nil, nil)
	SetGlobalThreadBindingService(bindingService)
	t.Cleanup(func() { SetGlobalThreadBindingService(nil) })

	cfg := &config.Config{}
	cfg.ACP.Enabled = true
	cfg.ACP.Backend = backendID
	cfg.ACP.DefaultAgent = "main"

	sessionKey := "agent:main:acp:test-close-unbind"
	_, _, err := GetOrCreateGlobalManager(cfg).InitializeSession(context.Background(), InitializeSessionInput{
		Cfg:        cfg,
		SessionKey: sessionKey,
		Agent:      "main",
		Mode:       acpruntime.AcpSessionModePersistent,
	})
	require.NoError(t, err)

	_, err = bindingService.Bind(channels.BindInput{
		TargetSessionKey: sessionKey,
		TargetKind:       "session",
		Conversation: channels.ThreadBindingConversation{
			Channel:        "telegram",
			AccountID:      "acc-1",
			ConversationID: "thread-1",
		},
		Placement: "child",
	})
	require.NoError(t, err)
	require.Len(t, bindingService.GetBySession(sessionKey), 1)

	_, err = GetOrCreateGlobalManager(cfg).CloseSession(context.Background(), CloseSessionInput{
		Cfg:        cfg,
		SessionKey: sessionKey,
		Reason:     "test-close-unbind",
	})
	require.NoError(t, err)
	assert.Len(t, bindingService.GetBySession(sessionKey), 0)
}

func TestSpawnThreadBindingServiceUnavailableCleansSession(t *testing.T) {
	backendID := "test-spawn-service-unavailable-backend"
	rt := newTestRuntime(backendID)
	rt.blockOnCtx = true

	require.NoError(t, acpruntime.RegisterAcpRuntimeBackend(acpruntime.AcpRuntimeBackend{
		ID:      backendID,
		Runtime: rt,
		Healthy: func() bool { return true },
	}))
	t.Cleanup(func() {
		acpruntime.UnregisterAcpRuntimeBackend(backendID)
	})

	globalManagerMu.Lock()
	globalManager = nil
	globalManagerMu.Unlock()
	SetGlobalThreadBindingService(nil)

	cfg := &config.Config{}
	cfg.ACP.Enabled = true
	cfg.ACP.Backend = backendID
	cfg.ACP.DefaultAgent = "main"

	result, err := SpawnAcpDirect(context.Background(), cfg, SpawnAcpParams{
		Task:   "task",
		Mode:   SpawnModeSession,
		Thread: true,
	}, SpawnAcpContext{
		AgentChannel:   "telegram",
		AgentAccountID: "acc-1",
		AgentThreadID:  "thread-1",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "error", result.Status)
	assert.Contains(t, result.Error, "thread binding service is unavailable")
	assert.Eventually(t, func() bool {
		return rt.getCloseCalls() > 0
	}, 2*time.Second, 20*time.Millisecond)
}

func TestRunTrackedTurnRejectsConcurrentTurns(t *testing.T) {
	backendID := "test-concurrent-turn-reject-backend"
	rt := newTestRuntime(backendID)
	rt.blockOnCtx = true

	require.NoError(t, acpruntime.RegisterAcpRuntimeBackend(acpruntime.AcpRuntimeBackend{
		ID:      backendID,
		Runtime: rt,
		Healthy: func() bool { return true },
	}))
	t.Cleanup(func() {
		acpruntime.UnregisterAcpRuntimeBackend(backendID)
	})

	globalManagerMu.Lock()
	globalManager = nil
	globalManagerMu.Unlock()

	cfg := &config.Config{}
	cfg.ACP.Enabled = true
	cfg.ACP.Backend = backendID
	cfg.ACP.DefaultAgent = "main"

	mgr := GetOrCreateGlobalManager(cfg)
	_, _, err := mgr.InitializeSession(context.Background(), InitializeSessionInput{
		Cfg:        cfg,
		SessionKey: "agent:main:acp:test-concurrent-turn",
		Agent:      "main",
		Mode:       acpruntime.AcpSessionModePersistent,
	})
	require.NoError(t, err)

	firstTurn, err := mgr.RunTrackedTurn(context.Background(), RunTrackedTurnInput{
		Cfg:        cfg,
		SessionKey: "agent:main:acp:test-concurrent-turn",
		Text:       "first",
		Mode:       acpruntime.AcpPromptModePrompt,
		RequestID:  "req-first",
	})
	require.NoError(t, err)
	require.NotNil(t, firstTurn)

	select {
	case <-rt.turnStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first turn did not start")
	}

	_, err = mgr.RunTrackedTurn(context.Background(), RunTrackedTurnInput{
		Cfg:        cfg,
		SessionKey: "agent:main:acp:test-concurrent-turn",
		Text:       "second",
		Mode:       acpruntime.AcpPromptModePrompt,
		RequestID:  "req-second",
	})
	require.Error(t, err)
	assert.Equal(t, acpruntime.ErrCodeTurnFailed, acpruntime.GetAcpErrorCode(err))
	assert.Contains(t, err.Error(), "already active")

	require.NoError(t, mgr.CancelSession(context.Background(), CancelSessionInput{
		Cfg:        cfg,
		SessionKey: "agent:main:acp:test-concurrent-turn",
		Reason:     "cleanup",
	}))
	for range firstTurn.EventChan {
	}
}

func TestInitializeSessionSameKeyDoesNotHitLimit(t *testing.T) {
	backendID := "test-idempotent-init-limit-backend"
	rt := newTestRuntime(backendID)

	require.NoError(t, acpruntime.RegisterAcpRuntimeBackend(acpruntime.AcpRuntimeBackend{
		ID:      backendID,
		Runtime: rt,
		Healthy: func() bool { return true },
	}))
	t.Cleanup(func() {
		acpruntime.UnregisterAcpRuntimeBackend(backendID)
	})

	globalManagerMu.Lock()
	globalManager = nil
	globalManagerMu.Unlock()

	cfg := &config.Config{}
	cfg.ACP.Enabled = true
	cfg.ACP.Backend = backendID
	cfg.ACP.DefaultAgent = "main"
	cfg.ACP.MaxConcurrentSessions = 1

	mgr := GetOrCreateGlobalManager(cfg)
	firstHandle, _, err := mgr.InitializeSession(context.Background(), InitializeSessionInput{
		Cfg:        cfg,
		SessionKey: "agent:main:acp:test-idempotent-init",
		Agent:      "main",
		Mode:       acpruntime.AcpSessionModePersistent,
	})
	require.NoError(t, err)
	require.NotNil(t, firstHandle)

	secondHandle, _, err := mgr.InitializeSession(context.Background(), InitializeSessionInput{
		Cfg:        cfg,
		SessionKey: "agent:main:acp:test-idempotent-init",
		Agent:      "main",
		Mode:       acpruntime.AcpSessionModePersistent,
	})
	require.NoError(t, err)
	require.NotNil(t, secondHandle)
	assert.Equal(t, firstHandle.SessionKey, secondHandle.SessionKey)
}
