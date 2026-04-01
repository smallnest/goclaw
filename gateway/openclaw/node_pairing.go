package openclaw

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// NodePairingManager Node 配对管理器
type NodePairingManager struct {
	mu            sync.RWMutex
	authContext   *AuthContext
	broadcastMgr  *BroadcastManager
	requests      map[string]*NodePairRequest // requestID -> request
	metadataLocks map[string]bool             // nodeID -> locked
}

// NodePairRequest Node 配对请求
type NodePairRequest struct {
	ID           string                 `json:"id"`
	NodeID       string                 `json:"nodeId"`
	Name         string                 `json:"name"`
	Capabilities []string               `json:"capabilities"`
	Metadata     map[string]interface{} `json:"metadata"`
	RequestedAt  time.Time              `json:"requestedAt"`
	ExpiresAt    time.Time              `json:"expiresAt"`
	Status       string                 `json:"status"` // "pending" | "approved" | "rejected" | "expired"
}

// NewNodePairingManager 创建 Node 配对管理器
func NewNodePairingManager(authContext *AuthContext, broadcastMgr *BroadcastManager) *NodePairingManager {
	return &NodePairingManager{
		authContext:   authContext,
		broadcastMgr:  broadcastMgr,
		requests:      make(map[string]*NodePairRequest),
		metadataLocks: make(map[string]bool),
	}
}

// Request 请求配对
func (npm *NodePairingManager) Request(nodeID, name string, capabilities []string, metadata map[string]interface{}) (*NodePairRequest, error) {
	npm.mu.Lock()
	defer npm.mu.Unlock()

	// 检查是否已经配对
	if _, ok := npm.authContext.GetNodePair(nodeID); ok {
		return nil, fmt.Errorf("node already paired: %s", nodeID)
	}

	// 生成请求 ID
	requestID, err := GeneratePairID()
	if err != nil {
		return nil, err
	}

	// 创建请求
	request := &NodePairRequest{
		ID:           requestID,
		NodeID:       nodeID,
		Name:         name,
		Capabilities: capabilities,
		Metadata:     metadata,
		RequestedAt:  time.Now(),
		ExpiresAt:    time.Now().Add(5 * time.Minute),
		Status:       "pending",
	}

	npm.requests[requestID] = request

	// 广播配对请求
	_ = npm.broadcastMgr.BroadcastNodePairRequested(requestID, nodeID, metadata)

	return request, nil
}

// List 列出待处理的配对请求
func (npm *NodePairingManager) List() []*NodePairRequest {
	npm.mu.RLock()
	defer npm.mu.RUnlock()

	result := make([]*NodePairRequest, 0)
	for _, req := range npm.requests {
		if req.Status == "pending" && time.Now().Before(req.ExpiresAt) {
			// 返回副本
			copy := *req
			result = append(result, &copy)
		}
	}
	return result
}

// Approve 批准配对
func (npm *NodePairingManager) Approve(requestID string, metadata map[string]string) error {
	npm.mu.Lock()
	defer npm.mu.Unlock()

	request, ok := npm.requests[requestID]
	if !ok {
		return fmt.Errorf("pair request not found: %s", requestID)
	}

	if request.Status != "pending" {
		return fmt.Errorf("pair request already %s", request.Status)
	}

	if time.Now().After(request.ExpiresAt) {
		request.Status = "expired"
		return fmt.Errorf("pair request expired")
	}

	// 创建配对
	pair := &NodePair{
		NodeID:       request.NodeID,
		Name:         request.Name,
		Capabilities: request.Capabilities,
		Metadata:     metadata,
		PairedAt:     time.Now(),
		LastSeenAt:   time.Now(),
	}

	npm.authContext.AddNodePair(pair)
	request.Status = "approved"

	// 广播配对解决
	_ = npm.broadcastMgr.BroadcastNodePairResolved(requestID, true, "")

	return nil
}

// Reject 拒绝配对
func (npm *NodePairingManager) Reject(requestID, reason string) error {
	npm.mu.Lock()
	defer npm.mu.Unlock()

	request, ok := npm.requests[requestID]
	if !ok {
		return fmt.Errorf("pair request not found: %s", requestID)
	}

	if request.Status != "pending" {
		return fmt.Errorf("pair request already %s", request.Status)
	}

	request.Status = "rejected"

	// 广播配对解决
	_ = npm.broadcastMgr.BroadcastNodePairResolved(requestID, false, reason)

	return nil
}

// Verify 验证配对
func (npm *NodePairingManager) Verify(nodeID string) (*NodePair, error) {
	npm.mu.RLock()
	defer npm.mu.RUnlock()

	pair, ok := npm.authContext.GetNodePair(nodeID)
	if !ok {
		return nil, fmt.Errorf("node not paired: %s", nodeID)
	}

	return pair, nil
}

// GetRequest 获取配对请求
func (npm *NodePairingManager) GetRequest(requestID string) (*NodePairRequest, error) {
	npm.mu.RLock()
	defer npm.mu.RUnlock()

	request, ok := npm.requests[requestID]
	if !ok {
		return nil, fmt.Errorf("pair request not found: %s", requestID)
	}

	// 返回副本
	copy := *request
	return &copy, nil
}

// CleanupExpired 清理过期的请求
func (npm *NodePairingManager) CleanupExpired() {
	npm.mu.Lock()
	defer npm.mu.Unlock()

	now := time.Now()
	for _, req := range npm.requests {
		if req.Status == "pending" && now.After(req.ExpiresAt) {
			req.Status = "expired"
		}
	}
}

// LockMetadata 锁定元数据
func (npm *NodePairingManager) LockMetadata(nodeID string) bool {
	npm.mu.Lock()
	defer npm.mu.Unlock()

	if npm.metadataLocks[nodeID] {
		return false
	}

	npm.metadataLocks[nodeID] = true
	return true
}

// UnlockMetadata 解锁元数据
func (npm *NodePairingManager) UnlockMetadata(nodeID string) {
	npm.mu.Lock()
	defer npm.mu.Unlock()

	delete(npm.metadataLocks, nodeID)
}

// IsMetadataLocked 检查元数据是否被锁定
func (npm *NodePairingManager) IsMetadataLocked(nodeID string) bool {
	npm.mu.RLock()
	defer npm.mu.RUnlock()

	return npm.metadataLocks[nodeID]
}

// RegisterNodePairingMethods 注册 Node 配对方法
func RegisterNodePairingMethods(mh *MessageHandler, npm *NodePairingManager) {
	// node.pair.request
	mh.Register("node.pair.request", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			NodeID       string                 `json:"nodeId"`
			Name         string                 `json:"name"`
			Capabilities []string               `json:"capabilities,omitempty"`
			Metadata     map[string]interface{} `json:"metadata,omitempty"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.NodeID == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "nodeId is required")
		}

		if params.Name == "" {
			params.Name = params.NodeID
		}

		request, err := npm.Request(params.NodeID, params.Name, params.Capabilities, params.Metadata)
		if err != nil {
			return nil, NewErrorInfo(ErrorInternalError, err.Error())
		}

		return request, nil
	})

	// node.pair.list
	mh.Register("node.pair.list", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		requests := npm.List()
		return map[string]interface{}{
			"requests": requests,
			"count":    len(requests),
		}, nil
	})

	// node.pair.approve
	mh.Register("node.pair.approve", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			RequestID string            `json:"requestId"`
			Metadata  map[string]string `json:"metadata,omitempty"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.RequestID == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "requestId is required")
		}

		if err := npm.Approve(params.RequestID, params.Metadata); err != nil {
			return nil, NewErrorInfo(ErrorInternalError, err.Error())
		}

		return map[string]interface{}{
			"status":    "approved",
			"requestId": params.RequestID,
		}, nil
	})

	// node.pair.reject
	mh.Register("node.pair.reject", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			RequestID string `json:"requestId"`
			Reason    string `json:"reason,omitempty"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.RequestID == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "requestId is required")
		}

		if err := npm.Reject(params.RequestID, params.Reason); err != nil {
			return nil, NewErrorInfo(ErrorInternalError, err.Error())
		}

		return map[string]interface{}{
			"status":    "rejected",
			"requestId": params.RequestID,
		}, nil
	})

	// node.pair.verify
	mh.Register("node.pair.verify", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			NodeID string `json:"nodeId"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.NodeID == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "nodeId is required")
		}

		pair, err := npm.Verify(params.NodeID)
		if err != nil {
			return nil, NewErrorInfo(ErrorNotFound, err.Error())
		}

		return pair, nil
	})
}

// ParseNodePairRequest 从 JSON 解析 Node 配对请求
func ParseNodePairRequest(data []byte) (*NodePairRequest, error) {
	var req NodePairRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("failed to parse node pair request: %w", err)
	}
	return &req, nil
}
