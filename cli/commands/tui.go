package commands

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chzyer/readline"
	"github.com/smallnest/goclaw/agent"
	"github.com/smallnest/goclaw/agent/tools"
	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/cli/input"
	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/internal/logger"
	"github.com/smallnest/goclaw/providers"
	"github.com/smallnest/goclaw/session"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	tuiURL          string
	tuiToken        string
	tuiPassword     string
	tuiSession      string
	tuiDeliver      bool
	tuiThinking     bool
	tuiMessage      string
	tuiTimeoutMs    int
	tuiHistoryLimit int
)

// TUICommand returns the tui command
func TUICommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Open Terminal UI for goclaw",
		Long:  `Open an interactive terminal UI for interacting with goclaw agent.`,
		Run:   runTUI,
	}

	cmd.Flags().StringVar(&tuiURL, "url", "", "Gateway URL (default: ws://localhost:18789)")
	cmd.Flags().StringVar(&tuiToken, "token", "", "Authentication token")
	cmd.Flags().StringVar(&tuiPassword, "password", "", "Password for authentication")
	cmd.Flags().StringVar(&tuiSession, "session", "", "Session ID to resume")
	cmd.Flags().BoolVar(&tuiDeliver, "deliver", false, "Enable message delivery notifications")
	cmd.Flags().BoolVar(&tuiThinking, "thinking", false, "Show thinking indicator")
	cmd.Flags().StringVar(&tuiMessage, "message", "", "Send message on start")
	cmd.Flags().IntVar(&tuiTimeoutMs, "timeout-ms", 600000, "Timeout in milliseconds")
	cmd.Flags().IntVar(&tuiHistoryLimit, "history-limit", 50, "History limit")

	return cmd
}

// runTUI runs the terminal UI
func runTUI(cmd *cobra.Command, args []string) {
	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logLevel := "info"
	if tuiThinking {
		logLevel = "debug"
	}
	if err := logger.Init(logLevel, false); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync() // nolint:errcheck

	fmt.Println("🐾 goclaw Terminal UI")
	fmt.Println()

	// Create workspace
	workspace := os.Getenv("HOME") + "/.goclaw/workspace"

	// Create message bus
	messageBus := bus.NewMessageBus(100)
	defer messageBus.Close()

	// Create session manager
	sessionDir := os.Getenv("HOME") + "/.goclaw/sessions"
	sessionMgr, err := session.NewManager(sessionDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create session manager: %v\n", err)
		os.Exit(1)
	}

	// Create memory store
	memoryStore := agent.NewMemoryStore(workspace)
	_ = memoryStore.EnsureBootstrapFiles()

	// Create context builder
	contextBuilder := agent.NewContextBuilder(memoryStore, workspace)

	// Create tool registry
	toolRegistry := tools.NewRegistry()

	// Register file system tool
	fsTool := tools.NewFileSystemTool(cfg.Tools.FileSystem.AllowedPaths, cfg.Tools.FileSystem.DeniedPaths, workspace)
	for _, tool := range fsTool.GetTools() {
		_ = toolRegistry.Register(tool)
	}

	// Register use_skill tool
	_ = toolRegistry.Register(tools.NewUseSkillTool())

	// Register shell tool
	shellTool := tools.NewShellTool(
		cfg.Tools.Shell.Enabled,
		cfg.Tools.Shell.AllowedCmds,
		cfg.Tools.Shell.DeniedCmds,
		cfg.Tools.Shell.Timeout,
		cfg.Tools.Shell.WorkingDir,
		cfg.Tools.Shell.Sandbox,
	)
	for _, tool := range shellTool.GetTools() {
		_ = toolRegistry.Register(tool)
	}

	// Register web tool
	webTool := tools.NewWebTool(
		cfg.Tools.Web.SearchAPIKey,
		cfg.Tools.Web.SearchEngine,
		cfg.Tools.Web.Timeout,
	)
	for _, tool := range webTool.GetTools() {
		_ = toolRegistry.Register(tool)
	}

	// Register smart search
	browserTimeout := 30
	if cfg.Tools.Browser.Timeout > 0 {
		browserTimeout = cfg.Tools.Browser.Timeout
	}
	_ = toolRegistry.Register(tools.NewSmartSearch(webTool, true, browserTimeout).GetTool())

	// Register browser tool
	if cfg.Tools.Browser.Enabled {
		browserTool := tools.NewBrowserTool(
			cfg.Tools.Browser.Headless,
			cfg.Tools.Browser.Timeout,
		)
		for _, tool := range browserTool.GetTools() {
			_ = toolRegistry.Register(tool)
		}
	}

	// Create LLM provider
	provider, err := providers.NewProvider(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create LLM provider: %v\n", err)
		os.Exit(1)
	}
	defer provider.Close()

	// Create skills loader
	skillsLoader := agent.NewSkillsLoader(workspace, []string{})
	if err := skillsLoader.Discover(); err != nil {
		logger.Warn("Failed to discover skills", zap.Error(err))
	}

	// Always create a new session (unless explicitly specified)
	var sess *session.Session
	sessionKey := tuiSession
	if sessionKey == "" {
		// Always create a fresh session with timestamp
		sessionKey = "tui:" + strconv.FormatInt(time.Now().Unix(), 10)
	}

	sess, err = sessionMgr.GetOrCreate(sessionKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create session: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("New Session: %s\n", sessionKey)
	fmt.Printf("History limit: %d\n", tuiHistoryLimit)
	fmt.Printf("Timeout: %d ms\n", tuiTimeoutMs)
	fmt.Println()

	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create command registry for slash commands
	cmdRegistry := NewCommandRegistry()
	cmdRegistry.SetSessionManager(sessionMgr)

	// Handle message flag
	if tuiMessage != "" {
		fmt.Printf("Sending message: %s\n", tuiMessage)
		sess.AddMessage(session.Message{
			Role:    "user",
			Content: tuiMessage,
		})

		timeout := time.Duration(tuiTimeoutMs) * time.Millisecond
		msgCtx, msgCancel := context.WithTimeout(ctx, timeout)
		defer msgCancel()

		response, err := runAgentIteration(msgCtx, sess, provider, contextBuilder, toolRegistry, skillsLoader, cfg.Agents.Defaults.MaxIterations, cmdRegistry)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		} else {
			fmt.Println("\n" + response + "\n")
			sess.AddMessage(session.Message{
				Role:    "assistant",
				Content: response,
			})
			_ = sessionMgr.Save(sess)
		}

		if !tuiDeliver {
			return
		}
	}

	// Start interactive mode
	fmt.Println("Starting interactive TUI mode...")
	fmt.Println("Press Ctrl+C to exit")
	fmt.Println()
	fmt.Println("Arrow keys: ↑/↓ for history, ←/→ for edit")
	fmt.Println()

	// Create persistent readline instance for history navigation
	rl, err := input.NewReadline("➤ ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create readline: %v\n", err)
		os.Exit(1)
	}
	defer rl.Close()

	// Initialize history from session
	input.InitReadlineHistory(rl, getUserInputHistory(sess))

	// Input loop with persistent readline
	fmt.Println("Enter your message (or /help for commands):")
	for {
		line, err := rl.Readline()
		if err != nil {
			if err == readline.ErrInterrupt {
				fmt.Println("\nGoodbye!")
				break
			}
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			continue
		}

		// Save non-empty input to history
		if line != "" {
			_ = rl.SaveHistory(line)
		}

		if line == "" {
			continue
		}

		// Echo the input with prompt (readline doesn't automatically print after Enter)
		fmt.Printf("%s%s\n", "➤ ", line)

		// Check for commands
		result, isCommand, shouldExit := cmdRegistry.Execute(line)
		if isCommand {
			if shouldExit {
				fmt.Println("Goodbye!")
				break
			}
			if result != "" {
				fmt.Println(result)
			}
			continue
		}

		// Add user message
		sess.AddMessage(session.Message{
			Role:    "user",
			Content: line,
		})

		// Run agent
		timeout := time.Duration(tuiTimeoutMs) * time.Millisecond
		msgCtx, msgCancel := context.WithTimeout(ctx, timeout)

		response, err := runAgentIteration(msgCtx, sess, provider, contextBuilder, toolRegistry, skillsLoader, cfg.Agents.Defaults.MaxIterations, cmdRegistry)
		msgCancel()

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		} else {
			fmt.Println("\n" + response + "\n")
			sess.AddMessage(session.Message{
				Role:    "assistant",
				Content: response,
			})
			_ = sessionMgr.Save(sess)
		}

		// Force readline to refresh terminal state
		rl.Refresh()
	}
}

// runAgentIteration runs a single agent iteration (copied from chat.go)
func runAgentIteration(
	ctx context.Context,
	sess *session.Session,
	provider providers.Provider,
	contextBuilder *agent.ContextBuilder,
	toolRegistry *tools.Registry,
	skillsLoader *agent.SkillsLoader,
	maxIterations int,
	cmdRegistry *CommandRegistry,
) (string, error) {
	iteration := 0
	var lastResponse string

	// 重置停止标志
	if cmdRegistry != nil {
		cmdRegistry.ResetStop()
	}

	// 创建失败追踪器
	failureTracker := NewFailureTracker()

	// 获取可用的工具名称列表（用于错误提示）
	availableTools := getAvailableToolNames(toolRegistry)

	// Get loaded skills
	loadedSkills := getLoadedSkills(sess)

	for iteration < maxIterations {
		iteration++
		logger.Debug("Agent iteration",
			zap.Int("iteration", iteration),
			zap.Int("max_iterations", maxIterations))

		// 检查停止标志
		if cmdRegistry != nil && cmdRegistry.IsStopped() {
			logger.Info("Agent run stopped by /stop command")
			return lastResponse, nil
		}

		// Get available skills
		var skills []*agent.Skill
		if skillsLoader != nil {
			skills = skillsLoader.List()
		}

		// Build messages
		history := sess.GetHistory(tuiHistoryLimit)

		// 检查是否需要添加错误处理指导
		var errorGuidance string
		if shouldUseErrorGuidance(history) {
			failedTools := failureTracker.GetFailedToolNames()
			errorGuidance = "\n\n## 重要提示\n\n"
			errorGuidance += "检测到工具调用连续失败。请仔细分析错误原因，并尝试以下策略：\n"
			errorGuidance += "1. 检查失败的工具是否使用了正确的参数\n"
			errorGuidance += "2. 尝试使用其他可用的工具完成任务（参考上面的工具列表）\n"
			errorGuidance += "3. 如果所有工具都无法完成任务，向用户说明情况\n"
			if len(failedTools) > 0 {
				errorGuidance += fmt.Sprintf("\n**失败的工具**: %s\n", strings.Join(failedTools, ", "))
			}
			logger.Info("Added error guidance due to consecutive failures",
				zap.Strings("failed_tools", failedTools))
		}

		messages := contextBuilder.BuildMessages(history, errorGuidance, skills, loadedSkills)
		providerMessages := make([]providers.Message, len(messages))
		for i, msg := range messages {
			var tcs []providers.ToolCall
			for _, tc := range msg.ToolCalls {
				tcs = append(tcs, providers.ToolCall{
					ID:     tc.ID,
					Name:   tc.Name,
					Params: tc.Params,
				})
			}
			providerMessages[i] = providers.Message{
				Role:       msg.Role,
				Content:    msg.Content,
				ToolCallID: msg.ToolCallID,
				ToolCalls:  tcs,
			}
		}

		// Prepare tool definitions
		var toolDefs []providers.ToolDefinition
		if toolRegistry != nil {
			toolList := toolRegistry.List()
			for _, t := range toolList {
				toolDefs = append(toolDefs, providers.ToolDefinition{
					Name:        t.Name(),
					Description: t.Description(),
					Parameters:  t.Parameters(),
				})
			}
		}

		// Call LLM
		response, err := provider.Chat(ctx, providerMessages, toolDefs)
		if err != nil {
			return "", fmt.Errorf("LLM call failed: %w", err)
		}

		// Check for tool calls
		if len(response.ToolCalls) > 0 {
			logger.Debug("LLM returned tool calls",
				zap.Int("count", len(response.ToolCalls)),
				zap.Int("iteration", iteration))

			var assistantToolCalls []session.ToolCall
			for _, tc := range response.ToolCalls {
				assistantToolCalls = append(assistantToolCalls, session.ToolCall{
					ID:     tc.ID,
					Name:   tc.Name,
					Params: tc.Params,
				})
			}
			sess.AddMessage(session.Message{
				Role:      "assistant",
				Content:   response.Content,
				ToolCalls: assistantToolCalls,
			})

			// Execute tool calls
			hasNewSkill := false
			for _, tc := range response.ToolCalls {
				logger.Debug("Executing tool",
					zap.String("tool", tc.Name),
					zap.Int("iteration", iteration))

				fmt.Fprint(os.Stderr, ".")
				result, err := toolRegistry.Execute(ctx, tc.Name, tc.Params)
				fmt.Fprint(os.Stderr, "")

				if err != nil {
					logger.Error("Tool execution failed",
						zap.String("tool", tc.Name),
						zap.Error(err))
					failureTracker.RecordFailure(tc.Name)
					// 使用增强的错误格式化
					result = formatToolError(tc.Name, tc.Params, err, availableTools)
				} else {
					failureTracker.RecordSuccess(tc.Name)
				}

				// Check for use_skill
				if tc.Name == "use_skill" {
					hasNewSkill = true
					if skillName, ok := tc.Params["skill_name"].(string); ok {
						loadedSkills = append(loadedSkills, skillName)
						setLoadedSkills(sess, loadedSkills)
					}
				}

				sess.AddMessage(session.Message{
					Role:       "tool",
					Content:    result,
					ToolCallID: tc.ID,
					Metadata: map[string]interface{}{
						"tool_name": tc.Name,
					},
				})
			}

			if hasNewSkill {
				continue
			}
			continue
		}

		// No tool calls, return response
		lastResponse = response.Content
		break
	}

	if iteration >= maxIterations {
		logger.Warn("Agent reached max iterations",
			zap.Int("max", maxIterations))
	}

	return lastResponse, nil
}

// getLoadedSkills from session
func getLoadedSkills(sess *session.Session) []string {
	if sess.Metadata == nil {
		return []string{}
	}
	if v, ok := sess.Metadata["loaded_skills"].([]string); ok {
		return v
	}
	return []string{}
}

// setLoadedSkills in session
func setLoadedSkills(sess *session.Session, skills []string) {
	if sess.Metadata == nil {
		sess.Metadata = make(map[string]interface{})
	}
	sess.Metadata["loaded_skills"] = skills
}

// getUserInputHistory extracts user message history for readline
func getUserInputHistory(sess *session.Session) []string {
	history := sess.GetHistory(100)
	userInputs := make([]string, 0, len(history))

	// Extract only user messages (in reverse order - most recent first)
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "user" {
			userInputs = append(userInputs, history[i].Content)
		}
	}

	return userInputs
}

// findMostRecentTUISession finds the most recently updated tui session
func findMostRecentTUISession(mgr *session.Manager) string {
	keys, err := mgr.List()
	if err != nil {
		return ""
	}

	// Filter and collect tui sessions with their update time
	type sessionInfo struct {
		key       string
		updatedAt time.Time
	}

	var tuiSessions []sessionInfo
	for _, key := range keys {
		// Only consider sessions starting with "tui:" or "tui_"
		if !strings.HasPrefix(key, "tui:") && !strings.HasPrefix(key, "tui_") {
			continue
		}

		// Load the session to get its update time
		sess, err := mgr.GetOrCreate(key)
		if err != nil {
			continue
		}

		tuiSessions = append(tuiSessions, sessionInfo{
			key:       key,
			updatedAt: sess.UpdatedAt,
		})
	}

	// If no tui sessions found, return empty
	if len(tuiSessions) == 0 {
		return ""
	}

	// Sort by updated time (most recent first)
	sort.Slice(tuiSessions, func(i, j int) bool {
		return tuiSessions[i].updatedAt.After(tuiSessions[j].updatedAt)
	})

	return tuiSessions[0].key
}

// FailureTracker 追踪工具调用失败
type FailureTracker struct {
	toolFailures map[string]int // tool_name -> failure count
	totalCount   int
}

// NewFailureTracker 创建失败追踪器
func NewFailureTracker() *FailureTracker {
	return &FailureTracker{
		toolFailures: make(map[string]int),
		totalCount:   0,
	}
}

// RecordFailure 记录工具失败
func (ft *FailureTracker) RecordFailure(toolName string) {
	ft.toolFailures[toolName]++
	ft.totalCount++
	logger.Debug("Tool failure recorded",
		zap.String("tool", toolName),
		zap.Int("count", ft.toolFailures[toolName]),
		zap.Int("total", ft.totalCount))
}

// RecordSuccess 记录工具成功
func (ft *FailureTracker) RecordSuccess(toolName string) {
	// 同一工具成功后，可以重置其失败计数
	if count, ok := ft.toolFailures[toolName]; ok && count > 0 {
		ft.toolFailures[toolName] = 0
	}
}

// HasConsecutiveFailures 检查是否有连续失败
func (ft *FailureTracker) HasConsecutiveFailures(threshold int) bool {
	return ft.totalCount >= threshold
}

// GetFailedToolNames 获取失败的工具名称列表
func (ft *FailureTracker) GetFailedToolNames() []string {
	var names []string
	for name, count := range ft.toolFailures {
		if count > 0 {
			names = append(names, name)
		}
	}
	return names
}

// formatToolError 格式化工具错误，提供替代建议
func formatToolError(toolName string, params map[string]interface{}, err error, availableTools []string) string {
	errorMsg := err.Error()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## 工具执行失败: `%s`\n\n", toolName))
	sb.WriteString(fmt.Sprintf("**错误**: %s\n\n", errorMsg))

	// 提供降级建议
	var suggestions []string
	switch toolName {
	case "write_file":
		suggestions = []string{
			"1. **输出到控制台**: 直接将内容显示给用户",
			"2. **使用相对路径**: 尝试使用 `./filename`",
			"3. **使用完整路径**: 尝试使用绝对路径",
			"4. **检查权限**: 确认当前目录有写入权限",
		}
	case "read_file":
		suggestions = []string{
			"1. **检查路径**: 确认文件路径是否正确",
			"2. **列出目录**: 使用 `list_dir` 工具查看目录内容",
			"3. **使用相对路径**: 尝试使用 `./filename`",
		}
	case "smart_search", "web_search":
		suggestions = []string{
			"1. **简化查询**: 使用更简单的关键词",
			"2. **稍后重试**: 网络暂时不可用",
			"3. **告知用户**: 让用户自己搜索并提供结果",
		}
	case "browser":
		suggestions = []string{
			"1. **检查URL**: 确认URL格式正确",
			"2. **使用web_reader**: 尝试使用 web_reader 工具替代",
		}
	default:
		suggestions = []string{
			"1. **检查参数**: 确认工具参数是否正确",
			"2. **尝试替代方案**: 使用其他工具或方法",
		}
	}

	if len(suggestions) > 0 {
		sb.WriteString("**建议的替代方案**:\n\n")
		for _, s := range suggestions {
			sb.WriteString(fmt.Sprintf("%s\n", s))
		}
	}

	// 显示可用的替代工具
	if len(availableTools) > 0 {
		sb.WriteString("\n**可用的工具列表**:\n\n")
		for _, tool := range availableTools {
			if tool != toolName {
				sb.WriteString(fmt.Sprintf("- %s\n", tool))
			}
		}
	}

	return sb.String()
}

// shouldUseErrorGuidance 判断是否需要添加错误处理指导
func shouldUseErrorGuidance(history []session.Message) bool {
	// 检查最近的消息中是否有工具失败
	if len(history) == 0 {
		return false
	}

	consecutiveFailures := 0
	for i := len(history) - 1; i >= 0 && i >= len(history)-6; i-- {
		msg := history[i]
		if msg.Role == "tool" {
			if strings.Contains(msg.Content, "## 工具执行失败") ||
				strings.Contains(msg.Content, "Error:") {
				consecutiveFailures++
			} else {
				break // 遇到成功的工具调用就停止
			}
		}
	}

	return consecutiveFailures >= 2
}

// getAvailableToolNames 获取可用的工具名称列表
func getAvailableToolNames(toolRegistry *tools.Registry) []string {
	if toolRegistry == nil {
		return []string{}
	}

	tools := toolRegistry.List()
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Name())
	}
	return names
}
