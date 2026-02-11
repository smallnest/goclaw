package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"

	"github.com/smallnest/dogclaw/goclaw/config"
	"github.com/spf13/cobra"
)

var agentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "Manage isolated agents",
	Long:  `Manage isolated agents with their own workspaces and configurations.`,
}

var agentsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all agents",
	Run:   runAgentsList,
}

var agentsAddCmd = &cobra.Command{
	Use:   "add [name]",
	Short: "Add a new agent",
	Args:  cobra.MaximumNArgs(1),
	Run:   runAgentsAdd,
}

var agentsDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete an agent",
	Args:  cobra.ExactArgs(1),
	Run:   runAgentsDelete,
}

// Flags for agents list
var agentsListJSON bool
var agentsListBindings bool

// Flags for agents add
var agentsAddWorkspace string
var agentsAddModel string
var agentsAddAgentDir string
var agentsAddBind []string
var agentsAddNonInteractive bool
var agentsAddJSON bool

// Flags for agents delete
var agentsDeleteForce bool
var agentsDeleteJSON bool

func init() {
	// List flags
	agentsListCmd.Flags().BoolVar(&agentsListJSON, "json", false, "Output in JSON format")
	agentsListCmd.Flags().BoolVar(&agentsAddNonInteractive, "bindings", false, "Show channel bindings")

	// Add flags
	agentsAddCmd.Flags().StringVar(&agentsAddWorkspace, "workspace", "", "Workspace directory for the agent")
	agentsAddCmd.Flags().StringVar(&agentsAddModel, "model", "", "Model to use for the agent")
	agentsAddCmd.Flags().StringVar(&agentsAddAgentDir, "agent-dir", "", "Directory containing agent definitions")
	agentsAddCmd.Flags().StringSliceVar(&agentsAddBind, "bind", []string{}, "Bind agent to channels (e.g., telegram:123456)")
	agentsAddCmd.Flags().BoolVar(&agentsAddNonInteractive, "non-interactive", false, "Run in non-interactive mode")
	agentsAddCmd.Flags().BoolVar(&agentsAddJSON, "json", false, "Output in JSON format")

	// Delete flags
	agentsDeleteCmd.Flags().BoolVar(&agentsDeleteForce, "force", false, "Force deletion without confirmation")
	agentsDeleteCmd.Flags().BoolVar(&agentsDeleteJSON, "json", false, "Output in JSON format")

	agentsCmd.AddCommand(agentsListCmd)
	agentsCmd.AddCommand(agentsAddCmd)
	agentsCmd.AddCommand(agentsDeleteCmd)
}

// AgentInfo represents agent configuration information
type AgentInfo struct {
	Name       string            `json:"name"`
	Workspace  string            `json:"workspace"`
	Model      string            `json:"model"`
	AgentDir   string            `json:"agent_dir,omitempty"`
	Bindings   []string          `json:"bindings,omitempty"`
	ConfigPath string            `json:"config_path"`
	CreatedAt  string            `json:"created_at,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// runAgentsList lists all configured agents
func runAgentsList(cmd *cobra.Command, args []string) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting home directory: %v\n", err)
		os.Exit(1)
	}

	agentsDir := filepath.Join(homeDir, ".goclaw", "agents")
	agents, err := loadAgents(agentsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading agents: %v\n", err)
		os.Exit(1)
	}

	if agentsListJSON {
		data, err := json.MarshalIndent(agents, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
		return
	}

	if len(agents) == 0 {
		fmt.Println("No agents found.")
		fmt.Println("\nCreate a new agent with: goclaw agents add [name]")
		return
	}

	// Sort agents by name
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].Name < agents[j].Name
	})

	// Display in table format
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "NAME\tWORKSPACE\tMODEL\tBINDINGS\n")
	fmt.Fprintf(w, "----\t---------\t-----\t--------\n")
	for _, agent := range agents {
		bindings := ""
		if agentsListBindings && len(agent.Bindings) > 0 {
			bindings = fmt.Sprintf("%v", agent.Bindings)
		} else if len(agent.Bindings) > 0 {
			bindings = fmt.Sprintf("[%d]", len(agent.Bindings))
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", agent.Name, agent.Workspace, agent.Model, bindings)
	}
	w.Flush()
}

// runAgentsAdd adds a new agent
func runAgentsAdd(cmd *cobra.Command, args []string) {
	var name string

	if len(args) > 0 {
		name = args[0]
	} else if !agentsAddNonInteractive {
		// Prompt for name if not provided and in interactive mode
		fmt.Print("Enter agent name: ")
		_, _ = fmt.Scanln(&name)
	}

	if name == "" {
		fmt.Fprintf(os.Stderr, "Error: Agent name is required\n")
		os.Exit(1)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting home directory: %v\n", err)
		os.Exit(1)
	}

	agentsDir := filepath.Join(homeDir, ".goclaw", "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating agents directory: %v\n", err)
		os.Exit(1)
	}

	// Check if agent already exists
	agentConfigPath := filepath.Join(agentsDir, name+".json")
	if _, err := os.Stat(agentConfigPath); err == nil {
		fmt.Fprintf(os.Stderr, "Error: Agent '%s' already exists\n", name)
		os.Exit(1)
	}

	// Load default configuration
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not load config: %v\n", err)
		cfg = &config.Config{}
	}

	// Set defaults
	workspace := agentsAddWorkspace
	if workspace == "" {
		workspace = filepath.Join(homeDir, ".goclaw", "workspaces", name)
	}

	model := agentsAddModel
	if model == "" {
		model = cfg.Agents.Defaults.Model
		if model == "" {
			model = "gpt-4"
		}
	}

	// Create agent info
	agent := &AgentInfo{
		Name:       name,
		Workspace:  workspace,
		Model:      model,
		AgentDir:   agentsAddAgentDir,
		Bindings:   agentsAddBind,
		ConfigPath: agentConfigPath,
		Metadata:   make(map[string]string),
	}

	// Save agent configuration
	if err := saveAgent(agent); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving agent: %v\n", err)
		os.Exit(1)
	}

	// Create workspace directory
	if err := os.MkdirAll(workspace, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not create workspace: %v\n", err)
	}

	if agentsAddJSON {
		data, err := json.MarshalIndent(agent, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
	} else {
		fmt.Printf("Agent '%s' created successfully\n", name)
		fmt.Printf("  Workspace: %s\n", workspace)
		fmt.Printf("  Model: %s\n", model)
		if len(agentsAddBind) > 0 {
			fmt.Printf("  Bindings: %v\n", agentsAddBind)
		}
	}
}

// runAgentsDelete deletes an agent
func runAgentsDelete(cmd *cobra.Command, args []string) {
	name := args[0]

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting home directory: %v\n", err)
		os.Exit(1)
	}

	agentsDir := filepath.Join(homeDir, ".goclaw", "agents")
	agentConfigPath := filepath.Join(agentsDir, name+".json")

	// Check if agent exists
	if _, err := os.Stat(agentConfigPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: Agent '%s' not found\n", name)
		os.Exit(1)
	}

	// Confirm deletion unless force flag is set
	if !agentsDeleteForce && !agentsDeleteJSON {
		fmt.Printf("Are you sure you want to delete agent '%s'? [y/N]: ", name)
		var confirm string
		_, _ = fmt.Scanln(&confirm)
		if confirm != "y" && confirm != "Y" {
			fmt.Println("Deletion cancelled")
			return
		}
	}

	// Delete agent configuration
	if err := os.Remove(agentConfigPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting agent: %v\n", err)
		os.Exit(1)
	}

	if agentsDeleteJSON {
		result := map[string]interface{}{
			"status":  "deleted",
			"name":    name,
			"success": true,
		}
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
	} else {
		fmt.Printf("Agent '%s' deleted successfully\n", name)
		fmt.Println("Note: Workspace files are preserved. To remove them, delete the workspace directory manually.")
	}
}

// loadAgents loads all agent configurations from the agents directory
func loadAgents(agentsDir string) ([]*AgentInfo, error) {
	var agents []*AgentInfo

	// Ensure directory exists
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		return nil, err
	}

	// Read directory
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		agentPath := filepath.Join(agentsDir, entry.Name())
		agent, err := loadAgent(agentPath)
		if err != nil {
			// Skip invalid agent files
			continue
		}
		agents = append(agents, agent)
	}

	return agents, nil
}

// loadAgent loads a single agent configuration
func loadAgent(path string) (*AgentInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var agent AgentInfo
	if err := json.Unmarshal(data, &agent); err != nil {
		return nil, err
	}

	agent.ConfigPath = path
	return &agent, nil
}

// saveAgent saves an agent configuration
func saveAgent(agent *AgentInfo) error {
	data, err := json.MarshalIndent(agent, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(agent.ConfigPath, data, 0644)
}
