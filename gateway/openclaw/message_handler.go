package openclaw

import (
	"encoding/json"
	"fmt"
)

// MethodHandler 方法处理器函数类型
type MethodHandler func(conn *Connection, req *Request) (interface{}, *ErrorInfo)

// MessageHandler 消息处理器
type MessageHandler struct {
	methods     map[string]MethodHandler
	authContext *AuthContext
	snapshotMgr *SnapshotManager
	rateLimiter *RateLimiter
}

// NewMessageHandler 创建消息处理器
func NewMessageHandler(authContext *AuthContext, snapshotMgr *SnapshotManager) *MessageHandler {
	return &MessageHandler{
		methods:     make(map[string]MethodHandler),
		authContext: authContext,
		snapshotMgr: snapshotMgr,
		rateLimiter: NewRateLimiter(),
	}
}

// Register 注册方法处理器
func (mh *MessageHandler) Register(method string, handler MethodHandler) {
	mh.methods[method] = handler
}

// Handle 处理请求
func (mh *MessageHandler) Handle(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
	// 检查方法是否存在
	handler, ok := mh.methods[req.Method]
	if !ok {
		return nil, NewErrorInfo(ErrorMethodNotFound, fmt.Sprintf("method not found: %s", req.Method))
	}

	// 检查权限
	if err := mh.authorizeMethod(conn, req.Method); err != nil {
		return nil, err
	}

	// 检查速率限制（控制平面写操作）
	if IsControlPlaneWriteMethod(req.Method) {
		if !mh.rateLimiter.Allow(conn.ID()) {
			return nil, NewErrorInfo(ErrorRateLimited, "too many control plane write requests")
		}
	}

	// 调用处理器
	return handler(conn, req)
}

// authorizeMethod 授权方法调用
func (mh *MessageHandler) authorizeMethod(conn *Connection, method string) *ErrorInfo {
	// 根据方法检查所需权限
	requiredScopes := mh.getMethodScopes(method)
	if len(requiredScopes) == 0 {
		return nil // 无需权限
	}

	if !conn.HasAnyScope(requiredScopes) {
		return NewErrorInfo(ErrorUnauthorized, fmt.Sprintf("insufficient permissions for method: %s", method))
	}

	return nil
}

// getMethodScopes 获取方法所需权限
func (mh *MessageHandler) getMethodScopes(method string) []string {
	switch method {
	// 配置管理
	case "config.get", "config.schema":
		return []string{ScopeRead}
	case "config.set", "config.apply", "config.patch":
		return []string{ScopeConfig, ScopeWrite}

	// Agents 管理
	case "agents.list":
		return []string{ScopeRead, ScopeAgents}
	case "agents.create", "agents.update", "agents.delete":
		return []string{ScopeAgents, ScopeWrite}
	case "agents.files.get":
		return []string{ScopeRead, ScopeAgents}
	case "agents.files.set":
		return []string{ScopeAgents, ScopeWrite}

	// Sessions 管理
	case "sessions.list", "sessions.preview":
		return []string{ScopeRead, ScopeSessions}
	case "sessions.patch", "sessions.reset", "sessions.delete", "sessions.compact":
		return []string{ScopeSessions, ScopeWrite}

	// Cron 管理
	case "cron.list", "cron.status", "cron.runs":
		return []string{ScopeRead, ScopeCron}
	case "cron.add", "cron.update", "cron.remove", "cron.run":
		return []string{ScopeCron, ScopeWrite}

	// Channels 管理
	case "channels.status":
		return []string{ScopeRead, ScopeChannels}
	case "channels.logout":
		return []string{ScopeChannels, ScopeWrite}

	// Node 管理
	case "node.list", "node.describe":
		return []string{ScopeRead, ScopeNodes}
	case "node.rename", "node.pair.request", "node.pair.approve", "node.pair.reject", "node.pair.verify":
		return []string{ScopeNodes, ScopeWrite}
	case "node.invoke", "node.invoke.result", "node.event":
		return []string{ScopeNodes, ScopeExecute}

	// 设备管理
	case "device.pair.list":
		return []string{ScopeRead, ScopeDevices}
	case "device.pair.approve", "device.pair.reject", "device.pair.remove":
		return []string{ScopeDevices, ScopeWrite}
	case "device.token.rotate", "device.token.revoke":
		return []string{ScopeDevices, ScopeWrite}

	// 聊天
	case "chat.history", "chat.abort":
		return []string{ScopeRead}
	case "chat.send":
		return []string{ScopeWrite}

	// 执行批准
	case "exec.approvals.get", "exec.approvals.node.get":
		return []string{ScopeRead}
	case "exec.approvals.set", "exec.approvals.node.set", "exec.approval.resolve":
		return []string{ScopeWrite}
	case "exec.approval.request", "exec.approval.waitDecision":
		return []string{ScopeExecute}

	// 系统状态（通常不需要特殊权限）
	case "health", "status", "doctor.memory.status", "logs.tail":
		return nil

	// 通用发送
	case "send", "agent", "agent.wait":
		return []string{ScopeWrite}

	default:
		return []string{ScopeRead}
	}
}

// RegisterSystemMethods 注册系统方法
func (mh *MessageHandler) RegisterSystemMethods() {
	// health - 健康检查
	mh.Register("health", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		return map[string]interface{}{
			"status": "ok",
		}, nil
	})

	// status - 状态
	mh.Register("status", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		snapshot := mh.snapshotMgr.GetSnapshot()
		return map[string]interface{}{
			"uptime_ms":     snapshot.UptimeMs,
			"state_version": snapshot.StateVersion,
			"health":        snapshot.Health,
		}, nil
	})

	// doctor.memory.status - 内存状态
	mh.Register("doctor.memory.status", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		// 简化实现
		return map[string]interface{}{
			"status": "ok",
			"usage":  "normal",
		}, nil
	})

	// logs.tail - 日志尾部
	mh.Register("logs.tail", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			Lines int `json:"lines"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.Lines <= 0 {
			params.Lines = 100
		}

		return map[string]interface{}{
			"lines": params.Lines,
			"logs":  []string{}, // 实际应该返回日志
		}, nil
	})

	// last-heartbeat - 最后心跳
	mh.Register("last-heartbeat", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		return map[string]interface{}{
			"timestamp": conn.LastActivityAt().Unix(),
		}, nil
	})

	// wake - 唤醒
	mh.Register("wake", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		// 触发唤醒事件
		return map[string]interface{}{
			"status": "woken",
		}, nil
	})

	// system-presence - 系统在线状态
	mh.Register("system-presence", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		presence := mh.snapshotMgr.ListPresence()
		return presence, nil
	})
}

// RegisterConfigMethods 注册配置方法
func (mh *MessageHandler) RegisterConfigMethods() {
	// config.get
	mh.Register("config.get", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			Key string `json:"key"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		// 实际应该从配置中读取
		return map[string]interface{}{
			"key":   params.Key,
			"value": nil,
		}, nil
	})

	// config.set
	mh.Register("config.set", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			Key   string      `json:"key"`
			Value interface{} `json:"value"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		return map[string]interface{}{
			"key":   params.Key,
			"value": params.Value,
		}, nil
	})

	// config.schema
	mh.Register("config.schema", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		return map[string]interface{}{
			"schema": map[string]interface{}{},
		}, nil
	})
}

// parseParams 解析参数
func parseParams(params json.RawMessage, v interface{}) error {
	if len(params) == 0 {
		return nil
	}
	return json.Unmarshal(params, v)
}
