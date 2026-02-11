package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

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
				Bins    []string `yaml:"bins"`
				AnyBins []string `yaml:"anyBins"`
				Env     []string `yaml:"env"`
				Config  []string `yaml:"config"`
				OS      []string `yaml:"os"`
			} `yaml:"requires"`
		} `yaml:"goclaw"`
	} `yaml:"metadata"`
	Requires SkillRequirements `yaml:"requires"` // 兼容旧格式
	Content  string            `yaml:"-"`        // 技能内容（Markdown）
}

// SkillRequirements 技能需求 (旧格式)
type SkillRequirements struct {
	Bins []string `yaml:"bins"`
	Env  []string `yaml:"env"`
}

// SkillsLoader 技能加载器
type SkillsLoader struct {
	workspace    string
	skillsDirs   []string
	skills       map[string]*Skill
	alwaysSkills []string
}

// NewSkillsLoader 创建技能加载器
func NewSkillsLoader(workspace string, skillsDirs []string) *SkillsLoader {
	return &SkillsLoader{
		workspace:  workspace,
		skillsDirs: skillsDirs,
		skills:     make(map[string]*Skill),
	}
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

	// 检查需求
	if !l.checkRequirements(&skill) {
		return nil // 需求不满足，跳过
	}

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

// checkRequirements 检查技能需求
func (l *SkillsLoader) checkRequirements(skill *Skill) bool {
	// 优先检查 always 标记
	if skill.Always || skill.Metadata.OpenClaw.Always {
		return true
	}

	// 1. 检查操作系统兼容性
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

	// 2. 检查二进制文件 (metadata.openclaw)
	for _, bin := range skill.Metadata.OpenClaw.Requires.Bins {
		if _, err := exec.LookPath(bin); err != nil {
			return false
		}
	}

	// 3. 检查 AnyBins (至少一个存在即可)
	if len(skill.Metadata.OpenClaw.Requires.AnyBins) > 0 {
		foundAny := false
		for _, bin := range skill.Metadata.OpenClaw.Requires.AnyBins {
			if _, err := exec.LookPath(bin); err == nil {
				foundAny = true
				break
			}
		}
		if !foundAny {
			return false
		}
	}

	// 4. 检查旧格式二进制
	for _, bin := range skill.Requires.Bins {
		if _, err := exec.LookPath(bin); err != nil {
			return false
		}
	}

	// 5. 检查环境变量
	for _, env := range skill.Metadata.OpenClaw.Requires.Env {
		if os.Getenv(env) == "" {
			return false
		}
	}
	for _, env := range skill.Requires.Env {
		if os.Getenv(env) == "" {
			return false
		}
	}

	return true
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
