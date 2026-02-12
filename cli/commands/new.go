package commands

import (
	"fmt"
	"time"
)

// Register 注册 new 命令
func Register(registry *CommandRegistry) {
	registry.Register(&Command{
		Name:        "new",
		Usage:       "/new [session_name]",
		Description: "Create a new chat session with optional name",
		Handler: func(args []string) (string, bool) {
			return handleNew(registry, args)
		},
	})
}

// handleNew 处理 new 命令
func handleNew(registry *CommandRegistry, args []string) (string, bool) {
	sessionMgr := registry.GetSessionManager()
	if sessionMgr == nil {
		return "Error: Session manager not available", true
	}

	var sessionName string
	if len(args) > 0 {
		sessionName = args[0]
	}

	// 如果没有指定名称，生成一个默认名称
	if sessionName == "" {
		sessionName = generateSessionName()
	}

	// 创建新会话
	sessionKey := "cli:" + sessionName
	_, err := sessionMgr.GetOrCreate(sessionKey)
	if err != nil {
		return fmt.Sprintf("Failed to create session: %v", err), true
	}

	return fmt.Sprintf("✅ Created new session: %s\nUse this session for future messages.", sessionName), false
}

// generateSessionName 生成会话名称
func generateSessionName() string {
	// 使用时间戳生成唯一名称
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	return fmt.Sprintf("session_%s", timestamp)
}
