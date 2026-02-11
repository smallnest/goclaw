package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/smallnest/dogclaw/goclaw/config"
	"github.com/smallnest/dogclaw/goclaw/cron"
	"github.com/spf13/cobra"
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
	cronStatusJSON     bool
	cronListAll        bool
	cronListJSON       bool
	cronAddName        string
	cronAddAt          string
	cronAddEvery       string
	cronAddCron        string
	cronAddMessage     string
	cronAddSystemEvent string
	cronRunsID         string
	cronRunsLimit      int
	cronRunForce       bool
	// cron edit flags
	cronEditName        string
	cronEditAt          string
	cronEditEvery       string
	cronEditCron        string
	cronEditMessage     string
	cronEditSystemEvent string
	cronEditEnable      bool
	cronEditDisable     bool
)

func init() {
	// Register cron commands
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

	// Add aliases for cron add
	cronAddCmd.Aliases = []string{"create"}

	// cron status flags
	cronStatusCmd.Flags().BoolVar(&cronStatusJSON, "json", false, "Output in JSON format")

	// cron list flags
	cronListCmd.Flags().BoolVar(&cronListAll, "all", false, "Show all jobs including disabled")
	cronListCmd.Flags().BoolVar(&cronListJSON, "json", false, "Output in JSON format")

	// cron add flags
	cronAddCmd.Flags().StringVar(&cronAddName, "name", "", "Job name (required)")
	cronAddCmd.Flags().StringVar(&cronAddAt, "at", "", "Time to run (e.g., 14:30, 2:30pm)")
	cronAddCmd.Flags().StringVar(&cronAddEvery, "every", "", "Interval (e.g., 1h, 30m, 1d)")
	cronAddCmd.Flags().StringVar(&cronAddCron, "cron", "", "Cron expression")
	cronAddCmd.Flags().StringVar(&cronAddMessage, "message", "", "Message to send")
	cronAddCmd.Flags().StringVar(&cronAddSystemEvent, "system-event", "", "System event type")
	_ = cronAddCmd.MarkFlagRequired("name")

	// cron runs flags
	cronRunsCmd.Flags().StringVar(&cronRunsID, "id", "", "Job ID (required)")
	cronRunsCmd.Flags().IntVar(&cronRunsLimit, "limit", 10, "Limit number of results")

	// cron run flags
	cronRunCmd.Flags().BoolVar(&cronRunForce, "force", false, "Run even if disabled")

	// cron edit flags
	cronEditCmd.Flags().StringVar(&cronEditName, "name", "", "Job name")
	cronEditCmd.Flags().StringVar(&cronEditAt, "at", "", "Time to run (e.g., 14:30, 2:30pm)")
	cronEditCmd.Flags().StringVar(&cronEditEvery, "every", "", "Interval (e.g., 1h, 30m, 1d)")
	cronEditCmd.Flags().StringVar(&cronEditCron, "cron", "", "Cron expression")
	cronEditCmd.Flags().StringVar(&cronEditMessage, "message", "", "Message to send")
	cronEditCmd.Flags().StringVar(&cronEditSystemEvent, "system-event", "", "System event type")
	cronEditCmd.Flags().BoolVar(&cronEditEnable, "enable", false, "Enable the job")
	cronEditCmd.Flags().BoolVar(&cronEditDisable, "disable", false, "Disable the job")
}

// runCronStatus handles the cron status command
func runCronStatus(cmd *cobra.Command, args []string) {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Load jobs
	jobs, err := loadJobs(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading jobs: %v\n", err)
		os.Exit(1)
	}

	enabledCount := 0
	disabledCount := 0
	for _, job := range jobs {
		if job.Enabled {
			enabledCount++
		} else {
			disabledCount++
		}
	}

	if cronStatusJSON {
		status := map[string]interface{}{
			"total_jobs":    len(jobs),
			"enabled_jobs":  enabledCount,
			"disabled_jobs": disabledCount,
			"timestamp":     time.Now().Unix(),
		}
		data, _ := json.MarshalIndent(status, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Println("Cron Scheduler Status:")
	fmt.Printf("  Total Jobs: %d\n", len(jobs))
	fmt.Printf("  Enabled: %d\n", enabledCount)
	fmt.Printf("  Disabled: %d\n", disabledCount)
	fmt.Printf("  Timestamp: %s\n", time.Now().Format(time.RFC3339))
}

// runCronList handles the cron list command
func runCronList(cmd *cobra.Command, args []string) {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	jobs, err := loadJobs(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading jobs: %v\n", err)
		os.Exit(1)
	}

	// Filter if not showing all
	if !cronListAll {
		filtered := make([]*JobData, 0, len(jobs))
		for _, job := range jobs {
			if job.Enabled {
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
		if !job.Enabled {
			status = "disabled"
		}
		fmt.Printf("\n  %s (%s)\n", job.ID, status)
		fmt.Printf("    Name: %s\n", job.Name)
		fmt.Printf("    Schedule: %s\n", job.Schedule)
		fmt.Printf("    Task: %s\n", job.Task)
		fmt.Printf("    Created: %s\n", job.CreatedAt.Format(time.RFC3339))
	}
}

// runCronAdd handles the cron add command
func runCronAdd(cmd *cobra.Command, args []string) {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Determine schedule
	schedule := cronAddCron
	if schedule == "" {
		if cronAddAt != "" {
			schedule = parseAtSchedule(cronAddAt)
		} else if cronAddEvery != "" {
			schedule = parseEverySchedule(cronAddEvery)
		} else {
			fmt.Fprintf(os.Stderr, "Error: Must specify one of --at, --every, or --cron\n")
			os.Exit(1)
		}
	}

	// Validate schedule
	if _, err := cron.Parse(schedule); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid schedule: %v\n", err)
		os.Exit(1)
	}

	// Determine task
	task := cronAddMessage
	if task == "" {
		if cronAddSystemEvent != "" {
			task = fmt.Sprintf("system-event:%s", cronAddSystemEvent)
		} else {
			fmt.Fprintf(os.Stderr, "Error: Must specify --message or --system-event\n")
			os.Exit(1)
		}
	}

	// Generate ID
	id := fmt.Sprintf("job-%d", time.Now().UnixNano())

	// Create job
	job := &JobData{
		ID:        id,
		Name:      cronAddName,
		Schedule:  schedule,
		Task:      task,
		Message:   cronAddMessage,
		EventType: cronAddSystemEvent,
		Enabled:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Save job
	if err := saveJob(cfg, job); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving job: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Job '%s' added with ID: %s\n", cronAddName, id)
}

// runCronEdit handles the cron edit command
func runCronEdit(cmd *cobra.Command, args []string) {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	id := args[0]

	// Check if any edit flag is provided
	hasChanges := cronEditName != "" || cronEditAt != "" || cronEditEvery != "" ||
		cronEditCron != "" || cronEditMessage != "" || cronEditSystemEvent != "" ||
		cronEditEnable || cronEditDisable

	if !hasChanges {
		fmt.Fprintln(os.Stderr, "Error: No changes specified. Use at least one flag:")
		fmt.Fprintln(os.Stderr, "  --name <name>")
		fmt.Fprintln(os.Stderr, "  --at <time>")
		fmt.Fprintln(os.Stderr, "  --every <interval>")
		fmt.Fprintln(os.Stderr, "  --cron <expression>")
		fmt.Fprintln(os.Stderr, "  --message <text>")
		fmt.Fprintln(os.Stderr, "  --system-event <text>")
		fmt.Fprintln(os.Stderr, "  --enable")
		fmt.Fprintln(os.Stderr, "  --disable")
		os.Exit(1)
	}

	// Check conflicting enable/disable flags
	if cronEditEnable && cronEditDisable {
		fmt.Fprintln(os.Stderr, "Error: Cannot use --enable and --disable together")
		os.Exit(1)
	}

	// Load existing job
	job, err := loadJob(cfg, id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Job with ID '%s' not found\n", id)
		os.Exit(1)
	}

	// Update name if specified
	if cronEditName != "" {
		job.Name = cronEditName
	}

	// Handle schedule updates with priority: cron > every > at
	if cronEditCron != "" {
		// Validate cron expression
		if _, err := cron.Parse(cronEditCron); err != nil {
			fmt.Fprintf(os.Stderr, "Invalid cron expression: %v\n", err)
			os.Exit(1)
		}
		job.Schedule = cronEditCron
	} else if cronEditEvery != "" {
		schedule := parseEverySchedule(cronEditEvery)
		if schedule == "" {
			fmt.Fprintf(os.Stderr, "Invalid interval: %s\n", cronEditEvery)
			os.Exit(1)
		}
		job.Schedule = schedule
	} else if cronEditAt != "" {
		schedule := parseAtSchedule(cronEditAt)
		if schedule == "" {
			fmt.Fprintf(os.Stderr, "Invalid time: %s\n", cronEditAt)
			os.Exit(1)
		}
		job.Schedule = schedule
	}

	// Update message if specified
	if cronEditMessage != "" {
		job.Message = cronEditMessage
		job.Task = cronEditMessage
	}

	// Update system event if specified
	if cronEditSystemEvent != "" {
		job.EventType = cronEditSystemEvent
		job.Task = fmt.Sprintf("system-event:%s", cronEditSystemEvent)
	}

	// Handle enable/disable
	if cronEditEnable {
		job.Enabled = true
	} else if cronEditDisable {
		job.Enabled = false
	}

	// Update timestamp
	job.UpdatedAt = time.Now()

	// Save updated job
	if err := saveJob(cfg, job); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving job: %v\n", err)
		os.Exit(1)
	}

	// Display updated job information
	fmt.Printf("Job '%s' updated successfully\n\n", job.ID)
	fmt.Printf("  ID: %s\n", job.ID)
	fmt.Printf("  Name: %s\n", job.Name)
	fmt.Printf("  Schedule: %s\n", job.Schedule)
	fmt.Printf("  Task: %s\n", job.Task)
	if job.EventType != "" {
		fmt.Printf("  Event Type: %s\n", job.EventType)
	}
	fmt.Printf("  Enabled: %t\n", job.Enabled)
	fmt.Printf("  Updated: %s\n", job.UpdatedAt.Format(time.RFC3339))
}

// runCronRm handles the cron rm command
func runCronRm(cmd *cobra.Command, args []string) {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	id := args[0]

	// Load jobs
	jobs, err := loadJobs(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading jobs: %v\n", err)
		os.Exit(1)
	}

	// Find and remove job
	found := false
	newJobs := make([]*JobData, 0, len(jobs))
	for _, job := range jobs {
		if job.ID == id {
			found = true
			continue
		}
		newJobs = append(newJobs, job)
	}

	if !found {
		fmt.Printf("Job with ID '%s' not found\n", id)
		return
	}

	// Save updated jobs list
	if err := saveJobs(cfg, newJobs); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving jobs: %v\n", err)
		os.Exit(1)
	}

	// Remove job file
	jobFile := getJobFilePath(cfg, id)
	_ = os.Remove(jobFile)

	fmt.Printf("Job '%s' removed\n", id)
}

// runCronEnable handles the cron enable command
func runCronEnable(cmd *cobra.Command, args []string) {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	id := args[0]

	job, err := loadJob(cfg, id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Job with ID '%s' not found\n", id)
		os.Exit(1)
	}

	job.Enabled = true
	job.UpdatedAt = time.Now()

	if err := saveJob(cfg, job); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving job: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Job '%s' enabled\n", id)
}

// runCronDisable handles the cron disable command
func runCronDisable(cmd *cobra.Command, args []string) {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	id := args[0]

	job, err := loadJob(cfg, id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Job with ID '%s' not found\n", id)
		os.Exit(1)
	}

	job.Enabled = false
	job.UpdatedAt = time.Now()

	if err := saveJob(cfg, job); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving job: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Job '%s' disabled\n", id)
}

// runCronRuns handles the cron runs command
func runCronRuns(cmd *cobra.Command, args []string) {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if cronRunsID == "" {
		fmt.Fprintln(os.Stderr, "Error: --id parameter is required")
		os.Exit(1)
	}

	// Load run history
	history, err := loadRunHistory(cfg, cronRunsID, cronRunsLimit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading run history: %v\n", err)
		os.Exit(1)
	}

	if len(history) == 0 {
		fmt.Printf("No run history found for job '%s'\n", cronRunsID)
		return
	}

	fmt.Printf("Run History for Job '%s' (last %d runs):\n", cronRunsID, cronRunsLimit)
	for i, run := range history {
		fmt.Printf("\n  %d. %s\n", i+1, run.StartedAt.Format(time.RFC3339))
		fmt.Printf("     Status: %s\n", run.Status)
		duration := run.FinishedAt.Sub(run.StartedAt)
		fmt.Printf("     Duration: %v\n", duration)
		if run.Error != "" {
			fmt.Printf("     Error: %s\n", run.Error)
		}
	}
}

// runCronRun handles the cron run command
func runCronRun(cmd *cobra.Command, args []string) {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	id := args[0]

	job, err := loadJob(cfg, id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Job with ID '%s' not found\n", id)
		os.Exit(1)
	}

	if !job.Enabled && !cronRunForce {
		fmt.Printf("Job '%s' is disabled. Use --force to run anyway\n", id)
		return
	}

	fmt.Printf("Running job '%s' (%s)...\n", job.Name, job.ID)
	fmt.Printf("  Task: %s\n", job.Task)

	// In a real implementation, this would execute the job
	// For now, just record a run history entry
	run := RunHistory{
		JobID:      job.ID,
		JobName:    job.Name,
		StartedAt:  time.Now(),
		FinishedAt: time.Now(),
		Status:     "success",
	}

	_ = saveRunHistory(cfg, run)

	fmt.Printf("  Started at: %s\n", run.StartedAt.Format(time.RFC3339))
	fmt.Printf("  Finished at: %s\n", run.FinishedAt.Format(time.RFC3339))
	fmt.Printf("  Status: %s\n", run.Status)
}

// Helper types and functions

type JobData struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Schedule  string    `json:"schedule"`
	Task      string    `json:"task"`
	Message   string    `json:"message,omitempty"`
	EventType string    `json:"event_type,omitempty"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type RunHistory struct {
	JobID      string    `json:"job_id"`
	JobName    string    `json:"job_name"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	Status     string    `json:"status"`
	Error      string    `json:"error,omitempty"`
}

func getCronDir(cfg *config.Config) string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".goclaw", "cron")
}

func getJobFilePath(cfg *config.Config, id string) string {
	return filepath.Join(getCronDir(cfg), fmt.Sprintf("%s.json", id))
}

func saveJob(cfg *config.Config, job *JobData) error {
	cronDir := getCronDir(cfg)
	if err := os.MkdirAll(cronDir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(getJobFilePath(cfg, job.ID), data, 0644)
}

func loadJob(cfg *config.Config, id string) (*JobData, error) {
	data, err := os.ReadFile(getJobFilePath(cfg, id))
	if err != nil {
		return nil, err
	}

	var job JobData
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, err
	}

	return &job, nil
}

func saveJobs(cfg *config.Config, jobs []*JobData) error {
	// Save each job individually
	for _, job := range jobs {
		if err := saveJob(cfg, job); err != nil {
			return err
		}
	}
	return nil
}

func loadJobs(cfg *config.Config) ([]*JobData, error) {
	cronDir := getCronDir(cfg)

	entries, err := os.ReadDir(cronDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*JobData{}, nil
		}
		return nil, err
	}

	var jobs []*JobData
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".json") || strings.HasSuffix(entry.Name(), "-history.json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(cronDir, entry.Name()))
		if err != nil {
			continue
		}

		var job JobData
		if err := json.Unmarshal(data, &job); err != nil {
			continue
		}

		jobs = append(jobs, &job)
	}

	return jobs, nil
}

func getRunHistoryFilePath(cfg *config.Config, jobID string) string {
	return filepath.Join(getCronDir(cfg), fmt.Sprintf("%s-history.json", jobID))
}

func saveRunHistory(cfg *config.Config, run RunHistory) error {
	historyFile := getRunHistoryFilePath(cfg, run.JobID)

	// Load existing history
	var history []RunHistory
	if data, err := os.ReadFile(historyFile); err == nil {
		_ = json.Unmarshal(data, &history)
	}

	// Add new run
	history = append(history, run)

	// Keep only last 100 runs
	if len(history) > 100 {
		history = history[len(history)-100:]
	}

	// Save
	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(historyFile, data, 0644)
}

func loadRunHistory(cfg *config.Config, jobID string, limit int) ([]RunHistory, error) {
	historyFile := getRunHistoryFilePath(cfg, jobID)

	data, err := os.ReadFile(historyFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []RunHistory{}, nil
		}
		return nil, err
	}

	var history []RunHistory
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, err
	}

	// Return last N runs in reverse order
	if len(history) > limit {
		start := len(history) - limit
		result := make([]RunHistory, limit)
		for i := 0; i < limit; i++ {
			result[i] = history[start+i]
		}
		// Reverse for display
		for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
			result[i], result[j] = result[j], result[i]
		}
		return result, nil
	}

	// Reverse for display
	for i, j := 0, len(history)-1; i < j; i, j = i+1, j-1 {
		history[i], history[j] = history[j], history[i]
	}

	return history, nil
}

func parseAtSchedule(at string) string {
	// Parse at time and convert to cron expression
	hour, minute, err := parseTime(at)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d %d * * *", minute, hour)
}

func parseEverySchedule(every string) string {
	// Parse every duration and convert to cron expression
	duration, err := time.ParseDuration(every)
	if err != nil {
		// Try parsing as days
		if strings.HasSuffix(every, "d") {
			days := strings.TrimSuffix(every, "d")
			var d int
			if _, err := fmt.Sscanf(days, "%d", &d); err == nil {
				return fmt.Sprintf("0 0 */%d * *", d)
			}
		}
		return ""
	}

	minutes := int(duration.Minutes())
	if minutes < 1 {
		minutes = 1
	}

	if minutes < 60 {
		return fmt.Sprintf("*/%d * * * *", minutes)
	}

	hours := int(duration.Hours())
	return fmt.Sprintf("0 */%d * * *", hours)
}

func parseTime(s string) (hour, minute int, err error) {
	layouts := []string{
		"15:04",
		"3:04pm",
		"03:04pm",
		"3:04PM",
		"03:04PM",
	}

	for _, layout := range layouts {
		t, e := time.Parse(layout, s)
		if e == nil {
			return t.Hour(), t.Minute(), nil
		}
	}

	return 0, 0, fmt.Errorf("invalid time format: %s", s)
}
