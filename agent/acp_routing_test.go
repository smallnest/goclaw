package agent

import (
	"context"
	"testing"
	"time"

	"github.com/smallnest/goclaw/acp"
	acpruntime "github.com/smallnest/goclaw/acp/runtime"
	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/channels"
	"github.com/smallnest/goclaw/config"
)

type testRouteRuntime struct {
	backendID string
}

func (r *testRouteRuntime) EnsureSession(ctx context.Context, input acpruntime.AcpRuntimeEnsureInput) (acpruntime.AcpRuntimeHandle, error) {
	return acpruntime.AcpRuntimeHandle{
		SessionKey:         input.SessionKey,
		Backend:            r.backendID,
		RuntimeSessionName: "route-test-session",
		Cwd:                input.Cwd,
		BackendSessionId:   "route-test-backend-session",
	}, nil
}

func (r *testRouteRuntime) RunTurn(ctx context.Context, input acpruntime.AcpRuntimeTurnInput) (<-chan acpruntime.AcpRuntimeEvent, error) {
	ch := make(chan acpruntime.AcpRuntimeEvent, 2)
	go func() {
		defer close(ch)
		ch <- &acpruntime.AcpEventTextDelta{Text: "acp:" + input.Text, Stream: "output"}
		ch <- &acpruntime.AcpEventDone{StopReason: "completed"}
	}()
	return ch, nil
}

func (r *testRouteRuntime) GetCapabilities(ctx context.Context, handle *acpruntime.AcpRuntimeHandle) (acpruntime.AcpRuntimeCapabilities, error) {
	return acpruntime.AcpRuntimeCapabilities{}, nil
}
func (r *testRouteRuntime) GetStatus(ctx context.Context, handle acpruntime.AcpRuntimeHandle) (*acpruntime.AcpRuntimeStatus, error) {
	return &acpruntime.AcpRuntimeStatus{Summary: "ok"}, nil
}
func (r *testRouteRuntime) SetMode(ctx context.Context, handle acpruntime.AcpRuntimeHandle, mode string) error {
	return nil
}
func (r *testRouteRuntime) SetConfigOption(ctx context.Context, handle acpruntime.AcpRuntimeHandle, key, value string) error {
	return nil
}
func (r *testRouteRuntime) Doctor(ctx context.Context) (acpruntime.AcpRuntimeDoctorReport, error) {
	return acpruntime.AcpRuntimeDoctorReport{Ok: true, Message: "ok"}, nil
}
func (r *testRouteRuntime) Cancel(ctx context.Context, handle acpruntime.AcpRuntimeHandle, reason string) error {
	return nil
}
func (r *testRouteRuntime) Close(ctx context.Context, handle acpruntime.AcpRuntimeHandle, reason string) error {
	return nil
}

type slowRouteRuntime struct {
	testRouteRuntime
	delay time.Duration
}

func (r *slowRouteRuntime) RunTurn(ctx context.Context, input acpruntime.AcpRuntimeTurnInput) (<-chan acpruntime.AcpRuntimeEvent, error) {
	ch := make(chan acpruntime.AcpRuntimeEvent, 2)
	go func() {
		defer close(ch)
		select {
		case <-ctx.Done():
			return
		case <-time.After(r.delay):
		}
		ch <- &acpruntime.AcpEventTextDelta{Text: "acp:" + input.Text, Stream: "output"}
		ch <- &acpruntime.AcpEventDone{StopReason: "completed"}
	}()
	return ch, nil
}

type errorRouteRuntime struct {
	testRouteRuntime
}

func (r *errorRouteRuntime) RunTurn(ctx context.Context, input acpruntime.AcpRuntimeTurnInput) (<-chan acpruntime.AcpRuntimeEvent, error) {
	return nil, acpruntime.NewTurnError("forced error", nil)
}

type staticRouter struct {
	sessionKey string
}

func (r *staticRouter) RouteToAcpSession(channel, accountID, conversationID string) string {
	return r.sessionKey
}
func (r *staticRouter) IsACPThreadBinding(channel, accountID, conversationID string) bool {
	return r.sessionKey != ""
}

func TestHandleInboundMessageRoutesToAcpThreadSession(t *testing.T) {
	backendID := "agent-acp-route-test-backend"
	rt := &testRouteRuntime{backendID: backendID}
	if err := acpruntime.RegisterAcpRuntimeBackend(acpruntime.AcpRuntimeBackend{
		ID:      backendID,
		Runtime: rt,
		Healthy: func() bool { return true },
	}); err != nil {
		t.Fatalf("register backend: %v", err)
	}
	t.Cleanup(func() { acpruntime.UnregisterAcpRuntimeBackend(backendID) })

	cfg := &config.Config{}
	cfg.ACP.Enabled = true
	cfg.ACP.Backend = backendID
	cfg.ACP.DefaultAgent = "main"

	acpMgr := acp.NewManager(cfg)
	sessionKey := "agent:main:acp:routed"
	if _, _, err := acpMgr.InitializeSession(context.Background(), acp.InitializeSessionInput{
		Cfg:        cfg,
		SessionKey: sessionKey,
		Agent:      "main",
		Mode:       acpruntime.AcpSessionModePersistent,
	}); err != nil {
		t.Fatalf("initialize acp session: %v", err)
	}

	messageBus := bus.NewMessageBus(16)
	channelMgr := channels.NewManager(messageBus)
	channelMgr.SetAcpRouter(&staticRouter{sessionKey: sessionKey})

	manager := &AgentManager{
		bus:        messageBus,
		cfg:        cfg,
		channelMgr: channelMgr,
		acpManager: acpMgr,
	}

	sub := messageBus.SubscribeOutbound()
	defer sub.Unsubscribe()

	err := manager.handleInboundMessage(context.Background(), &bus.InboundMessage{
		ID:        "msg-1",
		Channel:   "telegram",
		AccountID: "acc-1",
		ChatID:    "thread-1",
		Content:   "hello",
		Timestamp: time.Now(),
	}, nil)
	if err != nil {
		t.Fatalf("handle inbound: %v", err)
	}

	select {
	case out := <-sub.Channel:
		if out == nil {
			t.Fatalf("expected outbound message, got nil")
		}
		if out.Content != "acp:hello" {
			t.Fatalf("unexpected outbound content: %q", out.Content)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for outbound message")
	}
}

func TestHandleInboundMessageAcpRoutingIsNonBlocking(t *testing.T) {
	backendID := "agent-acp-route-nonblocking-backend"
	rt := &slowRouteRuntime{
		testRouteRuntime: testRouteRuntime{backendID: backendID},
		delay:            300 * time.Millisecond,
	}
	if err := acpruntime.RegisterAcpRuntimeBackend(acpruntime.AcpRuntimeBackend{
		ID:      backendID,
		Runtime: rt,
		Healthy: func() bool { return true },
	}); err != nil {
		t.Fatalf("register backend: %v", err)
	}
	t.Cleanup(func() { acpruntime.UnregisterAcpRuntimeBackend(backendID) })

	cfg := &config.Config{}
	cfg.ACP.Enabled = true
	cfg.ACP.Backend = backendID
	cfg.ACP.DefaultAgent = "main"

	acpMgr := acp.NewManager(cfg)
	sessionKey := "agent:main:acp:routed-nonblocking"
	if _, _, err := acpMgr.InitializeSession(context.Background(), acp.InitializeSessionInput{
		Cfg:        cfg,
		SessionKey: sessionKey,
		Agent:      "main",
		Mode:       acpruntime.AcpSessionModePersistent,
	}); err != nil {
		t.Fatalf("initialize acp session: %v", err)
	}

	messageBus := bus.NewMessageBus(16)
	channelMgr := channels.NewManager(messageBus)
	channelMgr.SetAcpRouter(&staticRouter{sessionKey: sessionKey})

	manager := &AgentManager{
		bus:        messageBus,
		cfg:        cfg,
		channelMgr: channelMgr,
		acpManager: acpMgr,
	}

	start := time.Now()
	err := manager.handleInboundMessage(context.Background(), &bus.InboundMessage{
		ID:        "msg-2",
		Channel:   "telegram",
		AccountID: "acc-1",
		ChatID:    "thread-1",
		Content:   "hello",
		Timestamp: time.Now(),
	}, nil)
	if err != nil {
		t.Fatalf("handle inbound: %v", err)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatalf("expected non-blocking ACP routing, took %s", time.Since(start))
	}
}

func TestHandleInboundMessageAcpRoutingPublishesErrorOnTurnStartFailure(t *testing.T) {
	backendID := "agent-acp-route-error-backend"
	rt := &errorRouteRuntime{
		testRouteRuntime: testRouteRuntime{backendID: backendID},
	}
	if err := acpruntime.RegisterAcpRuntimeBackend(acpruntime.AcpRuntimeBackend{
		ID:      backendID,
		Runtime: rt,
		Healthy: func() bool { return true },
	}); err != nil {
		t.Fatalf("register backend: %v", err)
	}
	t.Cleanup(func() { acpruntime.UnregisterAcpRuntimeBackend(backendID) })

	cfg := &config.Config{}
	cfg.ACP.Enabled = true
	cfg.ACP.Backend = backendID
	cfg.ACP.DefaultAgent = "main"

	acpMgr := acp.NewManager(cfg)
	sessionKey := "agent:main:acp:routed-error"
	if _, _, err := acpMgr.InitializeSession(context.Background(), acp.InitializeSessionInput{
		Cfg:        cfg,
		SessionKey: sessionKey,
		Agent:      "main",
		Mode:       acpruntime.AcpSessionModePersistent,
	}); err != nil {
		t.Fatalf("initialize acp session: %v", err)
	}

	messageBus := bus.NewMessageBus(16)
	channelMgr := channels.NewManager(messageBus)
	channelMgr.SetAcpRouter(&staticRouter{sessionKey: sessionKey})
	manager := &AgentManager{
		bus:        messageBus,
		cfg:        cfg,
		channelMgr: channelMgr,
		acpManager: acpMgr,
	}

	sub := messageBus.SubscribeOutbound()
	defer sub.Unsubscribe()

	err := manager.handleInboundMessage(context.Background(), &bus.InboundMessage{
		ID:        "msg-3",
		Channel:   "telegram",
		AccountID: "acc-1",
		ChatID:    "thread-1",
		Content:   "hello",
		Timestamp: time.Now(),
	}, nil)
	if err != nil {
		t.Fatalf("handle inbound: %v", err)
	}

	select {
	case out := <-sub.Channel:
		if out == nil {
			t.Fatalf("expected outbound error message, got nil")
		}
		if out.Content == "" {
			t.Fatalf("expected non-empty outbound error message")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for outbound error message")
	}
}
