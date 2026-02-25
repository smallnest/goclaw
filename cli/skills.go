package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/smallnest/goclaw/agent"
	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/internal"
	"github.com/smallnest/goclaw/internal/logger"
	"github.com/smallnest/goclaw/providers"
	"github.com/spf13/cobra"
)

var skillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "Skills management",
}

var skillsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all discovered skills",
	Run:   runSkillsList,
}

var skillsListVerbose bool

var skillsValidateCmd = &cobra.Command{
	Use:   "validate [skill-name]",
	Short: "Validate skill dependencies",
	Args:  cobra.ExactArgs(1),
	Run:   runSkillsValidate,
}

var skillsTestCmd = &cobra.Command{
	Use:   "test [skill-name] --prompt \"test prompt\"",
	Short: "Test a skill with a prompt",
	Args:  cobra.ExactArgs(1),
	Run:   runSkillsTest,
}

var skillsTestPrompt string

var skillsInstallCmd = &cobra.Command{
	Use:   "install [url|path]",
	Short: "Install a skill from URL or local path",
	Args:  cobra.ExactArgs(1),
	Run:   runSkillsInstall,
}

var skillsUpdateCmd = &cobra.Command{
	Use:   "update [skill-name]",
	Short: "Update an installed skill",
	Args:  cobra.ExactArgs(1),
	Run:   runSkillsUpdate,
}

var skillsUninstallCmd = &cobra.Command{
	Use:   "uninstall [skill-name]",
	Short: "Uninstall an installed skill",
	Args:  cobra.ExactArgs(1),
	Run:   runSkillsUninstall,
}

var skillsConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Skills configuration management",
}

var skillsConfigShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show skills configuration",
	Run:   runSkillsConfigShow,
}

var skillsConfigSetCmd = &cobra.Command{
	Use:   "set [key] [value]",
	Short: "Set a configuration value (e.g., 'disabled.skill-name' or 'env.skill-name.KEY=value')",
	Args:  cobra.ExactArgs(2),
	Run:   runSkillsConfigSet,
}

var skillsInstallDepsCmd = &cobra.Command{
	Use:   "install-deps [skill-name]",
	Short: "Install dependencies for a skill",
	Args:  cobra.ExactArgs(1),
	Run:   runSkillsInstallDeps,
}

var skillsSearchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search for skills by name, description, or keywords",
	Args:  cobra.MinimumNArgs(1),
	Run:   runSkillsSearch,
}

func init() {
	rootCmd.AddCommand(skillsCmd)

	// list 命令
	skillsListCmd.Flags().BoolVarP(&skillsListVerbose, "verbose", "v", false, "Show detailed information including prompt content")
	skillsCmd.AddCommand(skillsListCmd)

	// search 命令
	skillsCmd.AddCommand(skillsSearchCmd)

	// validate 命令
	skillsCmd.AddCommand(skillsValidateCmd)

	// test 命令
	skillsTestCmd.Flags().StringVar(&skillsTestPrompt, "prompt", "", "Test prompt to use")
	_ = skillsTestCmd.MarkFlagRequired("prompt")
	skillsCmd.AddCommand(skillsTestCmd)

	// install 命令
	skillsCmd.AddCommand(skillsInstallCmd)

	// update 命令
	skillsCmd.AddCommand(skillsUpdateCmd)

	// uninstall 命令
	skillsCmd.AddCommand(skillsUninstallCmd)

	// config 命令
	skillsConfigCmd.AddCommand(skillsConfigShowCmd)
	skillsConfigCmd.AddCommand(skillsConfigSetCmd)
	skillsCmd.AddCommand(skillsConfigCmd)

	// install-deps 命令
	skillsCmd.AddCommand(skillsInstallDepsCmd)
}

func runSkillsList(cmd *cobra.Command, args []string) {
	// 确保内置技能被复制到用户目录
	if err := internal.EnsureBuiltinSkills(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to ensure builtin skills: %v\n", err)
	}

	// 加载配置
	_, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to load config: %v\n", err)
	}

	// 初始化日志
	if err := logger.Init("warn", false); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = logger.Sync() }()

	// 创建技能加载器
	// 加载顺序（后加载的同名技能会覆盖前面的）：
	// 1. ./skills/ (当前目录，最高优先级)
	// 2. ${WORKSPACE}/skills/ (工作区目录)
	// 3. ~/.goclaw/skills/ (用户全局目录)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get home directory: %v\n", err)
		os.Exit(1)
	}
	goclawDir := homeDir + "/.goclaw"
	globalSkillsDir := goclawDir + "/skills"
	workspaceSkillsDir := goclawDir + "/workspace/skills"
	currentSkillsDir := "./skills"

	skillsLoader := agent.NewSkillsLoader(goclawDir, []string{
		globalSkillsDir,    // 最先加载（最低优先级）
		workspaceSkillsDir, // 其次加载
		currentSkillsDir,   // 最后加载（最高优先级）
	})
	if err := skillsLoader.Discover(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to discover skills: %v\n", err)
		os.Exit(1)
	}

	skills := skillsLoader.List()
	if len(skills) == 0 {
		fmt.Println("No skills found.")
		return
	}

	fmt.Printf("Found %d skills:\n\n", len(skills))
	for _, skill := range skills {
		fmt.Printf("📦 %s\n", skill.Name)
		if skill.Description != "" {
			fmt.Printf("   %s\n", skill.Description)
		}

		// 显示元数据信息
		emoji := skill.Metadata.OpenClaw.Emoji
		if emoji != "" {
			fmt.Printf("   Icon: %s\n", emoji)
		}

		requires := skill.Metadata.OpenClaw.Requires
		if len(requires.Bins) > 0 {
			fmt.Printf("   Requires: %v\n", requires.Bins)
		}
		if len(requires.AnyBins) > 0 {
			fmt.Printf("   Requires (any): %v\n", requires.AnyBins)
		}
		if len(requires.Env) > 0 {
			fmt.Printf("   Env: %v\n", requires.Env)
		}
		if len(requires.OS) > 0 {
			fmt.Printf("   OS: %v\n", requires.OS)
		}

		// 详细模式：显示 Prompt 内容
		if skillsListVerbose {
			fmt.Printf("\n   --- Content ---\n")
			content := strings.TrimSpace(skill.Content)
			lines := strings.Split(content, "\n")
			for _, line := range lines {
				fmt.Printf("   %s\n", line)
			}
		}

		fmt.Println()
	}
}

func runSkillsValidate(cmd *cobra.Command, args []string) {
	skillName := args[0]

	// 加载配置
	_, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to load config: %v\n", err)
	}

	// 初始化日志
	if err := logger.Init("warn", false); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = logger.Sync() }()

	// 创建技能加载器
	// 加载顺序（后加载的同名技能会覆盖前面的）：
	// 1. ./skills/ (当前目录，最高优先级)
	// 2. ${WORKSPACE}/skills/ (工作区目录)
	// 3. ~/.goclaw/skills/ (用户全局目录)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get home directory: %v\n", err)
		os.Exit(1)
	}
	goclawDir := homeDir + "/.goclaw"
	globalSkillsDir := goclawDir + "/skills"
	workspaceSkillsDir := goclawDir + "/workspace/skills"
	currentSkillsDir := "./skills"

	skillsLoader := agent.NewSkillsLoader(goclawDir, []string{
		globalSkillsDir,    // 最先加载（最低优先级）
		workspaceSkillsDir, // 其次加载
		currentSkillsDir,   // 最后加载（最高优先级）
	})
	if err := skillsLoader.Discover(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to discover skills: %v\n", err)
		os.Exit(1)
	}

	skill, ok := skillsLoader.Get(skillName)
	if !ok {
		fmt.Printf("❌ Skill '%s' not found\n", skillName)
		os.Exit(1)
	}

	fmt.Printf("Validating skill: %s\n\n", skillName)

	// 检查二进制依赖
	allValid := true
	if len(skill.Metadata.OpenClaw.Requires.Bins) > 0 {
		fmt.Println("Binary dependencies:")
		for _, bin := range skill.Metadata.OpenClaw.Requires.Bins {
			path, err := exec.LookPath(bin)
			if err != nil {
				fmt.Printf("  ❌ %s: NOT FOUND\n", bin)
				allValid = false
			} else {
				fmt.Printf("  ✅ %s: %s\n", bin, path)
			}
		}
	}

	// 检查 AnyBins
	if len(skill.Metadata.OpenClaw.Requires.AnyBins) > 0 {
		fmt.Println("\nBinary dependencies (any):")
		foundAny := false
		for _, bin := range skill.Metadata.OpenClaw.Requires.AnyBins {
			path, err := exec.LookPath(bin)
			if err == nil {
				fmt.Printf("  ✅ %s: %s\n", bin, path)
				foundAny = true
			} else {
				fmt.Printf("  ⚠️  %s: NOT FOUND\n", bin)
			}
		}
		if !foundAny {
			fmt.Println("  ❌ No required binary found")
			allValid = false
		}
	}

	// 检查环境变量
	if len(skill.Metadata.OpenClaw.Requires.Env) > 0 {
		fmt.Println("\nEnvironment variables:")
		for _, env := range skill.Metadata.OpenClaw.Requires.Env {
			val := os.Getenv(env)
			if val == "" {
				fmt.Printf("  ❌ %s: NOT SET\n", env)
				allValid = false
			} else {
				fmt.Printf("  ✅ %s: %s\n", env, maskSecret(env, val))
			}
		}
	}

	// 检查配置依赖
	if len(skill.Metadata.OpenClaw.Requires.Config) > 0 {
		fmt.Println("\nConfig dependencies:")
		cfg, _ := config.Load("")
		for _, configKey := range skill.Metadata.OpenClaw.Requires.Config {
			// 简单检查：查看配置是否加载成功
			if cfg != nil {
				fmt.Printf("  ✅ %s: Config loaded\n", configKey)
			} else {
				fmt.Printf("  ❌ %s: Config not loaded\n", configKey)
				allValid = false
			}
		}
	}

	fmt.Println()
	if allValid {
		fmt.Println("✅ All dependencies satisfied!")
	} else {
		fmt.Println("❌ Some dependencies are missing!")
		os.Exit(1)
	}
}

// maskSecret 隐藏敏感环境变量的值
func maskSecret(key, value string) string {
	secretKeys := []string{"KEY", "TOKEN", "SECRET", "PASSWORD"}
	upperKey := strings.ToUpper(key)
	for _, suffix := range secretKeys {
		if strings.HasSuffix(upperKey, suffix) {
			if len(value) <= 4 {
				return "****"
			}
			return value[:2] + "****" + value[len(value)-2:]
		}
	}
	return value
}

func runSkillsTest(cmd *cobra.Command, args []string) {
	skillName := args[0]

	if skillsTestPrompt == "" {
		fmt.Fprintf(os.Stderr, "Error: --prompt is required\n")
		os.Exit(1)
	}

	// 加载配置
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 初始化日志
	if err := logger.Init("warn", false); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = logger.Sync() }()

	// 创建技能加载器
	// 加载顺序（后加载的同名技能会覆盖前面的）：
	// 1. ./skills/ (当前目录，最高优先级)
	// 2. ${WORKSPACE}/skills/ (工作区目录)
	// 3. ~/.goclaw/skills/ (用户全局目录)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get home directory: %v\n", err)
		os.Exit(1)
	}
	goclawDir := homeDir + "/.goclaw"
	globalSkillsDir := goclawDir + "/skills"
	workspaceSkillsDir := goclawDir + "/workspace/skills"
	currentSkillsDir := "./skills"

	skillsLoader := agent.NewSkillsLoader(goclawDir, []string{
		globalSkillsDir,    // 最先加载（最低优先级）
		workspaceSkillsDir, // 其次加载
		currentSkillsDir,   // 最后加载（最高优先级）
	})
	if err := skillsLoader.Discover(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to discover skills: %v\n", err)
		os.Exit(1)
	}

	skill, ok := skillsLoader.Get(skillName)
	if !ok {
		fmt.Printf("❌ Skill '%s' not found\n", skillName)
		os.Exit(1)
	}

	fmt.Printf("Testing skill: %s\n", skillName)
	fmt.Printf("Prompt: %s\n\n", skillsTestPrompt)

	// 创建 LLM 提供商
	provider, err := providers.NewProvider(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create LLM provider: %v\n", err)
		os.Exit(1)
	}
	defer provider.Close()

	// 构建仅包含该技能的测试 prompt
	systemPrompt := fmt.Sprintf(`You are testing the '%s' skill.

## Skill: %s

%s

## User Request
%s

Please respond as if you were using this skill to handle the user's request. Show your reasoning process.
`, skillName, skillName, skill.Content, skillsTestPrompt)

	// 调用 LLM
	ctx := context.Background()
	messages := []providers.Message{
		{Role: "system", Content: systemPrompt},
	}

	response, err := provider.Chat(ctx, messages, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "LLM call failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("=== LLM Response ===")
	fmt.Println(response.Content)
}

func runSkillsInstall(cmd *cobra.Command, args []string) {
	source := args[0]

	// 加载配置
	_, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to load config: %v\n", err)
	}

	// 初始化日志
	if err := logger.Init("warn", false); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = logger.Sync() }()

	// 目标目录
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get home directory: %v\n", err)
		os.Exit(1)
	}
	userSkillsDir := homeDir + "/.goclaw/skills"
	if err := os.MkdirAll(userSkillsDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create skills directory: %v\n", err)
		os.Exit(1)
	}

	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		// 从 Git 仓库安装
		fmt.Printf("Installing from URL: %s\n", source)

		// 提取仓库名
		parts := strings.Split(source, "/")
		repoName := strings.TrimSuffix(parts[len(parts)-1], ".git")
		targetPath := filepath.Join(userSkillsDir, repoName)

		// 检查是否已存在
		if _, err := os.Stat(targetPath); err == nil {
			fmt.Printf("⚠️  Skill already exists at %s\n", targetPath)
			fmt.Print("Overwrite? (y/N): ")
			var confirm string
			_, _ = fmt.Scanln(&confirm)
			if strings.ToLower(confirm) != "y" {
				fmt.Println("Installation cancelled.")
				return
			}
			os.RemoveAll(targetPath)
		}

		// 克隆仓库
		fmt.Printf("Cloning to %s...\n", targetPath)
		gitCmd := exec.Command("git", "clone", source, targetPath)
		gitCmd.Stdout = os.Stdout
		gitCmd.Stderr = os.Stderr
		if err := gitCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to clone repository: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✅ Skill installed to %s\n", targetPath)
	} else {
		// 从本地目录安装
		fmt.Printf("Installing from local path: %s\n", source)

		// 解析源路径
		sourcePath, err := filepath.Abs(source)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to resolve path: %v\n", err)
			os.Exit(1)
		}

		// 检查源路径是否存在
		if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Source path does not exist: %s\n", sourcePath)
			os.Exit(1)
		}

		// 获取技能目录名
		skillName := filepath.Base(sourcePath)
		targetPath := filepath.Join(userSkillsDir, skillName)

		// 检查是否已存在
		if _, err := os.Stat(targetPath); err == nil {
			fmt.Printf("⚠️  Skill already exists at %s\n", targetPath)
			fmt.Print("Overwrite? (y/N): ")
			var confirm string
			_, _ = fmt.Scanln(&confirm)
			if strings.ToLower(confirm) != "y" {
				fmt.Println("Installation cancelled.")
				return
			}
			os.RemoveAll(targetPath)
		}

		// 复制目录
		fmt.Printf("Copying to %s...\n", targetPath)
		if err := copyDir(sourcePath, targetPath); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to copy directory: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✅ Skill installed to %s\n", targetPath)
	}
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get the relative path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		// Skip the root directory
		if relPath == "." {
			return nil
		}

		// Build destination path
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			// Create directory
			return os.MkdirAll(dstPath, info.Mode())
		}

		// Copy file
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, info.Mode())
	})
}

func runSkillsUpdate(cmd *cobra.Command, args []string) {
	skillName := args[0]

	// 初始化日志
	if err := logger.Init("warn", false); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = logger.Sync() }()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get home directory: %v\n", err)
		os.Exit(1)
	}
	userSkillsDir := homeDir + "/.goclaw/skills"
	skillPath := filepath.Join(userSkillsDir, skillName)

	// 检查是否是 Git 仓库
	gitDir := filepath.Join(skillPath, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		fmt.Printf("⚠️  Skill '%s' is not a Git repository, cannot update\n", skillName)
		os.Exit(1)
	}

	fmt.Printf("Updating skill: %s\n", skillName)

	// 执行 git pull
	gitCmd := exec.Command("git", "-C", skillPath, "pull")
	gitCmd.Stdout = os.Stdout
	gitCmd.Stderr = os.Stderr
	if err := gitCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to update: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ Skill updated successfully")
}

func runSkillsUninstall(cmd *cobra.Command, args []string) {
	skillName := args[0]

	// 初始化日志
	if err := logger.Init("warn", false); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = logger.Sync() }()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get home directory: %v\n", err)
		os.Exit(1)
	}
	userSkillsDir := homeDir + "/.goclaw/skills"
	skillPath := filepath.Join(userSkillsDir, skillName)

	// 检查技能是否存在
	if _, err := os.Stat(skillPath); os.IsNotExist(err) {
		fmt.Printf("⚠️  Skill '%s' is not installed\n", skillName)
		os.Exit(1)
	}

	fmt.Printf("Uninstalling skill: %s\n", skillName)
	fmt.Printf("Path: %s\n", skillPath)
	fmt.Print("Confirm? (y/N): ")

	var confirm string
	_, _ = fmt.Scanln(&confirm)
	if strings.ToLower(confirm) != "y" {
		fmt.Println("Uninstallation cancelled.")
		return
	}

	// 删除目录
	if err := os.RemoveAll(skillPath); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to remove skill: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ Skill uninstalled successfully")
}

func runSkillsConfigShow(cmd *cobra.Command, args []string) {
	// 加载配置
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Skills Configuration:")
	fmt.Println("===================")

	// 检查是否有专门的 skills 配置文件
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get home directory: %v\n", err)
		os.Exit(1)
	}
	skillsConfigPath := homeDir + "/.goclaw/skills.yaml"
	if _, err := os.Stat(skillsConfigPath); err == nil {
		fmt.Printf("\nConfig file: %s\n", skillsConfigPath)
		// TODO: 解析并显示 skills.yaml 内容
	} else {
		fmt.Println("\nNo custom skills configuration found.")
		fmt.Println("Using default configuration.")
	}

	// 显示工具配置中与技能相关的部分
	fmt.Println("\nRelevant Tool Configuration:")
	fmt.Printf("  Shell enabled: %v\n", cfg.Tools.Shell.Enabled)
	if len(cfg.Tools.Shell.AllowedCmds) > 0 {
		fmt.Printf("  Allowed commands: %v\n", cfg.Tools.Shell.AllowedCmds)
	}
}

func runSkillsConfigSet(cmd *cobra.Command, args []string) {
	key := args[0]
	value := args[1]

	// 初始化日志
	if err := logger.Init("warn", false); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = logger.Sync() }()

	parts := strings.SplitN(key, ".", 2)
	if len(parts) < 2 {
		fmt.Fprintf(os.Stderr, "Invalid key format. Use 'disabled.skill-name' or 'env.skill-name.VAR'\n")
		os.Exit(1)
	}

	configType := parts[0]
	skillKey := parts[1]

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get home directory: %v\n", err)
		os.Exit(1)
	}
	userSkillsDir := homeDir + "/.goclaw"
	skillsConfigPath := filepath.Join(userSkillsDir, "skills.yaml")

	// TODO: 实现 skills.yaml 的读写
	fmt.Printf("Setting configuration: %s = %s\n", key, value)
	fmt.Printf("Config type: %s, skill: %s\n", configType, skillKey)
	fmt.Println("⚠️  Skills configuration file editing is not yet implemented.")
	fmt.Println("   Please manually edit:", skillsConfigPath)
}

func runSkillsInstallDeps(cmd *cobra.Command, args []string) {
	skillName := args[0]

	// 初始化日志
	if err := logger.Init("warn", false); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = logger.Sync() }()

	// 创建工作区
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get home directory: %v\n", err)
		os.Exit(1)
	}
	goclawDir := homeDir + "/.goclaw"
	workspace := goclawDir + "/workspace"
	globalSkillsDir := goclawDir + "/skills"
	workspaceSkillsDir := workspace + "/skills"
	currentSkillsDir := "./skills"

	// 创建技能加载器并启用自动安装
	skillsLoader := agent.NewSkillsLoader(goclawDir, []string{
		globalSkillsDir,    // 最先加载（最低优先级）
		workspaceSkillsDir, // 其次加载
		currentSkillsDir,   // 最后加载（最高优先级）
	})
	skillsLoader.SetAutoInstall(true)

	if err := skillsLoader.Discover(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to discover skills: %v\n", err)
		os.Exit(1)
	}

	_, ok := skillsLoader.Get(skillName)
	if !ok {
		fmt.Printf("❌ Skill '%s' not found\n", skillName)
		os.Exit(1)
	}

	fmt.Printf("Installing dependencies for skill: %s\n\n", skillName)

	if err := skillsLoader.InstallDependencies(skillName); err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ Failed to install dependencies: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n✅ All dependencies installed successfully")
}

func runSkillsSearch(cmd *cobra.Command, args []string) {
	query := strings.Join(args, " ")

	// 初始化日志
	if err := logger.Init("warn", false); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = logger.Sync() }()

	// 检查 npx 是否可用
	if _, err := exec.LookPath("npx"); err != nil {
		fmt.Fprintf(os.Stderr, "❌ npx is not installed. Please install Node.js and npm.\n")
		fmt.Println("\nVisit: https://nodejs.org/")
		os.Exit(1)
	}

	// 调用 npx skills find 命令
	fmt.Printf("Searching for skills: %s\n\n", query)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmdFind := exec.CommandContext(ctx, "npx", "skills", "find", query)
	cmdFind.Stdout = os.Stdout
	cmdFind.Stderr = os.Stderr

	if err := cmdFind.Run(); err != nil {
		// npx skills find 可能返回非零退出码但仍然有输出
		// 检查是否有任何输出
		if ctx.Err() == context.DeadlineExceeded {
			fmt.Fprintf(os.Stderr, "\n⚠️  Search timed out after 30 seconds\n")
			fmt.Println("\nTry a more specific search term.")
		}
		os.Exit(1)
	}

	fmt.Println("\nTo install a skill:")
	fmt.Println("  goclaw skills install <url>")
}
