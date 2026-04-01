package openclaw

// BaseMethods 基础方法列表（99个）
var BaseMethods = []string{
	// 系统状态
	"health",
	"doctor.memory.status",

	// 日志
	"logs.tail",

	// 频道管理
	"channels.status",
	"channels.logout",

	// 状态和成本
	"status",
	"usage.status",
	"usage.cost",

	// TTS（文本转语音）
	"tts.status",
	"tts.providers",
	"tts.enable",
	"tts.disable",
	"tts.convert",
	"tts.setProvider",

	// 配置管理
	"config.get",
	"config.set",
	"config.apply",
	"config.patch",
	"config.schema",

	// 执行批准
	"exec.approvals.get",
	"exec.approvals.set",
	"exec.approvals.node.get",
	"exec.approvals.node.set",
	"exec.approval.request",
	"exec.approval.waitDecision",
	"exec.approval.resolve",

	// 向导
	"wizard.start",
	"wizard.next",
	"wizard.cancel",
	"wizard.status",

	// 语音唤醒
	"talk.config",
	"talk.mode",

	// 模型和工具
	"models.list",
	"tools.catalog",

	// Agents 管理
	"agents.list",
	"agents.create",
	"agents.update",
	"agents.delete",
	"agents.files.list",
	"agents.files.get",
	"agents.files.set",

	// Skills
	"skills.status",
	"skills.bins",
	"skills.install",
	"skills.update",

	// 更新
	"update.run",
	"voicewake.get",
	"voicewake.set",
	"secrets.reload",

	// Sessions
	"sessions.list",
	"sessions.preview",
	"sessions.patch",
	"sessions.reset",
	"sessions.delete",
	"sessions.compact",

	// 心跳和唤醒
	"last-heartbeat",
	"set-heartbeats",
	"wake",

	// Node 配对
	"node.pair.request",
	"node.pair.list",
	"node.pair.approve",
	"node.pair.reject",
	"node.pair.verify",

	// 设备配对
	"device.pair.list",
	"device.pair.approve",
	"device.pair.reject",
	"device.pair.remove",
	"device.token.rotate",
	"device.token.revoke",

	// Node 管理
	"node.rename",
	"node.list",
	"node.describe",
	"node.invoke",
	"node.invoke.result",
	"node.event",
	"node.canvas.capability.refresh",

	// Cron
	"cron.list",
	"cron.status",
	"cron.add",
	"cron.update",
	"cron.remove",
	"cron.run",
	"cron.runs",

	// 系统事件
	"system-presence",
	"system-event",

	// 发送和 Agent
	"send",
	"agent",
	"agent.identity.get",
	"agent.wait",

	// 浏览器
	"browser.request",

	// WebChat WebSocket 原生聊天方法
	"chat.history",
	"chat.abort",
	"chat.send",
}

// GatewayEvents Gateway 事件列表（20个）
var GatewayEvents = []string{
	"connect.challenge",       // 连接挑战
	"agent",                   // Agent 事件
	"chat",                    // 聊天事件
	"presence",                // 在线状态
	"tick",                    // 定时心跳
	"talk.mode",               // 语音模式
	"shutdown",                // 关闭事件
	"health",                  // 健康状态
	"heartbeat",               // 心跳
	"cron",                    // Cron 事件
	"node.pair.requested",     // Node 配对请求
	"node.pair.resolved",      // Node 配对解决
	"node.invoke.request",     // Node 调用请求
	"device.pair.requested",   // 设备配对请求
	"device.pair.resolved",    // 设备配对解决
	"voicewake.changed",       // 语音唤醒变更
	"exec.approval.requested", // 执行批准请求
	"exec.approval.resolved",  // 执行批准解决
	"update.available",        // 更新可用
}

// ControlPlaneWriteMethods 控制平面写操作方法（需要速率限制）
var ControlPlaneWriteMethods = map[string]bool{
	"config.set":            true,
	"config.apply":          true,
	"config.patch":          true,
	"agents.create":         true,
	"agents.update":         true,
	"agents.delete":         true,
	"agents.files.set":      true,
	"skills.install":        true,
	"skills.update":         true,
	"sessions.reset":        true,
	"sessions.delete":       true,
	"sessions.patch":        true,
	"cron.add":              true,
	"cron.update":           true,
	"cron.remove":           true,
	"node.pair.approve":     true,
	"node.pair.reject":      true,
	"device.pair.approve":   true,
	"device.pair.reject":    true,
	"device.pair.remove":    true,
	"device.token.rotate":   true,
	"device.token.revoke":   true,
	"exec.approval.resolve": true,
}

// GetFeatures 获取功能列表
func GetFeatures() Features {
	return Features{
		Methods: BaseMethods,
		Events:  GatewayEvents,
	}
}

// IsControlPlaneWriteMethod 检查是否是控制平面写操作
func IsControlPlaneWriteMethod(method string) bool {
	return ControlPlaneWriteMethods[method]
}

// IsValidMethod 检查方法是否有效
func IsValidMethod(method string) bool {
	for _, m := range BaseMethods {
		if m == method {
			return true
		}
	}
	return false
}

// IsValidEvent 检查事件是否有效
func IsValidEvent(event string) bool {
	for _, e := range GatewayEvents {
		if e == event {
			return true
		}
	}
	return false
}
