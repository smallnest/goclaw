package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/chzyer/readline"
	"github.com/smallnest/dogclaw/goclaw/agent"
	"github.com/smallnest/dogclaw/goclaw/agent/tools"
	"github.com/smallnest/dogclaw/goclaw/bus"
	"github.com/smallnest/dogclaw/goclaw/cli/commands"
	"github.com/smallnest/dogclaw/goclaw/config"
	"github.com/smallnest/dogclaw/goclaw/internal/logger"
	"github.com/smallnest/dogclaw/goclaw/providers"
	"github.com/smallnest/dogclaw/goclaw/session"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Interactive chat mode",
	Run:   runChat,
}

var (
	chatDebugPrompt bool
	chatLogLevel    string
)

func init() {
	chatCmd.Flags().BoolVar(&chatDebugPrompt, "debug-prompt", false, "Print the full system prompt including injected skills")
	chatCmd.Flags().StringVar(&chatLogLevel, "log-level", "info", "Log level (debug, info, warn, error)")
}

// runChat 交互式聊天
func runChat(cmd *cobra.Command, args []string) {
	// 加载配置
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 初始化日志
	logLevel := chatLogLevel
	if logLevel == "" {
		logLevel = "info"
	}
	if err := logger.Init(logLevel, false); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	fmt.Println("🐾 goclaw Interactive Chat")
	fmt.Println()
	cmdRegistry := commands.NewCommandRegistry()
	fmt.Println(cmdRegistry.GetCommandPrompt())
	fmt.Println()

	// 创建工作区
	workspace := os.Getenv("HOME") + "/.goclaw/workspace"

	// 创建消息总线
	messageBus := bus.NewMessageBus(100)
	defer messageBus.Close()

	// 创建会话管理器
	sessionDir := os.Getenv("HOME") + "/.goclaw/sessions"
	sessionMgr, err := session.NewManager(sessionDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create session manager: %v\n", err)
		os.Exit(1)
	}

	// 创建记忆存储
	memoryStore := agent.NewMemoryStore(workspace)
	_ = memoryStore.EnsureBootstrapFiles()

	// 创建上下文构建器
	contextBuilder := agent.NewContextBuilder(memoryStore, workspace)

	// 创建工具注册表
	toolRegistry := tools.NewRegistry()

	// 创建技能加载器
	skillsLoader := agent.NewSkillsLoader(workspace, []string{})
	if err := skillsLoader.Discover(); err != nil {
		logger.Warn("Failed to discover skills", zap.Error(err))
	} else {
		skills := skillsLoader.List()
		if len(skills) > 0 {
			fmt.Printf("Loaded %d skills\n", len(skills))
		}
	}

	// 注册文件系统工具
	fsTool := tools.NewFileSystemTool(cfg.Tools.FileSystem.AllowedPaths, cfg.Tools.FileSystem.DeniedPaths)
	for _, tool := range fsTool.GetTools() {
		_ = toolRegistry.Register(tool)
	}

	// 注册 Shell 工具
	shellTool := tools.NewShellTool(
		cfg.Tools.Shell.Enabled,
		cfg.Tools.Shell.AllowedCmds,
		cfg.Tools.Shell.DeniedCmds,
		cfg.Tools.Shell.Timeout,
		cfg.Tools.Shell.WorkingDir,
	)
	for _, tool := range shellTool.GetTools() {
		_ = toolRegistry.Register(tool)
	}

	// 注册 Web 工具
	webTool := tools.NewWebTool(
		cfg.Tools.Web.SearchAPIKey,
		cfg.Tools.Web.SearchEngine,
		cfg.Tools.Web.Timeout,
	)
	for _, tool := range webTool.GetTools() {
		_ = toolRegistry.Register(tool)
	}

	// 注册浏览器工具（如果启用）
	if cfg.Tools.Browser.Enabled {
		browserTool := tools.NewBrowserTool(
			cfg.Tools.Browser.Headless,
			cfg.Tools.Browser.Timeout,
		)
		for _, tool := range browserTool.GetTools() {
			_ = toolRegistry.Register(tool)
		}
	}

	// 创建 LLM 提供商
	provider, err := providers.NewProvider(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create LLM provider: %v\n", err)
		os.Exit(1)
	}
	defer provider.Close()

	// 创建子代理管理器
	subagentMgr := agent.NewSubagentManager()
	_ = subagentMgr // 暂不使用，避免编译错误

	// 获取或创建会话
	const sessionKey = "cli:direct"
	sess, err := sessionMgr.GetOrCreate(sessionKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create session: %v\n", err)
		os.Exit(1)
	}

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 处理信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n\nGoodbye!")
		cancel()
		os.Exit(0)
	}()

	// 主循环 - 使用 readline 提供更好的输入体验
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          "➤ ",
		HistoryFile:     os.Getenv("HOME") + "/.goclaw/history",
		HistoryLimit:    1000,
		InterruptPrompt: "^C",
		EOFPrompt:       "^D",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create readline: %v\n", err)
		os.Exit(1)
	}
	defer rl.Close()

	// 如果开启 debug-prompt，打印完整的 system prompt
	if chatDebugPrompt {
		fmt.Println("=== Debug: System Prompt ===")
		skills := skillsLoader.List()
		systemPrompt := contextBuilder.BuildSystemPrompt(skills)
		fmt.Println(systemPrompt)
		fmt.Println("=== End of System Prompt ===")
	}

	for {
		// 读取输入
		input, err := rl.Readline()
		if err != nil {
			if err == readline.ErrInterrupt {
				if input == "" {
					// Ctrl-C on empty line - exit
					fmt.Println("\nGoodbye!")
					break
				}
				// Ctrl-C with input - clear input and continue
				continue
			}
			if err == io.EOF {
				fmt.Println("\nGoodbye!")
				break
			}
			fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			continue
		}

		input = strings.TrimSpace(input)

		// 检查是否是命令
		result, isCommand, shouldExit := cmdRegistry.Execute(input)
		if isCommand {
			if shouldExit {
				fmt.Println("Goodbye!")
				break
			}
			if result != "" {
				fmt.Println(result)
			}
			// 如果是 clear 命令，需要清空会话
			if input == "/clear" {
				sess.Clear()
				_ = sessionMgr.Save(sess)
			}
			continue
		}

		if input == "" {
			continue
		}

		// 添加用户消息
		sess.AddMessage(session.Message{
			Role:    "user",
			Content: input,
		})

		// 运行 Agent
		response, err := runAgentIteration(ctx, sess, provider, contextBuilder, toolRegistry, skillsLoader, cfg.Agents.Defaults.MaxIterations)
		if err != nil {
			fmt.Printf("Error: %v\n\n", err)
			continue
		}

		// 显示响应
		fmt.Printf("\n%s\n\n", response)

		// 添加助手响应
		sess.AddMessage(session.Message{
			Role:    "assistant",
			Content: response,
		})

		// 保存会话
		if err := sessionMgr.Save(sess); err != nil {
			logger.Error("Failed to save session", zap.Error(err))
		}
	}
}

// runAgentIteration 运行 Agent 迭代
func runAgentIteration(
	ctx context.Context,
	sess *session.Session,
	provider providers.Provider,
	contextBuilder *agent.ContextBuilder,
	toolRegistry *tools.Registry,
	skillsLoader *agent.SkillsLoader,
	maxIterations int,
) (string, error) {
	iteration := 0
	var lastResponse string

	for iteration < maxIterations {
		iteration++

		// 获取可用技能
		var skills []*agent.Skill
		if skillsLoader != nil {
			skills = skillsLoader.List()
		}

		// 构建消息
		history := sess.GetHistory(50)
		messages := contextBuilder.BuildMessages(history, "", skills)
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

		// 准备工具定义
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

		// 调用 LLM
		response, err := provider.Chat(ctx, providerMessages, toolDefs)
		if err != nil {
			return "", fmt.Errorf("LLM call failed: %w", err)
		}

		// 检查是否有工具调用
		if len(response.ToolCalls) > 0 {
			// 重要：必须先把带有工具调用的助手消息存入历史记录
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

			// 执行工具调用
			for _, tc := range response.ToolCalls {
				fmt.Printf("[Tool: %s]\n", tc.Name)
				result, err := toolRegistry.Execute(ctx, tc.Name, tc.Params)
				if err != nil {
					result = fmt.Sprintf("Error: %v", err)
				}
				fmt.Printf("[Result: %s]\n", truncateString(result, 200))

				// 添加工具结果到会话
				sess.AddMessage(session.Message{
					Role:       "tool",
					Content:    result,
					ToolCallID: tc.ID,
					Metadata: map[string]interface{}{
						"tool_name": tc.Name,
					},
				})
			}

			// 继续下一次迭代
			continue
		}

		// 没有工具调用，返回响应
		lastResponse = response.Content
		break
	}

	return lastResponse, nil
}

// truncateString 截断字符串
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
