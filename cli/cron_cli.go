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
	"github.com/smallnest/goclaw/cron"
	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/internal/logger"
	"github.com/smallnest/goclaw/internal/workspace"
	"github.com/smallnest/goclaw/providers"
	"github.com/smallnest/goclaw/session"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var cronCmd = &cobra.Command{
	Use:   "cron",
	Short: "Scheduled jobs management",
}

var cronStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show scheduler status",
	Run:   runCronStatus,
}

var cronListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all jobs",
	Run:   runCronList,
}

var cronAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new scheduled job",
	Run:   runCronAdd,
}

var cronEditCmd = &cobra.Command{
	Use:   "edit <id>",
	Short: "Edit an existing job",
	Args:  cobra.ExactArgs(1),
	Run:   runCronEdit,
}

var cronRmCmd = &cobra.Command{
	Use:   "rm <id>",
	Short: "Delete a job",
	Args:  cobra.ExactArgs(1),
	Run:   runCronRm,
}

var cronEnableCmd = &cobra.Command{
	Use:   "enable <id>",
	Short: "Enable a job",
	Args:  cobra.ExactArgs(1),
	Run:   runCronEnable,
}

var cronDisableCmd = &cobra.Command{
	Use:   "disable <id>",
	Short: "Disable a job",
	Args:  cobra.ExactArgs(1),
	Run:   runCronDisable,
}

var cronRunsCmd = &cobra.Command{
	Use:   "runs",
	Short: "View job run history",
	Run:   runCronRuns,
}

var cronRunCmd = &cobra.Command{
	Use:   "run <id>",
	Short: "Run a job immediately",
	Args:  cobra.ExactArgs(1),
	Run:   runCronRun,
}

// Cron flags
var (
	cronStatusJSON bool
	cronListAll    bool
	cronListJSON   bool

	// add flags
	cronAddName        string
	cronAddAt          string
	cronAddEvery       string
	cronAddCron        string
	cronAddMessage     string
	cronAddSystemEvent string
	cronAddWebhook     string
	cronAddWebhookToken string
	cronAddBestEffort  bool
	cronAddIsolated    bool

	// edit flags
	cronEditName        string
	cronEditAt          string
	cronEditEvery       string
	cronEditCron        string
	cronEditMessage     string
	cronEditSystemEvent string
	cronEditEnable      bool
	cronEditDisable     bool

	// runs flags
	cronRunsID    string
	cronRunsLimit int
	cronRunsAfter string

	// run flags
	cronRunForce bool
)

func init() {
	rootCmd.AddCommand(cronCmd)
	cronCmd.AddCommand(cronStatusCmd)
	cronCmd.AddCommand(cronListCmd)
	cronCmd.AddCommand(cronAddCmd)
	cronCmd.AddCommand(cronEditCmd)
	cronCmd.AddCommand(cronRmCmd)
	cronCmd.AddCommand(cronEnableCmd)
	cronCmd.AddCommand(cronDisableCmd)
	cronCmd.AddCommand(cronRunsCmd)
	cronCmd.AddCommand(cronRunCmd)

	cronAddCmd.Aliases = []string{"create"}

	cronStatusCmd.Flags().BoolVar(&cronStatusJSON, "json", false, "Output in JSON format")

	cronListCmd.Flags().BoolVar(&cronListAll, "all", false, "Show all jobs including disabled")
	cronListCmd.Flags().BoolVar(&cronListJSON, "json", false, "Output in JSON format")

	cronAddCmd.Flags().StringVar(&cronAddName, "name", "", "Job name (required)")
	cronAddCmd.Flags().StringVar(&cronAddAt, "at", "", "Time to run (ISO 8601 format)")
	cronAddCmd.Flags().StringVar(&cronAddEvery, "every", "", "Interval (e.g., 30s, 5m, 2h, 1d)")
	cronAddCmd.Flags().StringVar(&cronAddCron, "cron", "", "Cron expression")
	cronAddCmd.Flags().StringVar(&cronAddMessage, "message", "", "Message to send")
	cronAddCmd.Flags().StringVar(&cronAddSystemEvent, "system-event", "", "System event type")
	cronAddCmd.Flags().StringVar(&cronAddWebhook, "webhook", "", "Webhook URL for delivery")
	cronAddCmd.Flags().StringVar(&cronAddWebhookToken, "webhook-token", "", "Webhook bearer token")
	cronAddCmd.Flags().BoolVar(&cronAddBestEffort, "best-effort", false, "Best-effort delivery")
	cronAddCmd.Flags().BoolVar(&cronAddIsolated, "isolated", false, "Run in isolated session")
	_ = cronAddCmd.MarkFlagRequired("name")

	cronRunsCmd.Flags().StringVar(&cronRunsID, "id", "", "Job ID (required)")
	cronRunsCmd.Flags().IntVar(&cronRunsLimit, "limit", 10, "Limit number of results")
	cronRunsCmd.Flags().StringVar(&cronRunsAfter, "after", "", "Show runs after this time")

	cronRunCmd.Flags().BoolVar(&cronRunForce, "force", false, "Run even if disabled")

	cronEditCmd.Flags().StringVar(&cronEditName, "name", "", "Job name")
	cronEditCmd.Flags().StringVar(&cronEditAt, "at", "", "Time to run (ISO 8601 format)")
	cronEditCmd.Flags().StringVar(&cronEditEvery, "every", "", "Interval (e.g., 30s, 5m, 2h, 1d)")
	cronEditCmd.Flags().StringVar(&cronEditCron, "cron", "", "Cron expression")
	cronEditCmd.Flags().StringVar(&cronEditMessage, "message", "", "Message to send")
	cronEditCmd.Flags().StringVar(&cronEditSystemEvent, "system-event", "", "System event type")
	cronEditCmd.Flags().BoolVar(&cronEditEnable, "enable", false, "Enable the job")
	cronEditCmd.Flags().BoolVar(&cronEditDisable, "disable", false, "Disable the job")
}

func runCronStatus(cmd *cobra.Command, args []string) {
	service, err := cron.NewService(cron.DefaultCronConfig(), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating cron service: %v\n", err)
		os.Exit(1)
	}

	status := service.GetStatus()

	if cronStatusJSON {
		data, _ := json.MarshalIndent(status, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Println("Cron Scheduler Status:")
	fmt.Printf("  Running: %v\n", status["running"])
	fmt.Printf("  Total Jobs: %v\n", status["total_jobs"])
	fmt.Printf("  Enabled: %v\n", status["enabled_jobs"])
	fmt.Printf("  Disabled: %v\n", status["disabled_jobs"])
	fmt.Printf("  Running Jobs: %v\n", status["running_jobs"])
}

func runCronList(cmd *cobra.Command, args []string) {
	service, err := cron.NewService(cron.DefaultCronConfig(), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating cron service: %v\n", err)
		os.Exit(1)
	}

	jobs := service.ListJobs()

	// Filter if not showing all
	if !cronListAll {
		filtered := make([]*cron.Job, 0, len(jobs))
		for _, job := range jobs {
			if job.State.Enabled {
				filtered = append(filtered, job)
			}
		}
		jobs = filtered
	}

	if cronListJSON {
		data, _ := json.MarshalIndent(jobs, "", "  ")
		fmt.Println(string(data))
		return
	}

	if len(jobs) == 0 {
		fmt.Println("No jobs found")
		return
	}

	fmt.Println("Scheduled Jobs:")
	for _, job := range jobs {
		status := "enabled"
		if !job.State.Enabled {
			status = "disabled"
		}
		fmt.Printf("\n  %s (%s)\n", job.ID, status)
		fmt.Printf("    Name: %s\n", job.Name)
		fmt.Printf("    Schedule: %s\n", formatSchedule(job))
		fmt.Printf("    Payload: %s\n", formatPayload(job))
		fmt.Printf("    Next Run: %s\n", formatTimePtr(job.State.NextRunAt))
		fmt.Printf("    Created: %s\n", job.CreatedAt.Format(time.RFC3339))
	}
}

func runCronAdd(cmd *cobra.Command, args []string) {
	service, err := cron.NewService(cron.DefaultCronConfig(), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating cron service: %v\n", err)
		os.Exit(1)
	}

	// Determine schedule
	schedule, err := parseScheduleFlags()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid schedule: %v\n", err)
		os.Exit(1)
	}

	// Determine payload
	payload, err := parsePayloadFlags()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid payload: %v\n", err)
		os.Exit(1)
	}

	// Determine delivery
	var delivery *cron.Delivery
	if cronAddWebhook != "" {
		delivery = &cron.Delivery{
			Mode:         cron.DeliveryModeWebhook,
			WebhookURL:   cronAddWebhook,
			WebhookToken: cronAddWebhookToken,
			BestEffort:   cronAddBestEffort,
		}
	}

	// Determine session target
	sessionTarget := cron.SessionTargetMain
	if cronAddIsolated {
		sessionTarget = cron.SessionTargetIsolated
	}

	job := &cron.Job{
		Name:          cronAddName,
		Schedule:      schedule,
		SessionTarget: sessionTarget,
		WakeMode:      cron.WakeModeNow,
		Payload:       payload,
		Delivery:      delivery,
		State: cron.JobState{
			Enabled: true,
		},
	}

	if err := service.AddJob(job); err != nil {
		fmt.Fprintf(os.Stderr, "Error adding job: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Job '%s' added with ID: %s\n", cronAddName, job.ID)
}

func runCronEdit(cmd *cobra.Command, args []string) {
	id := args[0]

	service, err := cron.NewService(cron.DefaultCronConfig(), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating cron service: %v\n", err)
		os.Exit(1)
	}

	// Check if job exists
	if _, err := service.GetJob(id); err != nil {
		fmt.Fprintf(os.Stderr, "Job not found: %s\n", id)
		os.Exit(1)
	}

	hasChanges := cronEditName != "" || cronEditAt != "" || cronEditEvery != "" ||
		cronEditCron != "" || cronEditMessage != "" || cronEditSystemEvent != "" ||
		cronEditEnable || cronEditDisable

	if !hasChanges {
		fmt.Fprintln(os.Stderr, "Error: No changes specified")
		os.Exit(1)
	}

	if cronEditEnable && cronEditDisable {
		fmt.Fprintln(os.Stderr, "Error: Cannot use --enable and --disable together")
		os.Exit(1)
	}

	// Apply updates
	err = service.UpdateJob(id, func(job *cron.Job) error {
		if cronEditName != "" {
			job.Name = cronEditName
		}

		if cronEditCron != "" {
			job.Schedule.Type = cron.ScheduleTypeCron
			job.Schedule.CronExpression = cronEditCron
		} else if cronEditEvery != "" {
			duration, err := cron.ParseHumanDuration(cronEditEvery)
			if err != nil {
				return fmt.Errorf("invalid interval: %w", err)
			}
			job.Schedule.Type = cron.ScheduleTypeEvery
			job.Schedule.EveryDuration = duration
		} else if cronEditAt != "" {
			t, err := time.Parse(time.RFC3339, cronEditAt)
			if err != nil {
				return fmt.Errorf("invalid time format: %w", err)
			}
			job.Schedule.Type = cron.ScheduleTypeAt
			job.Schedule.At = t
		}

		if cronEditMessage != "" {
			job.Payload.Type = cron.PayloadTypeAgentTurn
			job.Payload.Message = cronEditMessage
		}

		if cronEditSystemEvent != "" {
			job.Payload.Type = cron.PayloadTypeSystemEvent
			job.Payload.SystemEventType = cronEditSystemEvent
		}

		if cronEditEnable {
			job.State.Enabled = true
		}
		if cronEditDisable {
			job.State.Enabled = false
		}

		return nil
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error updating job: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Job '%s' updated successfully\n", id)
}

func runCronRm(cmd *cobra.Command, args []string) {
	id := args[0]

	service, err := cron.NewService(cron.DefaultCronConfig(), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating cron service: %v\n", err)
		os.Exit(1)
	}

	if err := service.RemoveJob(id); err != nil {
		fmt.Fprintf(os.Stderr, "Error removing job: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Job '%s' removed\n", id)
}

func runCronEnable(cmd *cobra.Command, args []string) {
	id := args[0]

	service, err := cron.NewService(cron.DefaultCronConfig(), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating cron service: %v\n", err)
		os.Exit(1)
	}

	if err := service.EnableJob(id); err != nil {
		fmt.Fprintf(os.Stderr, "Error enabling job: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Job '%s' enabled\n", id)
}

func runCronDisable(cmd *cobra.Command, args []string) {
	id := args[0]

	service, err := cron.NewService(cron.DefaultCronConfig(), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating cron service: %v\n", err)
		os.Exit(1)
	}

	if err := service.DisableJob(id); err != nil {
		fmt.Fprintf(os.Stderr, "Error disabling job: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Job '%s' disabled\n", id)
}

func runCronRuns(cmd *cobra.Command, args []string) {
	if cronRunsID == "" {
		fmt.Fprintln(os.Stderr, "Error: --id parameter is required")
		os.Exit(1)
	}

	service, err := cron.NewService(cron.DefaultCronConfig(), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating cron service: %v\n", err)
		os.Exit(1)
	}

	filter := cron.RunLogFilter{
		JobID: cronRunsID,
		Limit: cronRunsLimit,
	}

	if cronRunsAfter != "" {
		if t, err := time.Parse(time.RFC3339, cronRunsAfter); err == nil {
			filter.After = t
		}
	}

	runs, err := service.GetRunLogs(cronRunsID, filter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading run history: %v\n", err)
		os.Exit(1)
	}

	if len(runs) == 0 {
		fmt.Printf("No run history found for job '%s'\n", cronRunsID)
		return
	}

	fmt.Printf("Run History for Job '%s' (last %d runs):\n", cronRunsID, len(runs))
	for i, run := range runs {
		fmt.Printf("\n  %d. %s\n", i+1, run.StartedAt.Format(time.RFC3339))
		fmt.Printf("     Status: %s\n", run.Status)
		fmt.Printf("     Duration: %v\n", run.Duration)
		if run.Error != "" {
			fmt.Printf("     Error: %s\n", run.Error)
		}
	}
}

func runCronRun(cmd *cobra.Command, args []string) {
	id := args[0]

	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger if not already initialized
	if err := logger.Init("info", false); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = logger.Sync() }()

	// Validate configuration
	if err := config.Validate(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid configuration: %v\n", err)
		os.Exit(1)
	}

	// Get workspace directory
	workspaceDir, err := config.GetWorkspacePath(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get workspace path: %v\n", err)
		os.Exit(1)
	}

	// Create workspace manager
	workspaceMgr := workspace.NewManager(workspaceDir)
	if err := workspaceMgr.Ensure(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to ensure workspace: %v\n", err)
	}

	// Create message bus
	messageBus := bus.NewMessageBus(100)
	defer messageBus.Close()

	// Create session manager
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get home directory: %v\n", err)
		os.Exit(1)
	}
	sessionDir := homeDir + "/.goclaw/sessions"
	sessionMgr, err := session.NewManager(sessionDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create session manager: %v\n", err)
		os.Exit(1)
	}

	// Create memory store
	memoryStore := agent.NewMemoryStore(workspaceDir)

	// Create context builder
	contextBuilder := agent.NewContextBuilder(memoryStore, workspaceDir)

	// Create tool registry
	toolRegistry := agent.NewToolRegistry()

	// Create skills loader
	goclawDir := homeDir + "/.goclaw"
	globalSkillsDir := goclawDir + "/skills"
	workspaceSkillsDir := workspaceDir + "/skills"
	currentSkillsDir := "./skills"

	skillsLoader := agent.NewSkillsLoader(goclawDir, []string{
		globalSkillsDir,
		workspaceSkillsDir,
		currentSkillsDir,
	})
	if err := skillsLoader.Discover(); err != nil {
		logger.Warn("Failed to discover skills", zap.Error(err))
	}

	// Register filesystem tool
	fsTool := tools.NewFileSystemTool(cfg.Tools.FileSystem.AllowedPaths, cfg.Tools.FileSystem.DeniedPaths, workspaceDir)
	for _, tool := range fsTool.GetTools() {
		if err := toolRegistry.RegisterExisting(tool); err != nil {
			logger.Warn("Failed to register tool", zap.String("tool", tool.Name()))
		}
	}

	// Register use_skill tool
	if err := toolRegistry.RegisterExisting(tools.NewUseSkillTool()); err != nil {
		logger.Warn("Failed to register use_skill tool", zap.Error(err))
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
		if err := toolRegistry.RegisterExisting(tool); err != nil {
			logger.Warn("Failed to register tool", zap.String("tool", tool.Name()))
		}
	}

	// Register web tool
	webTool := tools.NewWebTool(
		cfg.Tools.Web.SearchAPIKey,
		cfg.Tools.Web.SearchEngine,
		cfg.Tools.Web.Timeout,
	)
	for _, tool := range webTool.GetTools() {
		if err := toolRegistry.RegisterExisting(tool); err != nil {
			logger.Warn("Failed to register tool", zap.String("tool", tool.Name()))
		}
	}

	// Register smart search tool
	browserTimeout := 30
	if cfg.Tools.Browser.Timeout > 0 {
		browserTimeout = cfg.Tools.Browser.Timeout
	}
	if err := toolRegistry.RegisterExisting(tools.NewSmartSearch(webTool, true, browserTimeout).GetTool()); err != nil {
		logger.Warn("Failed to register smart_search tool", zap.Error(err))
	}

	// Register browser tool if enabled
	if cfg.Tools.Browser.Enabled {
		browserTool := tools.NewBrowserTool(
			cfg.Tools.Browser.Headless,
			cfg.Tools.Browser.Timeout,
		)
		for _, tool := range browserTool.GetTools() {
			if err := toolRegistry.RegisterExisting(tool); err != nil {
				logger.Warn("Failed to register tool", zap.String("tool", tool.Name()))
			}
		}
	}

	// Register cron tool
	cronTool := tools.NewCronTool(cfg.Tools.Cron.Enabled, cfg.Tools.Cron.StorePath, messageBus)
	for _, tool := range cronTool.GetTools() {
		if err := toolRegistry.RegisterExisting(tool); err != nil {
			logger.Warn("Failed to register tool", zap.String("tool", tool.Name()))
		}
	}

	// Create LLM provider
	provider, err := providers.NewProvider(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create LLM provider: %v\n", err)
		os.Exit(1)
	}
	defer provider.Close()

	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create cron service with message bus
	cronService, err := cron.NewService(cron.DefaultCronConfig(), messageBus)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating cron service: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = cronService.Stop() }()

	// Get job
	job, err := cronService.GetJob(id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Job not found: %s\n", id)
		os.Exit(1)
	}

	if !job.State.Enabled && !cronRunForce {
		fmt.Printf("Job '%s' is disabled. Use --force to run anyway\n", id)
		return
	}

	fmt.Printf("Running job '%s' (%s)...\n", job.Name, job.ID)
	fmt.Printf("  Schedule: %s\n", formatSchedule(job))
	fmt.Printf("  Payload: %s\n", formatPayload(job))
	fmt.Println()

	// Create AgentManager for execution
	agentManager := agent.NewAgentManager(&agent.NewAgentManagerConfig{
		Bus:            messageBus,
		Provider:       provider,
		SessionMgr:     sessionMgr,
		Tools:          toolRegistry,
		DataDir:        workspaceDir,
		ContextBuilder: contextBuilder,
		SkillsLoader:   skillsLoader,
	})

	// Setup agents from config
	if err := agentManager.SetupFromConfig(cfg, contextBuilder); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup agent manager: %v\n", err)
		os.Exit(1)
	}

	// Start agent manager
	go func() {
		if err := agentManager.Start(ctx); err != nil {
			logger.Error("AgentManager error", zap.Error(err))
		}
	}()
	defer func() {
		if err := agentManager.Stop(); err != nil {
			logger.Error("Failed to stop agent manager", zap.Error(err))
		}
	}()

	// Execute the job
	if err := cronService.RunJob(ctx, id, cronRunForce); err != nil {
		fmt.Fprintf(os.Stderr, "Error running job: %v\n", err)
		os.Exit(1)
	}

	// Wait for the job to complete (give it some time to process)
	fmt.Println("Waiting for job to complete...")
	time.Sleep(5 * time.Second)

	fmt.Println("\nJob execution completed. Check run logs for details:")
	fmt.Printf("  ./goclaw cron runs %s\n", id)
}

// Helper functions

func parseScheduleFlags() (cron.Schedule, error) {
	var schedule cron.Schedule

	count := 0
	if cronAddAt != "" {
		count++
	}
	if cronAddEvery != "" {
		count++
	}
	if cronAddCron != "" {
		count++
	}

	if count == 0 {
		return schedule, fmt.Errorf("must specify one of --at, --every, or --cron")
	}
	if count > 1 {
		return schedule, fmt.Errorf("can only specify one of --at, --every, or --cron")
	}

	if cronAddAt != "" {
		t, err := time.Parse(time.RFC3339, cronAddAt)
		if err != nil {
			return schedule, fmt.Errorf("invalid time format: %w", err)
		}
		schedule.Type = cron.ScheduleTypeAt
		schedule.At = t
	}

	if cronAddEvery != "" {
		duration, err := cron.ParseHumanDuration(cronAddEvery)
		if err != nil {
			return schedule, fmt.Errorf("invalid interval: %w", err)
		}
		schedule.Type = cron.ScheduleTypeEvery
		schedule.EveryDuration = duration
	}

	if cronAddCron != "" {
		schedule.Type = cron.ScheduleTypeCron
		schedule.CronExpression = cronAddCron
	}

	return schedule, nil
}

func parsePayloadFlags() (cron.Payload, error) {
	var payload cron.Payload

	count := 0
	if cronAddMessage != "" {
		count++
	}
	if cronAddSystemEvent != "" {
		count++
	}

	if count == 0 {
		return payload, fmt.Errorf("must specify one of --message or --system-event")
	}
	if count > 1 {
		return payload, fmt.Errorf("can only specify one of --message or --system-event")
	}

	if cronAddMessage != "" {
		payload.Type = cron.PayloadTypeAgentTurn
		payload.Message = cronAddMessage
	}

	if cronAddSystemEvent != "" {
		payload.Type = cron.PayloadTypeSystemEvent
		payload.SystemEventType = cronAddSystemEvent
	}

	return payload, nil
}

func formatSchedule(job *cron.Job) string {
	switch job.Schedule.Type {
	case cron.ScheduleTypeAt:
		return "at " + job.Schedule.At.Format(time.RFC3339)
	case cron.ScheduleTypeEvery:
		return "every " + cron.FormatDuration(job.Schedule.EveryDuration)
	case cron.ScheduleTypeCron:
		return job.Schedule.CronExpression
	default:
		return "unknown"
	}
}

func formatPayload(job *cron.Job) string {
	switch job.Payload.Type {
	case cron.PayloadTypeAgentTurn:
		return "message: " + job.Payload.Message
	case cron.PayloadTypeSystemEvent:
		return "event: " + job.Payload.SystemEventType
	default:
		return "unknown"
	}
}

func formatTimePtr(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format(time.RFC3339)
}
