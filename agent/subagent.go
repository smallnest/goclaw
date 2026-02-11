package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/smallnest/dogclaw/goclaw/bus"
	"github.com/smallnest/dogclaw/goclaw/internal/logger"
	"go.uber.org/zap"
)

// Subagent 子代理
type Subagent struct {
	ID            string
	Task          string
	Label         string
	OriginChannel string
	OriginChatID  string
	Status        string
	CreatedAt     time.Time
}

// SubagentManager 子代理管理器
type SubagentManager struct {
	agents      map[string]*Subagent
	mu          sync.RWMutex
	loopFactory func(cfg *Config) (*Loop, error)
	baseCfg     *Config
}

// NewSubagentManager 创建子代理管理器
func NewSubagentManager() *SubagentManager {
	return &SubagentManager{
		agents: make(map[string]*Subagent),
	}
}

// Setup 设置管理器依赖
func (m *SubagentManager) Setup(baseCfg *Config, factory func(cfg *Config) (*Loop, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.baseCfg = baseCfg
	m.loopFactory = factory
}

// Spawn 启动子代理
func (m *SubagentManager) Spawn(ctx context.Context, task, label, originChannel, originChatID string) (string, error) {
	m.mu.Lock()
	factory := m.loopFactory
	baseCfg := m.baseCfg
	m.mu.Unlock()

	if factory == nil || baseCfg == nil {
		return "", fmt.Errorf("subagent manager not properly initialized")
	}

	id := uuid.New().String()
	agent := &Subagent{
		ID:            id,
		Task:          task,
		Label:         label,
		OriginChannel: originChannel,
		OriginChatID:  originChatID,
		Status:        "running",
		CreatedAt:     time.Now(),
	}

	m.mu.Lock()
	m.agents[id] = agent
	m.mu.Unlock()

	logger.Info("Spawned subagent",
		zap.String("id", id),
		zap.String("task", task),
		zap.String("label", label),
	)

	// 启动一个新的 Agent Loop
	go func() {
		// 为子代理创建独立的配置
		cfg := *baseCfg
		// 子代理使用独立的会话，或者在任务消息中包含上下文

		loop, err := factory(&cfg)
		if err != nil {
			logger.Error("Failed to create subagent loop", zap.Error(err))
			m.mu.Lock()
			agent.Status = "error"
			m.mu.Unlock()
			return
		}

		// 构建任务消息
		taskMsg := &bus.InboundMessage{
			Channel:  "system",
			SenderID: "system",
			ChatID:   id,
			Content:  task,
			Metadata: map[string]interface{}{
				"task_id":        id,
				"origin_channel": originChannel,
				"origin_chat_id": originChatID,
				"is_subagent":    true,
			},
			Timestamp: time.Now(),
		}

		// 发布到总线
		if err := cfg.Bus.PublishInbound(ctx, taskMsg); err != nil {
			logger.Error("Failed to publish subagent task", zap.Error(err))
			return
		}

		// 启动循环 (仅处理这条消息后退出，或者持续运行)
		// 注意：这里的 Start 可能需要调整以支持单次任务
		if err := loop.Start(ctx); err != nil {
			logger.Error("Subagent loop error", zap.Error(err))
		}

		m.mu.Lock()
		agent.Status = "completed"
		m.mu.Unlock()
	}()

	return id, nil
}

// Get 获取子代理
func (m *SubagentManager) Get(id string) (*Subagent, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	agent, ok := m.agents[id]
	return agent, ok
}

// List 列出子代理
func (m *SubagentManager) List() []*Subagent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]*Subagent, 0, len(m.agents))
	for _, agent := range m.agents {
		list = append(list, agent)
	}
	return list
}
