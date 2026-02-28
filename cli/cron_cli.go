package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/smallnest/goclaw/config"
	"github.com/spf13/cobra"
)

var cronCmd = &cobra.Command{
	Use:   "cron",
	Short: "Scheduled jobs management (via Gateway)",
}

func init() {
	// Register cron command to rootCmd
	rootCmd.AddCommand(cronCmd)
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
	cronAddSession     string

	// run flags
	cronRunForce bool // force run even if disabled

	// runs flags
	cronRunsID    string
	cronRunsLimit int
	cronRunsJSON  bool
)

func init() {
	// Status command flags
	cronStatusCmd.Flags().BoolVar(&cronStatusJSON, "json", false, "Output JSON")

	// List command flags
	cronListCmd.Flags().BoolVarP(&cronListAll, "all", "a", false, "Include disabled jobs")
	cronListCmd.Flags().BoolVar(&cronListJSON, "json", false, "Output JSON")

	// Add command flags
	cronAddCmd.Flags().StringVarP(&cronAddName, "name", "n", "", "Job name (required)")
	cronAddCmd.Flags().StringVar(&cronAddAt, "at", "", "Run at specific time (RFC3339 format)")
	cronAddCmd.Flags().StringVar(&cronAddEvery, "every", "", "Run every interval (e.g., 30s, 5m, 2h, 1d)")
	cronAddCmd.Flags().StringVar(&cronAddCron, "cron", "", "Cron expression (e.g., '0 8 * * *')")
	cronAddCmd.Flags().StringVarP(&cronAddMessage, "message", "m", "", "Message to send (agent-turn payload)")
	cronAddCmd.Flags().StringVar(&cronAddSystemEvent, "system-event", "", "System event type (system-event payload)")
	cronAddCmd.Flags().StringVar(&cronAddWebhook, "webhook", "", "Webhook URL for delivery")
	cronAddCmd.Flags().StringVar(&cronAddSession, "session", "main", "Session target (main or isolated)")
	if err := cronAddCmd.MarkFlagRequired("name"); err != nil {
		panic(err)
	}

	// Run command flags
	cronRunCmd.Flags().BoolVarP(&cronRunForce, "force", "f", false, "Force run even if disabled")

	// Runs command flags
	cronRunsCmd.Flags().StringVar(&cronRunsID, "id", "", "Job ID (optional)")
	cronRunsCmd.Flags().IntVar(&cronRunsLimit, "limit", 50, "Max number of runs to show")
	cronRunsCmd.Flags().BoolVar(&cronRunsJSON, "json", false, "Output JSON")

	// Register subcommands
	cronCmd.AddCommand(cronStatusCmd)
	cronCmd.AddCommand(cronListCmd)
	cronCmd.AddCommand(cronAddCmd)
	cronCmd.AddCommand(cronEditCmd)
	cronCmd.AddCommand(cronRmCmd)
	cronCmd.AddCommand(cronEnableCmd)
	cronCmd.AddCommand(cronDisableCmd)
	cronCmd.AddCommand(cronRunsCmd)
	cronCmd.AddCommand(cronRunCmd)
}

func runCronStatus(cmd *cobra.Command, args []string) {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	result, err := callGatewayRPC(cfg, "cron.status", map[string]interface{}{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if cronStatusJSON {
		printJSON(result)
		return
	}

	status := result.(map[string]interface{})

	fmt.Println("Cron Scheduler Status")
	fmt.Println("======================")
	fmt.Printf("Running: %v\n", status["running"])
	fmt.Printf("Job Count: %v\n", status["job_count"])
	if jobCount, ok := status["job_count"].(map[string]interface{}); ok {
		if enabled, ok := jobCount["enabled"]; ok {
			fmt.Printf("  Enabled: %v\n", enabled)
		}
		if disabled, ok := jobCount["disabled"]; ok {
			fmt.Printf("  Disabled: %v\n", disabled)
		}
	}
}

func runCronList(cmd *cobra.Command, args []string) {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}
	result, err := callGatewayRPC(cfg, "cron.list", map[string]interface{}{
		"include_disabled": cronListAll,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if cronListJSON {
		printJSON(result)
		return
	}

	data := result.(map[string]interface{})
	jobs, _ := data["jobs"].([]interface{})

	if len(jobs) == 0 {
		fmt.Println("No jobs found.")
		return
	}

	fmt.Printf("Found %d job(s):\n\n", len(jobs))
	for _, j := range jobs {
		job := j.(map[string]interface{})
		printJob(job)
		fmt.Println()
	}
}

func runCronAdd(cmd *cobra.Command, args []string) {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Build schedule
	schedule := make(map[string]interface{})
	if cronAddAt != "" {
		schedule["type"] = "at"
		schedule["at"] = cronAddAt
	} else if cronAddEvery != "" {
		schedule["type"] = "every"
		schedule["every"] = cronAddEvery
	} else if cronAddCron != "" {
		schedule["type"] = "cron"
		schedule["cron"] = cronAddCron
	} else {
		fmt.Fprintf(os.Stderr, "Error: must specify one of --at, --every, or --cron\n")
		os.Exit(1)
	}

	// Build payload
	payload := make(map[string]interface{})
	if cronAddMessage != "" {
		payload["type"] = "agent-turn"
		payload["message"] = cronAddMessage
	} else if cronAddSystemEvent != "" {
		payload["type"] = "system-event"
		payload["system_event_type"] = cronAddSystemEvent
	} else {
		fmt.Fprintf(os.Stderr, "Error: must specify --message or --system-event\n")
		os.Exit(1)
	}

	// Build job params
	params := map[string]interface{}{
		"name":           cronAddName,
		"schedule":       schedule,
		"payload":        payload,
		"session_target": cronAddSession,
	}

	if cronAddWebhook != "" {
		delivery := map[string]interface{}{
			"mode":        "webhook",
			"webhook_url": cronAddWebhook,
		}
		params["delivery"] = delivery
	}

	result, err := callGatewayRPC(cfg, "cron.add", params)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	job := result.(map[string]interface{})
	fmt.Printf("Job '%s' added with ID: %s\n", job["name"], job["id"])
}

func runCronEdit(cmd *cobra.Command, args []string) {
	fmt.Println("Edit command is not yet implemented via CLI.")
	fmt.Println("Use cron update RPC method directly or implement edit functionality.")
}

func runCronRm(cmd *cobra.Command, args []string) {
	id := args[0]
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	result, err := callGatewayRPC(cfg, "cron.remove", map[string]interface{}{
		"id": id,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	res := result.(map[string]interface{})
	fmt.Printf("Job '%s' %s\n", id, res["status"])
}

func runCronEnable(cmd *cobra.Command, args []string) {
	id := args[0]
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	_, err = callGatewayRPC(cfg, "cron.update", map[string]interface{}{
		"id": id,
		"patch": map[string]interface{}{
			"enabled": true,
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Job '%s' enabled\n", id)
}

func runCronDisable(cmd *cobra.Command, args []string) {
	id := args[0]
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	_, err = callGatewayRPC(cfg, "cron.update", map[string]interface{}{
		"id": id,
		"patch": map[string]interface{}{
			"enabled": false,
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Job '%s' disabled\n", id)
}

func runCronRuns(cmd *cobra.Command, args []string) {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Use provided id or show usage note
	if cronRunsID == "" && cmd.Flags().Changed("id") {
		fmt.Fprintf(os.Stderr, "Error: --id is required\n")
		os.Exit(1)
	}

	if cronRunsID == "" {
		fmt.Println("Usage: goclaw cron runs --id <job-id>")
		fmt.Println("       goclaw cron runs --id <job-id> --limit 100")
		return
	}

	result, err := callGatewayRPC(cfg, "cron.runs", map[string]interface{}{
		"id":    cronRunsID,
		"limit": cronRunsLimit,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if cronRunsJSON {
		printJSON(result)
		return
	}

	data := result.(map[string]interface{})
	runs := data["runs"].([]interface{})

	fmt.Printf("Run History for Job '%s' (last %d runs):\n\n", cronRunsID, len(runs))

	for i, r := range runs {
		run := r.(map[string]interface{})
		fmt.Printf("  %d. %s\n", i+1, formatTimeStr(run["started_at"]))
		fmt.Printf("     Status: %s\n", run["status"])
		if dur, ok := run["duration"].(string); ok && dur != "" {
			fmt.Printf("     Duration: %s\n", dur)
		}
		if err, ok := run["error"].(string); ok && err != "" {
			fmt.Printf("     Error: %s\n", err)
		}
		fmt.Println()
	}
}

func runCronRun(cmd *cobra.Command, args []string) {
	id := args[0]
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	mode := "normal"
	if cronRunForce {
		mode = "force"
	}

	result, err := callGatewayRPC(cfg, "cron.run", map[string]interface{}{
		"id":   id,
		"mode": mode,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	res := result.(map[string]interface{})
	fmt.Printf("Job '%s' run initiated\n", id)
	fmt.Printf("Status: %s\n", res["status"])

	// Suggest viewing run logs
	fmt.Printf("\nView run logs: ./goclaw cron runs --id %s\n", id)
}

func printJob(job map[string]interface{}) {
	fmt.Printf("ID: %s\n", job["id"])
	fmt.Printf("Name: %s\n", job["name"])
	enabled := "-"
	if state, ok := job["state"].(map[string]interface{}); ok {
		if v, exists := state["enabled"]; exists {
			enabled = fmt.Sprintf("%v", v)
		}
	}
	fmt.Printf("Enabled: %s\n", enabled)
	fmt.Printf("Schedule: %s\n", formatScheduleFromMap(job))

	if payload, ok := job["payload"].(map[string]interface{}); ok {
		ptype := payload["type"]
		if ptype == "agent-turn" {
			fmt.Printf("Payload: message: %s\n", payload["message"])
		} else if ptype == "system-event" {
			fmt.Printf("Payload: event: %s\n", payload["system_event_type"])
		}
	}

	if delivery, ok := job["delivery"].(map[string]interface{}); ok && delivery != nil {
		fmt.Printf("Delivery: %s", delivery["mode"])
		if webhook, ok := delivery["webhook_url"].(string); ok && webhook != "" {
			fmt.Printf(" (%s)", webhook)
		}
		fmt.Println()
	}

	fmt.Printf("Created: %s\n", formatTimeStr(job["created_at"]))

	if state, ok := job["state"].(map[string]interface{}); ok {
		if nextRun, ok := state["next_run_at"]; ok && nextRun != nil {
			fmt.Printf("Next Run: %s\n", formatTimeStr(nextRun))
		}
		if lastRun, ok := state["last_run_at"]; ok && lastRun != nil {
			fmt.Printf("Last Run: %s\n", formatTimeStr(lastRun))
		}
	}
}

func formatScheduleFromMap(job map[string]interface{}) string {
	schedule, ok := job["schedule"].(map[string]interface{})
	if !ok {
		return "unknown"
	}

	typ, _ := schedule["type"].(string)
	switch typ {
	case "at":
		if at, ok := schedule["at"]; ok && at != nil {
			return "at " + formatTimeStr(at)
		}
		if atISO, ok := schedule["at_iso"]; ok && atISO != nil {
			return "at " + formatTimeStr(atISO)
		}
		return "at <invalid>"
	case "every":
		if every, ok := schedule["every"].(string); ok && every != "" {
			return "every " + every
		}
		if everyMs, ok := schedule["every_duration_ms"]; ok && everyMs != nil {
			if ms, ok := toInt64(everyMs); ok && ms > 0 {
				return "every " + (time.Duration(ms) * time.Millisecond).String()
			}
		}
		return "every <invalid>"
	case "cron":
		if cronExpr, ok := schedule["cron_expression"].(string); ok && cronExpr != "" {
			return cronExpr
		}
		if cronExpr, ok := schedule["cron"].(string); ok && cronExpr != "" {
			return cronExpr
		}
		return "<invalid cron>"
	default:
		return "unknown"
	}
}

func toInt64(v interface{}) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case float32:
		return int64(n), true
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}

func formatTimeStr(t interface{}) string {
	if t == nil {
		return "-"
	}
	if s, ok := t.(string); ok {
		// Try to parse and format
		if pt, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return pt.Format("2006-01-02 15:04:05")
		}
		return s
	}
	return fmt.Sprintf("%v", t)
}

func printJSON(v interface{}) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}
