package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/internal/logger"
	"github.com/smallnest/goclaw/providers"
	"github.com/smallnest/goclaw/session"
	"go.uber.org/zap"
)

// Agent represents the main AI agent
// New implementation inspired by pi-mono architecture
type Agent struct {
	orchestrator *Orchestrator
	bus          *bus.MessageBus
	provider     providers.Provider
	sessionMgr   *session.Manager
	tools        *ToolRegistry
	context      *ContextBuilder
	workspace    string

	mu        sync.RWMutex
	state     *AgentState
	eventSubs []chan *Event
	running   bool
}

// NewAgentConfig configures the agent
type NewAgentConfig struct {
	Bus          *bus.MessageBus
	Provider     providers.Provider
	SessionMgr   *session.Manager
	Tools        *ToolRegistry
	Context      *ContextBuilder
	Workspace    string
	MaxIteration int
}

// NewAgent creates a new agent
func NewAgent(cfg *NewAgentConfig) (*Agent, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	if cfg.MaxIteration <= 0 {
		cfg.MaxIteration = 15
	}

	state := NewAgentState()
	state.SystemPrompt = cfg.Context.BuildSystemPrompt(nil)
	state.Model = getModelName(cfg.Provider)
	state.Provider = "provider"
	state.SessionKey = "main"
	state.Tools = ToAgentTools(cfg.Tools.ListExisting())

	loopConfig := &LoopConfig{
		Model:            state.Model,
		Provider:         cfg.Provider,
		SessionMgr:       cfg.SessionMgr,
		MaxIterations:    cfg.MaxIteration,
		ConvertToLLM:     defaultConvertToLLM,
		TransformContext: nil,
		GetSteeringMessages: func() ([]AgentMessage, error) {
			state := state // Capture state
			return state.DequeueSteeringMessages(), nil
		},
		GetFollowUpMessages: func() ([]AgentMessage, error) {
			state := state // Capture state
			return state.DequeueFollowUpMessages(), nil
		},
	}

	orchestrator := NewOrchestrator(loopConfig, state)

	return &Agent{
		orchestrator: orchestrator,
		bus:          cfg.Bus,
		provider:     cfg.Provider,
		sessionMgr:   cfg.SessionMgr,
		tools:        cfg.Tools,
		context:      cfg.Context,
		workspace:    cfg.Workspace,
		state:        state,
		eventSubs:    make([]chan *Event, 0),
		running:      false,
	}, nil
}

// Start starts the agent loop
func (a *Agent) Start(ctx context.Context) error {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return fmt.Errorf("agent already running")
	}
	a.running = true
	a.mu.Unlock()

	logger.Info("Starting agent loop")

	// Start event dispatcher
	go a.dispatchEvents(ctx)

	// Start message processor
	go a.processMessages(ctx)

	return nil
}

// Stop stops the agent
func (a *Agent) Stop() error {
	a.mu.Lock()
	a.running = false
	a.mu.Unlock()

	logger.Info("Stopping agent")
	a.orchestrator.Stop()
	return nil
}

// Prompt sends a user message to the agent
func (a *Agent) Prompt(ctx context.Context, content string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	msg := AgentMessage{
		Role:      RoleUser,
		Content:   []ContentBlock{TextContent{Text: content}},
		Timestamp: time.Now().UnixMilli(),
	}

	// Run orchestrator
	finalMessages, err := a.orchestrator.Run(ctx, []AgentMessage{msg})
	if err != nil {
		logger.Error("Agent execution failed", zap.Error(err))
		return err
	}

	// Update state
	a.mu.Lock()
	a.state.Messages = finalMessages
	a.mu.Unlock()

	// Publish final response
	if len(finalMessages) > 0 {
		lastMsg := finalMessages[len(finalMessages)-1]
		if lastMsg.Role == RoleAssistant {
			a.publishResponse(ctx, lastMsg)
		}
	}

	return nil
}

// processMessages processes inbound messages from the bus
func (a *Agent) processMessages(ctx context.Context) {
	for a.isRunning() {
		select {
		case <-ctx.Done():
			logger.Info("Message processor stopped")
			return

		default:
			msg, err := a.bus.ConsumeInbound(ctx)
			if err != nil {
				if err == context.DeadlineExceeded || err == context.Canceled {
					continue
				}
				logger.Error("Failed to consume inbound", zap.Error(err))
				continue
			}

			a.handleInboundMessage(ctx, msg)
		}
	}
}

// handleInboundMessage processes a single inbound message
func (a *Agent) handleInboundMessage(ctx context.Context, msg *bus.InboundMessage) {
	logger.Info("Processing inbound message",
		zap.String("channel", msg.Channel),
		zap.String("chat_id", msg.ChatID),
	)

	// Generate fresh session key with timestamp for new sessions
	sessionKey := msg.SessionKey()
	if msg.ChatID == "default" || msg.ChatID == "" {
		// For CLI/default chat, always create a fresh session with timestamp
		sessionKey = fmt.Sprintf("%s:%d", msg.Channel, time.Now().Unix())
		logger.Info("Creating fresh session", zap.String("session_key", sessionKey))
	}

	// Get or create session
	sess, err := a.sessionMgr.GetOrCreate(sessionKey)
	if err != nil {
		logger.Error("Failed to get session", zap.Error(err))
		return
	}

	// Convert to agent message
	agentMsg := AgentMessage{
		Role:      RoleUser,
		Content:   []ContentBlock{TextContent{Text: msg.Content}},
		Timestamp: msg.Timestamp.UnixMilli(),
	}

	// Add media as image content
	for _, m := range msg.Media {
		if m.Type == "image" {
			imgContent := ImageContent{
				URL:      m.URL,
				Data:     m.Base64,
				MimeType: m.MimeType,
			}
			agentMsg.Content = append(agentMsg.Content, imgContent)
		}
	}

	// Run agent
	finalMessages, err := a.orchestrator.Run(ctx, []AgentMessage{agentMsg})
	if err != nil {
		logger.Error("Agent execution failed", zap.Error(err))

		// Send error response
		a.publishError(ctx, msg.Channel, msg.ChatID, err)
		return
	}

	// Update session
	a.updateSession(sess, finalMessages)

	// Publish response
	if len(finalMessages) > 0 {
		lastMsg := finalMessages[len(finalMessages)-1]
		if lastMsg.Role == RoleAssistant {
			a.publishToBus(ctx, msg.Channel, msg.ChatID, lastMsg)
		}
	}
}

// updateSession updates the session with new messages
func (a *Agent) updateSession(sess *session.Session, messages []AgentMessage) {
	for _, msg := range messages {
		sessMsg := session.Message{
			Role:      string(msg.Role),
			Content:   extractTextContent(msg),
			Timestamp: time.Unix(extractTimestamp(msg)/1000, 0),
		}

		// Handle tool calls
		if msg.Role == RoleAssistant {
			for _, block := range msg.Content {
				if tc, ok := block.(ToolCallContent); ok {
					sessMsg.ToolCalls = []session.ToolCall{
						{
							ID:     tc.ID,
							Name:   tc.Name,
							Params: tc.Arguments,
						},
					}
				}
			}
		}

		// Handle tool results
		if msg.Role == RoleToolResult {
			if id, ok := msg.Metadata["tool_call_id"].(string); ok {
				sessMsg.ToolCallID = id
			}
		}

		sess.AddMessage(sessMsg)
	}

	if err := a.sessionMgr.Save(sess); err != nil {
		logger.Error("Failed to save session", zap.Error(err))
	}
}

// publishResponse publishes the agent response to the bus
func (a *Agent) publishResponse(ctx context.Context, msg AgentMessage) {
	content := extractTextContent(msg)

	outbound := &bus.OutboundMessage{
		Channel:   a.GetCurrentChannel(),
		ChatID:    a.GetCurrentChatID(),
		Content:   content,
		Timestamp: time.Now(),
	}

	if err := a.bus.PublishOutbound(ctx, outbound); err != nil {
		logger.Error("Failed to publish outbound", zap.Error(err))
	}
}

// publishError publishes an error message
func (a *Agent) publishError(ctx context.Context, channel, chatID string, err error) {
	errorMsg := fmt.Sprintf("An error occurred: %v", err)

	outbound := &bus.OutboundMessage{
		Channel:   channel,
		ChatID:    chatID,
		Content:   errorMsg,
		Timestamp: time.Now(),
	}

	_ = a.bus.PublishOutbound(ctx, outbound)
}

// publishToBus publishes a message to the bus
func (a *Agent) publishToBus(ctx context.Context, channel, chatID string, msg AgentMessage) {
	content := extractTextContent(msg)

	outbound := &bus.OutboundMessage{
		Channel:   channel,
		ChatID:    chatID,
		Content:   content,
		Timestamp: time.Now(),
	}

	if err := a.bus.PublishOutbound(ctx, outbound); err != nil {
		logger.Error("Failed to publish outbound", zap.Error(err))
	}
}

// Subscribe subscribes to agent events
func (a *Agent) Subscribe() <-chan *Event {
	ch := make(chan *Event, 10)

	a.mu.Lock()
	a.eventSubs = append(a.eventSubs, ch)
	a.mu.Unlock()

	return ch
}

// Unsubscribe removes an event subscription
func (a *Agent) Unsubscribe(ch <-chan *Event) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for i, sub := range a.eventSubs {
		if sub == ch {
			a.eventSubs = append(a.eventSubs[:i], a.eventSubs[i+1:]...)
			// Don't close receive-only channel
			break
		}
	}
}

// dispatchEvents sends events to all subscribers
func (a *Agent) dispatchEvents(ctx context.Context) {
	eventChan := a.orchestrator.Subscribe()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-eventChan:
			if !ok {
				return
			}

			a.mu.RLock()
			subs := make([]chan *Event, len(a.eventSubs))
			copy(subs, a.eventSubs)
			a.mu.RUnlock()

			for _, ch := range subs {
				select {
				case ch <- event:
				default:
					// Channel full, skip
				}
			}
		}
	}
}

// isRunning checks if agent is running
func (a *Agent) isRunning() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.running
}

// GetState returns a copy of the current agent state
func (a *Agent) GetState() *AgentState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.state.Clone()
}

// SetSystemPrompt updates the system prompt
func (a *Agent) SetSystemPrompt(prompt string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.state.SystemPrompt = prompt
}

// SetTools updates the available tools
func (a *Agent) SetTools(tools []Tool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.state.Tools = tools
}

// GetCurrentChannel returns the current output channel
func (a *Agent) GetCurrentChannel() string {
	return "cli"
}

// GetCurrentChatID returns the current chat ID
func (a *Agent) GetCurrentChatID() string {
	return "main"
}

// GetOrchestrator 获取 orchestrator（供 AgentManager 使用）
func (a *Agent) GetOrchestrator() *Orchestrator {
	return a.orchestrator
}

// Helper functions

// getModelName extracts model name from provider
func getModelName(p providers.Provider) string {
	// This is a placeholder - actual implementation would depend on provider type
	return "default"
}

// defaultConvertToLLM converts agent messages to provider messages
func defaultConvertToLLM(messages []AgentMessage) ([]providers.Message, error) {
	result := make([]providers.Message, 0, len(messages))

	for _, msg := range messages {
		// Skip system messages
		if msg.Role == RoleSystem {
			continue
		}

		providerMsg := providers.Message{
			Role: string(msg.Role),
		}

		// Extract content
		for _, block := range msg.Content {
			switch b := block.(type) {
			case TextContent:
				providerMsg.Content = b.Text
			case ImageContent:
				if b.Data != "" {
					providerMsg.Images = []string{b.Data}
				} else if b.URL != "" {
					providerMsg.Images = []string{b.URL}
				}
			}
		}

		// Handle tool calls
		if msg.Role == RoleAssistant {
			var toolCalls []providers.ToolCall
			for _, block := range msg.Content {
				if tc, ok := block.(ToolCallContent); ok {
					toolCalls = append(toolCalls, providers.ToolCall{
						ID:     tc.ID,
						Name:   tc.Name,
						Params: convertMapAnyToInterface(tc.Arguments),
					})
				}
			}
			providerMsg.ToolCalls = toolCalls
		}

		// Handle tool_call_id for tool result messages
		if msg.Role == RoleToolResult {
			if toolCallID, ok := msg.Metadata["tool_call_id"].(string); ok {
				providerMsg.ToolCallID = toolCallID
			}
		}

		result = append(result, providerMsg)
	}

	return result, nil
}

// convertMapAnyToInterface converts map[string]any to map[string]interface{}
func convertMapAnyToInterface(m map[string]any) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m {
		result[k] = v
	}
	return result
}

// extractTextContent extracts text from content blocks
func extractTextContent(msg AgentMessage) string {
	for _, block := range msg.Content {
		if text, ok := block.(TextContent); ok {
			return text.Text
		}
	}
	return ""
}

// extractTimestamp extracts timestamp from message
func extractTimestamp(msg AgentMessage) int64 {
	if msg.Timestamp > 0 {
		return msg.Timestamp
	}
	return time.Now().UnixMilli()
}
