package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/smallnest/goclaw/acp"
	acpruntime "github.com/smallnest/goclaw/acp/runtime"
	"github.com/smallnest/goclaw/agent/tools"
	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/channels"
	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/internal/logger"
	"github.com/smallnest/goclaw/providers"
	"github.com/smallnest/goclaw/session"
	"go.uber.org/zap"
)

var cronJobIDPattern = regexp.MustCompile(`\bjob-[a-zA-Z0-9]+\b`)
var cronListLinePattern = regexp.MustCompile(`^(job-[a-zA-Z0-9]+)\s+\((enabled|disabled)\)$`)

// AgentManager 管理多个 Agent 实例
type AgentManager struct {
	agents         map[string]*Agent        // agentID -> Agent
	bindings       map[string]*BindingEntry // channel:accountID -> BindingEntry
	defaultAgent   *Agent                   // 默认 Agent
	bus            *bus.MessageBus
	sessionMgr     *session.Manager
	provider       providers.Provider
	tools          *ToolRegistry
	mu             sync.RWMutex
	cfg            *config.Config
	contextBuilder *ContextBuilder
	skillsLoader   *SkillsLoader
	helper         *AgentHelper
	channelMgr     *channels.Manager
	acpManager     *acp.Manager
	manualCronMu   sync.Mutex
	manualCronLast map[string]time.Time
	// 分身支持
	subagentRegistry  *SubagentRegistry
	subagentAnnouncer *SubagentAnnouncer
	dataDir           string
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
	Bus            *bus.MessageBus
	Provider       providers.Provider
	SessionMgr     *session.Manager
	Tools          *ToolRegistry
	DataDir        string          // 数据目录，用于存储分身注册表
	ContextBuilder *ContextBuilder // 上下文构建器
	SkillsLoader   *SkillsLoader   // 技能加载器
	ChannelMgr     *channels.Manager
	AcpManager     *acp.Manager
}

// NewAgentManager 创建 Agent 管理器
func NewAgentManager(cfg *NewAgentManagerConfig) *AgentManager {
	// 创建分身注册表
	subagentRegistry := NewSubagentRegistry(cfg.DataDir)

	// 创建分身宣告器
	subagentAnnouncer := NewSubagentAnnouncer(nil) // 回调在 Start 中设置

	return &AgentManager{
		agents:            make(map[string]*Agent),
		bindings:          make(map[string]*BindingEntry),
		bus:               cfg.Bus,
		sessionMgr:        cfg.SessionMgr,
		provider:          cfg.Provider,
		tools:             cfg.Tools,
		subagentRegistry:  subagentRegistry,
		subagentAnnouncer: subagentAnnouncer,
		dataDir:           cfg.DataDir,
		contextBuilder:    cfg.ContextBuilder,
		skillsLoader:      cfg.SkillsLoader,
		helper:            NewAgentHelper(cfg.SessionMgr),
		channelMgr:        cfg.ChannelMgr,
		acpManager:        cfg.AcpManager,
		manualCronLast:    make(map[string]time.Time),
	}
}

// handleSubagentCompletion 处理分身完成事件
func (m *AgentManager) handleSubagentCompletion(runID string, record *SubagentRunRecord) {

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
		RunID:               params.RunID,
		ChildSessionKey:     params.ChildSessionKey,
		RequesterSessionKey: params.RequesterSessionKey,
		RequesterOrigin:     requesterOrigin,
		RequesterDisplayKey: params.RequesterDisplayKey,
		Task:                params.Task,
		Cleanup:             params.Cleanup,
		Label:               params.Label,
		ArchiveAfterMinutes: params.ArchiveAfterMinutes,
	})
}

// handleSubagentSpawn 处理分身生成
func (m *AgentManager) handleSubagentSpawn(result *tools.SubagentSpawnResult) error {
	// 解析子会话密钥
	agentID, subagentID, isSubagent := ParseAgentSessionKey(result.ChildSessionKey)
	if !isSubagent {
		return fmt.Errorf("invalid subagent session key: %s", result.ChildSessionKey)
	}

	// Get the agent to use for this subagent
	var agent *Agent
	if agentID != "" {
		var ok bool
		agent, ok = m.GetAgent(agentID)
		if !ok {
			agent = m.GetDefaultAgent()
		}
	} else {
		agent = m.GetDefaultAgent()
	}

	if agent == nil {
		return fmt.Errorf("no agent available for subagent: %s", result.ChildSessionKey)
	}

	// Set the system prompt if provided
	if result.ChildSystemPrompt != "" {
		// For subagent, we need to pass this through context
		// This will be used when the subagent processes messages
		logger.Debug("Subagent system prompt set",
			zap.String("run_id", result.RunID),
			zap.String("subagent_id", subagentID),
			zap.Int("prompt_length", len(result.ChildSystemPrompt)))
	}

	logger.Debug("Subagent spawn handled",
		zap.String("run_id", result.RunID),
		zap.String("subagent_id", subagentID),
		zap.String("child_session_key", result.ChildSessionKey))

	return nil
}

// sendToSession 发送消息到指定会话
func (m *AgentManager) sendToSession(sessionKey, message string) error {
	// Parse session key to get delivery context
	// Format: agent:<agentId>:subagent:<uuid> or agent:<agentId>:<sessionKey>
	parts := strings.Split(sessionKey, ":")
	if len(parts) < 3 {
		return fmt.Errorf("invalid session key format: %s", sessionKey)
	}

	// Extract channel and chat_id from session key
	// For now, we publish to CLI as default
	// In a real implementation, this should route to the appropriate channel
	logger.Debug("Message sent to session",
		zap.String("session_key", sessionKey),
		zap.Int("message_length", len(message)))

	// Publish the message as an outbound message
	// The message will be delivered to the user via the configured channel
	outbound := &bus.OutboundMessage{
		Channel:   "cli", // Default to CLI, could be extracted from session key
		ChatID:    sessionKey,
		Content:   message,
		Timestamp: time.Now(),
	}

	ctx := context.Background()
	if err := m.bus.PublishOutbound(ctx, outbound); err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

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

	// 获取最大历史消息数
	maxHistoryMessages := globalCfg.Agents.Defaults.MaxHistoryMessages
	if maxHistoryMessages == 0 {
		maxHistoryMessages = 100
	}

	// 创建 Agent
	agent, err := NewAgent(&NewAgentConfig{
		Bus:                m.bus,
		Provider:           m.provider,
		SessionMgr:         m.sessionMgr,
		Tools:              m.tools,
		Context:            contextBuilder,
		Workspace:          workspace,
		MaxIteration:       maxIterations,
		MaxHistoryMessages: maxHistoryMessages,
		SkillsLoader:       m.skillsLoader,
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
	logger.Info("[Manager] Processing inbound message",
		zap.String("message_id", msg.ID),
		zap.String("channel", msg.Channel),
		zap.String("account_id", msg.AccountID),
		zap.String("chat_id", msg.ChatID),
		zap.String("content", msg.Content),
	)

	if handled, err := m.handleAcpThreadBindingInbound(ctx, msg); handled {
		logger.Info("[Manager] Message handled by ACP thread binding", zap.String("message_id", msg.ID))
		return err
	}
	if handled, err := m.handleDirectCronOneShot(ctx, msg); handled {
		logger.Info("[Manager] Message handled by cron oneshot", zap.String("message_id", msg.ID))
		return err
	}

	// 调用 Agent 处理消息（内部逻辑和 agent.go 中的 handleInboundMessage 类似）
	logger.Debug("[Manager] Routing to agent",
		zap.String("channel", msg.Channel),
		zap.String("account_id", msg.AccountID),
		zap.String("chat_id", msg.ChatID))

	// 生成会话键（包含 account_id 以区分不同账号的消息）
	sessionKey := fmt.Sprintf("%s:%s:%s", msg.Channel, msg.AccountID, msg.ChatID)
	if msg.ChatID == "default" || msg.ChatID == "" {
		sessionKey = fmt.Sprintf("%s:%s:%d", msg.Channel, msg.AccountID, msg.Timestamp.Unix())
		logger.Debug("[Manager] Creating fresh session", zap.String("session_key", sessionKey))
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
	// 使用配置的最大历史消息数限制，避免 token 超限
	// 使用 GetHistorySafe 确保不会在工具调用中间截断消息
	maxHistory := m.cfg.Agents.Defaults.MaxHistoryMessages
	if maxHistory <= 0 {
		maxHistory = 100 // 默认值
	}
	history := sess.GetHistorySafe(maxHistory)
	historyAgentMsgs := sessionMessagesToAgentMessages(history)
	allMessages := append(historyAgentMsgs, agentMsg)

	// 执行 Agent
	logger.Info("[Manager] Starting agent execution",
		zap.String("message_id", msg.ID),
		zap.Int("history_count", len(history)),
		zap.Int("total_messages", len(allMessages)),
	)
	finalMessages, err := orchestrator.Run(ctx, allMessages)
	logger.Info("[Manager] Agent execution completed",
		zap.String("message_id", msg.ID),
		zap.Int("final_messages", len(finalMessages)),
		zap.Error(err),
	)
	if err != nil {
		// Check if error is related to tool_call_id mismatch (old session format)
		errStr := err.Error()
		if strings.Contains(errStr, "tool_call_id") && strings.Contains(errStr, "mismatch") {
			logger.Warn("Detected old session format, clearing session",
				zap.String("session_key", sessionKey),
				zap.Error(err))
			// Clear old session and retry
			if delErr := m.sessionMgr.Delete(sessionKey); delErr != nil {
				logger.Error("Failed to clear old session", zap.Error(delErr))
			} else {
				logger.Debug("Cleared old session, retrying with fresh session")
				// Get fresh session
				sess, getErr := m.sessionMgr.GetOrCreate(sessionKey)
				if getErr != nil {
					logger.Error("Failed to create fresh session", zap.Error(getErr))
					return getErr
				}
				// Retry with fresh session (no history)
				finalMessages, retryErr := orchestrator.Run(ctx, []AgentMessage{agentMsg})
				if retryErr != nil {
					logger.Error("Agent execution failed on retry", zap.Error(retryErr))
					return retryErr
				}
				// Update session with new messages
				m.updateSession(sess, finalMessages, 0)
				// Publish response
				if len(finalMessages) > 0 {
					lastMsg := finalMessages[len(finalMessages)-1]
					if lastMsg.Role == RoleAssistant {
						m.publishToBus(ctx, msg.Channel, msg.ChatID, lastMsg, msg.ID)
					}
				}
				return nil
			}
		}
		logger.Error("Agent execution failed", zap.Error(err))
		return err
	}

	// 更新会话（只保存新产生的消息）
	m.updateSession(sess, finalMessages, len(history))

	// 发布响应
	if len(finalMessages) > 0 {
		lastMsg := finalMessages[len(finalMessages)-1]
		if lastMsg.Role == RoleAssistant {
			m.publishToBus(ctx, msg.Channel, msg.ChatID, lastMsg, msg.ID)
		}
	}

	return nil
}

func (m *AgentManager) handleDirectCronOneShot(ctx context.Context, msg *bus.InboundMessage) (bool, error) {
	if msg == nil || m.tools == nil {
		return false, nil
	}

	content := strings.TrimSpace(msg.Content)
	if !isCronOneShotRequest(content) {
		return false, nil
	}

	jobID, err := m.resolveCronJobIDForOneShot(ctx, content)
	if err != nil {
		m.publishAcpThreadBindingError(ctx, msg, "已识别为一次性测试请求，但未找到可执行任务："+err.Error())
		return true, nil
	}
	if ok, wait := m.allowManualCronRun(jobID, time.Now()); !ok {
		m.publishAcpThreadBindingError(ctx, msg, fmt.Sprintf("任务 `%s` 刚刚手工触发过，请 %d 秒后再试。", jobID, wait))
		return true, nil
	}

	ack := AgentMessage{
		Role:      RoleAssistant,
		Content:   []ContentBlock{TextContent{Text: fmt.Sprintf("收到，开始手工执行一次任务 `%s`。", jobID)}},
		Timestamp: time.Now().UnixMilli(),
	}
	m.publishToBus(ctx, msg.Channel, msg.ChatID, ack, msg.ID)

	go func(channel, chatID, replyTo, id string) {
		runCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		_, runErr := m.tools.Execute(runCtx, "cron", map[string]interface{}{
			"command": fmt.Sprintf("run %s", id),
		})

		text := fmt.Sprintf("已手工执行一次任务 `%s`。", id)
		if runErr != nil {
			text = fmt.Sprintf("手工执行任务 `%s` 失败：%v", id, runErr)
		}

		done := AgentMessage{
			Role:      RoleAssistant,
			Content:   []ContentBlock{TextContent{Text: text}},
			Timestamp: time.Now().UnixMilli(),
		}
		m.publishToBus(context.Background(), channel, chatID, done, replyTo)
	}(msg.Channel, msg.ChatID, msg.ID, jobID)

	return true, nil
}

func (m *AgentManager) allowManualCronRun(jobID string, now time.Time) (bool, int) {
	const cooldown = 60 * time.Second
	m.manualCronMu.Lock()
	defer m.manualCronMu.Unlock()

	if last, ok := m.manualCronLast[jobID]; ok {
		if delta := now.Sub(last); delta < cooldown {
			wait := int((cooldown - delta).Round(time.Second).Seconds())
			if wait < 1 {
				wait = 1
			}
			return false, wait
		}
	}
	m.manualCronLast[jobID] = now
	return true, 0
}

func isCronOneShotRequest(text string) bool {
	if text == "" {
		return false
	}
	normalized := strings.ToLower(strings.TrimSpace(text))
	if strings.Contains(normalized, "cron run") {
		return true
	}
	keywords := []string{
		"执行一次定时任务",
		"只测试一次定时任务",
		"手工执行一次定时任务",
		"临时执行一次定时任务",
		"测试一次定时任务",
	}
	for _, kw := range keywords {
		if strings.Contains(normalized, kw) {
			return true
		}
	}
	return false
}

func (m *AgentManager) resolveCronJobIDForOneShot(ctx context.Context, text string) (string, error) {
	if id := cronJobIDPattern.FindString(text); id != "" {
		return id, nil
	}

	listOut, err := m.tools.Execute(ctx, "cron", map[string]interface{}{"command": "list"})
	if err != nil {
		return "", fmt.Errorf("获取任务列表失败: %w", err)
	}

	enabledIDs := extractEnabledCronJobIDs(listOut)
	switch len(enabledIDs) {
	case 0:
		return "", fmt.Errorf("没有启用中的任务")
	case 1:
		return enabledIDs[0], nil
	default:
		return "", fmt.Errorf("存在多个启用任务，请在消息中指定 job-id")
	}
}

func extractEnabledCronJobIDs(listOutput string) []string {
	lines := strings.Split(listOutput, "\n")
	ids := make([]string, 0)
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		matches := cronListLinePattern.FindStringSubmatch(line)
		if len(matches) != 3 {
			continue
		}
		if matches[2] == "enabled" {
			ids = append(ids, matches[1])
		}
	}
	return ids
}

func (m *AgentManager) resolveAcpThreadBindingSession(msg *bus.InboundMessage) string {
	if m.channelMgr == nil || m.acpManager == nil || msg == nil {
		return ""
	}
	return m.channelMgr.RouteToAcpSession(msg.Channel, msg.AccountID, msg.ChatID)
}

func (m *AgentManager) handleAcpThreadBindingInbound(ctx context.Context, msg *bus.InboundMessage) (bool, error) {
	sessionKey := m.resolveAcpThreadBindingSession(msg)
	if sessionKey == "" {
		return false, nil
	}

	go m.runAcpThreadBindingTurn(ctx, sessionKey, msg)
	return true, nil
}

func (m *AgentManager) runAcpThreadBindingTurn(ctx context.Context, sessionKey string, msg *bus.InboundMessage) {
	requestID := msg.ID
	if requestID == "" {
		requestID = uuid.NewString()
	}

	result, err := m.acpManager.RunTrackedTurn(ctx, acp.RunTrackedTurnInput{
		Cfg:        m.cfg,
		SessionKey: sessionKey,
		Text:       msg.Content,
		Mode:       acpruntime.AcpPromptModePrompt,
		RequestID:  requestID,
	})
	if err != nil {
		logger.Error("Failed to run ACP turn for thread binding",
			zap.String("session_key", sessionKey),
			zap.String("channel", msg.Channel),
			zap.String("account_id", msg.AccountID),
			zap.String("chat_id", msg.ChatID),
			zap.Error(err))
		m.publishAcpThreadBindingError(ctx, msg, "ACP session is currently unavailable. Please retry.")
		return
	}

	var response strings.Builder
	for event := range result.EventChan {
		switch e := event.(type) {
		case *acpruntime.AcpEventTextDelta:
			if e.Stream == "" || e.Stream == "output" {
				response.WriteString(e.Text)
			}
		case *acpruntime.AcpEventError:
			logger.Error("ACP turn failed for thread binding",
				zap.String("session_key", sessionKey),
				zap.String("channel", msg.Channel),
				zap.String("account_id", msg.AccountID),
				zap.String("chat_id", msg.ChatID),
				zap.String("error_message", e.Message))
			m.publishAcpThreadBindingError(ctx, msg, "ACP session failed to complete this request.")
			return
		}
	}

	reply := response.String()
	if strings.TrimSpace(reply) == "" {
		reply = "ACP task finished."
	}

	outbound := AgentMessage{
		Role:      RoleAssistant,
		Content:   []ContentBlock{TextContent{Text: reply}},
		Timestamp: time.Now().UnixMilli(),
	}
	m.publishToBus(ctx, msg.Channel, msg.ChatID, outbound, msg.ID)
}

func (m *AgentManager) publishAcpThreadBindingError(ctx context.Context, msg *bus.InboundMessage, text string) {
	if msg == nil || strings.TrimSpace(text) == "" {
		return
	}
	outbound := AgentMessage{
		Role:      RoleAssistant,
		Content:   []ContentBlock{TextContent{Text: text}},
		Timestamp: time.Now().UnixMilli(),
	}
	m.publishToBus(ctx, msg.Channel, msg.ChatID, outbound, msg.ID)
}

// updateSession 更新会话
func (m *AgentManager) updateSession(sess *session.Session, messages []AgentMessage, historyLen int) {
	// 只保存新产生的消息（不包括历史消息）
	newMessages := messages
	if historyLen >= 0 && len(messages) > historyLen {
		newMessages = messages[historyLen:]
	}

	_ = m.helper.UpdateSession(sess, newMessages, &UpdateSessionOptions{SaveImmediately: true})
}

// publishToBus 发布消息到总线
func (m *AgentManager) publishToBus(ctx context.Context, channel, chatID string, msg AgentMessage, replyTo string) {
	content := extractTextContent(msg)

	outbound := &bus.OutboundMessage{
		Channel:   channel,
		ChatID:    chatID,
		Content:   content,
		ReplyTo:   replyTo,
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
			// Restore tool_name from metadata if exists
			if toolName, ok := sessMsg.Metadata["tool_name"].(string); ok {
				if agentMsg.Metadata == nil {
					agentMsg.Metadata = make(map[string]any)
				}
				agentMsg.Metadata["tool_name"] = toolName
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

	for id := range m.agents {
		logger.Info("Agent registered under manager (inbound loop handled by AgentManager)",
			zap.String("agent_id", id))
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

			logger.Debug("[Manager] Consumed inbound message from bus",
				zap.String("message_id", msg.ID),
				zap.String("channel", msg.Channel),
				zap.String("chat_id", msg.ChatID),
			)
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
			"parameters":  tool.Parameters(),
		}
	}

	return result, nil
}
