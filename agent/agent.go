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
	orchestrator       *Orchestrator
	bus                *bus.MessageBus
	provider           providers.Provider
	sessionMgr         *session.Manager
	tools              *ToolRegistry
	context            *ContextBuilder
	workspace          string
	skillsLoader       *SkillsLoader
	helper             *AgentHelper
	maxHistoryMessages int // 最大历史消息数量

	mu        sync.RWMutex
	state     *AgentState
	eventSubs []chan *Event
	running   bool
}

// NewAgentConfig configures the agent
type NewAgentConfig struct {
	Bus                *bus.MessageBus
	Provider           providers.Provider
	SessionMgr         *session.Manager
	Tools              *ToolRegistry
	Context            *ContextBuilder
	Workspace          string
	MaxIteration       int
	MaxHistoryMessages int // 最大历史消息数量
	SkillsLoader       *SkillsLoader
}

// NewAgent creates a new agent
func NewAgent(cfg *NewAgentConfig) (*Agent, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	if cfg.MaxIteration <= 0 {
		cfg.MaxIteration = 15
	}

	// 设置默认的最大历史消息数
	if cfg.MaxHistoryMessages <= 0 {
		cfg.MaxHistoryMessages = 100
	}

	state := NewAgentState()
	state.SystemPrompt = cfg.Context.BuildSystemPrompt(nil)
	state.Model = getModelName(cfg.Provider)
	state.Provider = "provider"
	state.SessionKey = "main"
	state.Tools = ToAgentTools(cfg.Tools.ListExisting())
	state.LoadedSkills = []string{} // Initialize with empty loaded skills

	// Load skills list
	var skills []*Skill
	if cfg.SkillsLoader != nil {
		if err := cfg.SkillsLoader.Discover(); err == nil {
			skills = cfg.SkillsLoader.List()
			logger.Info("Skills discovered for agent",
				zap.Int("count", len(skills)))
		} else {
			logger.Warn("Failed to discover skills", zap.Error(err))
		}
	}

	loopConfig := &LoopConfig{
		Model:            state.Model,
		Provider:         cfg.Provider,
		SessionMgr:       cfg.SessionMgr,
		MaxIterations:    cfg.MaxIteration,
		ConvertToLLM:     defaultConvertToLLM,
		TransformContext: nil,
		Skills:           skills,
		LoadedSkills:     state.LoadedSkills,
		ContextBuilder:   cfg.Context,
		GetSteeringMessages: func(s *AgentState) func() ([]AgentMessage, error) {
			return func() ([]AgentMessage, error) {
				return s.DequeueSteeringMessages(), nil
			}
		}(state),
		GetFollowUpMessages: func(s *AgentState) func() ([]AgentMessage, error) {
			return func() ([]AgentMessage, error) {
				return s.DequeueFollowUpMessages(), nil
			}
		}(state),
	}

	orchestrator := NewOrchestrator(loopConfig, state)

	return &Agent{
		orchestrator:       orchestrator,
		bus:                cfg.Bus,
		provider:           cfg.Provider,
		sessionMgr:         cfg.SessionMgr,
		tools:              cfg.Tools,
		context:            cfg.Context,
		workspace:          cfg.Workspace,
		skillsLoader:       cfg.SkillsLoader,
		helper:             NewAgentHelper(cfg.SessionMgr),
		maxHistoryMessages: cfg.MaxHistoryMessages,
		state:              state,
		eventSubs:          make([]chan *Event, 0),
		running:            false,
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
	a.cleanupSubscriptions()
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

	// Update state (already have lock from above)
	a.state.Messages = finalMessages

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
		zap.String("message_id", msg.ID),
		zap.String("content", msg.Content),
	)

	// Generate session key
	sessionKey := msg.SessionKey()
	logger.Debug("Generated session key", zap.String("session_key", sessionKey))

	// Get or create session
	sess, err := a.sessionMgr.GetOrCreate(sessionKey)
	if err != nil {
		logger.Error("Failed to get session", zap.Error(err))
		return
	}
	logger.Debug("Session retrieved/created", zap.String("session_key", sess.Key))

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

	// Load history messages and add current message
	// Use maxHistoryMessages to limit history and avoid token limit exceeded errors
	// Use GetHistorySafe to ensure we don't break tool call pairs
	history := sess.GetHistorySafe(a.maxHistoryMessages)
	logger.Debug("History loaded", zap.Int("history_count", len(history)))
	historyAgentMsgs := sessionMessagesToAgentMessages(history)
	allMessages := append(historyAgentMsgs, agentMsg)

	// Run agent
	logger.Info("Starting agent execution",
		zap.String("message_id", msg.ID),
		zap.Int("total_messages", len(allMessages)),
	)
	finalMessages, err := a.orchestrator.Run(ctx, allMessages)
	logger.Info("Agent execution completed",
		zap.String("message_id", msg.ID),
		zap.Int("final_messages", len(finalMessages)),
		zap.Error(err),
	)
	if err != nil {
		logger.Error("Agent execution failed", zap.Error(err))

		// Send error response
		a.publishError(ctx, msg.Channel, msg.ChatID, err)
		return
	}

	// Update session (only save new messages, skip history)
	// orchestrator.Run returns all messages including input and history
	historyLen := len(history)
	if len(finalMessages) > historyLen {
		newMessages := finalMessages[historyLen:]
		a.updateSession(sess, newMessages)
	}

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
	_ = a.helper.UpdateSession(sess, messages, &UpdateSessionOptions{SaveImmediately: true})
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
// Returns a read-only channel. Call Unsubscribe to clean up.
// IMPORTANT: Always call Unsubscribe when done to prevent memory leaks.
func (a *Agent) Subscribe() <-chan *Event {
	ch := make(chan *Event, 10)

	a.mu.Lock()
	a.eventSubs = append(a.eventSubs, ch)
	a.mu.Unlock()

	return ch
}

// Unsubscribe removes an event subscription
// The channel will be removed from the subscriber list but not closed
// (since it's receive-only from the caller's perspective).
// Any pending events in the channel can still be read by the caller.
func (a *Agent) Unsubscribe(ch <-chan *Event) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for i, sub := range a.eventSubs {
		if sub == ch {
			// Remove from slice
			a.eventSubs = append(a.eventSubs[:i], a.eventSubs[i+1:]...)
			break
		}
	}
}

// cleanupSubscriptions removes all subscriptions and closes their channels
// This should only be called when the agent is being shut down.
func (a *Agent) cleanupSubscriptions() {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Close all subscriber channels
	for _, ch := range a.eventSubs {
		close(ch)
	}
	a.eventSubs = make([]chan *Event, 0)
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
					// Channel full or closed, skip without blocking
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

// Steer adds a steering message to interrupt the agent mid-run
// Inspired by pi-mono's Agent.steer() method
func (a *Agent) Steer(msg AgentMessage) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.state.Steer(msg)
}

// FollowUp adds a follow-up message to be processed after agent finishes
// Inspired by pi-mono's Agent.followUp() method
func (a *Agent) FollowUp(msg AgentMessage) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.state.FollowUp(msg)
}

// WaitForIdle waits until the agent is not streaming
// Inspired by pi-mono's Agent.waitForIdle() method
func (a *Agent) WaitForIdle(ctx context.Context) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			a.mu.RLock()
			isStreaming := a.state.IsStreaming
			a.mu.RUnlock()
			if !isStreaming {
				return nil
			}
		}
	}
}

// Abort aborts the current agent execution
// Inspired by pi-mono's Agent.abort() method
func (a *Agent) Abort() {
	a.orchestrator.Stop()
}

// Reset resets the agent state
// Inspired by pi-mono's Agent.reset() method
func (a *Agent) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.state = NewAgentState()
	a.state.SystemPrompt = a.context.BuildSystemPrompt(nil)
	a.state.Model = getModelName(a.provider)
	a.state.Provider = "provider"
	a.state.SessionKey = "main"
	a.state.Tools = ToAgentTools(a.tools.ListExisting())
}

// SetSteeringMode sets how steering messages are delivered
func (a *Agent) SetSteeringMode(mode MessageQueueMode) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.SteeringMode = mode
}

// SetFollowUpMode sets how follow-up messages are delivered
func (a *Agent) SetFollowUpMode(mode MessageQueueMode) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.FollowUpMode = mode
}

// ReplaceMessages replaces the message history
// Inspired by pi-mono's Agent.replaceMessages() method
func (a *Agent) ReplaceMessages(messages []AgentMessage) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.state.Messages = make([]AgentMessage, len(messages))
	copy(a.state.Messages, messages)
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

		// Skip tool messages that don't have a matching tool_call_id
		if msg.Role == RoleToolResult {
			toolCallID, hasID := msg.Metadata["tool_call_id"].(string)
			toolName, hasName := msg.Metadata["tool_name"].(string)
			if !hasID || !hasName || toolCallID == "" || toolName == "" {
				logger.Warn("Skipping tool message without tool_call_id or tool_name",
					zap.String("role", string(msg.Role)),
					zap.Bool("has_id", hasID),
					zap.Bool("has_name", hasName),
					zap.String("tool_call_id", toolCallID),
					zap.String("tool_name", toolName))
				continue
			}
		}

		providerMsg := providers.Message{
			Role: string(msg.Role),
		}

		// Extract content
		for _, block := range msg.Content {
			switch b := block.(type) {
			case TextContent:
				if providerMsg.Content != "" {
					providerMsg.Content += "\n" + b.Text
				} else {
					providerMsg.Content = b.Text
				}
			case ImageContent:
				if b.Data != "" {
					providerMsg.Images = append(providerMsg.Images, b.Data)
				} else if b.URL != "" {
					providerMsg.Images = append(providerMsg.Images, b.URL)
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

		// Handle tool_call_id and tool_name for tool result messages
		if msg.Role == RoleToolResult {
			if toolCallID, ok := msg.Metadata["tool_call_id"].(string); ok {
				providerMsg.ToolCallID = toolCallID
			}
			if toolName, ok := msg.Metadata["tool_name"].(string); ok {
				providerMsg.ToolName = toolName
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
