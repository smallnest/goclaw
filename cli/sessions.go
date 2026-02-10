package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/smallnest/dogclaw/goclaw/session"
	"github.com/spf13/cobra"
)

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "Manage conversation sessions",
	Long:  `List and manage conversation sessions stored in the sessions directory.`,
}

var sessionsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all sessions",
	Run:   runSessionsList,
}

// Flags for sessions list
var sessionsListJSON bool
var sessionsListVerbose bool
var sessionsListStore string
var sessionsListActive bool

func init() {
	sessionsListCmd.Flags().BoolVar(&sessionsListJSON, "json", false, "Output in JSON format")
	sessionsListCmd.Flags().BoolVar(&sessionsListVerbose, "verbose", false, "Show detailed information")
	sessionsListCmd.Flags().StringVar(&sessionsListStore, "store", "", "Path to sessions directory")
	sessionsListCmd.Flags().BoolVar(&sessionsListActive, "active", false, "Show only active sessions")

	sessionsCmd.AddCommand(sessionsListCmd)
}

// SessionInfo represents session information for display
type SessionInfo struct {
	Key          string            `json:"key"`
	MessageCount int               `json:"message_count"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	LastMessage  string            `json:"last_message,omitempty"`
	Channel      string            `json:"channel,omitempty"`
	ChatID       string            `json:"chat_id,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Active       bool              `json:"active"`
}

// runSessionsList lists all sessions
func runSessionsList(cmd *cobra.Command, args []string) {
	// Determine sessions directory
	var sessionDir string
	var err error

	if sessionsListStore != "" {
		sessionDir = sessionsListStore
	} else {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting home directory: %v\n", err)
			os.Exit(1)
		}
		sessionDir = filepath.Join(homeDir, ".goclaw", "sessions")
	}

	// Create session manager
	sessionMgr, err := session.NewManager(sessionDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating session manager: %v\n", err)
		os.Exit(1)
	}

	// List sessions
	sessionKeys, err := sessionMgr.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing sessions: %v\n", err)
		os.Exit(1)
	}

	if len(sessionKeys) == 0 {
		fmt.Println("No sessions found.")
		return
	}

	// Load session information
	sessions := make([]*SessionInfo, 0, len(sessionKeys))
	for _, key := range sessionKeys {
		sess, err := sessionMgr.GetOrCreate(key)
		if err != nil {
			if sessionsListVerbose {
				fmt.Fprintf(os.Stderr, "Warning: Could not load session '%s': %v\n", key, err)
			}
			continue
		}

		info := &SessionInfo{
			Key:          key,
			MessageCount: len(sess.Messages),
			CreatedAt:    sess.CreatedAt,
			UpdatedAt:    sess.UpdatedAt,
			Metadata:     make(map[string]string),
			Active:       isSessionActive(sess),
		}

		// Extract channel and chat ID from key
		if strings.Contains(key, ":") {
			parts := strings.SplitN(key, ":", 2)
			info.Channel = parts[0]
			info.ChatID = parts[1]
		}

		// Get last message preview
		if len(sess.Messages) > 0 {
			lastMsg := sess.Messages[len(sess.Messages)-1]
			info.LastMessage = truncateString(lastMsg.Content, 50)
		}

		// Extract metadata
		if sess.Metadata != nil {
			for k, v := range sess.Metadata {
				if strVal, ok := v.(string); ok {
					info.Metadata[k] = strVal
				}
			}
		}

		sessions = append(sessions, info)
	}

	// Filter active sessions if requested
	if sessionsListActive {
		activeSessions := make([]*SessionInfo, 0)
		for _, sess := range sessions {
			if sess.Active {
				activeSessions = append(activeSessions, sess)
			}
		}
		sessions = activeSessions
	}

	if len(sessions) == 0 {
		if sessionsListActive {
			fmt.Println("No active sessions found.")
		} else {
			fmt.Println("No sessions found.")
		}
		return
	}

	// Sort by updated time (most recent first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	// Output
	if sessionsListJSON {
		data, err := json.MarshalIndent(sessions, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
		return
	}

	// Display in table format
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	if sessionsListVerbose {
		// Verbose output
		fmt.Fprintf(w, "KEY\tCHANNEL\tCHAT ID\tMESSAGES\tCREATED\tUPDATED\tLAST MESSAGE\tACTIVE\n")
		fmt.Fprintf(w, "---\t-------\t-------\t--------\t-------\t-------\t------------\t------\n")
		for _, sess := range sessions {
			activeStr := " "
			if sess.Active {
				activeStr = "*"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\t%s\t%s\n",
				sess.Key,
				sess.Channel,
				sess.ChatID,
				sess.MessageCount,
				formatTime(sess.CreatedAt),
				formatTime(sess.UpdatedAt),
				sess.LastMessage,
				activeStr,
			)
		}
	} else {
		// Simple output
		fmt.Fprintf(w, "KEY\tMESSAGES\tUPDATED\tACTIVE\n")
		fmt.Fprintf(w, "---\t--------\t-------\t------\n")
		for _, sess := range sessions {
			activeStr := " "
			if sess.Active {
				activeStr = "*"
			}
			fmt.Fprintf(w, "%s\t%d\t%s\t%s\n",
				sess.Key,
				sess.MessageCount,
				formatTime(sess.UpdatedAt),
				activeStr,
			)
		}
	}
	w.Flush()

	// Print summary
	fmt.Printf("\nTotal: %d session(s)", len(sessions))
	if sessionsListActive {
		fmt.Print(" (active only)")
	}
	fmt.Println()

	// Print legend
	fmt.Println("\n* = Active (updated within last 24 hours)")
}

// isSessionActive checks if a session is considered active
func isSessionActive(sess *session.Session) bool {
	// Consider a session active if updated within 24 hours
	return time.Since(sess.UpdatedAt) < 24*time.Hour
}

// formatTime formats a time value for display
func formatTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	if diff < time.Minute {
		return "just now"
	}
	if diff < time.Hour {
		minutes := int(diff.Minutes())
		return fmt.Sprintf("%dm ago", minutes)
	}
	if diff < 24*time.Hour {
		hours := int(diff.Hours())
		return fmt.Sprintf("%dh ago", hours)
	}
	if diff < 7*24*time.Hour {
		days := int(diff.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	}
	return t.Format("2006-01-02")
}

// truncateString truncates a string to a maximum length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
