// Package commands 提供可扩展的 slash 命令处理
package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ergochat/readline"
	"github.com/manifoldco/promptui"
	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/session"
)

// SpecialMarker 是用于触发菜单选择的特殊标记
const SpecialMarker = "__MENU_SELECT__"

// Command 命令定义
type Command struct {
	Name        string
	Usage       string
	Description string
	Handler     func(args []string) (string, bool) // 返回结果和是否应该退出
	ArgsSpec    []ArgSpec                          // 参数定义（用于补全）
}

// ArgSpec 参数定义
type ArgSpec struct {
	Name        string
	Description string
	Type        string // "file", "directory", "enum"
	EnumValues  []string
}

// CommandRegistry 命令注册表
type CommandRegistry struct {
	commands     map[string]*Command
	homeDir      string
	menuMode     bool // 是否在菜单选择模式
	sessionMgr   *session.Manager
	stopped      bool                                   // 停止标志，用于中止正在运行的 agent
	toolGetter   func() (map[string]interface{}, error) // 获取工具列表的函数
	skillsGetter func() ([]*SkillInfo, error)           // 获取技能列表的函数
}

// SkillInfo 技能信息
type SkillInfo struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Version     string           `json:"version"`
	Author      string           `json:"author"`
	Homepage    string           `json:"homepage"`
	Always      bool             `json:"always"`
	Emoji       string           `json:"emoji"`
	MissingDeps *MissingDepsInfo `json:"missing_deps,omitempty"`
}

// MissingDepsInfo 缺失依赖信息
type MissingDepsInfo struct {
	Bins       []string `json:"bins,omitempty"`
	AnyBins    []string `json:"any_bins,omitempty"`
	Env        []string `json:"env,omitempty"`
	PythonPkgs []string `json:"python_pkgs,omitempty"`
	NodePkgs   []string `json:"node_pkgs,omitempty"`
}

// NewCommandRegistry 创建命令注册表
func NewCommandRegistry() *CommandRegistry {
	homeDir, _ := os.UserHomeDir()
	registry := &CommandRegistry{
		commands: make(map[string]*Command),
		homeDir:  homeDir,
	}
	registry.registerBuiltInCommands()
	return registry
}

// SetSessionManager 设置会话管理器
func (r *CommandRegistry) SetSessionManager(mgr *session.Manager) {
	r.sessionMgr = mgr
}

// SetToolGetter 设置工具获取函数
func (r *CommandRegistry) SetToolGetter(getter func() (map[string]interface{}, error)) {
	r.toolGetter = getter
}

// SetSkillsGetter 设置技能获取函数
func (r *CommandRegistry) SetSkillsGetter(getter func() ([]*SkillInfo, error)) {
	r.skillsGetter = getter
}

// SetTUIAgent 设置 TUIAgent（用于从工具和技能信息获取数据）
func (r *CommandRegistry) SetTUIAgent(agent *TUIAgent) {
	r.toolGetter = func() (map[string]interface{}, error) {
		// 从 agent 获取工具信息
		tools := agent.GetState().Tools
		result := make(map[string]interface{})
		for _, t := range tools {
			result[t.Name()] = map[string]interface{}{
				"name":        t.Name(),
				"description": t.Description(),
				"parameters":  t.Parameters(),
			}
		}
		return result, nil
	}

	r.skillsGetter = func() ([]*SkillInfo, error) {
		// 从 skillsLoader 获取技能信息
		agentSkills := agent.skillsLoader.List()
		result := make([]*SkillInfo, 0, len(agentSkills))
		for _, skill := range agentSkills {
			skillInfo := &SkillInfo{
				Name:        skill.Name,
				Description: skill.Description,
				Version:     skill.Version,
				Author:      skill.Author,
				Homepage:    skill.Homepage,
				Always:      skill.Always,
				Emoji:       skill.Metadata.OpenClaw.Emoji,
			}
			// 转换缺失依赖信息
			if skill.MissingDeps != nil {
				skillInfo.MissingDeps = &MissingDepsInfo{
					Bins:       skill.MissingDeps.Bins,
					AnyBins:    skill.MissingDeps.AnyBins,
					Env:        skill.MissingDeps.Env,
					PythonPkgs: skill.MissingDeps.PythonPkgs,
					NodePkgs:   skill.MissingDeps.NodePkgs,
				}
			}
			result = append(result, skillInfo)
		}
		return result, nil
	}
}

// GetSessionManager 获取会话管理器
func (r *CommandRegistry) GetSessionManager() *session.Manager {
	return r.sessionMgr
}

// Stop 设置停止标志，用于中止正在运行的 agent
func (r *CommandRegistry) Stop() {
	r.stopped = true
}

// ResetStop 重置停止标志
func (r *CommandRegistry) ResetStop() {
	r.stopped = false
}

// IsStopped 检查是否被停止
func (r *CommandRegistry) IsStopped() bool {
	return r.stopped
}

// registerBuiltInCommands 注册内置命令
func (r *CommandRegistry) registerBuiltInCommands() {
	// /quit - 退出
	r.Register(&Command{
		Name:        "quit",
		Usage:       "/quit",
		Description: "Exit the chat session",
		Handler: func(args []string) (string, bool) {
			return "", true // true 表示退出
		},
	})

	// /exit - 退出
	r.Register(&Command{
		Name:        "exit",
		Usage:       "/exit",
		Description: "Exit the chat session",
		Handler: func(args []string) (string, bool) {
			return "", true // true 表示退出
		},
	})

	// /clear - 清空历史
	r.Register(&Command{
		Name:        "clear",
		Usage:       "/clear",
		Description: "Clear chat history (current session only)",
		Handler: func(args []string) (string, bool) {
			return "History cleared.", false
		},
	})

	// /clear-sessions - 清除所有会话文件
	r.Register(&Command{
		Name:        "clear-sessions",
		Usage:       "/clear-sessions",
		Description: "Clear all saved session files (restart recommended)",
		Handler: func(args []string) (string, bool) {
			sessionDir := filepath.Join(r.homeDir, ".goclaw", "sessions")
			// 检查目录是否存在
			if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
				return "No sessions to clear.", false
			}
			// 删除目录中的所有文件
			entries, err := os.ReadDir(sessionDir)
			if err != nil {
				return fmt.Sprintf("Error reading sessions directory: %v", err), false
			}
			count := 0
			for _, entry := range entries {
				if err := os.Remove(filepath.Join(sessionDir, entry.Name())); err == nil {
					count++
				}
			}
			if count > 0 {
				return fmt.Sprintf("Cleared %d session file(s). Restart the application to clear in-memory sessions.", count), false
			}
			return "No session files to clear.", false
		},
	})

	// /help - 帮助
	r.Register(&Command{
		Name:        "help",
		Usage:       "/help [command]",
		Description: "Show available commands or command help",
		Handler: func(args []string) (string, bool) {
			return r.buildHelp(args), false
		},
	})

	// /read - 读取文件
	r.Register(&Command{
		Name:        "read",
		Usage:       "/read <file>",
		Description: "Read and display file contents",
		ArgsSpec: []ArgSpec{
			{Name: "file", Description: "File path to read", Type: "file"},
		},
		Handler: func(args []string) (string, bool) {
			if len(args) == 0 {
				return "Usage: /read <file>", false
			}
			content, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Sprintf("Error reading file: %v", err), false
			}
			return string(content), false
		},
	})

	// /cd - 切换目录
	r.Register(&Command{
		Name:        "cd",
		Usage:       "/cd [directory]",
		Description: "Change current working directory (no args = home)",
		ArgsSpec: []ArgSpec{
			{Name: "directory", Description: "Directory to change to", Type: "directory"},
		},
		Handler: func(args []string) (string, bool) {
			target := r.homeDir
			if len(args) > 0 {
				target = args[0]
			}
			if err := os.Chdir(target); err != nil {
				return fmt.Sprintf("Error changing directory: %v", err), false
			}
			pwd, _ := os.Getwd()
			return fmt.Sprintf("Current directory: %s", pwd), false
		},
	})

	// /pwd - 显示当前目录
	r.Register(&Command{
		Name:        "pwd",
		Usage:       "/pwd",
		Description: "Print current working directory",
		Handler: func(args []string) (string, bool) {
			pwd, _ := os.Getwd()
			return pwd, false
		},
	})

	// /ls - 列出文件
	r.Register(&Command{
		Name:        "ls",
		Usage:       "/ls [directory]",
		Description: "List directory contents",
		ArgsSpec: []ArgSpec{
			{Name: "directory", Description: "Directory to list (default: current)", Type: "directory"},
		},
		Handler: func(args []string) (string, bool) {
			target := "."
			if len(args) > 0 {
				target = args[0]
			}
			entries, err := os.ReadDir(target)
			if err != nil {
				return fmt.Sprintf("Error listing directory: %v", err), false
			}
			var result []string
			for _, e := range entries {
				if e.IsDir() {
					result = append(result, e.Name()+"/")
				} else {
					result = append(result, e.Name())
				}
			}
			return strings.Join(result, "  "), false
		},
	})

	// /status - 显示状态
	r.Register(&Command{
		Name:        "status",
		Usage:       "/status",
		Description: "Show session and gateway status",
		Handler: func(args []string) (string, bool) {
			return r.handleStatus(args), false
		},
	})

	// /tools - 显示可用工具
	r.Register(&Command{
		Name:        "tools",
		Usage:       "/tools",
		Description: "List available tools",
		Handler: func(args []string) (string, bool) {
			return r.handleTools(args), false
		},
	})

	// /skills - 显示可用技能
	r.Register(&Command{
		Name:        "skills",
		Usage:       "/skills [search]",
		Description: "List available skills or search for a skill",
		Handler: func(args []string) (string, bool) {
			return r.handleSkills(args), false
		},
	})

	// /stop - 停止当前运行的 agent
	r.Register(&Command{
		Name:        "stop",
		Usage:       "/stop",
		Description: "Stop the current agent run",
		Handler: func(args []string) (string, bool) {
			r.Stop()
			return "⚙️ Agent run stopped.", false
		},
	})
}

// Register 注册命令
func (r *CommandRegistry) Register(cmd *Command) {
	r.commands[cmd.Name] = cmd
}

// Unregister 注销命令
func (r *CommandRegistry) Unregister(name string) {
	delete(r.commands, name)
}

// IsMenuMode 检查是否在菜单模式
func (r *CommandRegistry) IsMenuMode() bool {
	return r.menuMode
}

// SetMenuMode 设置菜单模式
func (r *CommandRegistry) SetMenuMode(enabled bool) {
	r.menuMode = enabled
}

// Execute 执行命令
// 返回 (响应消息, 是否是命令, 是否应该退出)
func (r *CommandRegistry) Execute(input string) (string, bool, bool) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return "", false, false // 不是命令
	}

	parts := strings.Fields(input)
	if len(parts) == 0 {
		return "", false, false
	}

	cmdName := strings.TrimPrefix(parts[0], "/")
	cmd, ok := r.commands[cmdName]
	if !ok {
		return fmt.Sprintf("Unknown command: /%s. Type /help for available commands.", cmdName), true, false
	}

	// 执行命令
	result, shouldExit := cmd.Handler(parts[1:])
	return result, true, shouldExit
}

// List 列出所有命令
func (r *CommandRegistry) List() []*Command {
	var cmds []*Command
	for _, cmd := range r.commands {
		cmds = append(cmds, cmd)
	}
	return cmds
}

// buildHelp 构建帮助信息
func (r *CommandRegistry) buildHelp(args []string) string {
	if len(args) > 0 {
		// 显示特定命令的帮助
		cmdName := strings.TrimPrefix(args[0], "/")
		cmd, ok := r.commands[cmdName]
		if !ok {
			return fmt.Sprintf("Unknown command: /%s", cmdName)
		}
		return fmt.Sprintf("%s\n\n%s", cmd.Usage, cmd.Description)
	}

	// 显示所有命令
	var sb strings.Builder
	sb.WriteString("Available commands:\n\n")
	for _, cmd := range r.List() {
		sb.WriteString(fmt.Sprintf("  %s  %s\n", cmd.Usage, cmd.Description))
	}
	return sb.String()
}

// handleStatus 处理 status 命令
func (r *CommandRegistry) handleStatus(args []string) string {
	var sb strings.Builder
	sb.WriteString("=== goclaw Status ===\n\n")

	// Gateway status
	gatewayStatus := r.checkGatewayStatus(5)
	sb.WriteString("Gateway:\n")
	if gatewayStatus.Online {
		sb.WriteString("  Status:  Online\n")
		sb.WriteString(fmt.Sprintf("  URL:     %s\n", gatewayStatus.URL))
		if gatewayStatus.Version != "" {
			sb.WriteString(fmt.Sprintf("  Version: %s\n", gatewayStatus.Version))
		}
		if gatewayStatus.Timestamp > 0 {
			t := time.Unix(gatewayStatus.Timestamp, 0)
			sb.WriteString(fmt.Sprintf("  Uptime:  %s\n", t.Format(time.RFC3339)))
		}
	} else {
		sb.WriteString("  Status:  Offline\n")
		sb.WriteString("  Tip:     Start gateway with 'goclaw gateway run'\n")
	}

	// Session status
	sessionDir := filepath.Join(r.homeDir, ".goclaw", "sessions")
	sb.WriteString("\nSessions:\n")

	var sessionKeys []string
	var sessionCount int

	if r.sessionMgr != nil {
		var err error
		sessionKeys, err = r.sessionMgr.List()
		if err != nil {
			sb.WriteString(fmt.Sprintf("  Error: %v\n", err))
		} else {
			sessionCount = len(sessionKeys)
		}
	} else {
		// Fallback: read directory directly
		if entries, err := os.ReadDir(sessionDir); err == nil {
			for _, e := range entries {
				if !e.IsDir() && filepath.Ext(e.Name()) == ".jsonl" {
					sessionKeys = append(sessionKeys, strings.TrimSuffix(e.Name(), ".jsonl"))
				}
			}
			sessionCount = len(sessionKeys)
		}
	}

	sb.WriteString(fmt.Sprintf("  Total:   %d\n", sessionCount))

	if len(sessionKeys) > 0 {
		sb.WriteString("\n  Recent sessions:\n")
		limit := 5
		if len(sessionKeys) < 5 {
			limit = len(sessionKeys)
		}

		for i := 0; i < limit; i++ {
			key := sessionKeys[i]
			sb.WriteString(fmt.Sprintf("    - %s\n", key))

			// Get message count if sessionMgr is available
			if r.sessionMgr != nil {
				if sess, err := r.sessionMgr.GetOrCreate(key); err == nil {
					sb.WriteString(fmt.Sprintf("      Messages: %d\n", len(sess.Messages)))
					sb.WriteString(fmt.Sprintf("      Created:  %s\n", sess.CreatedAt.Format("2006-01-02 15:04")))
					updatedAt := time.Since(sess.UpdatedAt)
					if updatedAt < time.Minute {
						sb.WriteString("      Updated:  just now\n")
					} else if updatedAt < time.Hour {
						sb.WriteString(fmt.Sprintf("      Updated:  %d min ago\n", int(updatedAt.Minutes())))
					} else if updatedAt < 24*time.Hour {
						sb.WriteString(fmt.Sprintf("      Updated:  %d hours ago\n", int(updatedAt.Hours())))
					} else {
						sb.WriteString(fmt.Sprintf("      Updated:  %s\n", sess.UpdatedAt.Format("2006-01-02 15:04")))
					}
				}
			}
		}

		if sessionCount > limit {
			sb.WriteString(fmt.Sprintf("\n  ... and %d more\n", sessionCount-limit))
		}
	}

	// Working directory
	pwd, _ := os.Getwd()
	sb.WriteString(fmt.Sprintf("\nWorking Directory:\n  %s\n", pwd))

	return sb.String()
}

// checkGatewayStatus checks if gateway is running
func (r *CommandRegistry) checkGatewayStatus(timeout int) GatewayStatus {
	result := GatewayStatus{Online: false}

	// Load config to get gateway port
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to load config, using default port: %v\n", err)
	}

	// Get port from config (defaults to 28789 if not configured)
	port := config.GetGatewayHTTPPort(cfg)

	client := &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
	}

	url := fmt.Sprintf("http://localhost:%d/health", port)
	resp, err := client.Get(url)
	if err == nil {
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			var health map[string]interface{}
			_ = json.Unmarshal(body, &health)

			result.Online = true
			result.URL = url
			result.Status = "ok"

			if status, ok := health["status"].(string); ok {
				result.Status = status
			}
			if version, ok := health["version"].(string); ok {
				result.Version = version
			}
			if ts, ok := health["time"].(float64); ok {
				result.Timestamp = int64(ts)
			}

			return result
		}
	}

	return result
}

// Completer 自动补全器
type Completer struct {
	registry *CommandRegistry
}

// Do 实现 AutoCompleter 接口
func (c *Completer) Do(line []rune, pos int) (newLine [][]rune, length int) {
	// 获取当前输入的字符串
	input := string(line[:pos])

	// 如果输入为空，返回空
	if len(input) == 0 {
		return nil, 0
	}

	// 分割输入
	words := strings.Fields(input)
	var currentWord string

	if len(words) > 0 {
		// 检查是否在输入最后一个词（有空格在最后）
		if strings.HasSuffix(input, " ") {
			currentWord = ""
		} else {
			currentWord = words[len(words)-1]
		}
	} else {
		currentWord = input
	}

	var suggestions [][]rune

	// 情况1: 输入以 "/" 开头，补全命令名
	if strings.HasPrefix(input, "/") {
		// 提取当前要补全的部分（去掉前导/）
		var toMatch string
		var replaceLen int // 从行首要删除的字符数

		if input == "/" {
			// 输入只有 "/"，不删除任何字符
			toMatch = ""
			replaceLen = 0
		} else if len(words) == 1 {
			// 正在输入命令名，如 /qui
			toMatch = strings.TrimPrefix(input, "/")
			// 删除整个 input，因为要替换成完整命令
			replaceLen = len(input) // 删除整个输入
		} else {
			// 已输入完整命令，准备补全参数
			toMatch = ""
			replaceLen = len(currentWord)
		}

		// 补全命令名 - 返回带 / 的完整命令名（因为要删除整个输入）
		for name := range c.registry.commands {
			if toMatch == "" || strings.HasPrefix(name, toMatch) {
				suggestions = append(suggestions, []rune("/"+name))
			}
		}
		if len(suggestions) > 0 {
			return suggestions, replaceLen
		}
	}

	// 情况2: 补全参数（文件路径、目录等）
	if len(words) > 0 && strings.HasPrefix(words[0], "/") {
		cmdName := strings.TrimPrefix(words[0], "/")
		if cmd, ok := c.registry.commands[cmdName]; ok {
			// 确定当前是第几个参数
			argIndex := len(words) - 1
			if strings.HasSuffix(input, " ") {
				argIndex = len(words)
			}

			if argIndex < len(cmd.ArgsSpec) {
				argSpec := cmd.ArgsSpec[argIndex]
				switch argSpec.Type {
				case "file", "directory":
					suggestions = c.completePath(currentWord, argSpec.Type == "directory")
					return suggestions, len(input) - len(currentWord)
				case "enum":
					for _, val := range argSpec.EnumValues {
						if strings.HasPrefix(val, currentWord) {
							suggestions = append(suggestions, []rune(val))
						}
					}
					return suggestions, len(input) - len(currentWord)
				}
			}
		}
	}

	// 情况3: 通用文件路径补全
	suggestions = c.completePath(currentWord, false)
	if len(suggestions) > 0 {
		return suggestions, len(input) - len(currentWord)
	}

	return nil, 0
}

// completePath 补全文件路径
func (c *Completer) completePath(pattern string, onlyDirs bool) [][]rune {
	// 确定目录和前缀
	var dir, prefix string
	if strings.Contains(pattern, "/") {
		lastSlash := strings.LastIndex(pattern, "/")
		dir = pattern[:lastSlash+1]
		prefix = pattern[lastSlash+1:]
	} else {
		dir = ""
		prefix = pattern
	}

	// 如果是绝对路径
	if strings.HasPrefix(pattern, "/") || strings.HasPrefix(pattern, "~") {
		if strings.HasPrefix(pattern, "~") {
			// 处理 ~ 路径
			if c.registry.homeDir != "" {
				dir = c.registry.homeDir + dir[1:]
			}
		}
	} else {
		// 相对路径，使用当前目录
		if dir == "" {
			pwd, _ := os.Getwd()
			dir = pwd + "/"
		} else {
			pwd, _ := os.Getwd()
			dir = filepath.Join(pwd, dir)
		}
	}

	// 读取目录
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var suggestions [][]rune
	for _, entry := range entries {
		name := entry.Name()
		// 过滤匹配前缀的
		if strings.HasPrefix(name, prefix) {
			displayName := name
			if entry.IsDir() {
				displayName += "/"
			}
			// 如果只有目录，过滤掉文件
			if !onlyDirs || entry.IsDir() {
				// 对于非隐藏文件或者匹配的隐藏文件
				if !strings.HasPrefix(displayName, ".") || strings.HasPrefix(displayName, ".") {
					suggestions = append(suggestions, []rune(displayName))
				}
			}
		}
	}

	// 如果只有一个建议，直接完成路径
	if len(suggestions) == 1 {
		fullPath := filepath.Join(dir, string(suggestions[0]))
		if info, err := os.Stat(fullPath); err == nil && info.IsDir() {
			suggestions[0] = []rune(string(suggestions[0]) + "/")
		}
	}

	return suggestions
}

// NewCompleter 创建自动补全器
func (r *CommandRegistry) NewCompleter() readline.AutoCompleter {
	return &Completer{registry: r}
}

// GetCommandPrompt 获取命令提示信息
func (r *CommandRegistry) GetCommandPrompt() string {
	var sb strings.Builder
	sb.WriteString("Available commands: /quit /exit /clear /clear-sessions /help /status /tools /skills /stop /read /cd /pwd /ls (Tab to show menu)")
	return sb.String()
}

// SelectCommand 使用交互式菜单选择命令
// 返回选择的命令名，空字符串表示取消
func (r *CommandRegistry) SelectCommand() string {
	// 获取所有命令并转换为 promptui 格式
	var items []string
	for name, cmd := range r.commands {
		items = append(items, fmt.Sprintf("%s  %s", name, cmd.Description))
	}

	if len(items) == 0 {
		return ""
	}

	// 创建选择器
	prompt := promptui.Select{
		Label:        "Select a command",
		Items:        items,
		Size:         10, // 显示10个选项
		HideHelp:     true,
		HideSelected: true,
	}

	// 提取选择的命令名
	_, result, err := prompt.Run()
	if err != nil {
		return ""
	}

	// 解析命令名（去掉描述部分）
	return strings.Fields(result)[0]
}

// GetCommandListAsText 获取命令列表文本格式
// 用于显示给用户
func (r *CommandRegistry) GetCommandListAsText() string {
	var sb strings.Builder
	sb.WriteString("Available commands:\n")
	for name, cmd := range r.commands {
		sb.WriteString(fmt.Sprintf("  %s  %s\n", name, cmd.Description))
	}
	return sb.String()
}

// handleTools 处理 tools 命令
func (r *CommandRegistry) handleTools(args []string) string {
	var sb strings.Builder
	sb.WriteString("=== Available Tools ===\n\n")

	if r.toolGetter == nil {
		sb.WriteString("Tool registry not available. Please start the agent first.\n")
		return sb.String()
	}

	tools, err := r.toolGetter()
	if err != nil {
		sb.WriteString(fmt.Sprintf("Error fetching tools: %v\n", err))
		return sb.String()
	}

	if len(tools) == 0 {
		sb.WriteString("No tools registered.\n")
		return sb.String()
	}

	// 分类工具
	coreTools := []string{}
	fileTools := []string{}
	webTools := []string{}
	browserTools := []string{}
	otherTools := []string{}

	for name, tool := range tools {
		toolMap, ok := tool.(map[string]interface{})
		if !ok {
			otherTools = append(otherTools, name)
			continue
		}

		desc := "No description"
		if d, ok := toolMap["description"].(string); ok {
			desc = d
		}

		toolEntry := fmt.Sprintf("  %-20s  %s", name, desc)

		// 简单分类
		if strings.Contains(name, "read") || strings.Contains(name, "write") || strings.Contains(name, "exec") || strings.Contains(name, "file") || strings.Contains(name, "shell") {
			fileTools = append(fileTools, toolEntry)
		} else if strings.Contains(name, "web") || strings.Contains(name, "search") {
			webTools = append(webTools, toolEntry)
		} else if strings.Contains(name, "browser") {
			browserTools = append(browserTools, toolEntry)
		} else if strings.Contains(name, "spawn") {
			coreTools = append(coreTools, toolEntry)
		} else {
			otherTools = append(otherTools, toolEntry)
		}
	}

	// 按分类显示
	if len(coreTools) > 0 {
		sb.WriteString("Core:\n")
		for _, t := range coreTools {
			sb.WriteString(t + "\n")
		}
		sb.WriteString("\n")
	}

	if len(fileTools) > 0 {
		sb.WriteString("File System:\n")
		for _, t := range fileTools {
			sb.WriteString(t + "\n")
		}
		sb.WriteString("\n")
	}

	if len(webTools) > 0 {
		sb.WriteString("Web:\n")
		for _, t := range webTools {
			sb.WriteString(t + "\n")
		}
		sb.WriteString("\n")
	}

	if len(browserTools) > 0 {
		sb.WriteString("Browser:\n")
		for _, t := range browserTools {
			sb.WriteString(t + "\n")
		}
		sb.WriteString("\n")
	}

	if len(otherTools) > 0 {
		sb.WriteString("Other:\n")
		for _, t := range otherTools {
			sb.WriteString(t + "\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("Total: %d tools\n", len(tools)))

	return sb.String()
}

// handleSkills 处理 skills 命令
func (r *CommandRegistry) handleSkills(args []string) string {
	var sb strings.Builder

	// 如果有搜索参数，执行搜索
	if len(args) > 0 {
		return r.searchSkills(strings.Join(args, " "))
	}

	sb.WriteString("=== Available Skills ===\n\n")

	if r.skillsGetter == nil {
		sb.WriteString("Skills registry not available. Please start the agent first.\n")
		return sb.String()
	}

	skills, err := r.skillsGetter()
	if err != nil {
		sb.WriteString(fmt.Sprintf("Error fetching skills: %v\n", err))
		return sb.String()
	}

	if len(skills) == 0 {
		sb.WriteString("No skills registered.\n")
		return sb.String()
	}

	// 分类技能
	alwaysSkills := []*SkillInfo{}
	otherSkills := []*SkillInfo{}

	for _, skill := range skills {
		if skill.Always {
			alwaysSkills = append(alwaysSkills, skill)
		} else {
			otherSkills = append(otherSkills, skill)
		}
	}

	// 显示始终加载的技能
	if len(alwaysSkills) > 0 {
		sb.WriteString("Always Loaded:\n")
		for _, s := range alwaysSkills {
			skillEntry := r.formatSkillEntry(s)
			sb.WriteString(skillEntry + "\n")
		}
		sb.WriteString("\n")
	}

	// 显示其他技能
	if len(otherSkills) > 0 {
		sb.WriteString("Available:\n")
		for _, s := range otherSkills {
			skillEntry := r.formatSkillEntry(s)
			sb.WriteString(skillEntry + "\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("Total: %d skills\n", len(skills)))
	sb.WriteString("\nUse /skills <keyword> to search for specific skills.\n")

	return sb.String()
}

// searchSkills 搜索技能
func (r *CommandRegistry) searchSkills(query string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== Search Results: \"%s\" ===\n\n", query))

	if r.skillsGetter == nil {
		sb.WriteString("Skills registry not available.\n")
		return sb.String()
	}

	skills, err := r.skillsGetter()
	if err != nil {
		sb.WriteString(fmt.Sprintf("Error fetching skills: %v\n", err))
		return sb.String()
	}

	if len(skills) == 0 {
		sb.WriteString("No skills available.\n")
		return sb.String()
	}

	query = strings.ToLower(query)
	results := []*SkillInfo{}

	for _, skill := range skills {
		score := 0.0

		// 检查名称匹配
		if strings.Contains(strings.ToLower(skill.Name), query) {
			if strings.EqualFold(skill.Name, query) {
				score += 1.0
			} else {
				score += 0.8
			}
		}

		// 检查描述匹配
		if strings.Contains(strings.ToLower(skill.Description), query) {
			score += 0.6
		}

		// 检查作者匹配
		if strings.Contains(strings.ToLower(skill.Author), query) {
			score += 0.4
		}

		if score > 0 {
			results = append(results, skill)
		}
	}

	if len(results) == 0 {
		sb.WriteString("No matching skills found.\n")
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("Found %d skill(s):\n\n", len(results)))
	for _, s := range results {
		skillEntry := r.formatSkillEntry(s)
		sb.WriteString(skillEntry + "\n")
	}

	return sb.String()
}

// formatSkillEntry 格式化技能条目
func (r *CommandRegistry) formatSkillEntry(skill *SkillInfo) string {
	var sb strings.Builder

	// Emoji + 名称
	emoji := skill.Emoji
	if emoji == "" {
		emoji = "📦"
	}
	sb.WriteString(fmt.Sprintf("  %s %-25s", emoji, skill.Name))

	// 描述
	if skill.Description != "" {
		// 截断过长的描述
		desc := skill.Description
		if len(desc) > 50 {
			desc = desc[:50] + "..."
		}
		sb.WriteString(fmt.Sprintf("  %s", desc))
	}

	// 版本
	if skill.Version != "" {
		sb.WriteString(fmt.Sprintf("  [%s]", skill.Version))
	}

	// 缺失依赖标记
	if skill.MissingDeps != nil && r.hasMissingDeps(skill.MissingDeps) {
		sb.WriteString("  [⚠️]")
	}

	// 始终加载标记
	if skill.Always {
		sb.WriteString("  [★]")
	}

	return sb.String()
}

// hasMissingDeps 检查是否有缺失依赖
func (r *CommandRegistry) hasMissingDeps(deps *MissingDepsInfo) bool {
	if deps == nil {
		return false
	}
	return len(deps.Bins) > 0 || len(deps.AnyBins) > 0 ||
		len(deps.PythonPkgs) > 0 || len(deps.NodePkgs) > 0 ||
		len(deps.Env) > 0
}
