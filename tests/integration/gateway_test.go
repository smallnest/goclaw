package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/smallnest/dogclaw/goclaw/bus"
	"github.com/smallnest/dogclaw/goclaw/channels"
	"github.com/smallnest/dogclaw/goclaw/config"
	"github.com/smallnest/dogclaw/goclaw/gateway"
	"github.com/smallnest/dogclaw/goclaw/session"
)

// TestGatewayWebSocketConnection tests WebSocket connection establishment
func TestGatewayWebSocketConnection(t *testing.T) {
	// Create test dependencies
	messageBus := bus.NewMessageBus(100)
	sessionMgr, _ := session.NewManager("/tmp/test-sessions")
	channelMgr := channels.NewManager(messageBus)

	cfg := &config.GatewayConfig{
		Host:         "127.0.0.1",
		Port:         18080,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Create gateway server
	server := gateway.NewServer(cfg, messageBus, channelMgr, sessionMgr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start server
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start gateway server: %v", err)
	}
	defer func() { _ = server.Stop() }()

	// Wait for server to be ready
	time.Sleep(100 * time.Millisecond)

	// Create WebSocket client
	wsURL := fmt.Sprintf("ws://127.0.0.1:18789%s", "/ws")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer conn.Close()

	// Read welcome message
	var msg map[string]interface{}
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("Failed to read welcome message: %v", err)
	}

	if msg["method"] != "connected" {
		t.Errorf("Expected 'connected' method, got: %v", msg["method"])
	}

	t.Log("WebSocket connection test passed")
}

// TestGatewayRPCMethods tests JSON-RPC method calls
func TestGatewayRPCMethods(t *testing.T) {
	// Create test dependencies
	messageBus := bus.NewMessageBus(100)
	sessionMgr, _ := session.NewManager("/tmp/test-sessions-rpc")
	channelMgr := channels.NewManager(messageBus)

	cfg := &config.GatewayConfig{
		Host:         "127.0.0.1",
		Port:         18081,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	server := gateway.NewServer(cfg, messageBus, channelMgr, sessionMgr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start gateway server: %v", err)
	}
	defer func() { _ = server.Stop() }()

	time.Sleep(100 * time.Millisecond)

	// Create WebSocket connection
	wsURL := fmt.Sprintf("ws://127.0.0.1:18789%s", "/ws")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Read welcome message
	var welcome map[string]interface{}
	_ = conn.ReadJSON(&welcome)

	// Test health check
	healthReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "1",
		"method":  "health",
		"params":  map[string]interface{}{},
	}

	if err := conn.WriteJSON(healthReq); err != nil {
		t.Fatalf("Failed to send health request: %v", err)
	}

	var healthResp map[string]interface{}
	if err := conn.ReadJSON(&healthResp); err != nil {
		t.Fatalf("Failed to read health response: %v", err)
	}

	if healthResp["error"] != nil {
		t.Errorf("Health check returned error: %v", healthResp["error"])
	}

	result := healthResp["result"].(map[string]interface{})
	if result["status"] != "ok" {
		t.Errorf("Expected status 'ok', got: %v", result["status"])
	}

	t.Log("RPC methods test passed")
}

// TestGatewaySessionMethods tests session-related methods
func TestGatewaySessionMethods(t *testing.T) {
	messageBus := bus.NewMessageBus(100)
	sessionMgr, _ := session.NewManager("/tmp/test-sessions-sess")
	channelMgr := channels.NewManager(messageBus)

	cfg := &config.GatewayConfig{
		Host:         "127.0.0.1",
		Port:         18082,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	server := gateway.NewServer(cfg, messageBus, channelMgr, sessionMgr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() { _ = server.Stop() }()

	time.Sleep(100 * time.Millisecond)

	wsURL := fmt.Sprintf("ws://127.0.0.1:18789%s", "/ws")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	var welcome map[string]interface{}
	_ = conn.ReadJSON(&welcome)

	// Create a test session
	testKey := "test:session:123"
	sess, _ := sessionMgr.GetOrCreate(testKey)
	sess.AddMessage(session.Message{
		Role:      "user",
		Content:   "Hello, test!",
		Timestamp: time.Now(),
	})
	_ = sessionMgr.Save(sess)

	// Test sessions.list
	listReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "2",
		"method":  "sessions.list",
		"params":  map[string]interface{}{},
	}

	if err := conn.WriteJSON(listReq); err != nil {
		t.Fatalf("Failed to send list request: %v", err)
	}

	var listResp map[string]interface{}
	if err := conn.ReadJSON(&listResp); err != nil {
		t.Fatalf("Failed to read list response: %v", err)
	}

	if listResp["error"] != nil {
		t.Errorf("sessions.list returned error: %v", listResp["error"])
	}

	t.Log("Session methods test passed")
}

// TestGatewayChannelMethods tests channel-related methods
func TestGatewayChannelMethods(t *testing.T) {
	messageBus := bus.NewMessageBus(100)
	sessionMgr, _ := session.NewManager("/tmp/test-sessions-chan")
	channelMgr := channels.NewManager(messageBus)

	cfg := &config.GatewayConfig{
		Host:         "127.0.0.1",
		Port:         18083,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	server := gateway.NewServer(cfg, messageBus, channelMgr, sessionMgr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() { _ = server.Stop() }()

	time.Sleep(100 * time.Millisecond)

	wsURL := fmt.Sprintf("ws://127.0.0.1:18789%s", "/ws")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	var welcome map[string]interface{}
	_ = conn.ReadJSON(&welcome)

	// Test channels.list
	listReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "3",
		"method":  "channels.list",
		"params":  map[string]interface{}{},
	}

	if err := conn.WriteJSON(listReq); err != nil {
		t.Fatalf("Failed to send channels.list request: %v", err)
	}

	var listResp map[string]interface{}
	if err := conn.ReadJSON(&listResp); err != nil {
		t.Fatalf("Failed to read channels.list response: %v", err)
	}

	if listResp["error"] != nil {
		t.Errorf("channels.list returned error: %v", listResp["error"])
	}

	result := listResp["result"].(map[string]interface{})
	channels, ok := result["channels"].([]string)
	if !ok {
		t.Error("Expected channels to be a string array")
	}

	t.Logf("Available channels: %v", channels)

	t.Log("Channel methods test passed")
}

// TestGatewayAuthentication tests token-based authentication
func TestGatewayAuthentication(t *testing.T) {
	messageBus := bus.NewMessageBus(100)
	sessionMgr, _ := session.NewManager("/tmp/test-sessions-auth")
	channelMgr := channels.NewManager(messageBus)

	cfg := &config.GatewayConfig{
		Host:         "127.0.0.1",
		Port:         18084,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	server := gateway.NewServer(cfg, messageBus, channelMgr, sessionMgr)

	// Configure authentication
	wsCfg := &gateway.WebSocketConfig{
		Host:       "127.0.0.1",
		Port:       18790,
		Path:       "/ws",
		EnableAuth: true,
		AuthToken:  "test-token-123",
	}
	server.SetWebSocketConfig(wsCfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() { _ = server.Stop() }()

	time.Sleep(100 * time.Millisecond)

	// Test without token (should fail)
	wsURL := fmt.Sprintf("ws://127.0.0.1:18790%s", "/ws")
	_, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Error("Expected connection to fail without auth token")
	}

	// Test with correct token
	wsURLWithToken := fmt.Sprintf("ws://127.0.0.1:18790%s?token=%s", "/ws", "test-token-123")
	conn, _, err := websocket.DefaultDialer.Dial(wsURLWithToken, nil)
	if err != nil {
		t.Fatalf("Failed to connect with valid token: %v", err)
	}
	defer conn.Close()

	var welcome map[string]interface{}
	if err := conn.ReadJSON(&welcome); err != nil {
		t.Fatalf("Failed to read welcome message: %v", err)
	}

	if welcome["method"] != "connected" {
		t.Error("Expected connected message after successful auth")
	}

	t.Log("Authentication test passed")
}

// TestGatewayHeartbeat tests heartbeat/ping-pong mechanism
func TestGatewayHeartbeat(t *testing.T) {
	messageBus := bus.NewMessageBus(100)
	sessionMgr, _ := session.NewManager("/tmp/test-sessions-heartbeat")
	channelMgr := channels.NewManager(messageBus)

	cfg := &config.GatewayConfig{
		Host:         "127.0.0.1",
		Port:         18085,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	server := gateway.NewServer(cfg, messageBus, channelMgr, sessionMgr)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() { _ = server.Stop() }()

	time.Sleep(100 * time.Millisecond)

	wsURL := fmt.Sprintf("ws://127.0.0.1:18789%s", "/ws")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Set ping handler
	conn.SetPingHandler(func(appData string) error {
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(time.Second))
	})

	// Wait for ping from server
	done := make(chan bool, 1)
	go func() {
		for i := 0; i < 3; i++ {
			messageType, p, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if messageType == websocket.PingMessage {
				_ = conn.WriteMessage(websocket.PongMessage, p)
			}
		}
		done <- true
	}()

	select {
	case <-done:
		t.Log("Heartbeat test passed - received pings from server")
	case <-time.After(5 * time.Second):
		t.Error("Timeout waiting for heartbeat pings")
	}
}

// TestGatewayBroadcastOutbound tests outbound message broadcasting
func TestGatewayBroadcastOutbound(t *testing.T) {
	messageBus := bus.NewMessageBus(100)
	sessionMgr, _ := session.NewManager("/tmp/test-sessions-broadcast")
	channelMgr := channels.NewManager(messageBus)

	cfg := &config.GatewayConfig{
		Host:         "127.0.0.1",
		Port:         18086,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	server := gateway.NewServer(cfg, messageBus, channelMgr, sessionMgr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() { _ = server.Stop() }()

	time.Sleep(100 * time.Millisecond)

	// Create multiple connections
	connections := []*websocket.Conn{}
	for i := 0; i < 3; i++ {
		wsURL := fmt.Sprintf("ws://127.0.0.1:18789%s", "/ws")
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Failed to create connection %d: %v", i, err)
		}
		defer conn.Close()
		_ = append(connections, conn)

		// Read welcome message
		var welcome map[string]interface{}
		_ = conn.ReadJSON(&welcome)
	}

	// Publish an outbound message
	outboundMsg := &bus.OutboundMessage{
		Channel:   "test",
		ChatID:    "123",
		Content:   "Test broadcast message",
		Timestamp: time.Now(),
	}

	_ = messageBus.PublishOutbound(ctx, outboundMsg)

	// Give broadcast time to propagate
	time.Sleep(200 * time.Millisecond)

	t.Log("Broadcast outbound test completed")
}
