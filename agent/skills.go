package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
				Bins         []string `yaml:"bins"`
				AnyBins       []string `yaml:"anyBins"`
				Env          []string `yaml:"env"`
				Config       []string `yaml:"config"`
				OS           []string `yaml:"os"`
				PythonPkgs   []string `yaml:"pythonPkgs"`   // Python包依赖
				NodePkgs     []string `yaml:"nodePkgs"`     // Node.js包依赖
			} `yaml:"requires"`
			Install []SkillInstall `yaml:"install"`
		} `yaml:"goclaw"`
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
	NodePkgs   []string `yaml:"nodePkgs"`  // 缺失的Node.js包
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
	workspace    string
	skillsDirs   []string
	skills       map[string]*Skill
	alwaysSkills []string
	autoInstall  bool // 是否启用自动安装依赖
}

// NewSkillsLoader 创建技能加载器
func NewSkillsLoader(workspace string, skillsDirs []string) *SkillsLoader {
	return &SkillsLoader{
		workspace:   workspace,
		skillsDirs:  skillsDirs,
		skills:      make(map[string]*Skill),
		autoInstall: os.Getenv("GOCLAW_SKILL_AUTO_INSTALL") == "true",
	}
}

// SetAutoInstall 设置是否启用自动安装
func (l *SkillsLoader) SetAutoInstall(enabled bool) {
	l.autoInstall = enabled
}

// Discover 发现技能
func (l *SkillsLoader) Discover() error {
	// 获取可执行文件路径
	exePath, err := os.Executable()
	var exeDir string
	if err == nil {
		exeDir = filepath.Dir(exePath)
	}

	// 默认技能目录
	dirs := append(l.skillsDirs,
		filepath.Join(l.workspace, "skills"),
		filepath.Join(l.workspace, ".goclaw", "skills"),
	)

	// 添加可执行文件同级的 skills 目录
	if exeDir != "" {
		dirs = append(dirs, filepath.Join(exeDir, "skills"))
	}

	// 添加当前目录下的 skills 目录（开发调试用）
	dirs = append(dirs, "skills")

	for _, dir := range dirs {
		if err := l.discoverInDir(dir); err != nil {
			// 目录不存在是正常的，继续
			if !os.IsNotExist(err) {
				return err
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

	// 解析 YAML front matter
	var skill Skill
	if err := l.parseSkillMetadata(string(content), &skill); err != nil {
		return err
	}

	// 检查是否存在阻塞式需求（如 OS 不匹配），这类需求会导致跳过技能
	if !l.checkBlockingRequirements(&skill) {
		// 存在阻塞式需求，跳过该技能
		return nil
	}

	// 计算缺失的依赖（用于显示给LLM）
	skill.MissingDeps = l.getMissingDeps(&skill)

	// 保存内容（移除 YAML front matter）
	skill.Content = l.extractContent(string(content))

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

// parseSkillMetadata 解析技能元数据
func (l *SkillsLoader) parseSkillMetadata(content string, skill *Skill) error {
	// 查找 YAML 分隔符
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
	case "npm":
		cmd = exec.Command("npm", "install", "-g", installConfig.Package)
	case "pnpm":
		cmd = exec.Command("pnpm", "add", "-g", installConfig.Package)
	case "yarn":
		cmd = exec.Command("yarn", "global", installConfig.Package)
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
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
		fmt.Scanln(&response)
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
	// 获取当前shell路径并重新加载
	shellPaths := []string{"/bin", "/usr/bin", "/usr/local/bin", "/opt/homebrew/bin", "/opt/homebrew/opt/python3/bin"}
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		pathEnv = strings.Join(shellPaths, ":")
	} else {
		pathEnv = pathEnv + ":" + strings.Join(shellPaths, ":")
	}
	os.Setenv("PATH", pathEnv)
	return nil
}

// checkPythonPackage 检查Python包是否已安装
func (l *SkillsLoader) checkPythonPackage(pkg string) error {
	cmd := exec.Command("python3", "-c", fmt.Sprintf("import %s; print('OK')", pkg))
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "OK") {
		return fmt.Errorf("Python package not found: %s", pkg)
	}
	return nil
}

// npmPackageInfo npm包信息
type npmPackageInfo struct {
	Name string `json:"name"`
}

// checkNodePackage 检查Node.js包是否已安装
func (l *SkillsLoader) checkNodePackage(pkg string) error {
	cmd := exec.Command("npm", "list", "--global", "--json", "--depth=0", pkg)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("npm command failed: %w", err)
	}

	// npm list 返回空JSON列表时表示包未找到
	var result []npmPackageInfo
	if err := json.Unmarshal(output, &result); err != nil {
		return fmt.Errorf("failed to parse npm output: %w", err)
	}

	if len(result) == 0 {
		return fmt.Errorf("Node.js package not found: %s", pkg)
	}

	return nil
}

// tryInstallPythonPackage 尝试安装Python包
func (l *SkillsLoader) tryInstallPythonPackage(pkg string) error {
	logger.Info("Installing Python package", zap.String("package", pkg))

	// 执行安装，带超时
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "python3", "-m", "pip", "install", pkg)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pip install failed: %w, output: %s", err, string(output))
	}

	logger.Info("Python package installed successfully", zap.String("package", pkg))
	return nil
}

// tryInstallNodePackage 尝试安装Node.js包
func (l *SkillsLoader) tryInstallNodePackage(pkg string) error {
	logger.Info("Installing Node.js package", zap.String("package", pkg))

	// 执行安装，带超时
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "npm", "install", "-g", pkg)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("npm install failed: %w, output: %s", err, string(output))
	}

	logger.Info("Node.js package installed successfully", zap.String("package", pkg))
	return nil
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
