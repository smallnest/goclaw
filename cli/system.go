package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/gateway"
	"github.com/spf13/cobra"
)

var systemCmd = &cobra.Command{
	Use:   "system",
	Short: "System control",
}

var systemEventCmd = &cobra.Command{
	Use:   "event",
	Short: "Enqueue a system event",
	Run:   runSystemEvent,
}

var systemHeartbeatCmd = &cobra.Command{
	Use:   "heartbeat",
	Short: "Control heartbeat settings",
	Args:  cobra.ExactArgs(1),
	Run:   runSystemHeartbeat,
}

var systemPresenceCmd = &cobra.Command{
	Use:   "presence",
	Short: "List system presence entries",
	Run:   runSystemPresence,
}

// System flags
var (
	systemEventText string
	systemEventMode string
)

func init() {
	// Register system commands
	rootCmd.AddCommand(systemCmd)
	systemCmd.AddCommand(systemEventCmd)
	systemCmd.AddCommand(systemHeartbeatCmd)
	systemCmd.AddCommand(systemPresenceCmd)

	// system event flags
	systemEventCmd.Flags().StringVar(&systemEventText, "text", "", "Event text (required)")
	systemEventCmd.Flags().StringVar(&systemEventMode, "mode", "normal", "Event mode")
	_ = systemEventCmd.MarkFlagRequired("text")
}

// runSystemEvent handles the system event command
func runSystemEvent(cmd *cobra.Command, args []string) {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Create system event message
	msg := &bus.InboundMessage{
		Channel:  "system",
		SenderID: "cli",
		ChatID:   "system",
		Content:  systemEventText,
		Metadata: map[string]interface{}{
			"event_type": "system_event",
			"mode":       systemEventMode,
			"timestamp":  time.Now().Unix(),
		},
		Timestamp: time.Now(),
	}

	// Publish to message bus via gateway
	if err := publishViaGateway(cfg, msg); err != nil {
		fmt.Fprintf(os.Stderr, "Error publishing system event: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("System event enqueued: %s (mode: %s)\n", systemEventText, systemEventMode)
}

// runSystemHeartbeat handles the system heartbeat command
func runSystemHeartbeat(cmd *cobra.Command, args []string) {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	action := args[0]

	switch action {
	case "last":
		handleHeartbeatLast(cfg)
	case "enable":
		handleHeartbeatEnable(cfg)
	case "disable":
		handleHeartbeatDisable(cfg)
	default:
		fmt.Fprintf(os.Stderr, "Unknown heartbeat action: %s. Valid actions: last, enable, disable\n", action)
		os.Exit(1)
	}
}

// handleHeartbeatLast handles getting the last heartbeat
func handleHeartbeatLast(cfg *config.Config) {
	result, err := callGatewayRPC(cfg, "heartbeat.last", map[string]interface{}{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting last heartbeat: %v\n", err)
		os.Exit(1)
	}

	data, ok := result.(map[string]interface{})
	if !ok {
		fmt.Fprintln(os.Stderr, "Invalid response from gateway")
		os.Exit(1)
	}

	fmt.Println("Last Heartbeat:")
	if timestamp, ok := data["timestamp"].(float64); ok {
		fmt.Printf("  Timestamp: %s\n", time.Unix(int64(timestamp), 0).Format(time.RFC3339))
	}
	if status, ok := data["status"].(string); ok {
		fmt.Printf("  Status: %s\n", status)
	}
}

// handleHeartbeatEnable handles enabling heartbeat
func handleHeartbeatEnable(cfg *config.Config) {
	result, err := callGatewayRPC(cfg, "heartbeat.enable", map[string]interface{}{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error enabling heartbeat: %v\n", err)
		os.Exit(1)
	}

	data, ok := result.(map[string]interface{})
	if !ok {
		fmt.Fprintln(os.Stderr, "Invalid response from gateway")
		os.Exit(1)
	}

	if status, ok := data["status"].(string); ok {
		fmt.Printf("Heartbeat enabled: %s\n", status)
	} else {
		fmt.Println("Heartbeat enabled")
	}
}

// handleHeartbeatDisable handles disabling heartbeat
func handleHeartbeatDisable(cfg *config.Config) {
	result, err := callGatewayRPC(cfg, "heartbeat.disable", map[string]interface{}{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error disabling heartbeat: %v\n", err)
		os.Exit(1)
	}

	data, ok := result.(map[string]interface{})
	if !ok {
		fmt.Fprintln(os.Stderr, "Invalid response from gateway")
		os.Exit(1)
	}

	if status, ok := data["status"].(string); ok {
		fmt.Printf("Heartbeat disabled: %s\n", status)
	} else {
		fmt.Println("Heartbeat disabled")
	}
}

// runSystemPresence handles the system presence command
func runSystemPresence(cmd *cobra.Command, args []string) {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	result, err := callGatewayRPC(cfg, "presence.list", map[string]interface{}{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing presence entries: %v\n", err)
		os.Exit(1)
	}

	data, ok := result.(map[string]interface{})
	if !ok {
		fmt.Fprintln(os.Stderr, "Invalid response from gateway")
		os.Exit(1)
	}

	entries, ok := data["entries"].([]interface{})
	if !ok {
		fmt.Println("No presence entries found")
		return
	}

	if len(entries) == 0 {
		fmt.Println("No presence entries found")
		return
	}

	fmt.Println("System Presence Entries:")

	for i, entry := range entries {
		entryMap, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}

		fmt.Printf("\n  %d. ", i+1)

		if sessionID, ok := entryMap["session_id"].(string); ok {
			fmt.Printf("Session: %s\n", sessionID)
		}

		if channel, ok := entryMap["channel"].(string); ok {
			fmt.Printf("     Channel: %s\n", channel)
		}

		if timestamp, ok := entryMap["last_seen"].(float64); ok {
			fmt.Printf("     Last Seen: %s\n", time.Unix(int64(timestamp), 0).Format(time.RFC3339))
		}

		if status, ok := entryMap["status"].(string); ok {
			fmt.Printf("     Status: %s\n", status)
		}
	}
}

// callGatewayRPC calls a gateway RPC method
func callGatewayRPC(cfg *config.Config, method string, params map[string]interface{}) (interface{}, error) {
	// Build gateway URL
	host := cfg.Gateway.Host
	if host == "" {
		host = "localhost"
	}
	port := cfg.Gateway.Port
	if port == 0 {
		// Use WebSocket port if HTTP port is not configured
		port = config.GetGatewayHTTPPort(cfg)
	}

	url := fmt.Sprintf("http://%s:%d/rpc", host, port)

	// Create JSON-RPC request
	rpcRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      fmt.Sprintf("cli-%d", time.Now().UnixNano()),
		"method":  method,
		"params":  params,
	}

	requestBody, err := json.Marshal(rpcRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send HTTP request
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(requestBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w (is the gateway running?)", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gateway returned status %d", resp.StatusCode)
	}

	// Parse response
	var rpcResponse gateway.JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if rpcResponse.Error != nil {
		return nil, fmt.Errorf("RPC error: %s", rpcResponse.Error.Message)
	}

	return rpcResponse.Result, nil
}

// publishViaGateway publishes a message via the gateway RPC
func publishViaGateway(cfg *config.Config, msg *bus.InboundMessage) error {
	params := map[string]interface{}{
		"channel":   msg.Channel,
		"sender_id": msg.SenderID,
		"chat_id":   msg.ChatID,
		"content":   msg.Content,
		"metadata":  msg.Metadata,
	}

	_, err := callGatewayRPC(cfg, "agent.publish_inbound", params)
	return err
}

// _getGatewayStatus Helper function to get gateway status (未使用，保留供将来使用)
// nolint:unused
func _getGatewayStatus(cfg *config.Config) (map[string]interface{}, error) {
	result, err := callGatewayRPC(cfg, "health", map[string]interface{}{})
	if err != nil {
		return nil, err
	}

	data, ok := result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response")
	}

	return data, nil
}

// _listGatewaySessions Helper function to list gateway sessions (未使用，保留供将来使用)
// nolint:unused
func _listGatewaySessions(cfg *config.Config) ([]map[string]interface{}, error) {
	result, err := callGatewayRPC(cfg, "sessions.list", map[string]interface{}{})
	if err != nil {
		return nil, err
	}

	// Parse sessions array
	if sessions, ok := result.([]interface{}); ok {
		output := make([]map[string]interface{}, 0, len(sessions))
		for _, sess := range sessions {
			if sessMap, ok := sess.(map[string]interface{}); ok {
				output = append(output, sessMap)
			}
		}
		return output, nil
	}

	return nil, fmt.Errorf("invalid response format")
}

// _getChannelStatus Helper function to get channel status (未使用，保留供将来使用)
// nolint:unused
func _getChannelStatus(cfg *config.Config, channelName string) (map[string]interface{}, error) {
	result, err := callGatewayRPC(cfg, "channels.status", map[string]interface{}{
		"channel": channelName,
	})
	if err != nil {
		return nil, err
	}

	data, ok := result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response")
	}

	return data, nil
}
