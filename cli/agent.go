package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
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
	agentMessage       string
	agentTo            string
	agentID            string
	agentSessionID     string
	agentThinking      string
	agentVerbose       bool
	agentChannel       string
	agentLocal         bool
	agentDeliver       bool
	agentJSON          bool
	agentTimeout       int
	agentMaxIterations int
	agentStream        bool
)

func init() {
	agentCmd.Flags().StringVarP(&agentMessage, "message", "m", "", "Message to send to the agent")
	agentCmd.Flags().StringVar(&agentTo, "to", "", "Recipient number in E.164 used to derive the session key")
	agentCmd.Flags().StringVar(&agentID, "agent", "", "Agent id (overrides routing bindings)")
	agentCmd.Flags().StringVar(&agentSessionID, "session-id", "", "Use an explicit session id")
	agentCmd.Flags().StringVar(&agentThinking, "thinking", "off", "Thinking level: off | minimal | low | medium | high")
	agentCmd.Flags().BoolVar(&agentVerbose, "verbose", false, "Persist agent verbose level for the session")
	agentCmd.Flags().StringVar(&agentChannel, "channel", "", "Delivery channel: last|telegram|whatsapp|discord|irc|googlechat|slack|signal|imessage|feishu|nostr|msteams|mattermost|nextcloud-talk|matrix|bluebubbles|line|zalo|wecom|zalouser|synology-chat|tlon")
	agentCmd.Flags().BoolVar(&agentLocal, "local", false, "Run the embedded agent locally (requires model provider API keys in your shell)")
	agentCmd.Flags().BoolVar(&agentDeliver, "deliver", false, "Send the agent's reply back to the selected channel")
	agentCmd.Flags().BoolVar(&agentJSON, "json", false, "Output result as JSON")
	agentCmd.Flags().IntVar(&agentTimeout, "timeout", 600, "Override agent command timeout (seconds)")
	agentCmd.Flags().IntVar(&agentMaxIterations, "max-iterations", 15, "Maximum agent loop iterations")
	agentCmd.Flags().BoolVar(&agentStream, "stream", false, "Enable streaming output")

	_ = agentCmd.MarkFlagRequired("message")
}

// runAgent executes a single agent turn
func runAgent(cmd *cobra.Command, args []string) {
	// Validate message
	if agentMessage == "" {
		fmt.Fprintf(os.Stderr, "Error: --message is required\n")
		os.Exit(1)
	}

	// Validate that either --agent or --session-id is specified
	if agentID == "" && agentSessionID == "" {
		fmt.Fprintf(os.Stderr, "Error: either --agent or --session-id is required\n")
		os.Exit(1)
	}

	// Initialize logger if verbose or thinking mode is enabled
	if agentVerbose || agentThinking != "off" {
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
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get home directory: %v\n", err)
		os.Exit(1)
	}
	workspace := homeDir + "/.goclaw/workspace"
	if err := os.MkdirAll(workspace, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create workspace: %v\n", err)
		os.Exit(1)
	}

	// Create message bus
	messageBus := bus.NewMessageBus(100)
	defer messageBus.Close()

	// Create session manager
	sessionDir := homeDir + "/.goclaw/sessions"
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
		cfg.Tools.Web.SearXNGEndpoint,
		cfg.Tools.Web.MaxResults,
		cfg.Tools.Web.Timeout,
	)
	for _, tool := range webTool.GetTools() {
		if err := toolRegistry.RegisterExisting(tool); err != nil && agentVerbose {
			fmt.Fprintf(os.Stderr, "Warning: Failed to register tool %s: %v\n", tool.Name(), err)
		}
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
	// 加载顺序（后加载的同名技能会覆盖前面的）：
	// 1. ./skills/ (当前目录，最高优先级)
	// 2. ${WORKSPACE}/skills/ (工作区目录)
	// 3. ~/.goclaw/skills/ (用户全局目录)
	var homeDirErr error
	homeDir, homeDirErr = os.UserHomeDir()
	if homeDirErr != nil && agentVerbose {
		fmt.Fprintf(os.Stderr, "Warning: Failed to get home directory: %v\n", homeDirErr)
		homeDir = os.Getenv("HOME")
	}
	goclawDir := homeDir + "/.goclaw"
	globalSkillsDir := goclawDir + "/skills"
	workspaceSkillsDir := workspace + "/skills"
	currentSkillsDir := "./skills"

	skillsLoader := agent.NewSkillsLoader(goclawDir, []string{
		globalSkillsDir,    // 最先加载（最低优先级）
		workspaceSkillsDir, // 其次加载
		currentSkillsDir,   // 最后加载（最高优先级）
	})
	if skillsErr := skillsLoader.Discover(); skillsErr != nil && agentVerbose {
		fmt.Fprintf(os.Stderr, "Warning: Failed to discover skills: %v\n", skillsErr)
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

	// Create new agent first
	agentInstance, err := agent.NewAgent(&agent.NewAgentConfig{
		Bus:          messageBus,
		Provider:     provider,
		SessionMgr:   sessionMgr,
		Tools:        toolRegistry,
		Context:      contextBuilder,
		Workspace:    workspace,
		MaxIteration: agentMaxIterations,
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

	// Subscribe to agent events for streaming output
	var eventChan <-chan *agent.Event
	if agentStream {
		eventChan = agentInstance.Subscribe()
		defer agentInstance.Unsubscribe(eventChan)
	}

	// Start the agent to process the message
	go func() {
		if err := agentInstance.Start(ctx); err != nil && err != context.Canceled && err != context.DeadlineExceeded {
			logger.Error("Agent error", zap.Error(err))
		}
	}()

	var response string

	// Handle streaming output
	if agentStream {
		response = handleStreamingOutput(ctx, eventChan, messageBus, agentThinking)
	} else {
		// Non-streaming: consume outbound message
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
		response = outbound.Content
	}

	// Stop the agent
	if err := agentInstance.Stop(); err != nil && agentVerbose {
		fmt.Fprintf(os.Stderr, "Warning: Failed to stop agent: %v\n", err)
	}

	// Note: Messages are already saved to session by Agent.handleInboundMessage

	// Output response (for streaming, output is already done)
	if !agentStream {
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
			if agentThinking != "off" {
				fmt.Println("\n💡 Response:")
			}
			fmt.Println(response)
		}
	}

	// Deliver through channel if requested
	if agentDeliver && !agentLocal {
		if err := deliverResponse(ctx, messageBus, response); err != nil && agentVerbose {
			fmt.Fprintf(os.Stderr, "Warning: Failed to deliver response: %v\n", err)
		}
	}
}

// handleStreamingOutput handles streaming output from agent events
func handleStreamingOutput(ctx context.Context, eventChan <-chan *agent.Event, messageBus *bus.MessageBus, thinkingLevel string) string {
	var fullContent, thinkingContent, finalContent strings.Builder
	inThinking := false
	inFinal := false

	for {
		select {
		case <-ctx.Done():
			fmt.Println() // Ensure newline on interrupt
			return fullContent.String()
		case event, ok := <-eventChan:
			if !ok {
				// Channel closed, output any remaining content
				if finalContent.Len() > 0 {
					fmt.Print(finalContent.String())
				}
				fmt.Println()
				return fullContent.String()
			}

			switch event.Type {
			case agent.EventStreamContent:
				content := event.StreamContent
				fullContent.WriteString(content)
				if !inThinking && !inFinal {
					fmt.Print(content)
				}

			case agent.EventStreamThinking:
				thinkingContent.WriteString(event.StreamContent)
				if thinkingLevel != "off" && !inThinking {
					inThinking = true
					fmt.Print("\n🤔 Thinking: ")
				}
				if thinkingLevel != "off" {
					fmt.Print(event.StreamContent)
				}

			case agent.EventStreamFinal:
				finalContent.WriteString(event.StreamContent)
				if !inFinal {
					inFinal = true
					if inThinking {
						fmt.Print("\n")
					}
					fmt.Print("\n📤 Final: ")
				}
				fmt.Print(event.StreamContent)

			case agent.EventStreamDone:
				// Stream complete, wait for final response
				inThinking = false
				inFinal = false

			case agent.EventToolExecutionStart:
				fmt.Printf("\n🔧 Tool: %s\n", event.ToolName)

			case agent.EventToolExecutionEnd:
				if event.ToolError {
					fmt.Printf("   ❌ Error\n")
				} else {
					fmt.Printf("   ✅ Done\n")
				}

			case agent.EventAgentEnd:
				// Agent finished, output newline and return
				fmt.Println()
				return fullContent.String()
			}
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
