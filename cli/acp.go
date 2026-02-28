package cli

import (
	"context"
	"fmt"

	"github.com/smallnest/goclaw/acp"
	"github.com/smallnest/goclaw/acp/runtime"
	"github.com/smallnest/goclaw/config"
	"github.com/spf13/cobra"
)

func init() {
	// Register ACP command with root command
	// We use a placeholder that will load config when executed
	rootCmd.AddCommand(NewAcpCommand())
}

// NewAcpCommand creates the ACP CLI command group.
func NewAcpCommand() *cobra.Command {
	acpCmd := &cobra.Command{
		Use:   "acp",
		Short: "ACP (Agent Client Protocol) management commands",
		Long:  `Manage ACP sessions, backends, and configuration.`,
	}

	// Add subcommands
	acpCmd.AddCommand(newAcpDoctorCommand())
	acpCmd.AddCommand(newAcpListCommand())
	acpCmd.AddCommand(newAcpStatusCommand())
	acpCmd.AddCommand(newAcpCloseCommand())
	acpCmd.AddCommand(newAcpSpawnCommand())

	return acpCmd
}

// newAcpDoctorCommand creates the acp doctor command.
func newAcpDoctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check ACP backend health and configuration",
		Run: func(cmd *cobra.Command, args []string) {
			cfg, err := config.Load("")
			if err != nil {
				fmt.Printf("Failed to load config: %v\n", err)
				return
			}
			runAcpDoctor(cmd, cfg)
		},
	}
}

// runAcpDoctor runs the ACP health check.
func runAcpDoctor(cmd *cobra.Command, cfg *config.Config) {
	fmt.Println("ACP Backend Health Check")
	fmt.Println()

	// Check if ACP is enabled
	if !cfg.ACP.Enabled {
		fmt.Println("ACP is disabled in configuration")
		fmt.Println("Set acp.enabled = true in config.json to enable")
		return
	}
	fmt.Println("ACP is enabled")

	// Check backend configuration
	backend := cfg.ACP.Backend
	if backend == "" {
		backend = "acp-go-sdk" // default
	}
	fmt.Printf("Backend: %s\n", backend)

	// Get backend
	backendInstance := runtime.GetAcpRuntimeBackend(backend)
	if backendInstance == nil {
		fmt.Printf("Backend '%s' is not registered\n", backend)
		fmt.Println("Make sure the ACP backend is properly initialized")
		return
	}
	fmt.Println("Backend is registered")

	// Check backend health
	if backendInstance.Healthy != nil {
		if backendInstance.Healthy() {
			fmt.Println("Backend health check passed")
		} else {
			fmt.Println("Backend health check failed")
		}
	} else {
		fmt.Println("Backend does not provide health check")
	}

	// Check default agent
	defaultAgent := cfg.ACP.DefaultAgent
	if defaultAgent == "" {
		defaultAgent = "main"
	}
	fmt.Printf("Default agent: %s\n", defaultAgent)

	// Check agent authorization
	if len(cfg.ACP.AllowedAgents) > 0 {
		fmt.Println("Agent authorization: restricted")
		fmt.Printf("Allowed agents: %v\n", cfg.ACP.AllowedAgents)
	} else {
		fmt.Println("Agent authorization: all agents allowed")
	}

	// Check concurrent sessions limit
	maxSessions := cfg.ACP.MaxConcurrentSessions
	if maxSessions > 0 {
		fmt.Printf("Max concurrent sessions: %d\n", maxSessions)
	} else {
		fmt.Println("Max concurrent sessions: unlimited")
	}

	// Check idle timeout
	idleTimeout := cfg.ACP.IdleTimeoutMs
	if idleTimeout > 0 {
		fmt.Printf("Idle timeout: %d ms (%.1f min)\n", idleTimeout, float64(idleTimeout)/60000)
	} else {
		fmt.Println("Idle timeout: not configured")
	}

	// Run backend doctor if available
	// Try calling Doctor - if it returns an error about unsupported operation, skip it
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	report, err := backendInstance.Runtime.Doctor(ctx)
	if err != nil {
		// Check if this is just an unsupported operation warning
		acpErr, ok := err.(*runtime.AcpRuntimeError)
		if !ok || acpErr.Code != runtime.ErrCodeBackendUnsupportedControl {
			fmt.Printf("Doctor check failed: %v\n", err)
		}
		// Continue anyway even if Doctor is not supported
	} else {
		if report.Ok {
			fmt.Printf("OK: %s\n", report.Message)
		} else {
			fmt.Printf("ERROR [%s]: %s\n", report.Code, report.Message)
			if report.InstallCommand != "" {
				fmt.Printf("Install: %s\n", report.InstallCommand)
			}
		}

		if len(report.Details) > 0 {
			fmt.Println("Details:")
			for _, detail := range report.Details {
				fmt.Printf("  - %s\n", detail)
			}
		}
	}
}

// newAcpListCommand creates the acp list command.
func newAcpListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all active ACP sessions",
		Run: func(cmd *cobra.Command, args []string) {
			cfg, err := config.Load("")
			if err != nil {
				fmt.Printf("Failed to load config: %v\n", err)
				return
			}
			runAcpList(cfg)
		},
	}
}

// runAcpList lists all active ACP sessions.
func runAcpList(cfg *config.Config) {
	fmt.Println("Active ACP Sessions")
	fmt.Println()

	// Use the global manager to see the actual runtime state
	manager := acp.GetOrCreateGlobalManager(cfg)

	// Get observability snapshot
	snapshot := manager.GetObservabilitySnapshot()

	fmt.Printf("Active Sessions: %d\n", snapshot.RuntimeCache.ActiveSessions)
	fmt.Printf("Evicted Total: %d\n", snapshot.RuntimeCache.EvictedTotal)

	if snapshot.RuntimeCache.LastEvictedAt != nil {
		fmt.Printf("Last Evicted At: %d\n", *snapshot.RuntimeCache.LastEvictedAt)
	}

	fmt.Println()
	fmt.Println("Turn Statistics:")
	fmt.Printf("  Active: %d\n", snapshot.Turns.Active)
	fmt.Printf("  Queue Depth: %d\n", snapshot.Turns.QueueDepth)
	fmt.Printf("  Completed: %d\n", snapshot.Turns.Completed)
	fmt.Printf("  Failed: %d\n", snapshot.Turns.Failed)
	fmt.Printf("  Avg Latency: %d ms\n", snapshot.Turns.AverageLatencyMs)
	fmt.Printf("  Max Latency: %d ms\n", snapshot.Turns.MaxLatencyMs)

	if len(snapshot.ErrorsByCode) > 0 {
		fmt.Println()
		fmt.Println("Errors by Code:")
		for code, count := range snapshot.ErrorsByCode {
			fmt.Printf("  %s: %d\n", code, count)
		}
	}
}

// newAcpStatusCommand creates the acp status command.
func newAcpStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status <session-key>",
		Short: "Show detailed status of an ACP session",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cfg, err := config.Load("")
			if err != nil {
				fmt.Printf("Failed to load config: %v\n", err)
				return
			}
			runAcpStatus(cmd, cfg, args[0])
		},
	}
}

// runAcpStatus shows the status of an ACP session.
func runAcpStatus(cmd *cobra.Command, cfg *config.Config, sessionKey string) {
	fmt.Printf("ACP Session Status: %s\n", sessionKey)
	fmt.Println()

	// Use the global manager to see the actual runtime state
	manager := acp.GetOrCreateGlobalManager(cfg)

	// Get session status
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	status, err := manager.GetSessionStatus(ctx, acp.GetSessionStatusInput{
		Cfg:        cfg,
		SessionKey: sessionKey,
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Session Key: %s\n", status.SessionKey)
	fmt.Printf("Backend: %s\n", status.Backend)
	fmt.Printf("Agent: %s\n", status.Agent)
	fmt.Printf("State: %s\n", status.State)
	fmt.Printf("Mode: %s\n", status.Mode)

	if status.Identity != nil {
		fmt.Printf("Identity State: %s\n", status.Identity.State)
		fmt.Printf("Identity Source: %s\n", status.Identity.Source)
		if status.Identity.BackendSessionID != "" {
			fmt.Printf("Backend Session ID: %s\n", status.Identity.BackendSessionID)
		}
		if status.Identity.AgentSessionID != "" {
			fmt.Printf("Agent Session ID: %s\n", status.Identity.AgentSessionID)
		}
	}

	fmt.Printf("Last Activity: %d\n", status.LastActivityAt)

	if status.LastError != "" {
		fmt.Printf("Last Error: %s\n", status.LastError)
	}

	if status.RuntimeStatus != nil {
		fmt.Println()
		fmt.Println("Runtime Status:")
		if status.RuntimeStatus.Summary != "" {
			fmt.Printf("  Summary: %s\n", status.RuntimeStatus.Summary)
		}
		if len(status.RuntimeStatus.Details) > 0 {
			fmt.Println("  Details:")
			for k, v := range status.RuntimeStatus.Details {
				fmt.Printf("    %s: %v\n", k, v)
			}
		}
	}
}

// newAcpCloseCommand creates the acp close command.
func newAcpCloseCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "close <session-key>",
		Short: "Close an ACP session",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cfg, err := config.Load("")
			if err != nil {
				fmt.Printf("Failed to load config: %v\n", err)
				return
			}
			runAcpClose(cmd, cfg, args[0])
		},
	}
}

// runAcpClose closes an ACP session.
func runAcpClose(cmd *cobra.Command, cfg *config.Config, sessionKey string) {
	fmt.Printf("Closing ACP Session: %s\n", sessionKey)
	fmt.Println()

	// Use the global manager to close the actual runtime session
	manager := acp.GetOrCreateGlobalManager(cfg)

	// Close session
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	result, err := manager.CloseSession(ctx, acp.CloseSessionInput{
		Cfg:              cfg,
		SessionKey:       sessionKey,
		Reason:           "user_requested",
		RequireAcpSession: true,
		ClearMeta:        true,
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if result.RuntimeClosed {
		fmt.Println("Runtime closed successfully")
	} else {
		fmt.Println("Runtime not closed (may already be closed)")
	}

	if result.RuntimeNotice != "" {
		fmt.Printf("Notice: %s\n", result.RuntimeNotice)
	}

	if result.MetaCleared {
		fmt.Println("Metadata cleared")
	}
}

// newAcpSpawnCommand creates the acp spawn command.
func newAcpSpawnCommand() *cobra.Command {
	var task, label, agentID, cwd, mode string
	var thread bool

	cmd := &cobra.Command{
		Use:   "spawn",
		Short: "Spawn a new ACP session (CLI placeholder)",
		Run: func(cmd *cobra.Command, args []string) {
			cfg, err := config.Load("")
			if err != nil {
				fmt.Printf("Failed to load config: %v\n", err)
				return
			}
			runAcpSpawn(cfg, task, label, agentID, cwd, mode, thread)
		},
	}

	cmd.Flags().StringVar(&task, "task", "", "The coding task to execute")
	cmd.Flags().StringVar(&label, "label", "", "A descriptive label for the session")
	cmd.Flags().StringVar(&agentID, "agent", "", "Agent ID to use")
	cmd.Flags().StringVar(&cwd, "cwd", "", "Working directory")
	cmd.Flags().StringVar(&mode, "mode", "run", "Session mode (run or session)")
	cmd.Flags().BoolVar(&thread, "thread", false, "Bind to current thread")

	return cmd
}

// runAcpSpawn spawns a new ACP session.
func runAcpSpawn(cfg *config.Config, task, label, agentID, cwd, mode string, thread bool) {
	if task == "" {
		fmt.Println("Error: task parameter is required")
		return
	}

	fmt.Println("Spawning ACP Session")
	fmt.Println()
	fmt.Printf("Task: %s\n", task)
	fmt.Printf("Mode: %s\n", mode)
	fmt.Printf("Thread: %v\n", thread)
	fmt.Println()

	// Note: This is a placeholder implementation
	// In a real CLI usage, spawning would require more context
	// about the channel and conversation

	fmt.Println("ACP spawn via CLI requires additional context about the channel and conversation.")
	fmt.Println("Please use the spawn_acp tool from an agent or the gateway API instead.")
}

func init() {
	// ACP commands will be registered when rootCmd is available
	// This is handled in root.go
}
