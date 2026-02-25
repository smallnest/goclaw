package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/smallnest/goclaw/config"
	"github.com/spf13/cobra"
)

var (
	healthJSON    bool
	healthTimeout int
	healthVerbose bool
)

// HealthCommand returns the health command
func HealthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Check health of running Gateway",
		Long:  `Fetch health status from the running goclaw gateway server.`,
		Run:   runHealth,
	}

	cmd.Flags().BoolVarP(&healthJSON, "json", "j", false, "Output as JSON")
	cmd.Flags().IntVarP(&healthTimeout, "timeout", "t", 5, "Timeout in seconds")
	cmd.Flags().BoolVarP(&healthVerbose, "verbose", "v", false, "Verbose output")

	return cmd
}

// runHealth checks the health of the gateway
func runHealth(cmd *cobra.Command, args []string) {
	// Load config to get gateway port
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	port := config.GetGatewayHTTPPort(cfg)
	if len(args) > 0 {
		// Use provided port
		if _, err := fmt.Sscanf(args[0], "%d", &port); err == nil {
			// port updated from args
		}
	}

	url := fmt.Sprintf("http://localhost:%d/health", port)
	client := &http.Client{
		Timeout: time.Duration(healthTimeout) * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		if healthJSON {
			fmt.Printf(`{"status":"error","error":"%s"}`+"\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "Failed to connect to gateway: %v\n", err)
			fmt.Println("Make sure the gateway is running (use 'goclaw gateway run')")
		}
		os.Exit(1)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read response: %v\n", err)
		os.Exit(1)
	}

	if resp.StatusCode != http.StatusOK {
		if healthJSON {
			fmt.Printf(`{"status":"error","code":%d,"body":"%s"}`+"\n", resp.StatusCode, string(body))
		} else {
			fmt.Fprintf(os.Stderr, "Health check failed: status %d\n", resp.StatusCode)
			fmt.Printf("Response: %s\n", string(body))
		}
		os.Exit(1)
	}

	// Parse health response
	var health map[string]interface{}
	if err := json.Unmarshal(body, &health); err != nil {
		if healthJSON {
			fmt.Printf(`{"status":"ok","raw":"%s"}`+"\n", string(body))
		} else {
			fmt.Printf("Health: OK\n")
			fmt.Printf("Raw response: %s\n", string(body))
		}
		return
	}

	// Add URL to response
	health["url"] = url

	if healthJSON {
		// Output as JSON
		output, _ := json.MarshalIndent(health, "", "  ")
		fmt.Println(string(output))
	} else {
		// Output as text
		fmt.Println("Gateway Health: OK")
		fmt.Printf("  URL: %s\n", url)

		if status, ok := health["status"].(string); ok {
			fmt.Printf("  Status: %s\n", status)
		}

		if version, ok := health["version"].(string); ok {
			fmt.Printf("  Version: %s\n", version)
		}

		if timestamp, ok := health["time"].(float64); ok {
			t := time.Unix(int64(timestamp), 0)
			fmt.Printf("  Timestamp: %s\n", t.Format(time.RFC3339))
		}

		if healthVerbose {
			fmt.Println("\nFull response:")
			for k, v := range health {
				fmt.Printf("  %s: %v\n", k, v)
			}
		}
	}
}
