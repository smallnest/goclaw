// Package commands 提供可扩展的 slash 命令处理
package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chzyer/readline"
	"github.com/manifoldco/promptui"
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
	commands map[string]*Command
	homeDir  string
	menuMode  bool // 是否在菜单选择模式
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
		Name:  "read",
		Usage: "/read <file>",
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
		Name:  "cd",
		Usage: "/cd [directory]",
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
		Name:  "ls",
		Usage: "/ls [directory]",
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
	sb.WriteString("Available commands: /quit /exit /clear /clear-sessions /help /read /cd /pwd /ls (Tab to show menu)")
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
		Label:     "Select a command",
		Items:     items,
		Size:      10, // 显示10个选项
		HideHelp:  true,
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
