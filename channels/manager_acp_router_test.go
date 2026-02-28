package channels

import (
	"testing"

	"github.com/smallnest/goclaw/bus"
)

type testAcpRouter struct {
	sessionKey string
	bound      bool
}

func (r *testAcpRouter) RouteToAcpSession(channel, accountID, conversationID string) string {
	return r.sessionKey
}

func (r *testAcpRouter) IsACPThreadBinding(channel, accountID, conversationID string) bool {
	return r.bound
}

func TestManagerRouteUsesAcpRouter(t *testing.T) {
	mgr := NewManager(bus.NewMessageBus(16))
	mgr.SetAcpRouter(&testAcpRouter{
		sessionKey: "agent:main:acp:router-test",
		bound:      true,
	})

	got := mgr.RouteToAcpSession("telegram", "acc-1", "thread-1")
	if got != "agent:main:acp:router-test" {
		t.Fatalf("unexpected session key: %q", got)
	}

	if !mgr.IsACPThreadBinding("telegram", "acc-1", "thread-1") {
		t.Fatalf("expected ACP thread binding from router")
	}
}
