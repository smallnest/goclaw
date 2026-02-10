package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/smallnest/dogclaw/goclaw/session"
	"github.com/spf13/cobra"
)

var (
	statusJSON    bool
	statusAll     bool
	statusDeep    bool
	statusUsage   bool
	statusTimeout int
	statusVerbose bool
	statusDebug   bool
)

// StatusCommand returns the status command
func StatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show session health and status",
		Long:  `Display comprehensive status information about goclaw sessions and gateway.`,
		Run:   runStatus,
	}

	cmd.Flags().BoolVarP(&statusJSON, "json", "j", false, "Output as JSON")
	cmd.Flags().BoolVarP(&statusAll, "all", "a", false, "Show all sessions")
	cmd.Flags().BoolVarP(&statusDeep, "deep", "d", false, "Deep scan (include message details)")
	cmd.Flags().BoolVarP(&statusUsage, "usage", "u", false, "Show resource usage")
	cmd.Flags().IntVarP(&statusTimeout, "timeout", "t", 5, "Timeout in seconds")
	cmd.Flags().BoolVarP(&statusVerbose, "verbose", "v", false, "Verbose output")
	cmd.Flags().BoolVar(&statusDebug, "debug", false, "Debug output")

	return cmd
}

// SessionStatus represents session status information
type SessionStatus struct {
	Key           string    `json:"key"`
	MessageCount  int       `json:"message_count"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	Size          int64     `json:"size_bytes,omitempty"`
	HasMetadata   bool      `json:"has_metadata,omitempty"`
	LastMessageAt time.Time `json:"last_message_at,omitempty"`
}

// GatewayStatus represents gateway status information
type GatewayStatus struct {
	Online    bool   `json:"online"`
	URL       string `json:"url,omitempty"`
	Status    string `json:"status,omitempty"`
	Version   string `json:"version,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
}

// SystemStatus represents overall system status
type SystemStatus struct {
	Gateway     GatewayStatus    `json:"gateway"`
	Sessions    []SessionStatus  `json:"sessions"`
	SessionDir  string           `json:"session_dir"`
	TotalSize   int64            `json:"total_size_bytes"`
	SessionCount int             `json:"session_count"`
}

// runStatus displays status information
func runStatus(cmd *cobra.Command, args []string) {
	// Create status object
	status := &SystemStatus{
		SessionDir: filepath.Join(os.Getenv("HOME"), ".goclaw", "sessions"),
	}

	// Check gateway status
	status.Gateway = checkGatewayStatus(statusTimeout)

	// Get session information
	if err := getSessionStatus(status, statusAll, statusDeep); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to get session status: %v\n", err)
	}

	// Output status
	if statusJSON {
		outputStatusJSON(status)
	} else {
		outputStatusText(status)
	}
}

// checkGatewayStatus checks if gateway is running
func checkGatewayStatus(timeout int) GatewayStatus {
	result := GatewayStatus{Online: false}

	ports := []int{18789, 18790, 18890}
	client := &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
	}

	for _, port := range ports {
		url := fmt.Sprintf("http://localhost:%d/health", port)
		resp, err := client.Get(url)
		if err == nil {
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				var health map[string]interface{}
				json.Unmarshal(body, &health)

				result.Online = true
				result.URL = url
				result.Status = "ok"

				if status, ok := health["status"].(string); ok {
					result.Status = status
				}
				if version, ok := health["version"].(string); ok {
					result.Version = version
				}
				if ts, ok := health["time"].(float64); ok {
					result.Timestamp = int64(ts)
				}

				break
			}
		}
	}

	return result
}

// getSessionStatus retrieves session status information
func getSessionStatus(status *SystemStatus, all bool, deep bool) error {
	// Create session manager
	sessionMgr, err := session.NewManager(status.SessionDir)
	if err != nil {
		return err
	}

	// List sessions
	sessionKeys, err := sessionMgr.List()
	if err != nil {
		return err
	}

	status.SessionCount = len(sessionKeys)
	status.Sessions = make([]SessionStatus, 0, len(sessionKeys))

	for _, key := range sessionKeys {
		sess, err := sessionMgr.GetOrCreate(key)
		if err != nil {
			continue
		}

		sessStatus := SessionStatus{
			Key:          sess.Key,
			MessageCount: len(sess.Messages),
			CreatedAt:    sess.CreatedAt,
			UpdatedAt:    sess.UpdatedAt,
			HasMetadata:  len(sess.Metadata) > 0,
		}

		// Get last message timestamp
		if len(sess.Messages) > 0 {
			sessStatus.LastMessageAt = sess.Messages[len(sess.Messages)-1].Timestamp
		}

		// Deep scan - get file size and details
		if deep {
			sessionPath := filepath.Join(status.SessionDir, key+".jsonl")
			if info, err := os.Stat(sessionPath); err == nil {
				sessStatus.Size = info.Size()
				status.TotalSize += info.Size()
			}
		}

		status.Sessions = append(status.Sessions, sessStatus)

		// If not showing all, only show recent sessions
		if !all && len(status.Sessions) >= 10 {
			break
		}
	}

	return nil
}

// outputStatusJSON outputs status as JSON
func outputStatusJSON(status *SystemStatus) {
	output, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(output))
}

// outputStatusText outputs status as formatted text
func outputStatusText(status *SystemStatus) {
	fmt.Println("=== goclaw Status ===\n")

	// Gateway status
	fmt.Println("Gateway:")
	if status.Gateway.Online {
		fmt.Printf("  Status:  Online\n")
		fmt.Printf("  URL:     %s\n", status.Gateway.URL)
		if status.Gateway.Version != "" {
			fmt.Printf("  Version: %s\n", status.Gateway.Version)
		}
		if status.Gateway.Timestamp > 0 {
			t := time.Unix(status.Gateway.Timestamp, 0)
			fmt.Printf("  Uptime:  %s\n", t.Format(time.RFC3339))
		}
	} else {
		fmt.Printf("  Status:  Offline\n")
		fmt.Printf("  Tip:     Start gateway with 'goclaw gateway run'\n")
	}

	// Session status
	fmt.Printf("\nSessions (%d total):\n", status.SessionCount)
	if len(status.Sessions) == 0 {
		fmt.Println("  No sessions found")
	} else {
		for i, sess := range status.Sessions {
			fmt.Printf("  %d. %s\n", i+1, sess.Key)
			fmt.Printf("     Messages: %d\n", sess.MessageCount)
			fmt.Printf("     Created:  %s\n", sess.CreatedAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("     Updated:  %s\n", sess.UpdatedAt.Format("2006-01-02 15:04:05"))

			if sess.HasMetadata {
				fmt.Printf("     Metadata: yes\n")
			}

			if statusDeep {
				if sess.Size > 0 {
					fmt.Printf("     Size:     %d bytes\n", sess.Size)
				}
				if !sess.LastMessageAt.IsZero() {
					fmt.Printf("     Last msg: %s\n", sess.LastMessageAt.Format("2006-01-02 15:04:05"))
				}
			}
		}

		if !statusAll && status.SessionCount > len(status.Sessions) {
			fmt.Printf("\n  ... and %d more (use --all to show all)\n", status.SessionCount-len(status.Sessions))
		}
	}

	// Storage info
	if statusDeep {
		fmt.Printf("\nStorage:\n")
		fmt.Printf("  Session Dir: %s\n", status.SessionDir)
		if status.TotalSize > 0 {
			fmt.Printf("  Total Size:  %d bytes (%.2f MB)\n", status.TotalSize, float64(status.TotalSize)/(1024*1024))
		}
	}

	// Additional info
	if statusVerbose {
		fmt.Printf("\nVerbose Information:\n")
		fmt.Printf("  Session Directory: %s\n", status.SessionDir)
		fmt.Printf("  Configuration: %s\n", filepath.Join(os.Getenv("HOME"), ".goclaw", "config.yaml"))
	}

	if statusDebug {
		fmt.Printf("\nDebug Information:\n")
		fmt.Printf("  Status All:    %v\n", statusAll)
		fmt.Printf("  Status Deep:   %v\n", statusDeep)
		fmt.Printf("  Status Usage:  %v\n", statusUsage)
		fmt.Printf("  Status JSON:   %v\n", statusJSON)
	}

	fmt.Println()
}
