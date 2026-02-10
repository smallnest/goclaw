package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var approvalsCmd = &cobra.Command{
	Use:   "approvals",
	Short: "Approval management",
}

var approvalsGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get approval settings",
	Run:   runApprovalsGet,
}

var approvalsSetCmd = &cobra.Command{
	Use:   "set <behavior>",
	Short: "Set approval behavior (auto, manual, prompt)",
	Args:  cobra.ExactArgs(1),
	Run:   runApprovalsSet,
}

var approvalsAllowlistCmd = &cobra.Command{
	Use:   "allowlist",
	Short: "Manage approval allowlist",
}

var approvalsAllowlistAddCmd = &cobra.Command{
	Use:   "add <tool>",
	Short: "Add a tool to the approval allowlist",
	Args:  cobra.ExactArgs(1),
	Run:   runApprovalsAllowlistAdd,
}

var approvalsAllowlistRemoveCmd = &cobra.Command{
	Use:   "remove <tool>",
	Short: "Remove a tool from the approval allowlist",
	Args:  cobra.ExactArgs(1),
	Run:   runApprovalsAllowlistRemove,
}

func init() {
	// Register approvals commands
	rootCmd.AddCommand(approvalsCmd)
	approvalsCmd.AddCommand(approvalsGetCmd)
	approvalsCmd.AddCommand(approvalsSetCmd)
	approvalsCmd.AddCommand(approvalsAllowlistCmd)
	approvalsAllowlistCmd.AddCommand(approvalsAllowlistAddCmd)
	approvalsAllowlistCmd.AddCommand(approvalsAllowlistRemoveCmd)
}

// ApprovalsConfig represents the approval configuration
type ApprovalsConfig struct {
	Behavior             string   `yaml:"behavior" json:"behavior"`
	Allowlist            []string `yaml:"allowlist" json:"allowlist"`
	AskForDangerousTools bool     `yaml:"ask_for_dangerous_tools" json:"ask_for_dangerous_tools"`
}

// runApprovalsGet handles the approvals get command
func runApprovalsGet(cmd *cobra.Command, args []string) {
	cfg, err := loadApprovalsConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Approval Settings:")
	fmt.Printf("  Behavior: %s\n", cfg.Behavior)
	fmt.Printf("  Allowlist: %v\n", cfg.Allowlist)
	fmt.Printf("  Ask for dangerous tools: %t\n", cfg.AskForDangerousTools)
}

// runApprovalsSet handles the approvals set command
func runApprovalsSet(cmd *cobra.Command, args []string) {
	behavior := args[0]

	if behavior != "auto" && behavior != "manual" && behavior != "prompt" {
		fmt.Fprintf(os.Stderr, "Invalid behavior. Valid options: auto, manual, prompt\n")
		os.Exit(1)
	}

	cfg, err := loadApprovalsConfig()
	if err != nil {
		// Create default config if it doesn't exist
		cfg = &ApprovalsConfig{
			Behavior:             "manual",
			Allowlist:            []string{},
			AskForDangerousTools: true,
		}
	}

	cfg.Behavior = behavior

	if err := saveApprovalsConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Approval behavior set to: %s\n", behavior)
}

// runApprovalsAllowlistAdd handles the approvals allowlist add command
func runApprovalsAllowlistAdd(cmd *cobra.Command, args []string) {
	tool := args[0]

	cfg, err := loadApprovalsConfig()
	if err != nil {
		// Create default config if it doesn't exist
		cfg = &ApprovalsConfig{
			Behavior:             "manual",
			Allowlist:            []string{},
			AskForDangerousTools: true,
		}
	}

	// Check if tool is already in allowlist
	for _, t := range cfg.Allowlist {
		if t == tool {
			fmt.Printf("Tool '%s' is already in the allowlist\n", tool)
			return
		}
	}

	cfg.Allowlist = append(cfg.Allowlist, tool)

	if err := saveApprovalsConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Added '%s' to approval allowlist\n", tool)
}

// runApprovalsAllowlistRemove handles the approvals allowlist remove command
func runApprovalsAllowlistRemove(cmd *cobra.Command, args []string) {
	tool := args[0]

	cfg, err := loadApprovalsConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Find and remove the tool
	found := false
	newAllowlist := make([]string, 0, len(cfg.Allowlist))
	for _, t := range cfg.Allowlist {
		if t == tool {
			found = true
			continue
		}
		newAllowlist = append(newAllowlist, t)
	}

	if !found {
		fmt.Printf("Tool '%s' is not in the allowlist\n", tool)
		return
	}

	cfg.Allowlist = newAllowlist

	if err := saveApprovalsConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Removed '%s' from approval allowlist\n", tool)
}

// getApprovalsConfigPath returns the path to the approvals config file
func getApprovalsConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".goclaw", "approvals.yaml"), nil
}

// loadApprovalsConfig loads the approvals configuration
func loadApprovalsConfig() (*ApprovalsConfig, error) {
	configPath, err := getApprovalsConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var cfg ApprovalsConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Set defaults if not set
	if cfg.Behavior == "" {
		cfg.Behavior = "manual"
	}

	return &cfg, nil
}

// saveApprovalsConfig saves the approvals configuration
func saveApprovalsConfig(cfg *ApprovalsConfig) error {
	configPath, err := getApprovalsConfigPath()
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	// Ensure directory exists
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}
