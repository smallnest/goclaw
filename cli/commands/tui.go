package commands

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/smallnest/dogclaw/goclaw/agent"
	"github.com/smallnest/dogclaw/goclaw/agent/tools"
	"github.com/smallnest/dogclaw/goclaw/bus"
	"github.com/smallnest/dogclaw/goclaw/config"
	"github.com/smallnest/dogclaw/goclaw/internal/logger"
	"github.com/smallnest/dogclaw/goclaw/providers"
	"github.com/smallnest/dogclaw/goclaw/session"
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
	cmd.Flags().IntVar(&tuiTimeoutMs, "timeout-ms", 30000, "Timeout in milliseconds")
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
	defer logger.Sync()

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

	// Get or create session
	var sess *session.Session
	sessionKey := tuiSession
	if sessionKey == "" {
		sessionKey = "tui:" + strconv.FormatInt(time.Now().Unix(), 10)
	}

	sess, err = sessionMgr.GetOrCreate(sessionKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create session: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Session: %s\n", sessionKey)
	fmt.Printf("History limit: %d\n", tuiHistoryLimit)
	fmt.Printf("Timeout: %d ms\n", tuiTimeoutMs)
	fmt.Println()

	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

		response, err := runAgentIteration(msgCtx, sess, provider, contextBuilder, toolRegistry, skillsLoader, cfg.Agents.Defaults.MaxIterations)
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

	// Import the chat command registry for slash commands
	cmdRegistry := NewCommandRegistry()

	// Simple input loop
	fmt.Println("Enter your message (or /help for commands):")
	for {
		fmt.Print("➤ ")

		var input string
		fmt.Scanln(&input)

		if input == "" {
			continue
		}

		// Check for commands
		result, isCommand, shouldExit := cmdRegistry.Execute(input)
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
			Content: input,
		})

		// Run agent
		timeout := time.Duration(tuiTimeoutMs) * time.Millisecond
		msgCtx, msgCancel := context.WithTimeout(ctx, timeout)

		response, err := runAgentIteration(msgCtx, sess, provider, contextBuilder, toolRegistry, skillsLoader, cfg.Agents.Defaults.MaxIterations)
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
) (string, error) {
	iteration := 0
	var lastResponse string

	// Get loaded skills
	loadedSkills := getLoadedSkills(sess)

	for iteration < maxIterations {
		iteration++
		logger.Debug("Agent iteration",
			zap.Int("iteration", iteration),
			zap.Int("max_iterations", maxIterations))

		// Get available skills
		var skills []*agent.Skill
		if skillsLoader != nil {
			skills = skillsLoader.List()
		}

		// Build messages
		history := sess.GetHistory(tuiHistoryLimit)
		messages := contextBuilder.BuildMessages(history, "", skills, loadedSkills)
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
					result = fmt.Sprintf("Error: %v", err)
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
