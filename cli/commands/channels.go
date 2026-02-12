package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var (
	channelsJSON    bool
	channelsTimeout int
)

// ChannelsCommand returns the channels command
func ChannelsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "channels",
		Short: "Manage chat channels",
		Long:  `List and manage chat channels like Telegram, Feishu, WhatsApp, etc.`,
	}

	// Add list subcommand
	cmd.AddCommand(channelsListCmd())

	// Add status subcommand
	cmd.AddCommand(channelsStatusCmd())

	return cmd
}

// ChannelInfo represents information about a channel
type ChannelInfo struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// ChannelStatusResponse represents the response from gateway channels.status
type ChannelStatusResponse struct {
	Name    string                 `json:"name"`
	Enabled bool                   `json:"enabled"`
	Extra   map[string]interface{} `json:"extra,omitempty"`
}

// channelsListCmd returns the channels list command
func channelsListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all available channels",
		Long:  `Display a list of all configured channels and their status.`,
		Run:   runChannelsList,
	}

	cmd.Flags().BoolVarP(&channelsJSON, "json", "j", false, "Output as JSON")
	cmd.Flags().IntVarP(&channelsTimeout, "timeout", "t", 5, "Timeout in seconds")

	return cmd
}

// channelsStatusCmd returns the channels status command
func channelsStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status [channel]",
		Short: "Show channel status",
		Long:  `Display detailed status information for a specific channel or all channels.`,
		Args:  cobra.MaximumNArgs(1),
		Run:   runChannelsStatus,
	}

	cmd.Flags().BoolVarP(&channelsJSON, "json", "j", false, "Output as JSON")
	cmd.Flags().IntVarP(&channelsTimeout, "timeout", "t", 5, "Timeout in seconds")

	return cmd
}

// runChannelsList executes the channels list command
func runChannelsList(cmd *cobra.Command, args []string) {
	// Try to get channel list from gateway
	channels := getChannelsFromGateway(channelsTimeout)

	// Also show known supported channels
	allChannels := getAllKnownChannels()

	if channelsJSON {
		outputChannelsJSON(channels, allChannels)
	} else {
		outputChannelsText(channels, allChannels)
	}
}

// runChannelsStatus executes the channels status command
func runChannelsStatus(cmd *cobra.Command, args []string) {
	channelName := ""
	if len(args) > 0 {
		channelName = args[0]
	}

	status := getChannelStatusFromGateway(channelName, channelsTimeout)

	if channelsJSON {
		outputChannelStatusJSON(status)
	} else {
		outputChannelStatusText(channelName, status)
	}
}

// getAllKnownChannels returns all known supported channel types
func getAllKnownChannels() []ChannelInfo {
	return []ChannelInfo{
		{Name: "feishu", Enabled: false},
		{Name: "telegram", Enabled: false},
		{Name: "whatsapp", Enabled: false},
		{Name: "qq", Enabled: false},
		{Name: "wework", Enabled: false},
		{Name: "dingtalk", Enabled: false},
		{Name: "slack", Enabled: false},
		{Name: "discord", Enabled: false},
		{Name: "teams", Enabled: false},
		{Name: "googlechat", Enabled: false},
		{Name: "infoflow", Enabled: false},
	}
}

// getChannelsFromGateway retrieves channel list from gateway
func getChannelsFromGateway(timeout int) []ChannelInfo {
	client := &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
	}

	// Try different WebSocket gateway ports
	ports := []int{28789, 28790, 28791}
	var channels []ChannelInfo

	for _, port := range ports {
		// Try to get channels from the HTTP API
		url := fmt.Sprintf("http://localhost:%d/api/channels", port)
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		var result struct {
			Channels []map[string]interface{} `json:"channels"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			continue
		}

		// Parse channels
		for _, ch := range result.Channels {
			name, _ := ch["name"].(string)
			enabled, _ := ch["enabled"].(bool)
			channels = append(channels, ChannelInfo{
				Name:    name,
				Enabled: enabled,
			})
		}
		break
	}

	return channels
}

// getChannelStatusFromGateway retrieves channel status from gateway
func getChannelStatusFromGateway(channelName string, timeout int) map[string]interface{} {
	client := &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
	}

	// Try different WebSocket gateway ports
	ports := []int{28789, 28790, 28791}

	for _, port := range ports {
		// If channel name is specified, get specific channel status
		// Otherwise, get all channels
		url := fmt.Sprintf("http://localhost:%d/api/channels", port)
		if channelName != "" {
			url += "?channel=" + channelName
		}

		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			// Fall back to health check
			break
		}

		body, _ := io.ReadAll(resp.Body)

		if channelName != "" {
			// Specific channel status
			var status map[string]interface{}
			if err := json.Unmarshal(body, &status); err != nil {
				continue
			}
			status["online"] = true
			return status
		} else {
			// All channels
			var result struct {
				Channels []map[string]interface{} `json:"channels"`
				Count    int                       `json:"count"`
			}
			if err := json.Unmarshal(body, &result); err != nil {
				continue
			}
			return map[string]interface{}{
				"online":   true,
				"channels": result.Channels,
				"count":    result.Count,
			}
		}
	}

	// Gateway is offline or endpoint not available
	// Try health check as fallback
	for _, port := range ports {
		url := fmt.Sprintf("http://localhost:%d/health", port)
		resp, err := client.Get(url)
		if err == nil {
			defer resp.Body.Close()
			return map[string]interface{}{
				"online":  true,
				"channel": channelName,
				"message": "Channel API not available, but gateway is online",
			}
		}
	}

	// Gateway is offline
	return map[string]interface{}{
		"online":  false,
		"channel": channelName,
		"status":  "unavailable",
		"message": "Gateway is not running. Start with 'goclaw start' or 'goclaw gateway run'",
	}
}

// outputChannelsJSON outputs channel list as JSON
func outputChannelsJSON(activeChannels []ChannelInfo, allChannels []ChannelInfo) {
	type output struct {
		Active  []ChannelInfo `json:"active"`
		All     []ChannelInfo `json:"all"`
		Online  bool          `json:"gateway_online"`
		Message string        `json:"message,omitempty"`
	}

	// Check if gateway is online
	gatewayOnline := len(activeChannels) > 0 || checkGatewayOnline(channelsTimeout)

	out := output{
		Active:  activeChannels,
		All:     allChannels,
		Online:  gatewayOnline,
		Message: "Use 'goclaw start' to start the gateway with configured channels",
	}

	jsonOut, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(jsonOut))
}

// outputChannelsText outputs channel list as text
func outputChannelsText(activeChannels []ChannelInfo, allChannels []ChannelInfo) {
	fmt.Println("=== Channels ===")

	// Check gateway status
	gatewayOnline := checkGatewayOnline(channelsTimeout)
	if gatewayOnline {
		fmt.Println("Gateway: Online")
	} else {
		fmt.Println("Gateway: Offline (start with 'goclaw start')")
	}

	// Show configured channels
	if len(activeChannels) > 0 {
		fmt.Println("\nConfigured Channels:")
		for _, ch := range activeChannels {
			if ch.Enabled {
				fmt.Printf("  - %s (enabled)\n", ch.Name)
			} else {
				fmt.Printf("  - %s (disabled)\n", ch.Name)
			}
		}
	} else {
		fmt.Println("\nConfigured Channels: None")
	}

	// Show all available channels
	fmt.Println("\nSupported Channels:")
	for _, ch := range allChannels {
		fmt.Printf("  - %s\n", ch.Name)
	}

	fmt.Println("\nTip:")
	fmt.Println("  1. Edit ~/.goclaw/config.json to configure channels")
	fmt.Println("  2. Run 'goclaw start' to start the agent with channels enabled")
	fmt.Println("  3. Use 'goclaw channels status [name]' to check specific channel status")
}

// outputChannelStatusJSON outputs channel status as JSON
func outputChannelStatusJSON(status map[string]interface{}) {
	jsonOut, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(jsonOut))
}

// outputChannelStatusText outputs channel status as text
func outputChannelStatusText(channelName string, status map[string]interface{}) {
	online, _ := status["online"].(bool)

	fmt.Printf("=== Channel Status")
	if channelName != "" {
		fmt.Printf(" (%s)", channelName)
	}
	fmt.Println(" ===")

	if online {
		fmt.Println("Gateway: Online")

		// Show specific channel status if available
		if name, ok := status["name"].(string); ok {
			enabled, _ := status["enabled"].(bool)
			fmt.Printf("Name:    %s\n", name)
			fmt.Printf("Enabled: %v\n", enabled)
		} else if msg, ok := status["message"].(string); ok {
			fmt.Println("Message:", msg)
		} else if channelName != "" {
			fmt.Printf("Status:  %s not configured or not running\n", channelName)
		} else {
			// Show all channels
			if channels, ok := status["channels"].([]map[string]interface{}); ok {
				count, _ := status["count"].(int)
				fmt.Printf("Configured Channels (%d):\n", count)
				for _, ch := range channels {
					name, _ := ch["name"].(string)
					enabled, _ := ch["enabled"].(bool)
					fmt.Printf("  - %s (enabled: %v)\n", name, enabled)
				}
			}
		}
	} else {
		fmt.Println("Gateway: Offline")
		fmt.Println("Status:  Unavailable")
		if msg, ok := status["message"].(string); ok {
			fmt.Printf("Message: %s\n", msg)
		}
	}
}

// checkGatewayOnline checks if the gateway is running
func checkGatewayOnline(timeout int) bool {
	client := &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
	}

	ports := []int{28789, 28790, 28791}

	for _, port := range ports {
		url := fmt.Sprintf("http://localhost:%d/health", port)
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
	}

	return false
}
