package tools

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// SubagentManager 子代理管理器接口（避免循环导入）
type SubagentManager interface {
	Spawn(ctx context.Context, task, label, originChannel, originChatID string) (string, error)
}

// SpawnTool 子代理工具
type SpawnTool struct {
	subagentMgr SubagentManager
	currentChan string
	currentChat string
}

// NewSpawnTool 创建子代理工具
func NewSpawnTool(subagentMgr SubagentManager) *SpawnTool {
	return &SpawnTool{
		subagentMgr: subagentMgr,
	}
}

// SetCurrent 设置当前通道和聊天
func (t *SpawnTool) SetCurrent(channel, chatID string) {
	t.currentChan = channel
	t.currentChat = chatID
}

// Spawn 启动子代理
func (t *SpawnTool) Spawn(ctx context.Context, params map[string]interface{}) (string, error) {
	task, ok := params["task"].(string)
	if !ok {
		return "", fmt.Errorf("task parameter is required")
	}

	// 获取参数
	label, _ := params["label"].(string)
	if label == "" {
		label = uuid.New().String()[:8]
	}

	// 获取来源
	channel := t.currentChan
	if ch, ok := params["channel"].(string); ok && ch != "" {
		channel = ch
	}

	chatID := t.currentChat
	if cid, ok := params["chat_id"].(string); ok && cid != "" {
		chatID = cid
	}

	// 启动子代理
	taskID, err := t.subagentMgr.Spawn(ctx, task, label, channel, chatID)
	if err != nil {
		return "", fmt.Errorf("failed to spawn subagent: %w", err)
	}

	return fmt.Sprintf("Subagent spawned with task ID: %s", taskID), nil
}

// GetTools 获取所有子代理工具
func (t *SpawnTool) GetTools() []Tool {
	return []Tool{
		NewBaseTool(
			"spawn",
			"Spawn a subagent to execute a task in the background",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"task": map[string]interface{}{
						"type":        "string",
						"description": "Task description for the subagent",
					},
					"label": map[string]interface{}{
						"type":        "string",
						"description": "Optional label for the task",
					},
				},
				"required": []string{"task"},
			},
			t.Spawn,
		),
	}
}
