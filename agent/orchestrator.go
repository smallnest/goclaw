package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/smallnest/goclaw/internal/logger"
	"github.com/smallnest/goclaw/providers"
	"go.uber.org/zap"
)

// Orchestrator manages the agent execution loop
// Based on pi-mono's agent-loop.ts design
type Orchestrator struct {
	config     *LoopConfig
	state      *AgentState
	eventChan  chan *Event
	cancelFunc context.CancelFunc
}

// NewOrchestrator creates a new agent orchestrator
func NewOrchestrator(config *LoopConfig, initialState *AgentState) *Orchestrator {
	return &Orchestrator{
		config:    config,
		state:     initialState,
		eventChan: make(chan *Event, 100),
	}
}

// Run starts the agent loop with initial prompts
func (o *Orchestrator) Run(ctx context.Context, prompts []AgentMessage) ([]AgentMessage, error) {
	logger.Info("=== Orchestrator Run Start ===",
		zap.Int("prompts_count", len(prompts)))

	ctx, cancel := context.WithCancel(ctx)
	o.cancelFunc = cancel

	// Initialize state with prompts
	newMessages := make([]AgentMessage, len(prompts))
	copy(newMessages, prompts)
	currentState := o.state.Clone()
	currentState.AddMessages(newMessages)

	// Emit start event
	o.emit(NewEvent(EventAgentStart))

	// Main loop
	finalMessages, err := o.runLoop(ctx, currentState)

	logger.Info("=== Orchestrator Run End ===",
		zap.Int("final_messages_count", len(finalMessages)),
		zap.Error(err))

	// Emit end event
	endEvent := NewEvent(EventAgentEnd)
	if finalMessages != nil {
		endEvent = NewEvent(EventAgentEnd).WithFinalMessages(finalMessages)
	}
	o.emit(endEvent)

	cancel()
	if err != nil {
		return nil, fmt.Errorf("agent loop failed: %w", err)
	}

	return finalMessages, nil
}

// runLoop implements the main agent loop logic
func (o *Orchestrator) runLoop(ctx context.Context, state *AgentState) ([]AgentMessage, error) {
	firstTurn := true

	// Check for steering messages at start
	pendingMessages := o.fetchSteeringMessages()

	// Outer loop: continues when queued follow-up messages arrive
	for {
		hasMoreToolCalls := true
		steeringAfterTools := false

		// Inner loop: process tool calls and steering messages
		for hasMoreToolCalls || len(pendingMessages) > 0 {
			if !firstTurn {
				o.emit(NewEvent(EventTurnStart))
			} else {
				firstTurn = false
			}

			// Process pending messages (inject before next assistant response)
			if len(pendingMessages) > 0 {
				for _, msg := range pendingMessages {
					o.emit(NewEvent(EventMessageStart))
					state.AddMessage(msg)
					o.emit(NewEvent(EventMessageEnd))
				}
				pendingMessages = []AgentMessage{}
			}

			// Stream assistant response
			assistantMsg, err := o.streamAssistantResponse(ctx, state)
			if err != nil {
				o.emitErrorEnd(state, err)
				return state.Messages, err
			}

			state.AddMessage(assistantMsg)

			// Check for tool calls
			toolCalls := extractToolCalls(assistantMsg)
			hasMoreToolCalls = len(toolCalls) > 0

			if hasMoreToolCalls {
				results, steering := o.executeToolCalls(ctx, toolCalls, state)
				steeringAfterTools = len(steering) > 0

				// Add tool result messages
				for _, result := range results {
					state.AddMessage(result)
				}

				// If steering messages arrived, skip remaining tools
				if steeringAfterTools {
					pendingMessages = steering
					break
				}
			}

			o.emit(NewEvent(EventTurnEnd))

			// Get steering messages after turn completes
			if !steeringAfterTools && len(pendingMessages) == 0 {
				pendingMessages = o.fetchSteeringMessages()
			}
		}

		// Agent would stop here. Check for follow-up messages.
		followUpMessages := o.fetchFollowUpMessages()
		if len(followUpMessages) > 0 {
			pendingMessages = append(pendingMessages, followUpMessages...)
			continue
		}

		// No more messages, exit
		break
	}

	return state.Messages, nil
}

// streamAssistantResponse calls the LLM and streams the response
func (o *Orchestrator) streamAssistantResponse(ctx context.Context, state *AgentState) (AgentMessage, error) {
	logger.Info("=== streamAssistantResponse Start ===",
		zap.Int("message_count", len(state.Messages)),
		zap.Strings("loaded_skills", state.LoadedSkills))

	state.IsStreaming = true
	defer func() { state.IsStreaming = false }()

	// Apply context transform if configured
	messages := state.Messages
	if o.config.TransformContext != nil {
		transformed, err := o.config.TransformContext(messages)
		if err == nil {
			messages = transformed
		} else {
			logger.Warn("Context transform failed, using original", zap.Error(err))
		}
	}

	// Convert to provider messages
	var providerMsgs []providers.Message
	if o.config.ConvertToLLM != nil {
		converted, err := o.config.ConvertToLLM(messages)
		if err != nil {
			return AgentMessage{}, fmt.Errorf("convert to LLM failed: %w", err)
		}
		providerMsgs = converted
	} else {
		// Default conversion
		providerMsgs = convertToProviderMessages(messages)
	}

	// Prepare tool definitions
	toolDefs := convertToToolDefinitions(state.Tools)

	// Emit message start
	o.emit(NewEvent(EventMessageStart))

	// Call provider with system prompt as first message
	fullMessages := []providers.Message{}

	// Build system prompt with skills if context builder is available
	if o.config.ContextBuilder != nil {
		skillsContent := ""
		if len(state.LoadedSkills) > 0 {
			// Second phase: inject full content of loaded skills
			skillsContent = o.config.ContextBuilder.buildSelectedSkills(state.LoadedSkills, o.config.Skills)
		} else if len(o.config.Skills) > 0 {
			// First phase: inject skill summary (available skills list)
			skillsContent = o.config.ContextBuilder.buildSkillsPrompt(o.config.Skills, PromptModeFull)
		}
		systemPrompt := o.config.ContextBuilder.buildSystemPromptWithSkills(skillsContent, PromptModeFull)
		fullMessages = append(fullMessages, providers.Message{
			Role:    "system",
			Content: systemPrompt,
		})
	} else if state.SystemPrompt != "" {
		// Fallback to stored system prompt
		fullMessages = append(fullMessages, providers.Message{
			Role:    "system",
			Content: state.SystemPrompt,
		})
	}
	fullMessages = append(fullMessages, providerMsgs...)

	logger.Info("=== Calling LLM ===",
		zap.Int("messages_count", len(fullMessages)),
		zap.Int("tools_count", len(toolDefs)),
		zap.Bool("has_loaded_skills", len(state.LoadedSkills) > 0))

	response, err := o.config.Provider.Chat(ctx, fullMessages, toolDefs)
	if err != nil {
		logger.Error("LLM call failed", zap.Error(err))
		return AgentMessage{}, fmt.Errorf("LLM call failed: %w", err)
	}

	logger.Info("=== LLM Response Received ===",
		zap.Int("content_length", len(response.Content)),
		zap.Int("tool_calls_count", len(response.ToolCalls)),
		zap.String("content_preview", truncateString(response.Content, 200)))

	// Emit message end
	o.emit(NewEvent(EventMessageEnd))

	// Convert response to agent message
	assistantMsg := convertFromProviderResponse(response)

	logger.Info("=== streamAssistantResponse End ===",
		zap.Bool("has_tool_calls", len(response.ToolCalls) > 0),
		zap.Int("tool_calls_count", len(response.ToolCalls)))

	return assistantMsg, nil
}

// executeToolCalls executes tool calls with interruption support
func (o *Orchestrator) executeToolCalls(ctx context.Context, toolCalls []ToolCallContent, state *AgentState) ([]AgentMessage, []AgentMessage) {
	results := make([]AgentMessage, 0, len(toolCalls))

	logger.Info("=== Execute Tool Calls Start ===",
		zap.Int("count", len(toolCalls)))
	for _, tc := range toolCalls {
		logger.Info("Tool call start",
			zap.String("tool_id", tc.ID),
			zap.String("tool_name", tc.Name),
			zap.Any("arguments", tc.Arguments))

		// Emit tool execution start
		o.emit(NewEvent(EventToolExecutionStart).WithToolExecution(tc.ID, tc.Name, tc.Arguments))

		// Find tool
		var tool Tool
		for _, t := range state.Tools {
			if t.Name() == tc.Name {
				tool = t
				break
			}
		}

		var result ToolResult
		var err error

		if tool == nil {
			err = fmt.Errorf("tool %s not found", tc.Name)
			result = ToolResult{
				Content: []ContentBlock{TextContent{Text: fmt.Sprintf("Tool not found: %s", tc.Name)}},
				Details: map[string]any{"error": err.Error()},
			}
			logger.Error("Tool not found",
				zap.String("tool_name", tc.Name),
				zap.String("tool_id", tc.ID))
		} else {
			state.AddPendingTool(tc.ID)

			// Execute tool with streaming support
			result, err = tool.Execute(ctx, tc.Arguments, func(partial ToolResult) {
				// Emit update event
				o.emit(NewEvent(EventToolExecutionUpdate).
					WithToolExecution(tc.ID, tc.Name, tc.Arguments).
					WithToolResult(&partial, false))
			})

			state.RemovePendingTool(tc.ID)
		}

		// Log tool execution result
		if err != nil {
			logger.Error("Tool execution failed",
				zap.String("tool_id", tc.ID),
				zap.String("tool_name", tc.Name),
				zap.Any("arguments", tc.Arguments),
				zap.Error(err))
		} else {
			// Extract content for logging
			contentText := extractToolResultContent(result.Content)
			logger.Info("Tool execution success",
				zap.String("tool_id", tc.ID),
				zap.String("tool_name", tc.Name),
				zap.Any("arguments", tc.Arguments),
				zap.Int("result_length", len(contentText)),
				zap.String("result_preview", truncateString(contentText, 200)))
		}

		// Convert result to message
		resultMsg := AgentMessage{
			Role:      RoleToolResult,
			Content:   result.Content,
			Timestamp: time.Now().UnixMilli(),
			Metadata:  map[string]any{"tool_call_id": tc.ID, "tool_name": tc.Name},
		}

		if err != nil {
			resultMsg.Metadata["error"] = err.Error()
			result.Content = []ContentBlock{TextContent{Text: err.Error()}}
		}

		results = append(results, resultMsg)

		// Check for use_skill and update LoadedSkills
		if tc.Name == "use_skill" && err == nil {
			if skillName, ok := tc.Arguments["skill_name"].(string); ok && skillName != "" {
				// Add to LoadedSkills if not already present
				alreadyLoaded := false
				for _, loaded := range state.LoadedSkills {
					if loaded == skillName {
						alreadyLoaded = true
						break
					}
				}
				if !alreadyLoaded {
					state.LoadedSkills = append(state.LoadedSkills, skillName)
					logger.Info("=== Skill Loaded ===",
						zap.String("skill_name", skillName),
						zap.Int("total_loaded", len(state.LoadedSkills)),
						zap.Strings("loaded_skills", state.LoadedSkills))
				}
			}
		}

		// Emit tool execution end
		event := NewEvent(EventToolExecutionEnd).
			WithToolExecution(tc.ID, tc.Name, tc.Arguments).
			WithToolResult(&result, err != nil)
		o.emit(event)

		// Check for steering messages (interruption)
		steering := o.fetchSteeringMessages()
		if len(steering) > 0 {
			return results, steering
		}
	}

	logger.Info("=== Execute Tool Calls End ===",
		zap.Int("count", len(results)))
	return results, nil
}

// emit sends an event to the event channel
func (o *Orchestrator) emit(event *Event) {
	if o.eventChan != nil {
		o.eventChan <- event
	}
}

// emitErrorEnd emits an error end event
func (o *Orchestrator) emitErrorEnd(state *AgentState, err error) {
	event := NewEvent(EventTurnEnd).WithStopReason(err.Error())
	o.emit(event)
}

// fetchSteeringMessages gets steering messages from config
func (o *Orchestrator) fetchSteeringMessages() []AgentMessage {
	if o.config.GetSteeringMessages != nil {
		msgs, _ := o.config.GetSteeringMessages()
		return msgs
	}
	// Fall back to state queue
	return o.state.DequeueSteeringMessages()
}

// fetchFollowUpMessages gets follow-up messages from config
func (o *Orchestrator) fetchFollowUpMessages() []AgentMessage {
	if o.config.GetFollowUpMessages != nil {
		msgs, _ := o.config.GetFollowUpMessages()
		return msgs
	}
	// Fall back to state queue
	return o.state.DequeueFollowUpMessages()
}

// Stop stops the orchestrator
func (o *Orchestrator) Stop() {
	if o.cancelFunc != nil {
		o.cancelFunc()
	}
	if o.eventChan != nil {
		close(o.eventChan)
	}
}

// Subscribe returns the event channel
func (o *Orchestrator) Subscribe() <-chan *Event {
	return o.eventChan
}

// Helper functions

// convertToProviderMessages converts agent messages to provider messages
func convertToProviderMessages(messages []AgentMessage) []providers.Message {
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

		// Handle tool calls for assistant messages
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

	return result
}

// convertFromProviderResponse converts provider response to agent message
func convertFromProviderResponse(response *providers.Response) AgentMessage {
	content := []ContentBlock{TextContent{Text: response.Content}}

	// Handle tool calls
	for _, tc := range response.ToolCalls {
		content = append(content, ToolCallContent{
			ID:        tc.ID,
			Name:      tc.Name,
			Arguments: convertInterfaceToAny(tc.Params),
		})
	}

	return AgentMessage{
		Role:      RoleAssistant,
		Content:   content,
		Timestamp: time.Now().UnixMilli(),
		Metadata:  map[string]any{"stop_reason": response.FinishReason},
	}
}

// convertToToolDefinitions converts agent tools to provider tool definitions
func convertToToolDefinitions(tools []Tool) []providers.ToolDefinition {
	result := make([]providers.ToolDefinition, 0, len(tools))

	for _, tool := range tools {
		result = append(result, providers.ToolDefinition{
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters:  convertMapAnyToInterface(tool.Parameters()),
		})
	}

	return result
}

// extractToolCalls extracts tool calls from a message
func extractToolCalls(msg AgentMessage) []ToolCallContent {
	var toolCalls []ToolCallContent

	for _, block := range msg.Content {
		if tc, ok := block.(ToolCallContent); ok {
			toolCalls = append(toolCalls, tc)
		}
	}

	return toolCalls
}

// convertInterfaceToAny converts map[string]interface{} to map[string]any
func convertInterfaceToAny(m map[string]interface{}) map[string]any {
	result := make(map[string]any)
	for k, v := range m {
		result[k] = v
	}
	return result
}

// extractToolResultContent extracts text content from tool result
func extractToolResultContent(content []ContentBlock) string {
	var result strings.Builder
	for _, block := range content {
		if text, ok := block.(TextContent); ok {
			if result.Len() > 0 {
				result.WriteString("\n")
			}
			result.WriteString(text.Text)
		}
	}
	return result.String()
}

// truncateString truncates a string to a maximum length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen > 3 {
		return s[:maxLen-3] + "..."
	}
	return s[:maxLen]
}
