package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/smallnest/dogclaw/goclaw/agent"
	"github.com/smallnest/dogclaw/goclaw/agent/tools"
	"github.com/smallnest/dogclaw/goclaw/bus"
	"github.com/smallnest/dogclaw/goclaw/channels"
	"github.com/smallnest/dogclaw/goclaw/config"
	"github.com/smallnest/dogclaw/goclaw/cron"
	"github.com/smallnest/dogclaw/goclaw/gateway"
	"github.com/smallnest/dogclaw/goclaw/internal/logger"
	"github.com/smallnest/dogclaw/goclaw/providers"
	"github.com/smallnest/dogclaw/goclaw/session"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var rootCmd = &cobra.Command{
	Use:   "goclaw",
	Short: "Go-based AI Agent framework",
	Long:  `goclaw is a Go language implementation of an AI Agent framework, inspired by nanobot.`,
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the goclaw agent",
	Run:   runStart,
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Run:   runConfigShow,
}

func init() {
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configShowCmd)
	rootCmd.AddCommand(chatCmd)
}

// Execute 执行 CLI
func Execute() error {
	return rootCmd.Execute()
}

// runStart 启动 Agent
func runStart(cmd *cobra.Command, args []string) {
	// 加载配置
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 初始化日志
	if err := logger.Init("info", false); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("Starting goclaw agent")

	// 验证配置
	if err := config.Validate(cfg); err != nil {
		logger.Fatal("Invalid configuration", zap.Error(err))
	}

	// 创建工作区
	workspace := os.Getenv("HOME") + "/.goclaw/workspace"

	// 创建消息总线
	messageBus := bus.NewMessageBus(100)
	defer messageBus.Close()

	// 创建会话管理器
	sessionDir := os.Getenv("HOME") + "/.goclaw/sessions"
	sessionMgr, err := session.NewManager(sessionDir)
	if err != nil {
		logger.Fatal("Failed to create session manager", zap.Error(err))
	}

	// 创建记忆存储
	memoryStore := agent.NewMemoryStore(workspace)
	if err := memoryStore.EnsureBootstrapFiles(); err != nil {
		logger.Warn("Failed to create bootstrap files", zap.Error(err))
	}

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
			logger.Info("Skills loaded", zap.Int("count", len(skills)))
		}
	}

	// 注册文件系统工具
	fsTool := tools.NewFileSystemTool(cfg.Tools.FileSystem.AllowedPaths, cfg.Tools.FileSystem.DeniedPaths)
	for _, tool := range fsTool.GetTools() {
		if err := toolRegistry.Register(tool); err != nil {
			logger.Warn("Failed to register tool", zap.String("tool", tool.Name()))
		}
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
		if err := toolRegistry.Register(tool); err != nil {
			logger.Warn("Failed to register tool", zap.String("tool", tool.Name()))
		}
	}

	// 注册 Web 工具
	webTool := tools.NewWebTool(
		cfg.Tools.Web.SearchAPIKey,
		cfg.Tools.Web.SearchEngine,
		cfg.Tools.Web.Timeout,
	)
	for _, tool := range webTool.GetTools() {
		if err := toolRegistry.Register(tool); err != nil {
			logger.Warn("Failed to register tool", zap.String("tool", tool.Name()))
		}
	}

	// 注册浏览器工具（如果启用）
	if cfg.Tools.Browser.Enabled {
		browserTool := tools.NewBrowserTool(
			cfg.Tools.Browser.Headless,
			cfg.Tools.Browser.Timeout,
		)
		for _, tool := range browserTool.GetTools() {
			if err := toolRegistry.Register(tool); err != nil {
				logger.Warn("Failed to register tool", zap.String("tool", tool.Name()))
			}
		}
		logger.Info("Browser tools registered")
	}

	// 创建 LLM 提供商
	provider, err := providers.NewProvider(cfg)
	if err != nil {
		logger.Fatal("Failed to create LLM provider", zap.Error(err))
	}
	defer provider.Close()

	// 创建子代理管理器
	subagentMgr := agent.NewSubagentManager()

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建通道管理器
	channelMgr := channels.NewManager(messageBus)
	if err := channelMgr.SetupFromConfig(cfg); err != nil {
		logger.Warn("Failed to setup channels from config", zap.Error(err))
	}

	// 创建网关服务器
	gatewayServer := gateway.NewServer(&cfg.Gateway, messageBus, channelMgr)
	if err := gatewayServer.Start(ctx); err != nil {
		logger.Warn("Failed to start gateway server", zap.Error(err))
	}
	defer gatewayServer.Stop()

	// 创建调度器
	scheduler := cron.NewScheduler(messageBus, provider, sessionMgr)

	// 创建 Agent 循环配置
	loopCfg := &agent.Config{
		Bus:          messageBus,
		Provider:     provider,
		SessionMgr:   sessionMgr,
		Memory:       memoryStore,
		Context:      contextBuilder,
		Tools:        toolRegistry,
		SkillsLoader: skillsLoader,
		Subagents:    subagentMgr,
		Workspace:    workspace,
		MaxIteration: cfg.Agents.Defaults.MaxIterations,
	}

	// 初始化子代理管理器
	subagentMgr.Setup(loopCfg, agent.NewLoop)

	// 创建 Agent 循环
	loop, err := agent.NewLoop(loopCfg)
	if err != nil {
		logger.Fatal("Failed to create agent loop", zap.Error(err))
	}

	// 处理信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 启动通道
	if err := channelMgr.Start(ctx); err != nil {
		logger.Error("Failed to start channels", zap.Error(err))
	}
	defer channelMgr.Stop()

	// 启动调度器
	if err := scheduler.Start(ctx); err != nil {
		logger.Error("Failed to start scheduler", zap.Error(err))
	}
	defer scheduler.Stop()

	// 启动出站消息分发
	go channelMgr.DispatchOutbound(ctx)

	// 启动 Agent
	go func() {
		if err := loop.Start(ctx); err != nil {
			logger.Error("Agent loop error", zap.Error(err))
		}
	}()

	// 等待信号
	<-sigChan
	logger.Info("Received shutdown signal")

	// 停止 Agent
	if err := loop.Stop(); err != nil {
		logger.Error("Failed to stop agent loop", zap.Error(err))
	}

	logger.Info("goclaw agent stopped")
}

// runConfigShow 显示配置
func runConfigShow(cmd *cobra.Command, args []string) {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Current Configuration:")
	fmt.Printf("  Model: %s\n", cfg.Agents.Defaults.Model)
	fmt.Printf("  Max Iterations: %d\n", cfg.Agents.Defaults.MaxIterations)
	fmt.Printf("  Temperature: %.1f\n", cfg.Agents.Defaults.Temperature)
}
