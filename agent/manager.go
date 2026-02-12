package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/smallnest/goclaw/agent/tools"
	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/internal/logger"
	"github.com/smallnest/goclaw/providers"
	"github.com/smallnest/goclaw/session"
	"go.uber.org/zap"
)

// AgentManager 管理多个 Agent 实例
type AgentManager struct {
	agents         map[string]*Agent              // agentID -> Agent
	bindings       map[string]*BindingEntry       // channel:accountID -> BindingEntry
	defaultAgent   *Agent                          // 默认 Agent
	bus            *bus.MessageBus
	sessionMgr     *session.Manager
	provider       providers.Provider
	tools          *ToolRegistry
	mu             sync.RWMutex
	cfg            *config.Config
	contextBuilder *ContextBuilder
	// 分身支持
	subagentRegistry *SubagentRegistry
	subagentAnnouncer *SubagentAnnouncer
	dataDir         string
}

// BindingEntry Agent 绑定条目
type BindingEntry struct {
	AgentID   string
	Channel   string
	AccountID string
	Agent     *Agent
}

// NewAgentManagerConfig AgentManager 配置
type NewAgentManagerConfig struct {
	Bus        *bus.MessageBus
	Provider   providers.Provider
	SessionMgr *session.Manager
	Tools      *ToolRegistry
	DataDir    string // 数据目录，用于存储分身注册表
}

// NewAgentManager 创建 Agent 管理器
func NewAgentManager(cfg *NewAgentManagerConfig) *AgentManager {
	// 创建分身注册表
	subagentRegistry := NewSubagentRegistry(cfg.DataDir)

	// 创建分身宣告器
	subagentAnnouncer := NewSubagentAnnouncer(nil) // 回调在 Start 中设置

	return &AgentManager{
		agents:           make(map[string]*Agent),
		bindings:         make(map[string]*BindingEntry),
		bus:              cfg.Bus,
		sessionMgr:       cfg.SessionMgr,
		provider:         cfg.Provider,
		tools:            cfg.Tools,
		subagentRegistry: subagentRegistry,
		subagentAnnouncer: subagentAnnouncer,
		dataDir:          cfg.DataDir,
	}
}

// handleSubagentCompletion 处理分身完成事件
func (m *AgentManager) handleSubagentCompletion(runID string, record *SubagentRunRecord) {
	logger.Info("Subagent completed",
		zap.String("run_id", runID),
		zap.String("task", record.Task))

	// 启动宣告流程
	if record.Outcome != nil {
		announceParams := &SubagentAnnounceParams{
			ChildSessionKey:     record.ChildSessionKey,
			ChildRunID:          record.RunID,
			RequesterSessionKey: record.RequesterSessionKey,
			RequesterOrigin:     record.RequesterOrigin,
			RequesterDisplayKey: record.RequesterDisplayKey,
			Task:                record.Task,
			Label:               record.Label,
			StartedAt:           record.StartedAt,
			EndedAt:             record.EndedAt,
			Outcome:             record.Outcome,
			Cleanup:             record.Cleanup,
			AnnounceType:        SubagentAnnounceTypeTask,
		}

		if err := m.subagentAnnouncer.RunAnnounceFlow(announceParams); err != nil {
			logger.Error("Failed to announce subagent result",
				zap.String("run_id", runID),
				zap.Error(err))
		}

		// 标记清理完成
		m.subagentRegistry.Cleanup(runID, record.Cleanup, true)
	}
}

// SetupFromConfig 从配置设置 Agent 和绑定
func (m *AgentManager) SetupFromConfig(cfg *config.Config, contextBuilder *ContextBuilder) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cfg = cfg
	m.contextBuilder = contextBuilder

	logger.Info("Setting up agents from config")

	// 1. 创建 Agent 实例
	for _, agentCfg := range cfg.Agents.List {
		if err := m.createAgent(agentCfg, contextBuilder, cfg); err != nil {
			logger.Error("Failed to create agent",
				zap.String("agent_id", agentCfg.ID),
				zap.Error(err))
			continue
		}
	}

	// 2. 如果没有配置 Agent，创建默认 Agent
	if len(m.agents) == 0 {
		logger.Info("No agents configured, creating default agent")
		defaultAgentCfg := config.AgentConfig{
			ID:        "default",
			Name:      "Default Agent",
			Default:   true,
			Model:     cfg.Agents.Defaults.Model,
			Workspace: cfg.Workspace.Path,
		}
		if err := m.createAgent(defaultAgentCfg, contextBuilder, cfg); err != nil {
			return fmt.Errorf("failed to create default agent: %w", err)
		}
	}

	// 3. 设置绑定
	for _, binding := range cfg.Bindings {
		if err := m.setupBinding(binding); err != nil {
			logger.Error("Failed to setup binding",
				zap.String("agent_id", binding.AgentID),
				zap.String("channel", binding.Match.Channel),
				zap.String("account_id", binding.Match.AccountID),
				zap.Error(err))
		}
	}

	// 4. 设置分身支持
	m.setupSubagentSupport(cfg, contextBuilder)

	logger.Info("Agent manager setup complete",
		zap.Int("agents", len(m.agents)),
		zap.Int("bindings", len(m.bindings)))

	return nil
}

// setupSubagentSupport 设置分身支持
func (m *AgentManager) setupSubagentSupport(cfg *config.Config, contextBuilder *ContextBuilder) {
	// 加载分身注册表
	if err := m.subagentRegistry.LoadFromDisk(); err != nil {
		logger.Warn("Failed to load subagent registry", zap.Error(err))
	}

	// 设置分身运行完成回调
	m.subagentRegistry.SetOnRunComplete(func(runID string, record *SubagentRunRecord) {
		m.handleSubagentCompletion(runID, record)
	})

	// 更新宣告器回调
	m.subagentAnnouncer = NewSubagentAnnouncer(func(sessionKey, message string) error {
		// 发送宣告消息到指定会话
		return m.sendToSession(sessionKey, message)
	})

	// 创建分身注册表适配器
	registryAdapter := &subagentRegistryAdapter{registry: m.subagentRegistry}

	// 注册 sessions_spawn 工具
	spawnTool := tools.NewSubagentSpawnTool(registryAdapter)
	spawnTool.SetAgentConfigGetter(func(agentID string) *config.AgentConfig {
		for _, agentCfg := range cfg.Agents.List {
			if agentCfg.ID == agentID {
				return &agentCfg
			}
		}
		return nil
	})
	spawnTool.SetDefaultConfigGetter(func() *config.AgentDefaults {
		return &cfg.Agents.Defaults
	})
	spawnTool.SetAgentIDGetter(func(sessionKey string) string {
		// 从会话密钥中解析 agent ID
		agentID, _, _ := ParseAgentSessionKey(sessionKey)
		if agentID == "" {
			// 尝试从绑定中查找
			for _, entry := range m.bindings {
				if entry.Agent != nil {
					return entry.AgentID
				}
			}
		}
		return agentID
	})
	spawnTool.SetOnSpawn(func(result *tools.SubagentSpawnResult) error {
		return m.handleSubagentSpawn(result)
	})

	// 注册工具
	if err := m.tools.RegisterExisting(spawnTool); err != nil {
		logger.Error("Failed to register sessions_spawn tool", zap.Error(err))
	}

	logger.Info("Subagent support configured")
}

// subagentRegistryAdapter 分身注册表适配器
type subagentRegistryAdapter struct {
	registry *SubagentRegistry
}

// RegisterRun 注册分身运行
func (a *subagentRegistryAdapter) RegisterRun(params *tools.SubagentRunParams) error {
	// 转换 RequesterOrigin
	var requesterOrigin *DeliveryContext
	if params.RequesterOrigin != nil {
		requesterOrigin = &DeliveryContext{
			Channel:   params.RequesterOrigin.Channel,
			AccountID: params.RequesterOrigin.AccountID,
			To:        params.RequesterOrigin.To,
			ThreadID:  params.RequesterOrigin.ThreadID,
		}
	}

	return a.registry.RegisterRun(&SubagentRunParams{
		RunID:                 params.RunID,
		ChildSessionKey:       params.ChildSessionKey,
		RequesterSessionKey:   params.RequesterSessionKey,
		RequesterOrigin:       requesterOrigin,
		RequesterDisplayKey:   params.RequesterDisplayKey,
		Task:                  params.Task,
		Cleanup:               params.Cleanup,
		Label:                 params.Label,
		ArchiveAfterMinutes:   params.ArchiveAfterMinutes,
	})
}

// handleSubagentSpawn 处理分身生成
func (m *AgentManager) handleSubagentSpawn(result *tools.SubagentSpawnResult) error {
	// 解析子会话密钥
	_, subagentID, isSubagent := ParseAgentSessionKey(result.ChildSessionKey)
	if !isSubagent {
		return fmt.Errorf("invalid subagent session key: %s", result.ChildSessionKey)
	}

	// TODO: 启动分身运行
	// 这里需要创建新的 Agent 实例来运行分身任务
	logger.Info("Subagent spawn handled",
		zap.String("run_id", result.RunID),
		zap.String("subagent_id", subagentID),
		zap.String("child_session_key", result.ChildSessionKey))

	return nil
}

// sendToSession 发送消息到指定会话
func (m *AgentManager) sendToSession(sessionKey, message string) error {
	// 解析会话密钥获取 agent ID
	agentID, _, _ := ParseAgentSessionKey(sessionKey)

	// 获取对应的 Agent
	agent, ok := m.GetAgent(agentID)
	if !ok {
		// 尝试使用默认 Agent
		agent = m.defaultAgent
	}

	if agent == nil {
		return fmt.Errorf("no agent found for session: %s", sessionKey)
	}

	// TODO: 实现将消息发送到 Agent 的逻辑
	// 这可能需要将消息注入到 Agent 的消息队列中

	logger.Info("Message sent to session",
		zap.String("session_key", sessionKey),
		zap.Int("message_length", len(message)))

	return nil
}

// createAgent 创建 Agent 实例
func (m *AgentManager) createAgent(cfg config.AgentConfig, contextBuilder *ContextBuilder, globalCfg *config.Config) error {
	// 获取 workspace 路径
	workspace := cfg.Workspace
	if workspace == "" {
		workspace = globalCfg.Workspace.Path
	}

	// 获取模型
	model := cfg.Model
	if model == "" {
		model = globalCfg.Agents.Defaults.Model
	}

	// 获取最大迭代次数
	maxIterations := globalCfg.Agents.Defaults.MaxIterations
	if maxIterations == 0 {
		maxIterations = 15
	}

	// 创建 Agent
	agent, err := NewAgent(&NewAgentConfig{
		Bus:          m.bus,
		Provider:     m.provider,
		SessionMgr:   m.sessionMgr,
		Tools:        m.tools,
		Context:      contextBuilder,
		Workspace:    workspace,
		MaxIteration: maxIterations,
	})
	if err != nil {
		return fmt.Errorf("failed to create agent %s: %w", cfg.ID, err)
	}

	// 设置系统提示词
	if cfg.SystemPrompt != "" {
		agent.SetSystemPrompt(cfg.SystemPrompt)
	}

	// 存储到管理器
	m.agents[cfg.ID] = agent

	// 如果是默认 Agent，设置默认
	if cfg.Default {
		m.defaultAgent = agent
	}

	logger.Info("Agent created",
		zap.String("agent_id", cfg.ID),
		zap.String("name", cfg.Name),
		zap.String("workspace", workspace),
		zap.String("model", model),
		zap.Bool("is_default", cfg.Default))

	return nil
}

// setupBinding 设置 Agent 绑定
func (m *AgentManager) setupBinding(binding config.BindingConfig) error {
	// 获取 Agent
	agent, ok := m.agents[binding.AgentID]
	if !ok {
		return fmt.Errorf("agent not found: %s", binding.AgentID)
	}

	// 构建绑定键
	bindingKey := fmt.Sprintf("%s:%s", binding.Match.Channel, binding.Match.AccountID)

	// 存储绑定
	m.bindings[bindingKey] = &BindingEntry{
		AgentID:   binding.AgentID,
		Channel:   binding.Match.Channel,
		AccountID: binding.Match.AccountID,
		Agent:     agent,
	}

	logger.Info("Binding setup",
		zap.String("binding_key", bindingKey),
		zap.String("agent_id", binding.AgentID))

	return nil
}

// RouteInbound 路由入站消息到对应的 Agent
func (m *AgentManager) RouteInbound(ctx context.Context, msg *bus.InboundMessage) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 构建绑定键
	bindingKey := fmt.Sprintf("%s:%s", msg.Channel, msg.AccountID)

	// 查找绑定的 Agent
	entry, ok := m.bindings[bindingKey]
	var agent *Agent
	if ok {
		agent = entry.Agent
		logger.Debug("Message routed by binding",
			zap.String("binding_key", bindingKey),
			zap.String("agent_id", entry.AgentID))
	} else if m.defaultAgent != nil {
		// 使用默认 Agent
		agent = m.defaultAgent
		logger.Debug("Message routed to default agent",
			zap.String("channel", msg.Channel),
			zap.String("account_id", msg.AccountID))
	} else {
		return fmt.Errorf("no agent found for message: %s", bindingKey)
	}

	// 处理消息
	return m.handleInboundMessage(ctx, msg, agent)
}

// handleInboundMessage 处理入站消息
func (m *AgentManager) handleInboundMessage(ctx context.Context, msg *bus.InboundMessage, agent *Agent) error {
	// 调用 Agent 处理消息（内部逻辑和 agent.go 中的 handleInboundMessage 类似）
	logger.Info("Processing inbound message",
		zap.String("channel", msg.Channel),
		zap.String("account_id", msg.AccountID),
		zap.String("chat_id", msg.ChatID))

	// 生成会话键（包含 account_id 以区分不同账号的消息）
	sessionKey := fmt.Sprintf("%s:%s:%s", msg.Channel, msg.AccountID, msg.ChatID)
	if msg.ChatID == "default" || msg.ChatID == "" {
		sessionKey = fmt.Sprintf("%s:%s:%d", msg.Channel, msg.AccountID, msg.Timestamp.Unix())
		logger.Info("Creating fresh session", zap.String("session_key", sessionKey))
	}

	// 获取或创建会话
	sess, err := m.sessionMgr.GetOrCreate(sessionKey)
	if err != nil {
		logger.Error("Failed to get session", zap.Error(err))
		return err
	}

	// 转换为 Agent 消息
	agentMsg := AgentMessage{
		Role:      RoleUser,
		Content:   []ContentBlock{TextContent{Text: msg.Content}},
		Timestamp: msg.Timestamp.UnixMilli(),
	}

	// 添加媒体内容
	for _, media := range msg.Media {
		if media.Type == "image" {
			agentMsg.Content = append(agentMsg.Content, ImageContent{
				URL:      media.URL,
				Data:     media.Base64,
				MimeType: media.MimeType,
			})
		}
	}

	// 获取 Agent 的 orchestrator
	orchestrator := agent.GetOrchestrator()

	// 加载历史消息并添加当前消息
	history := sess.GetHistory(-1) // -1 表示加载所有历史消息
	historyAgentMsgs := sessionMessagesToAgentMessages(history)
	allMessages := append(historyAgentMsgs, agentMsg)

	// 执行 Agent
	finalMessages, err := orchestrator.Run(ctx, allMessages)
	if err != nil {
		logger.Error("Agent execution failed", zap.Error(err))
		return err
	}

	// 更新会话（只保存新产生的消息）
	m.updateSession(sess, finalMessages, len(history))

	// 发布响应
	if len(finalMessages) > 0 {
		lastMsg := finalMessages[len(finalMessages)-1]
		if lastMsg.Role == RoleAssistant {
			m.publishToBus(ctx, msg.Channel, msg.ChatID, lastMsg)
		}
	}

	return nil
}

// updateSession 更新会话
func (m *AgentManager) updateSession(sess *session.Session, messages []AgentMessage, historyLen int) {
	// 只保存新产生的消息（不包括历史消息）
	newMessages := messages
	if historyLen >= 0 && len(messages) > historyLen {
		newMessages = messages[historyLen:]
	}

	for _, msg := range newMessages {
		sessMsg := session.Message{
			Role:      string(msg.Role),
			Content:   extractTextContent(msg),
			Timestamp: time.Unix(msg.Timestamp/1000, 0),
		}

		if msg.Role == RoleAssistant {
			for _, block := range msg.Content {
				if tc, ok := block.(ToolCallContent); ok {
					sessMsg.ToolCalls = []session.ToolCall{
						{ID: tc.ID, Name: tc.Name, Params: tc.Arguments},
					}
				}
			}
		}

		if msg.Role == RoleToolResult {
			if id, ok := msg.Metadata["tool_call_id"].(string); ok {
				sessMsg.ToolCallID = id
			}
		}

		sess.AddMessage(sessMsg)
	}

	if err := m.sessionMgr.Save(sess); err != nil {
		logger.Error("Failed to save session", zap.Error(err))
	}
}

// publishToBus 发布消息到总线
func (m *AgentManager) publishToBus(ctx context.Context, channel, chatID string, msg AgentMessage) {
	content := extractTextContent(msg)

	outbound := &bus.OutboundMessage{
		Channel:   channel,
		ChatID:    chatID,
		Content:   content,
		Timestamp: time.Unix(msg.Timestamp/1000, 0),
	}

	if err := m.bus.PublishOutbound(ctx, outbound); err != nil {
		logger.Error("Failed to publish outbound", zap.Error(err))
	}
}

// sessionMessagesToAgentMessages 将 session 消息转换为 Agent 消息
func sessionMessagesToAgentMessages(sessMsgs []session.Message) []AgentMessage {
	result := make([]AgentMessage, 0, len(sessMsgs))
	for _, sessMsg := range sessMsgs {
		agentMsg := AgentMessage{
			Role:      MessageRole(sessMsg.Role),
			Content:   []ContentBlock{TextContent{Text: sessMsg.Content}},
			Timestamp: sessMsg.Timestamp.UnixMilli(),
		}

		// Handle tool calls in assistant messages
		if sessMsg.Role == "assistant" && len(sessMsg.ToolCalls) > 0 {
			// Clear the text content if there are tool calls
			agentMsg.Content = []ContentBlock{}
			for _, tc := range sessMsg.ToolCalls {
				agentMsg.Content = append(agentMsg.Content, ToolCallContent{
					ID:        tc.ID,
					Name:      tc.Name,
					Arguments: tc.Params,
				})
			}
		}

		// Handle tool result messages
		if sessMsg.Role == "tool" {
			agentMsg.Role = RoleToolResult
			// Set tool_call_id in metadata
			if sessMsg.ToolCallID != "" {
				if agentMsg.Metadata == nil {
					agentMsg.Metadata = make(map[string]any)
				}
				agentMsg.Metadata["tool_call_id"] = sessMsg.ToolCallID
			}
		}

		result = append(result, agentMsg)
	}
	return result
}

// GetAgent 获取 Agent
func (m *AgentManager) GetAgent(agentID string) (*Agent, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agent, ok := m.agents[agentID]
	return agent, ok
}

// ListAgents 列出所有 Agent ID
func (m *AgentManager) ListAgents() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.agents))
	for id := range m.agents {
		ids = append(ids, id)
	}
	return ids
}

// Start 启动所有 Agent
func (m *AgentManager) Start(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for id, agent := range m.agents {
		if err := agent.Start(ctx); err != nil {
			logger.Error("Failed to start agent",
				zap.String("agent_id", id),
				zap.Error(err))
		} else {
			logger.Info("Agent started", zap.String("agent_id", id))
		}
	}

	// 启动消息处理器
	go m.processMessages(ctx)

	return nil
}

// Stop 停止所有 Agent
func (m *AgentManager) Stop() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for id, agent := range m.agents {
		if err := agent.Stop(); err != nil {
			logger.Error("Failed to stop agent",
				zap.String("agent_id", id),
				zap.Error(err))
		}
	}

	return nil
}

// processMessages 处理入站消息
func (m *AgentManager) processMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			logger.Info("Agent manager message processor stopped")
			return
		default:
			msg, err := m.bus.ConsumeInbound(ctx)
			if err != nil {
				if err == context.DeadlineExceeded || err == context.Canceled {
					continue
				}
				logger.Error("Failed to consume inbound", zap.Error(err))
				continue
			}

			if err := m.RouteInbound(ctx, msg); err != nil {
				logger.Error("Failed to route message",
					zap.String("channel", msg.Channel),
					zap.String("account_id", msg.AccountID),
					zap.Error(err))
			}
		}
	}
}

// GetDefaultAgent 获取默认 Agent
func (m *AgentManager) GetDefaultAgent() *Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaultAgent
}

// GetToolsInfo 获取工具信息
func (m *AgentManager) GetToolsInfo() (map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 从 tool registry 获取工具列表
	existingTools := m.tools.ListExisting()
	result := make(map[string]interface{})

	for _, tool := range existingTools {
		result[tool.Name()] = map[string]interface{}{
			"name":        tool.Name(),
			"description": tool.Description(),
			"parameters": tool.Parameters(),
		}
	}

	return result, nil
}
