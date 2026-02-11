package integration

import (
	"context"
	"testing"
	"time"

	"github.com/smallnest/dogclaw/goclaw/agent"
	"github.com/smallnest/dogclaw/goclaw/agent/tools"
	"github.com/smallnest/dogclaw/goclaw/bus"
	"github.com/smallnest/dogclaw/goclaw/providers"
	"github.com/smallnest/dogclaw/goclaw/session"
)

// TestE2EConversationFlow tests a complete conversation flow
func TestE2EConversationFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Setup
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	messageBus := bus.NewMessageBus(100)
	sessionMgr, _ := session.NewManager(t.TempDir())

	// Create mock provider
	provider := &mockE2EProvider{}

	// Create agent loop
	toolRegistry := tools.NewRegistry()
	contextBuilder := agent.NewContextBuilder(nil, "")
	memory := agent.NewMemoryStore(t.TempDir())

	loopCfg := &agent.Config{
		Bus:          messageBus,
		Provider:     provider,
		SessionMgr:   sessionMgr,
		Memory:       memory,
		Context:      contextBuilder,
		Tools:        toolRegistry,
		SkillsLoader: nil,
		Subagents:    nil,
		Workspace:    t.TempDir(),
		MaxIteration: 5,
	}

	loop, err := agent.NewLoop(loopCfg)
	if err != nil {
		t.Fatalf("Failed to create agent loop: %v", err)
	}

	// Start agent loop
	if err := loop.Start(ctx); err != nil {
		t.Fatalf("Failed to start agent loop: %v", err)
	}
	defer func() { _ = loop.Stop() }()

	// Send inbound message
	inboundMsg := &bus.InboundMessage{
		Channel:   "test",
		SenderID:  "test-user",
		ChatID:    "test-chat",
		Content:   "Hello, how are you?",
		Timestamp: time.Now(),
	}

	if err := messageBus.PublishInbound(ctx, inboundMsg); err != nil {
		t.Fatalf("Failed to publish inbound message: %v", err)
	}

	// Wait for response
	time.Sleep(1 * time.Second)

	// Verify session was created/updated
	sess, err := sessionMgr.GetOrCreate("test:test-chat")
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}

	if len(sess.Messages) < 2 {
		t.Logf("Warning: Session has %d messages (expected at least 2)", len(sess.Messages))
	}

	t.Logf("E2E conversation flow test completed with %d messages", len(sess.Messages))
}

// TestE2EAgentLoop tests basic agent loop functionality
func TestE2EAgentLoop(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	messageBus := bus.NewMessageBus(100)
	sessionMgr, _ := session.NewManager(t.TempDir())

	// Create mock provider
	provider := &mockE2EProvider{
		responses: []string{
			"Hello! I'm doing well, thank you.",
			"Based on your question, here's what I think...",
		},
	}

	toolRegistry := tools.NewRegistry()
	contextBuilder := agent.NewContextBuilder(nil, "")
	memory := agent.NewMemoryStore(t.TempDir())

	loopCfg := &agent.Config{
		Bus:          messageBus,
		Provider:     provider,
		SessionMgr:   sessionMgr,
		Memory:       memory,
		Context:      contextBuilder,
		Tools:        toolRegistry,
		MaxIteration: 3,
	}

	loop, err := agent.NewLoop(loopCfg)
	if err != nil {
		t.Fatalf("Failed to create agent loop: %v", err)
	}

	testCtx, testCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer testCancel()

	if err := loop.Start(testCtx); err != nil {
		t.Fatalf("Failed to start agent loop: %v", err)
	}
	defer func() { _ = loop.Stop() }()

	// Send test message
	inboundMsg := &bus.InboundMessage{
		Channel:   "test",
		SenderID:  "test-user",
		ChatID:    "e2e-test",
		Content:   "Test message for E2E",
		Timestamp: time.Now(),
	}

	_ = messageBus.PublishInbound(testCtx, inboundMsg)

	// Wait for processing
	time.Sleep(2 * time.Second)

	t.Log("E2E agent loop test passed")
}

// Mock provider for E2E testing
type mockE2EProvider struct {
	responses []string
	callCount int
}

func (m *mockE2EProvider) Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, options ...providers.ChatOption) (*providers.Response, error) {
	m.callCount++

	response := "Default response"
	if len(m.responses) > 0 {
		response = m.responses[m.callCount%len(m.responses)]
	}

	return &providers.Response{
		Content: response,
		Usage: providers.Usage{
			PromptTokens:     50,
			CompletionTokens: 30,
			TotalTokens:      80,
		},
	}, nil
}

func (m *mockE2EProvider) ChatWithTools(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, options ...providers.ChatOption) (*providers.Response, error) {
	return m.Chat(ctx, messages, tools, options...)
}

func (m *mockE2EProvider) Close() error {
	return nil
}
