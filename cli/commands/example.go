// Package commands 示例：如何扩展 slash 命令系统
package commands

/*
扩展示例：

// 1. 创建命令注册表
registry := commands.NewCommandRegistry()

// 2. 注册自定义命令
registry.Register(&commands.Command{
    Name:        "status",
    Usage:       "/status",
    Description: "Show agent status and statistics",
    Handler: func(args []string) (string, bool) {
        // 返回状态信息
        return "Agent is running. Messages: 42, Tools used: 15", false
    },
})

// 3. 注册带参数的命令
registry.Register(&commands.Command{
    Name:        "config",
    Usage:       "/config <get|set> <key> [value]",
    Description: "Get or set configuration",
    Handler: func(args []string) (string, bool) {
        if len(args) < 1 {
            return "Usage: /config <get|set> <key> [value]", false
        }
        switch args[0] {
        case "get":
            if len(args) < 2 {
                return "Usage: /config get <key>", false
            }
            return fmt.Sprintf("Config[%s] = %s", args[1], "value"), false
        case "set":
            if len(args) < 3 {
                return "Usage: /config set <key> <value>", false
            }
            return fmt.Sprintf("Set %s = %s", args[1], args[2]), false
        default:
            return "Unknown action: " + args[0], false
        }
    },
})

// 4. 注册会话相关命令
registry.Register(&commands.Command{
    Name:        "session",
    Usage:       "/session <list|switch|new>",
    Description: "Manage chat sessions",
    Handler: func(args []string) (string, bool) {
        if len(args) < 1 {
            return "Usage: /session <list|switch|new>", false
        }
        switch args[0] {
        case "list":
            return "Sessions: default (current), work, personal", false
        case "switch":
            if len(args) < 2 {
                return "Usage: /session switch <name>", false
            }
            return "Switched to session: " + args[1], false
        case "new":
            if len(args) < 2 {
                return "Usage: /session new <name>", false
            }
            return "Created new session: " + args[1], false
        default:
            return "Unknown action: " + args[0], false
        }
    },
})

// 5. 在主循环中使用
result, isCommand, shouldExit := registry.Execute(input)
if isCommand {
    if shouldExit {
        fmt.Println("Goodbye!")
        break
    }
    if result != "" {
        fmt.Println(result)
    }
    continue
}
*/
