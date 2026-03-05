package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/internal"
	"github.com/spf13/cobra"
)

var (
	onboardAPIKey      string
	onboardBaseURL     string
	onboardModel       string
	onboardProvider    string
	onboardSkipPrompts bool
)

var onboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "Interactive setup wizard for goclaw",
	Long: `Guided setup wizard for goclaw.

This command helps you:
1. Initialize the config file and built-in skills
2. Configure your API key and model
3. Set up your workspace

Run without flags for interactive mode, or use flags for non-interactive setup.`,
	Run: runOnboard,
}

func init() {
	// Non-interactive flags
	onboardCmd.Flags().StringVarP(&onboardAPIKey, "api-key", "k", "", "API key for the provider (required in non-interactive mode)")
	onboardCmd.Flags().StringVarP(&onboardBaseURL, "base-url", "u", "", "Base URL for the provider API")
	onboardCmd.Flags().StringVarP(&onboardModel, "model", "m", "", "Model name to use")
	onboardCmd.Flags().StringVarP(&onboardProvider, "provider", "p", "openai", "Provider: openai, anthropic, or openrouter")
	onboardCmd.Flags().BoolVar(&onboardSkipPrompts, "skip-prompts", false, "Skip all prompts (use defaults)")
}

func runOnboard(cmd *cobra.Command, args []string) {
	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════════════════╗")
	fmt.Println("║                    GoClaw Onboarding                      ║")
	fmt.Println("╚════════════════════════════════════════════════════════╝")
	fmt.Println()

	// 1. Initialize config file and built-in skills
	fmt.Println("Step 1: Initializing goclaw environment...")
	goclawDir := internal.GetGoclawDir()
	fmt.Printf("  Config directory: %s\n", goclawDir)

	// Ensure config file exists
	configCreated, err := internal.EnsureConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Error: Failed to ensure config: %v\n", err)
		os.Exit(1)
	}
	if configCreated {
		fmt.Println("  ✓ Config file created")
	} else {
		fmt.Println("  ✓ Config file already exists")
	}

	// Ensure built-in skills exist
	if err := internal.EnsureBuiltinSkills(cfg.DisabledSkills); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: Failed to ensure built-in skills: %v\n", err)
	} else {
		fmt.Println("  ✓ Built-in skills ready")
	}
	fmt.Println()

	// 2. Load existing config
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 3. Interactive or non-interactive setup
	if cmd.Flags().Changed("api-key") {
		// Non-interactive mode
		if err := nonInteractiveSetup(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Interactive mode
		if err := interactiveSetup(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	// 4. Save config
	configPath := internal.GetConfigPath()
	if err := config.Save(cfg, configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to save config: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  ✓ Config saved")
	fmt.Println()

	// 5. Print summary
	printSummary(cfg)
}

func nonInteractiveSetup(cfg *config.Config) error {
	fmt.Println("Step 2: Non-interactive configuration...")

	if onboardAPIKey == "" {
		return fmt.Errorf("--api-key is required in non-interactive mode")
	}

	provider := strings.ToLower(onboardProvider)
	switch provider {
	case "openai":
		cfg.Providers.OpenAI.APIKey = onboardAPIKey
		if onboardBaseURL != "" {
			cfg.Providers.OpenAI.BaseURL = onboardBaseURL
		}
		if onboardModel != "" {
			cfg.Agents.Defaults.Model = onboardModel
		}
	case "anthropic":
		cfg.Providers.Anthropic.APIKey = onboardAPIKey
		if onboardBaseURL != "" {
			cfg.Providers.Anthropic.BaseURL = onboardBaseURL
		}
		if onboardModel != "" {
			cfg.Agents.Defaults.Model = onboardModel
		}
	case "openrouter":
		cfg.Providers.OpenRouter.APIKey = onboardAPIKey
		if onboardBaseURL != "" {
			cfg.Providers.OpenRouter.BaseURL = onboardBaseURL
		}
		if onboardModel != "" {
			cfg.Agents.Defaults.Model = onboardModel
		}
	default:
		return fmt.Errorf("invalid provider: %s (must be openai, anthropic, or openrouter)", provider)
	}

	fmt.Printf("  ✓ Provider configured: %s\n", provider)
	return nil
}

func interactiveSetup(cfg *config.Config) error {
	fmt.Println("Step 2: Interactive configuration")
	fmt.Println()

	// Check if any provider already has an API key
	hasAPIKey := cfg.Providers.OpenAI.APIKey != "" ||
		cfg.Providers.Anthropic.APIKey != "" ||
		cfg.Providers.OpenRouter.APIKey != ""

	if hasAPIKey {
		fmt.Println("  API key already configured. Press Enter to keep or enter new value:")
	} else {
		fmt.Println("  Let's configure your API key.")
		fmt.Println("  Supported providers: openai, anthropic, openrouter")
	}

	// Prompt for provider
	provider := promptString("Provider", "openai", true)
	provider = strings.ToLower(provider)

	// Prompt for API key
	apiKey := promptString("API Key", cfg.Providers.OpenAI.APIKey, true)

	// Prompt for base URL (optional)
	defaultBaseURL := ""
	switch provider {
	case "openai":
		if cfg.Providers.OpenAI.BaseURL != "" {
			defaultBaseURL = cfg.Providers.OpenAI.BaseURL
		} else {
			defaultBaseURL = "https://api.openai.com/v1"
		}
	case "anthropic":
		if cfg.Providers.Anthropic.BaseURL != "" {
			defaultBaseURL = cfg.Providers.Anthropic.BaseURL
		} else {
			defaultBaseURL = "https://api.anthropic.com"
		}
	case "openrouter":
		if cfg.Providers.OpenRouter.BaseURL != "" {
			defaultBaseURL = cfg.Providers.OpenRouter.BaseURL
		} else {
			defaultBaseURL = "https://openrouter.ai/api/v1"
		}
	}
	baseURL := promptString("Base URL (press Enter for default)", defaultBaseURL, false)

	// Prompt for model
	defaultModel := cfg.Agents.Defaults.Model
	if defaultModel == "" {
		switch provider {
		case "openai":
			defaultModel = "gpt-4o"
		case "anthropic":
			defaultModel = "claude-opus-4-5"
		case "openrouter":
			defaultModel = "anthropic/claude-opus-4-5"
		}
	}
	model := promptString("Model", defaultModel, false)

	// Apply configuration
	switch provider {
	case "openai":
		cfg.Providers.OpenAI.APIKey = apiKey
		cfg.Providers.OpenAI.BaseURL = baseURL
		cfg.Agents.Defaults.Model = model
	case "anthropic":
		cfg.Providers.Anthropic.APIKey = apiKey
		cfg.Providers.Anthropic.BaseURL = baseURL
		cfg.Agents.Defaults.Model = model
	case "openrouter":
		cfg.Providers.OpenRouter.APIKey = apiKey
		cfg.Providers.OpenRouter.BaseURL = baseURL
		cfg.Agents.Defaults.Model = model
	default:
		return fmt.Errorf("invalid provider: %s", provider)
	}

	fmt.Println("  ✓ Configuration saved")
	return nil
}

func promptString(prompt, defaultValue string, required bool) string {
	reader := bufio.NewReader(os.Stdin)

	if defaultValue != "" {
		fmt.Printf("  %s [%s]: ", prompt, defaultValue)
	} else {
		fmt.Printf("  %s: ", prompt)
	}

	input, err := reader.ReadString('\n')
	if err != nil {
		if required {
			fmt.Printf("    Error reading input, using default: %s\n", defaultValue)
		}
		return defaultValue
	}

	input = strings.TrimSpace(input)
	if input == "" {
		if defaultValue != "" {
			return defaultValue
		}
		if required {
			fmt.Printf("    Required field, using default: %s\n", defaultValue)
			return defaultValue
		}
	}

	// Mask API key in output
	if strings.Contains(strings.ToLower(prompt), "api") && strings.Contains(strings.ToLower(prompt), "key") {
		masked := maskAPIKey(input)
		fmt.Printf("    Set to: %s\n", masked)
	}

	return input
}

func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

func printSummary(cfg *config.Config) {
	fmt.Println("═════════════════════════════════════════════════════════")
	fmt.Println("                         Summary")
	fmt.Println("═════════════════════════════════════════════════════════")
	fmt.Println()

	// Provider info
	var providerName, providerAPIKey string
	if cfg.Providers.OpenAI.APIKey != "" {
		providerName = "OpenAI"
		providerAPIKey = maskAPIKey(cfg.Providers.OpenAI.APIKey)
	} else if cfg.Providers.Anthropic.APIKey != "" {
		providerName = "Anthropic"
		providerAPIKey = maskAPIKey(cfg.Providers.Anthropic.APIKey)
	} else if cfg.Providers.OpenRouter.APIKey != "" {
		providerName = "OpenRouter"
		providerAPIKey = maskAPIKey(cfg.Providers.OpenRouter.APIKey)
	}

	if providerName != "" {
		fmt.Printf("  Provider:  %s\n", providerName)
		fmt.Printf("  API Key:   %s\n", providerAPIKey)
	}

	fmt.Printf("  Model:     %s\n", cfg.Agents.Defaults.Model)

	// Workspace path
	workspacePath, _ := config.GetWorkspacePath(cfg)
	fmt.Printf("  Workspace: %s\n", workspacePath)

	// Gateway info
	fmt.Printf("  Gateway:   http://%s:%d\n", cfg.Gateway.Host, cfg.Gateway.Port)

	fmt.Println()
	fmt.Println("═════════════════════════════════════════════════════════")
	fmt.Println("                     Next Steps")
	fmt.Println("═════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("  1. Start goclaw:")
	fmt.Println("     $ goclaw start")
	fmt.Println()
	fmt.Println("  2. Connect via HTTP:")
	fmt.Printf("     $ curl http://localhost:%d/health\n", cfg.Gateway.Port)
	fmt.Println()
	fmt.Println("  3. Connect via WebSocket:")
	fmt.Printf("     ws://localhost:%d/ws\n", cfg.Gateway.Port)
	fmt.Println()
	fmt.Println("  4. View configuration:")
	fmt.Printf("     $ cat %s\n", internal.GetConfigPath())
	fmt.Println()
	fmt.Println("  5. List available skills:")
	fmt.Println("     $ goclaw skills list")
	fmt.Println()
	fmt.Println("For more information, visit: https://github.com/smallnest/goclaw")
	fmt.Println()
}
