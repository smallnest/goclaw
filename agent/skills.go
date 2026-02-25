package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// Skill 技能定义
type Skill struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Version     string `yaml:"version"`
	Author      string `yaml:"author"`
	Homepage    string `yaml:"homepage"`
	Always      bool   `yaml:"always"`
	Metadata    struct {
		OpenClaw struct {
			Emoji    string `yaml:"emoji"`
			Always   bool   `yaml:"always"`
			Requires struct {
				Bins       []string `yaml:"bins"`
				AnyBins    []string `yaml:"anyBins"`
				Env        []string `yaml:"env"`
				Config     []string `yaml:"config"`
				OS         []string `yaml:"os"`
				PythonPkgs []string `yaml:"pythonPkgs"` // Python包依赖
				NodePkgs   []string `yaml:"nodePkgs"`   // Node.js包依赖
			} `yaml:"requires"`
			Install []SkillInstall `yaml:"install"`
		} `yaml:"openclaw"`
	} `yaml:"metadata"`
	Requires SkillRequirements `yaml:"requires"` // 兼容旧格式
	Content  string            `yaml:"-"`        // 技能内容（Markdown）
	// 缺失的依赖信息
	MissingDeps *MissingDeps `yaml:"-"` // 解析时填充
}

// MissingDeps 缺失的依赖信息
type MissingDeps struct {
	Bins       []string `yaml:"bins"`       // 缺失的二进制
	AnyBins    []string `yaml:"anyBins"`    // 缺失的可选二进制
	Env        []string `yaml:"env"`        // 缺失的环境变量
	PythonPkgs []string `yaml:"pythonPkgs"` // 缺失的Python包
	NodePkgs   []string `yaml:"nodePkgs"`   // 缺失的Node.js包
}

// SkillRequirements 技能需求 (旧格式)
type SkillRequirements struct {
	Bins []string `yaml:"bins"`
	Env  []string `yaml:"env"`
}

// SkillInstall 技能安装配置
type SkillInstall struct {
	ID      string   `yaml:"id"`      // 安装方式唯一标识
	Kind    string   `yaml:"kind"`    // 安装方式: brew, apt, npm, pip, uv, go
	Formula string   `yaml:"formula"` // 包名 (brew, apt)
	Package string   `yaml:"package"` // 包名 (npm, pip, go)
	Bins    []string `yaml:"bins"`    // 安装后提供的可执行文件
	Label   string   `yaml:"label"`   // 安装说明
	OS      []string `yaml:"os"`      // 适用的操作系统
	Command string   `yaml:"command"` // 自定义安装命令
}

// SkillsLoader 技能加载器
type SkillsLoader struct {
	workspace      string
	skillsDirs     []string
	skills         map[string]*Skill
	alwaysSkills   []string
	autoInstall    bool          // 是否启用自动安装依赖
	installTimeout time.Duration // 安装超时时间
}

// Default installation timeout
const (
	DefaultInstallTimeout = 5 * time.Minute
)

// NewSkillsLoader 创建技能加载器
func NewSkillsLoader(workspace string, skillsDirs []string) *SkillsLoader {
	return &SkillsLoader{
		workspace:      workspace,
		skillsDirs:     skillsDirs,
		skills:         make(map[string]*Skill),
		autoInstall:    os.Getenv("GOCLAW_SKILL_AUTO_INSTALL") == "true",
		installTimeout: DefaultInstallTimeout,
	}
}

// SetAutoInstall 设置是否启用自动安装
func (l *SkillsLoader) SetAutoInstall(enabled bool) {
	l.autoInstall = enabled
}

// SetInstallTimeout 设置安装超时时间
func (l *SkillsLoader) SetInstallTimeout(timeout time.Duration) {
	l.installTimeout = timeout
}

// Discover 发现技能
// 按照顺序加载技能，后加载的同名技能会覆盖前面的
func (l *SkillsLoader) Discover() error {
	// 按照配置的技能目录顺序加载（后面的会覆盖前面的）
	for _, dir := range l.skillsDirs {
		if err := l.discoverInDir(dir); err != nil {
			// 目录不存在是正常的，继续
			if !os.IsNotExist(err) {
				logger.Warn("Failed to discover skills in directory",
					zap.String("dir", dir),
					zap.Error(err))
			}
		}
	}

	return nil
}

// discoverInDir 在目录中发现技能
func (l *SkillsLoader) discoverInDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			// 跳过非目录文件
			continue
		}

		skillPath := filepath.Join(dir, entry.Name())
		if err := l.loadSkill(skillPath); err != nil {
			// 跳过无法加载的技能
			continue
		}
	}

	return nil
}

// loadSkill 加载技能
func (l *SkillsLoader) loadSkill(path string) error {
	// 查找 SKILL.md 或 skill.md
	skillFile := filepath.Join(path, "SKILL.md")
	if _, err := os.Stat(skillFile); os.IsNotExist(err) {
		skillFile = filepath.Join(path, "skill.md")
		if _, err := os.Stat(skillFile); os.IsNotExist(err) {
			return nil // 没有技能文件
		}
	}

	// 读取文件
	content, err := os.ReadFile(skillFile)
	if err != nil {
		return err
	}

	// 解析 YAML front matter（使用新解析器）
	var skill Skill
	if err := l.parseSkillMetadata(string(content), &skill); err != nil {
		return err
	}

	// 检查是否存在阻塞式需求（如 OS 不匹配），这类需求会导致跳过技能
	if !l.checkBlockingRequirements(&skill) {
		// 存在阻塞式需求，跳过该技能
		return nil
	}

	// 如果新解析器已经提取了内容，使用它；否则回退到旧方法
	if skill.Content == "" {
		skill.Content = l.extractContent(string(content))
	}

	// 计算缺失的依赖（用于显示给LLM）
	skill.MissingDeps = l.getMissingDeps(&skill)

	// 使用目录名作为技能名
	if skill.Name == "" {
		skill.Name = filepath.Base(path)
	}

	l.skills[skill.Name] = &skill

	// 记录 always 技能
	if skill.Always {
		l.alwaysSkills = append(l.alwaysSkills, skill.Name)
	}

	return nil
}

// checkBlockingRequirements 检查阻塞性需求（如 OS 不匹配）
// 返回 false 表示技能因阻塞性需求无法使用，应跳过加载
func (l *SkillsLoader) checkBlockingRequirements(skill *Skill) bool {
	// always 技能总是加载
	if skill.Always || skill.Metadata.OpenClaw.Always {
		return true
	}

	// 检查操作系统兼容性（阻塞性）
	if len(skill.Metadata.OpenClaw.Requires.OS) > 0 {
		currentOS := runtime.GOOS
		compatible := false
		for _, osName := range skill.Metadata.OpenClaw.Requires.OS {
			if osName == currentOS {
				compatible = true
				break
			}
		}
		if !compatible {
			return false
		}
	}

	return true
}

// parseSkillMetadata 解析技能元数据（支持新旧格式）
func (l *SkillsLoader) parseSkillMetadata(content string, skill *Skill) error {
	// 首先尝试使用新的 frontmatter 解析器
	frontmatter := ParseFrontmatter(content)
	if len(frontmatter) > 0 {
		// 从 frontmatter 中解析基本字段
		if name := frontmatter["name"]; name != "" {
			skill.Name = name
		}
		if desc := frontmatter["description"]; desc != "" {
			skill.Description = desc
		}
		if homepage := frontmatter["homepage"]; homepage != "" {
			skill.Homepage = homepage
		}
		if always := frontmatter["always"]; always != "" {
			skill.Always = always == "true"
		}

		// 解析 OpenClaw/goclaw 元数据
		metadata := ParseOpenClawMetadata(frontmatter)
		if metadata != nil {
			// 映射到旧的 Skill 结构
			if emoji, ok := metadata["emoji"].(string); ok {
				skill.Metadata.OpenClaw.Emoji = emoji
			}
			if always, ok := metadata["always"].(bool); ok {
				skill.Metadata.OpenClaw.Always = always
			}

			// 映射依赖要求
			if requires, ok := metadata["requires"].(map[string]any); ok {
				if bins, ok := requires["bins"].([]string); ok {
					skill.Metadata.OpenClaw.Requires.Bins = bins
				}
				if anyBins, ok := requires["anyBins"].([]string); ok {
					skill.Metadata.OpenClaw.Requires.AnyBins = anyBins
				}
				if env, ok := requires["env"].([]string); ok {
					skill.Metadata.OpenClaw.Requires.Env = env
				}
				if config, ok := requires["config"].([]string); ok {
					skill.Metadata.OpenClaw.Requires.Config = config
				}
				if os, ok := requires["os"].([]string); ok {
					skill.Metadata.OpenClaw.Requires.OS = os
				}
			}

			// 映射安装配置（暂时跳过，需要更复杂的类型断言）
		}

		// 提取内容（移除 frontmatter）
		skill.Content = StripFrontmatter(content)
		return nil
	}

	// 回退到旧的 YAML 解析方式
	if !strings.HasPrefix(content, "---") {
		return nil // 没有 YAML front matter
	}

	endIndex := strings.Index(content[3:], "---")
	if endIndex == -1 {
		return nil // 没有结束分隔符
	}

	yamlContent := content[4 : endIndex+3]

	// 解析 YAML
	if err := yaml.Unmarshal([]byte(yamlContent), skill); err != nil {
		return err
	}

	return nil
}

// extractContent 提取内容（移除 YAML front matter）
func (l *SkillsLoader) extractContent(content string) string {
	if !strings.HasPrefix(content, "---") {
		return content
	}

	endIndex := strings.Index(content[3:], "---")
	if endIndex == -1 {
		return content
	}

	return content[endIndex+7:] // 跳过 "---\n"
}

// List 列出所有技能
func (l *SkillsLoader) List() []*Skill {
	result := make([]*Skill, 0, len(l.skills))
	for _, skill := range l.skills {
		result = append(result, skill)
	}
	return result
}

// Get 获取技能
func (l *SkillsLoader) Get(name string) (*Skill, bool) {
	skill, ok := l.skills[name]
	return skill, ok
}

// GetAlwaysSkills 获取始终加载的技能
func (l *SkillsLoader) GetAlwaysSkills() []string {
	return l.alwaysSkills
}

// BuildSummary 构建技能摘要
func (l *SkillsLoader) BuildSummary() string {
	if len(l.skills) == 0 {
		return "No skills available."
	}

	var summary string
	summary += fmt.Sprintf("# Available Skills (%d)\n\n", len(l.skills))

	for name, skill := range l.skills {
		summary += fmt.Sprintf("## %s\n", name)
		if skill.Description != "" {
			summary += fmt.Sprintf("%s\n", skill.Description)
		}
		if skill.Author != "" {
			summary += fmt.Sprintf("Author: %s\n", skill.Author)
		}
		if skill.Version != "" {
			summary += fmt.Sprintf("Version: %s\n", skill.Version)
		}
		summary += "\n"
	}

	return summary
}

// LoadContent 加载技能内容
func (l *SkillsLoader) LoadContent(name string) (string, error) {
	skill, ok := l.skills[name]
	if !ok {
		return "", fmt.Errorf("skill not found: %s", name)
	}

	return skill.Content, nil
}

// InstallDependencies 安装技能依赖
func (l *SkillsLoader) InstallDependencies(skillName string) error {
	skill, ok := l.skills[skillName]
	if !ok {
		return fmt.Errorf("skill not found: %s", skillName)
	}

	// 检查二进制依赖并安装
	for _, bin := range skill.Metadata.OpenClaw.Requires.Bins {
		if _, err := exec.LookPath(bin); err != nil {
			if err := l.tryInstallBinary(skill, bin); err != nil {
				return fmt.Errorf("failed to install %s for skill %s: %w", bin, skillName, err)
			}
		}
	}

	for _, bin := range skill.Metadata.OpenClaw.Requires.AnyBins {
		if _, err := exec.LookPath(bin); err == nil {
			// 有一个已经安装了，跳过
			break
		}
		if err := l.tryInstallBinary(skill, bin); err != nil {
			logger.Warn("Failed to install optional dependency",
				zap.String("skill", skillName),
				zap.String("bin", bin),
				zap.Error(err))
		}
	}

	// 检查Python包依赖并安装
	for _, pkg := range skill.Metadata.OpenClaw.Requires.PythonPkgs {
		if err := l.checkPythonPackage(pkg); err != nil {
			if err := l.tryInstallPythonPackage(pkg); err != nil {
				return fmt.Errorf("failed to install Python package %s for skill %s: %w", pkg, skillName, err)
			}
		}
	}

	// 检查Node.js包依赖并安装
	for _, pkg := range skill.Metadata.OpenClaw.Requires.NodePkgs {
		if err := l.checkNodePackage(pkg); err != nil {
			if err := l.tryInstallNodePackage(pkg); err != nil {
				return fmt.Errorf("failed to install Node.js package %s for skill %s: %w", pkg, skillName, err)
			}
		}
	}

	return nil
}

// tryInstallBinary 尝试安装二进制文件
func (l *SkillsLoader) tryInstallBinary(skill *Skill, bin string) error {
	installConfig := l.findInstallConfig(skill, bin)
	if installConfig == nil {
		return fmt.Errorf("no install config for %s", bin)
	}

	// 检查操作系统是否匹配
	if len(installConfig.OS) > 0 {
		currentOS := runtime.GOOS
		matches := false
		for _, osName := range installConfig.OS {
			if osName == currentOS {
				matches = true
				break
			}
		}
		if !matches {
			return fmt.Errorf("install not supported on %s", currentOS)
		}
	}

	// 获取用户确认
	if !l.confirmInstall(skill.Name, installConfig) {
		return fmt.Errorf("install cancelled by user")
	}

	logger.Info("Installing dependency for skill",
		zap.String("skill", skill.Name),
		zap.String("binary", bin),
		zap.String("kind", installConfig.Kind))

	var cmd *exec.Cmd
	switch installConfig.Kind {
	case "brew":
		cmd = exec.Command("brew", "install", installConfig.Formula)
	case "apt", "apt-get":
		cmd = exec.Command("sudo", "apt-get", "install", "-y", installConfig.Formula)
	case "node":
		// node kind: use configured node package manager (npm, pnpm, yarn, bun)
		nodeManager := "npm" // default
		if nm := os.Getenv("GOCLAW_NODE_MANAGER"); nm != "" {
			nodeManager = nm
		}
		cmd = exec.Command(nodeManager, "install", "-g", installConfig.Package)
	case "npm":
		cmd = exec.Command("npm", "install", "-g", installConfig.Package)
	case "pnpm":
		cmd = exec.Command("pnpm", "add", "-g", installConfig.Package)
	case "yarn":
		cmd = exec.Command("yarn", "global", installConfig.Package)
	case "bun":
		cmd = exec.Command("bun", "install", "-g", installConfig.Package)
	case "pip", "pip3":
		cmd = exec.Command("pip3", "install", installConfig.Package)
	case "uv":
		cmd = exec.Command("uv", "tool", "install", installConfig.Package)
	case "go":
		cmd = exec.Command("go", "install", installConfig.Package)
	case "command":
		cmd = exec.Command("sh", "-c", installConfig.Command)
	default:
		return fmt.Errorf("unsupported install kind: %s", installConfig.Kind)
	}

	// 执行安装，带超时
	ctx, cancel := context.WithTimeout(context.Background(), l.installTimeout)
	defer cancel()

	output, err := cmd.CombinedOutput()
	_ = ctx // 避免未使用警告
	if err != nil {
		return fmt.Errorf("install failed: %w, output: %s", err, string(output))
	}

	// 刷新PATH
	if err := l.refreshPath(); err != nil {
		logger.Warn("Failed to refresh PATH after install", zap.Error(err))
	}

	logger.Info("Dependency installed successfully",
		zap.String("skill", skill.Name),
		zap.String("binary", bin))

	return nil
}

// findInstallConfig 查找安装配置
func (l *SkillsLoader) findInstallConfig(skill *Skill, bin string) *SkillInstall {
	// 首先匹配bins列表中的bin
	for _, install := range skill.Metadata.OpenClaw.Install {
		for _, providedBin := range install.Bins {
			if providedBin == bin {
				return &install
			}
		}
	}
	// 匹配AnyBins
	for _, install := range skill.Metadata.OpenClaw.Install {
		for _, providedBin := range install.Bins {
			if providedBin == bin {
				return &install
			}
		}
	}
	return nil
}

// confirmInstall 请求用户确认安装
func (l *SkillsLoader) confirmInstall(skillName string, install *SkillInstall) bool {
	// 如果是交互式终端，询问用户
	if l.isTerminal() {
		label := install.Label
		if label == "" {
			label = fmt.Sprintf("Install %s (%s)", install.Kind, install.Formula)
		}
		fmt.Printf("\nSkill '%s' requires installing dependency:\n", skillName)
		fmt.Printf("  %s\n", label)
		fmt.Print("Install now? [Y/n]: ")

		var response string
		_, _ = fmt.Scanln(&response)
		return strings.ToLower(response) == "y" || response == ""
	}

	// 非交互式环境，自动安装
	return true
}

// isTerminal 检查是否在交互式终端
func (l *SkillsLoader) isTerminal() bool {
	stat, _ := os.Stdin.Stat()
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// refreshPath 刷新PATH
func (l *SkillsLoader) refreshPath() error {
	homeDir, _ := os.UserHomeDir()

	// 获取当前shell路径并重新加载
	shellPaths := []string{
		"/bin",
		"/usr/bin",
		"/usr/local/bin",
		"/opt/homebrew/bin",
		"/opt/homebrew/opt/python3/bin",
	}

	// 添加 Node.js 包管理器全局安装路径
	if homeDir != "" {
		shellPaths = append(shellPaths,
			homeDir+"/.npm-global/bin",           // npm
			homeDir+"/.local/share/pnpm",         // pnpm
			homeDir+"/.yarn/bin",                 // yarn
			homeDir+"/.bun/bin",                  // bun
			"/opt/homebrew/lib/node_modules/bin", // npm (brew-installed node)
		)
	}

	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		pathEnv = strings.Join(shellPaths, ":")
	} else {
		pathEnv = pathEnv + ":" + strings.Join(shellPaths, ":")
	}
	os.Setenv("PATH", pathEnv)
	return nil
}

// PackageType 包类型枚举
type PackageType string

const (
	PackageTypePython PackageType = "python"
	PackageTypeNode   PackageType = "node"
)

// checkPackageInstalled 检查包是否已安装（通用函数）
func (l *SkillsLoader) checkPackageInstalled(pkgType PackageType, pkg string) error {
	switch pkgType {
	case PackageTypePython:
		cmd := exec.Command("python3", "-c", fmt.Sprintf("import %s; print('OK')", pkg))
		output, err := cmd.CombinedOutput()
		if err != nil || !strings.Contains(string(output), "OK") {
			return fmt.Errorf("Python package not found: %s", pkg)
		}
		return nil
	case PackageTypeNode:
		cmd := exec.Command("npm", "list", "--global", "--json", "--depth=0", pkg)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("npm command failed: %w", err)
		}
		var result []npmPackageInfo
		if err := json.Unmarshal(output, &result); err != nil {
			return fmt.Errorf("failed to parse npm output: %w", err)
		}
		if len(result) == 0 {
			return fmt.Errorf("Node.js package not found: %s", pkg)
		}
		return nil
	default:
		return fmt.Errorf("unsupported package type: %s", pkgType)
	}
}

// tryInstallPackage 尝试安装包（通用函数）
func (l *SkillsLoader) tryInstallPackage(pkgType PackageType, pkg string) error {
	logger.Info("Installing package", zap.String("type", string(pkgType)), zap.String("package", pkg))

	ctx, cancel := context.WithTimeout(context.Background(), l.installTimeout)
	defer cancel()

	var cmd *exec.Cmd
	switch pkgType {
	case PackageTypePython:
		cmd = exec.CommandContext(ctx, "python3", "-m", "pip", "install", pkg)
	case PackageTypeNode:
		cmd = exec.CommandContext(ctx, "npm", "install", "-g", pkg)
	default:
		return fmt.Errorf("unsupported package type: %s", pkgType)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s install failed: %w, output: %s", pkgType, err, string(output))
	}

	logger.Info("Package installed successfully", zap.String("type", string(pkgType)), zap.String("package", pkg))
	return nil
}

// checkPythonPackage 检查Python包是否已安装
func (l *SkillsLoader) checkPythonPackage(pkg string) error {
	return l.checkPackageInstalled(PackageTypePython, pkg)
}

// npmPackageInfo npm包信息
type npmPackageInfo struct {
	Name string `json:"name"`
}

// checkNodePackage 检查Node.js包是否已安装
func (l *SkillsLoader) checkNodePackage(pkg string) error {
	return l.checkPackageInstalled(PackageTypeNode, pkg)
}

// tryInstallPythonPackage 尝试安装Python包
func (l *SkillsLoader) tryInstallPythonPackage(pkg string) error {
	return l.tryInstallPackage(PackageTypePython, pkg)
}

// tryInstallNodePackage 尝试安装Node.js包
func (l *SkillsLoader) tryInstallNodePackage(pkg string) error {
	return l.tryInstallPackage(PackageTypeNode, pkg)
}

// getMissingDeps 计算缺失的依赖
func (l *SkillsLoader) getMissingDeps(skill *Skill) *MissingDeps {
	var missing MissingDeps

	// 检查二进制文件
	for _, bin := range skill.Metadata.OpenClaw.Requires.Bins {
		if _, err := exec.LookPath(bin); err != nil {
			missing.Bins = append(missing.Bins, bin)
		}
	}

	// 检查 AnyBins
	for _, bin := range skill.Metadata.OpenClaw.Requires.AnyBins {
		found := false
		for _, b := range skill.Metadata.OpenClaw.Requires.AnyBins {
			if _, err := exec.LookPath(b); err == nil {
				found = true
				break
			}
		}
		if !found {
			missing.AnyBins = append(missing.AnyBins, bin)
		}
	}

	// 检查Python包
	for _, pkg := range skill.Metadata.OpenClaw.Requires.PythonPkgs {
		if err := l.checkPythonPackage(pkg); err != nil {
			missing.PythonPkgs = append(missing.PythonPkgs, pkg)
		}
	}

	// 检查Node.js包
	for _, pkg := range skill.Metadata.OpenClaw.Requires.NodePkgs {
		if err := l.checkNodePackage(pkg); err != nil {
			missing.NodePkgs = append(missing.NodePkgs, pkg)
		}
	}

	// 检查环境变量
	for _, env := range skill.Metadata.OpenClaw.Requires.Env {
		if os.Getenv(env) == "" {
			missing.Env = append(missing.Env, env)
		}
	}
	for _, env := range skill.Requires.Env {
		if os.Getenv(env) == "" {
			missing.Env = append(missing.Env, env)
		}
	}

	// 如果没有缺失依赖，返回nil
	if len(missing.Bins) == 0 &&
		len(missing.AnyBins) == 0 &&
		len(missing.PythonPkgs) == 0 &&
		len(missing.NodePkgs) == 0 &&
		len(missing.Env) == 0 {
		return nil
	}

	return &missing
}

// SearchResult 搜索结果
type SearchResult struct {
	Skill   *Skill
	Source  string // skill的来源路径
	Score   float64
	Matches []string // 匹配的字段
}

// Search 搜索技能
func (l *SkillsLoader) Search(query string) []*SearchResult {
	if len(l.skills) == 0 {
		return nil
	}

	query = strings.ToLower(query)
	var results []*SearchResult

	for name, skill := range l.skills {
		score := 0.0
		var matches []string

		// 检查名称匹配
		if strings.Contains(strings.ToLower(name), query) {
			// 精确匹配得分更高
			if strings.EqualFold(name, query) {
				score += 1.0
				matches = append(matches, "name (exact)")
			} else {
				score += 0.8
				matches = append(matches, "name")
			}
		}

		// 检查描述匹配
		lowerDesc := strings.ToLower(skill.Description)
		if strings.Contains(lowerDesc, query) {
			score += 0.6
			matches = append(matches, "description")
		}

		// 检查作者匹配
		if strings.Contains(strings.ToLower(skill.Author), query) {
			score += 0.4
			matches = append(matches, "author")
		}

		// 检查内容匹配（内容太长，只按关键词查找）
		keywords := strings.Fields(query)
		lowerContent := strings.ToLower(skill.Content)
		contentMatches := 0
		for _, keyword := range keywords {
			if strings.Contains(lowerContent, strings.ToLower(keyword)) {
				contentMatches++
			}
		}
		if contentMatches > 0 {
			contentScore := 0.3 * float64(contentMatches) / float64(len(keywords))
			score += contentScore
			if contentMatches == len(keywords) {
				matches = append(matches, "content")
			}
		}

		// 只返回有匹配的结果
		if score > 0 {
			results = append(results, &SearchResult{
				Skill:   skill,
				Source:  resolveSkillSource(skill),
				Score:   score,
				Matches: matches,
			})
		}
	}

	// 按得分排序
	if len(results) > 0 {
		sort.Slice(results, func(i, j int) bool {
			return results[i].Score > results[j].Score
		})
	}

	return results
}

// resolveSkillSource 解析技能来源
func resolveSkillSource(skill *Skill) string {
	// 检查是否来自远程仓库
	if strings.Contains(skill.Homepage, "github.com") || strings.Contains(skill.Homepage, "gitlab.com") {
		return "remote"
	}

	// 检查是否来自本地路径
	if skill.Homepage != "" && (strings.HasPrefix(skill.Homepage, "/") || strings.HasPrefix(skill.Homepage, ".")) {
		return "local"
	}

	return "builtin"
}

// ParseFrontmatter 解析 YAML frontmatter
func ParseFrontmatter(content string) map[string]string {
	// 检查是否以 --- 开头
	if !strings.HasPrefix(content, "---") {
		return nil
	}

	// 查找结束标记
	endIndex := strings.Index(content[4:], "---")
	if endIndex == -1 {
		return nil
	}

	// 提取 frontmatter 内容
	frontmatterContent := content[4 : endIndex+4]

	// 解析 YAML
	var metadata map[string]string
	if err := yaml.Unmarshal([]byte(frontmatterContent), &metadata); err != nil {
		return nil
	}

	return metadata
}

// StripFrontmatter 移除 YAML frontmatter
func StripFrontmatter(content string) string {
	// 检查是否以 --- 开头
	if !strings.HasPrefix(content, "---") {
		return content
	}

	// 查找结束标记
	endIndex := strings.Index(content[4:], "---")
	if endIndex == -1 {
		return content
	}

	// 返回移除 frontmatter 后的内容
	return content[endIndex+8:]
}

// ParseOpenClawMetadata 解析 OpenClaw/goclaw 特定的元数据
func ParseOpenClawMetadata(frontmatter map[string]string) map[string]any {
	metadata := make(map[string]any)

	// 解析基本字段
	if emoji := frontmatter["emoji"]; emoji != "" {
		metadata["emoji"] = emoji
	}
	if always := frontmatter["always"]; always != "" {
		metadata["always"] = always == "true"
	}

	// 解析依赖要求
	requires := make(map[string]any)
	if bins := frontmatter["requires_bins"]; bins != "" {
		requires["bins"] = strings.Split(strings.TrimSpace(bins), ",")
	}
	if anyBins := frontmatter["any_bins"]; anyBins != "" {
		requires["anyBins"] = strings.Split(strings.TrimSpace(anyBins), ",")
	}
	if env := frontmatter["requires_env"]; env != "" {
		requires["env"] = strings.Split(strings.TrimSpace(env), ",")
	}
	if config := frontmatter["requires_config"]; config != "" {
		requires["config"] = strings.Split(strings.TrimSpace(config), ",")
	}
	if os := frontmatter["requires_os"]; os != "" {
		requires["os"] = strings.Split(strings.TrimSpace(os), ",")
	}
	if len(requires) > 0 {
		metadata["requires"] = requires
	}

	// 解析安装配置
	// 这里可以扩展更多的安装配置解析

	return metadata
}
