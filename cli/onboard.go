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
	onboardCmd.Flags().StringVarP(&onboardProvider, "provider", "p", "qianfan", "Provider name (e.g., qianfan, openai, anthropic, openrouter, requesty)")
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
	if err := internal.EnsureBuiltinSkills(); err != nil {
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

	// Set up provider in models.providers format
	if cfg.Models.Providers == nil {
		cfg.Models.Providers = make(map[string]config.ModelProviderConfig)
	}

	// Get default base URL
	baseURL := onboardBaseURL
	if baseURL == "" {
		baseURL = getDefaultBaseURL(provider)
	}

	// Create model config
	modelID := onboardModel
	if modelID == "" {
		modelID = getDefaultModel(provider)
	}

	modelConfig := config.ModelDefinitionConfig{
		ID:            modelID,
		Name:          modelID,
		ContextWindow: 128000,
		MaxTokens:     8192,
		Input:         []string{"text", "image"},
	}

	// Set API type based on provider
	apiType := config.ModelAPIOpenAICompletions
	if provider == "anthropic" {
		apiType = config.ModelAPIAnthropicMessages
	}

	cfg.Models.Providers[provider] = config.ModelProviderConfig{
		BaseURL: baseURL,
		APIKey:  onboardAPIKey,
		API:     apiType,
		Models:  []config.ModelDefinitionConfig{modelConfig},
	}

	// Set default model
	cfg.Agents.Defaults.Model.Primary = fmt.Sprintf("%s/%s", provider, modelID)

	fmt.Printf("  ✓ Provider configured: %s\n", provider)
	return nil
}

func interactiveSetup(cfg *config.Config) error {
	fmt.Println("Step 2: Interactive configuration")
	fmt.Println()

	// Check if any provider already has an API key
	hasProvider := cfg.Models.HasProviders()

	if hasProvider {
		fmt.Println("  Provider already configured. Press Enter to keep or enter new value:")
	} else {
		fmt.Println("  Let's configure your LLM provider.")
		fmt.Println("  Supported providers:")
		fmt.Println("    - qianfan (百度千帆)")
		fmt.Println("    - openai")
		fmt.Println("    - anthropic")
		fmt.Println("    - openrouter")
		fmt.Println("    - requesty")
		fmt.Println("    - ollama (本地)")
		fmt.Println("    - google / google-vertex (Gemini)")
		fmt.Println("    - Or any OpenAI-compatible provider (e.g., deepseek, moonshot, grok, chutes)")
	}

	// Prompt for provider
	defaultProvider := "qianfan"
	if hasProvider {
		// Get first configured provider
		for name := range cfg.Models.Providers {
			defaultProvider = name
			break
		}
	}
	provider := promptString("Provider", defaultProvider, true)
	provider = strings.ToLower(provider)

	// Get existing provider config if exists
	existingProvider := cfg.Models.Providers[provider]

	// Prompt for API key
	defaultAPIKey := ""
	if existingProvider.APIKey != "" {
		defaultAPIKey = existingProvider.APIKey
	}
	apiKey := promptString("API Key (or env var name like QIANFAN_API_KEY)", defaultAPIKey, true)

	// Prompt for base URL (optional)
	defaultBaseURL := existingProvider.BaseURL
	if defaultBaseURL == "" {
		defaultBaseURL = getDefaultBaseURL(provider)
	}
	baseURL := promptString("Base URL (press Enter for default)", defaultBaseURL, false)

	// Prompt for model
	defaultModel := cfg.Agents.Defaults.Model.Effective()
	hasProviderPrefix := strings.HasPrefix(defaultModel, provider+":") || strings.HasPrefix(defaultModel, provider+"/")
	if defaultModel == "" || !hasProviderPrefix {
		defaultModel = getDefaultModel(provider)
	} else {
		// Strip provider prefix for display
		if strings.HasPrefix(defaultModel, provider+":") {
			defaultModel = strings.TrimPrefix(defaultModel, provider+":")
		} else {
			defaultModel = strings.TrimPrefix(defaultModel, provider+"/")
		}
	}
	modelID := promptString("Model ID", defaultModel, false)

	// Create model config
	modelConfig := config.ModelDefinitionConfig{
		ID:            modelID,
		Name:          modelID,
		ContextWindow: 128000,
		MaxTokens:     8192,
		Input:         []string{"text", "image"},
	}

	// Set API type based on provider
	apiType := config.ModelAPIOpenAICompletions
	if provider == "anthropic" {
		apiType = config.ModelAPIAnthropicMessages
	}

	// Initialize providers map if needed
	if cfg.Models.Providers == nil {
		cfg.Models.Providers = make(map[string]config.ModelProviderConfig)
	}

	// Apply configuration
	cfg.Models.Providers[provider] = config.ModelProviderConfig{
		BaseURL: baseURL,
		APIKey:  apiKey,
		API:     apiType,
		Models:  []config.ModelDefinitionConfig{modelConfig},
	}

	// Set default model with provider prefix
	cfg.Agents.Defaults.Model.Primary = fmt.Sprintf("%s/%s", provider, modelID)

	fmt.Println("  ✓ Configuration saved")
	return nil
}

func getDefaultBaseURL(provider string) string {
	switch provider {
	case "openai":
		return "https://api.openai.com/v1"
	case "qianfan":
		return "https://qianfan.baidubce.com/v2"
	case "anthropic":
		return "https://api.anthropic.com"
	case "openrouter":
		return "https://openrouter.ai/api/v1"
	case "requesty":
		return "https://router.requesty.ai/v1"
	case "ollama":
		return "http://localhost:11434/v1"
	default:
		return ""
	}
}

func getDefaultModel(provider string) string {
	switch provider {
	case "openai":
		return "gpt-4o"
	case "qianfan":
		return "kimi-k2.5"
	case "anthropic":
		return "claude-sonnet-4-20250514"
	case "openrouter":
		return "anthropic/claude-sonnet-4"
	case "requesty":
		return "anthropic/claude-sonnet-4-5"
	case "ollama":
		return "llama3.2"
	default:
		return ""
	}
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
	for name, provider := range cfg.Models.Providers {
		if provider.APIKey != "" {
			providerName = name
			providerAPIKey = maskAPIKey(provider.APIKey)
			break
		}
	}

	if providerName != "" {
		fmt.Printf("  Provider:  %s\n", providerName)
		fmt.Printf("  API Key:   %s\n", providerAPIKey)
	}

	fmt.Printf("  Model:     %s\n", cfg.Agents.Defaults.Model.Effective())

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
