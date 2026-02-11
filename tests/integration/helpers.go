package integration

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/smallnest/dogclaw/goclaw/bus"
	"github.com/smallnest/dogclaw/goclaw/channels"
	"github.com/smallnest/dogclaw/goclaw/config"
	"github.com/smallnest/dogclaw/goclaw/gateway"
	"github.com/smallnest/dogclaw/goclaw/session"
)

// SetupTestGateway creates a test gateway server with all dependencies
func SetupTestGateway(t *testing.T) (*gateway.Server, *bus.MessageBus, *session.Manager, func()) {
	messageBus := bus.NewMessageBus(100)
	sessionMgr, err := session.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}
	channelMgr := channels.NewManager(messageBus)

	cfg := &config.GatewayConfig{
		Host:         "127.0.0.1",
		Port:         19000 + rand.Intn(1000),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	server := gateway.NewServer(cfg, messageBus, channelMgr, sessionMgr)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Wait for server to be ready
	time.Sleep(200 * time.Millisecond)

	cleanup := func() {
		_ = server.Stop()
		cancel()
	}

	return server, messageBus, sessionMgr, cleanup
}

// SetupTestMessageBus creates a test message bus
func SetupTestMessageBus(t *testing.T) *bus.MessageBus {
	return bus.NewMessageBus(100)
}

// SetupTestSessionManager creates a test session manager
func SetupTestSessionManager(t *testing.T) *session.Manager {
	mgr, err := session.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}
	return mgr
}

// WaitForCondition waits for a condition to be true
func WaitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for condition")
		case <-ticker.C:
			if condition() {
				return
			}
		}
	}
}
