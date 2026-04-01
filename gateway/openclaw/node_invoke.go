package openclaw

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// NodeInvokeManager Node 调用管理器
type NodeInvokeManager struct {
	mu           sync.RWMutex
	authContext  *AuthContext
	broadcastMgr *BroadcastManager
	invokeMgr    *InvokeManager
}

// InvokeManager 调用管理器
type InvokeManager struct {
	mu      sync.RWMutex
	invokes map[string]*InvokeState // invokeID -> state
	seq     int64
}

// InvokeState 调用状态
type InvokeState struct {
	InvokeID  string
	NodeID    string
	Method    string
	Params    json.RawMessage
	ConnID    string
	CreatedAt time.Time
	ExpiresAt time.Time
	Status    string // "pending" | "delivered" | "running" | "completed" | "failed" | "timeout"
	Result    json.RawMessage
	Error     string
}

// NewNodeInvokeManager 创建 Node 调用管理器
func NewNodeInvokeManager(authContext *AuthContext, broadcastMgr *BroadcastManager) *NodeInvokeManager {
	return &NodeInvokeManager{
		authContext:  authContext,
		broadcastMgr: broadcastMgr,
		invokeMgr:    NewInvokeManager(),
	}
}

// NewInvokeManager 创建调用管理器
func NewInvokeManager() *InvokeManager {
	return &InvokeManager{
		invokes: make(map[string]*InvokeState),
		seq:     0,
	}
}

// Invoke 调用 Node 方法
func (nim *NodeInvokeManager) Invoke(nodeID, method string, params interface{}, connID string, timeout time.Duration) (*InvokeState, error) {
	nim.mu.Lock()
	defer nim.mu.Unlock()

	// 检查 Node 是否已配对
	if _, ok := nim.authContext.GetNodePair(nodeID); !ok {
		return nil, fmt.Errorf("node not paired: %s", nodeID)
	}

	// 生成调用 ID
	invokeID := fmt.Sprintf("invoke_%d", nim.invokeMgr.nextSeq())

	// 序列化参数
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal params: %w", err)
	}

	// 创建调用状态
	state := &InvokeState{
		InvokeID:  invokeID,
		NodeID:    nodeID,
		Method:    method,
		Params:    paramsJSON,
		ConnID:    connID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(timeout),
		Status:    "pending",
	}

	nim.invokeMgr.invokes[invokeID] = state

	// 广播调用请求
	_ = nim.broadcastMgr.BroadcastNodeInvokeRequest(nodeID, invokeID, method, params)

	return state, nil
}

// GetResult 获取调用结果
func (nim *NodeInvokeManager) GetResult(invokeID string) (*InvokeState, error) {
	nim.mu.RLock()
	defer nim.mu.RUnlock()

	state, ok := nim.invokeMgr.invokes[invokeID]
	if !ok {
		return nil, fmt.Errorf("invoke not found: %s", invokeID)
	}

	// 返回副本
	copy := *state
	return &copy, nil
}

// HandleInvokeResult 处理 Node 返回的结果
func (nim *NodeInvokeManager) HandleInvokeResult(invokeID string, result json.RawMessage, errMsg string) error {
	nim.mu.Lock()
	defer nim.mu.Unlock()

	state, ok := nim.invokeMgr.invokes[invokeID]
	if !ok {
		return fmt.Errorf("invoke not found: %s", invokeID)
	}

	if errMsg != "" {
		state.Status = "failed"
		state.Error = errMsg
	} else {
		state.Status = "completed"
		state.Result = result
	}

	return nil
}

// ListInvokes 列出调用（按 Node）
func (nim *NodeInvokeManager) ListInvokes(nodeID string) []*InvokeState {
	nim.mu.RLock()
	defer nim.mu.RUnlock()

	var result []*InvokeState
	for _, state := range nim.invokeMgr.invokes {
		if nodeID == "" || state.NodeID == nodeID {
			copy := *state
			result = append(result, &copy)
		}
	}
	return result
}

// CleanupExpired 清理过期的调用
func (nim *NodeInvokeManager) CleanupExpired() {
	nim.mu.Lock()
	defer nim.mu.Unlock()

	now := time.Now()
	for invokeID, state := range nim.invokeMgr.invokes {
		if now.After(state.ExpiresAt) && state.Status != "completed" && state.Status != "failed" {
			state.Status = "timeout"
		}

		// 清理超过 1 小时的旧记录
		if now.Sub(state.CreatedAt) > time.Hour {
			delete(nim.invokeMgr.invokes, invokeID)
		}
	}
}

// nextSeq 获取下一个序列号
func (im *InvokeManager) nextSeq() int64 {
	im.seq++
	return im.seq
}

// CanvasCapability Canvas 能力
type CanvasCapability struct {
	Enabled    bool      `json:"enabled"`
	ExpiresAt  time.Time `json:"expiresAt"`
	Capability string    `json:"capability"`
}

// RefreshCanvasCapability 刷新 Canvas 能力
func (nim *NodeInvokeManager) RefreshCanvasCapability(nodeID string) (*CanvasCapability, error) {
	nim.mu.Lock()
	defer nim.mu.Unlock()

	// 检查 Node 是否已配对
	if _, ok := nim.authContext.GetNodePair(nodeID); !ok {
		return nil, fmt.Errorf("node not paired: %s", nodeID)
	}

	// 调用 Node 的 canvas.capability.refresh 方法
	state, err := nim.Invoke(nodeID, "canvas.capability.refresh", nil, "", 30*time.Second)
	if err != nil {
		return nil, err
	}

	// 等待结果（简化实现，实际应该异步等待）
	capability := &CanvasCapability{
		Enabled:    true,
		ExpiresAt:  time.Now().Add(24 * time.Hour),
		Capability: state.InvokeID, // 使用 invokeID 作为临时能力 token
	}

	return capability, nil
}

// RenameNode 重命名 Node
func (nim *NodeInvokeManager) RenameNode(nodeID, newName string) error {
	nim.mu.Lock()
	defer nim.mu.Unlock()

	pair, ok := nim.authContext.GetNodePair(nodeID)
	if !ok {
		return fmt.Errorf("node not paired: %s", nodeID)
	}

	pair.Name = newName
	nim.authContext.AddNodePair(pair)

	return nil
}

// DescribeNode 描述 Node
func (nim *NodeInvokeManager) DescribeNode(nodeID string) (map[string]interface{}, error) {
	nim.mu.RLock()
	defer nim.mu.RUnlock()

	pair, ok := nim.authContext.GetNodePair(nodeID)
	if !ok {
		return nil, fmt.Errorf("node not paired: %s", nodeID)
	}

	return map[string]interface{}{
		"node_id":      pair.NodeID,
		"name":         pair.Name,
		"capabilities": pair.Capabilities,
		"metadata":     pair.Metadata,
		"paired_at":    pair.PairedAt,
		"last_seen":    pair.LastSeenAt,
	}, nil
}

// ListNode 列出 Node
func (nim *NodeInvokeManager) ListNode() []map[string]interface{} {
	pairs := nim.authContext.ListNodePairs()
	result := make([]map[string]interface{}, 0, len(pairs))

	for _, pair := range pairs {
		result = append(result, map[string]interface{}{
			"node_id":      pair.NodeID,
			"name":         pair.Name,
			"capabilities": pair.Capabilities,
			"paired_at":    pair.PairedAt,
			"last_seen":    pair.LastSeenAt,
		})
	}

	return result
}

// SendNodeEvent 发送 Node 事件
func (nim *NodeInvokeManager) SendNodeEvent(nodeID string, eventType string, payload interface{}) error {
	data := map[string]interface{}{
		"node_id": nodeID,
		"event":   eventType,
		"payload": payload,
	}
	return nim.broadcastMgr.Broadcast("node.event", data, nil)
}

// RegisterNodeInvokeMethods 注册 Node 调用方法
func RegisterNodeInvokeMethods(mh *MessageHandler, nim *NodeInvokeManager) {
	// node.invoke
	mh.Register("node.invoke", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			NodeID  string      `json:"nodeId"`
			Method  string      `json:"method"`
			Params  interface{} `json:"params,omitempty"`
			Timeout int64       `json:"timeoutMs,omitempty"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.NodeID == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "nodeId is required")
		}

		if params.Method == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "method is required")
		}

		timeout := 30 * time.Second
		if params.Timeout > 0 {
			timeout = time.Duration(params.Timeout) * time.Millisecond
		}

		state, err := nim.Invoke(params.NodeID, params.Method, params.Params, conn.ID(), timeout)
		if err != nil {
			return nil, NewErrorInfo(ErrorInternalError, err.Error())
		}

		return state, nil
	})

	// node.invoke.result
	mh.Register("node.invoke.result", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			InvokeID string          `json:"invokeId"`
			Result   json.RawMessage `json:"result,omitempty"`
			Error    string          `json:"error,omitempty"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.InvokeID == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "invokeId is required")
		}

		if err := nim.HandleInvokeResult(params.InvokeID, params.Result, params.Error); err != nil {
			return nil, NewErrorInfo(ErrorInternalError, err.Error())
		}

		return map[string]interface{}{
			"status":   "received",
			"invokeId": params.InvokeID,
		}, nil
	})

	// node.list
	mh.Register("node.list", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		nodes := nim.ListNode()
		return map[string]interface{}{
			"nodes": nodes,
			"count": len(nodes),
		}, nil
	})

	// node.describe
	mh.Register("node.describe", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			NodeID string `json:"nodeId"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.NodeID == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "nodeId is required")
		}

		desc, err := nim.DescribeNode(params.NodeID)
		if err != nil {
			return nil, NewErrorInfo(ErrorNotFound, err.Error())
		}

		return desc, nil
	})

	// node.rename
	mh.Register("node.rename", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			NodeID  string `json:"nodeId"`
			NewName string `json:"newName"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.NodeID == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "nodeId is required")
		}

		if params.NewName == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "newName is required")
		}

		if err := nim.RenameNode(params.NodeID, params.NewName); err != nil {
			return nil, NewErrorInfo(ErrorInternalError, err.Error())
		}

		return map[string]interface{}{
			"status":  "renamed",
			"nodeId":  params.NodeID,
			"newName": params.NewName,
		}, nil
	})

	// node.canvas.capability.refresh
	mh.Register("node.canvas.capability.refresh", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			NodeID string `json:"nodeId"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.NodeID == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "nodeId is required")
		}

		capability, err := nim.RefreshCanvasCapability(params.NodeID)
		if err != nil {
			return nil, NewErrorInfo(ErrorInternalError, err.Error())
		}

		return capability, nil
	})
}
