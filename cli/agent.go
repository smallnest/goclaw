package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/smallnest/goclaw/agent"
	"github.com/smallnest/goclaw/agent/tools"
	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/internal/logger"
	"github.com/smallnest/goclaw/providers"
	"github.com/smallnest/goclaw/session"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Run one agent turn",
	Long:  `Execute a single agent interaction with a message and optional parameters.`,
	Run:   runAgent,
}

// Flags for agent command
var (
	agentMessage   string
	agentTo        string
	agentSessionID string
	agentThinking  bool
	agentVerbose   bool
	agentChannel   string
	agentLocal     bool
	agentDeliver   bool
	agentJSON      bool
	agentTimeout   int
)

func init() {
	agentCmd.Flags().StringVar(&agentMessage, "message", "", "Message to send to the agent (required)")
	agentCmd.Flags().StringVar(&agentTo, "to", "", "Target agent name")
	agentCmd.Flags().StringVar(&agentSessionID, "session-id", "", "Session ID to use")
	agentCmd.Flags().BoolVar(&agentThinking, "thinking", false, "Show thinking process")
	agentCmd.Flags().BoolVar(&agentVerbose, "verbose", false, "Enable verbose output")
	agentCmd.Flags().StringVar(&agentChannel, "channel", "cli", "Channel to use (cli, telegram, etc.)")
	agentCmd.Flags().BoolVar(&agentLocal, "local", false, "Run in local mode without connecting to channels")
	agentCmd.Flags().BoolVar(&agentDeliver, "deliver", false, "Deliver response through the channel")
	agentCmd.Flags().BoolVar(&agentJSON, "json", false, "Output in JSON format")
	agentCmd.Flags().IntVar(&agentTimeout, "timeout", 120, "Timeout in seconds")

	_ = agentCmd.MarkFlagRequired("message")
}

// runAgent executes a single agent turn
func runAgent(cmd *cobra.Command, args []string) {
	// Validate message
	if agentMessage == "" {
		fmt.Fprintf(os.Stderr, "Error: --message is required\n")
		os.Exit(1)
	}

	// Initialize logger if verbose or thinking mode is enabled
	if agentVerbose || agentThinking {
		if err := logger.Init("debug", false); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
			os.Exit(1)
		}
		defer func() { _ = logger.Sync() }()
	}

	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Create workspace
	workspace := os.Getenv("HOME") + "/.goclaw/workspace"
	if err := os.MkdirAll(workspace, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create workspace: %v\n", err)
		os.Exit(1)
	}

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
	if err := memoryStore.EnsureBootstrapFiles(); err != nil {
		if agentVerbose {
			fmt.Fprintf(os.Stderr, "Warning: Failed to create bootstrap files: %v\n", err)
		}
	}

	// Create context builder
	contextBuilder := agent.NewContextBuilder(memoryStore, workspace)

	// Create tool registry
	toolRegistry := agent.NewToolRegistry()

	// Register file system tool
	fsTool := tools.NewFileSystemTool(cfg.Tools.FileSystem.AllowedPaths, cfg.Tools.FileSystem.DeniedPaths, workspace)
	for _, tool := range fsTool.GetTools() {
		if err := toolRegistry.RegisterExisting(tool); err != nil && agentVerbose {
			fmt.Fprintf(os.Stderr, "Warning: Failed to register tool %s: %v\n", tool.Name(), err)
		}
	}

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
		if err := toolRegistry.RegisterExisting(tool); err != nil && agentVerbose {
			fmt.Fprintf(os.Stderr, "Warning: Failed to register tool %s: %v\n", tool.Name(), err)
		}
	}

	// Register web tool
	webTool := tools.NewWebTool(
		cfg.Tools.Web.SearchAPIKey,
		cfg.Tools.Web.SearchEngine,
		cfg.Tools.Web.Timeout,
	)
	for _, tool := range webTool.GetTools() {
		if err := toolRegistry.RegisterExisting(tool); err != nil && agentVerbose {
			fmt.Fprintf(os.Stderr, "Warning: Failed to register tool %s: %v\n", tool.Name(), err)
		}
	}

	// Register smart search tool
	browserTimeout := 30
	if cfg.Tools.Browser.Timeout > 0 {
		browserTimeout = cfg.Tools.Browser.Timeout
	}
	if err := toolRegistry.RegisterExisting(tools.NewSmartSearch(webTool, true, browserTimeout).GetTool()); err != nil && agentVerbose {
		fmt.Fprintf(os.Stderr, "Warning: Failed to register smart_search: %v\n", err)
	}

	// Register browser tool if enabled
	if cfg.Tools.Browser.Enabled {
		browserTool := tools.NewBrowserTool(
			cfg.Tools.Browser.Headless,
			cfg.Tools.Browser.Timeout,
		)
		for _, tool := range browserTool.GetTools() {
			if err := toolRegistry.RegisterExisting(tool); err != nil && agentVerbose {
				fmt.Fprintf(os.Stderr, "Warning: Failed to register browser tool %s: %v\n", tool.Name(), err)
			}
		}
	}

	// Register use_skill tool
	if err := toolRegistry.RegisterExisting(tools.NewUseSkillTool()); err != nil && agentVerbose {
		fmt.Fprintf(os.Stderr, "Warning: Failed to register use_skill: %v\n", err)
	}

	// Create skills loader
	skillsLoader := agent.NewSkillsLoader(workspace, []string{})
	if err := skillsLoader.Discover(); err != nil && agentVerbose {
		fmt.Fprintf(os.Stderr, "Warning: Failed to discover skills: %v\n", err)
	}

	// Create LLM provider
	provider, err := providers.NewProvider(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create LLM provider: %v\n", err)
		os.Exit(1)
	}
	defer provider.Close()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(agentTimeout)*time.Second)
	defer cancel()

	// Determine session key
	sessionKey := agentSessionID
	if sessionKey == "" {
		sessionKey = agentChannel + ":default"
	}

	// Get or create session
	sess, err := sessionMgr.GetOrCreate(sessionKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get session: %v\n", err)
		os.Exit(1)
	}

	// Add user message to session
	sess.AddMessage(session.Message{
		Role:      "user",
		Content:   agentMessage,
		Timestamp: time.Now(),
	})

	// Create new agent
	agentInstance, err := agent.NewAgent(&agent.NewAgentConfig{
		Bus:          messageBus,
		Provider:     provider,
		SessionMgr:   sessionMgr,
		Tools:        toolRegistry,
		Context:      contextBuilder,
		Workspace:    workspace,
		MaxIteration: cfg.Agents.Defaults.MaxIterations,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create agent: %v\n", err)
		os.Exit(1)
	}

	// Publish message to bus for processing
	inboundMsg := &bus.InboundMessage{
		Channel:   agentChannel,
		SenderID:  "cli",
		ChatID:    "default",
		Content:   agentMessage,
		Timestamp: time.Now(),
	}

	if err := messageBus.PublishInbound(ctx, inboundMsg); err != nil {
		if agentJSON {
			errorResult := map[string]interface{}{
				"error":   err.Error(),
				"success": false,
			}
			data, _ := json.MarshalIndent(errorResult, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Fprintf(os.Stderr, "Error publishing message: %v\n", err)
		}
		os.Exit(1)
	}

	// Start the agent to process the message
	go func() {
		if err := agentInstance.Start(ctx); err != nil && err != context.Canceled && err != context.DeadlineExceeded {
			logger.Error("Agent error", zap.Error(err))
		}
	}()

	// Consume outbound message
	outbound, err := messageBus.ConsumeOutbound(ctx)
	if err != nil {
		if agentJSON {
			errorResult := map[string]interface{}{
				"error":   err.Error(),
				"success": false,
			}
			data, _ := json.MarshalIndent(errorResult, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Fprintf(os.Stderr, "Error consuming response: %v\n", err)
		}
		os.Exit(1)
	}

	response := outbound.Content

	// Stop the agent
	if err := agentInstance.Stop(); err != nil && agentVerbose {
		fmt.Fprintf(os.Stderr, "Warning: Failed to stop agent: %v\n", err)
	}

	// Add assistant response to session
	sess.AddMessage(session.Message{
		Role:      "assistant",
		Content:   response,
		Timestamp: time.Now(),
	})

	// Save session
	if err := sessionMgr.Save(sess); err != nil && agentVerbose {
		fmt.Fprintf(os.Stderr, "Warning: Failed to save session: %v\n", err)
	}

	// Output response
	if agentJSON {
		result := map[string]interface{}{
			"response": response,
			"success":  true,
			"session":  sessionKey,
		}
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
	} else {
		if agentThinking {
			fmt.Println("\n💡 Response:")
		}
		fmt.Println(response)
	}

	// Deliver through channel if requested
	if agentDeliver && !agentLocal {
		if err := deliverResponse(ctx, messageBus, response); err != nil && agentVerbose {
			fmt.Fprintf(os.Stderr, "Warning: Failed to deliver response: %v\n", err)
		}
	}
}

// deliverResponse delivers the response through the configured channel
func deliverResponse(ctx context.Context, messageBus *bus.MessageBus, content string) error {
	return messageBus.PublishOutbound(ctx, &bus.OutboundMessage{
		Channel:   agentChannel,
		ChatID:    "default",
		Content:   content,
		Timestamp: time.Now(),
	})
}
