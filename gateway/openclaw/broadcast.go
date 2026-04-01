package openclaw

import (
	"context"
	"sync"
	"time"
)

// BroadcastManager 广播管理器
type BroadcastManager struct {
	mu                sync.RWMutex
	connections       map[string]*Connection
	snapshotMgr       *SnapshotManager
	eventSeq          int64
	nodeSubscriptions map[string]map[string]string // nodeID -> sessionKey -> connID
	ctx               context.Context
	cancel            context.CancelFunc
}

// NewBroadcastManager 创建广播管理器
func NewBroadcastManager(snapshotMgr *SnapshotManager) *BroadcastManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &BroadcastManager{
		connections:       make(map[string]*Connection),
		snapshotMgr:       snapshotMgr,
		nodeSubscriptions: make(map[string]map[string]string),
		ctx:               ctx,
		cancel:            cancel,
	}
}

// RegisterConnection 注册连接
func (bm *BroadcastManager) RegisterConnection(conn *Connection) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	bm.connections[conn.ID()] = conn
}

// UnregisterConnection 注销连接
func (bm *BroadcastManager) UnregisterConnection(connID string) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	delete(bm.connections, connID)

	// 清理 Node 订阅
	for nodeID, sessions := range bm.nodeSubscriptions {
		for sessionKey, sid := range sessions {
			if sid == connID {
				delete(sessions, sessionKey)
				if len(sessions) == 0 {
					delete(bm.nodeSubscriptions, nodeID)
				}
				break
			}
		}
	}
}

// Broadcast 广播事件到所有连接
func (bm *BroadcastManager) Broadcast(event string, payload interface{}, opts *BroadcastOptions) error {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	// 获取状态版本
	var stateVersion *StateVersion
	if opts != nil && opts.IncludeStateVersion {
		stateVersion = bm.snapshotMgr.GetStateVersionPtr()
	}

	// 创建事件帧
	seq := bm.nextSeq()
	eventFrame, err := NewEvent(event, payload, seq, stateVersion)
	if err != nil {
		return err
	}

	data, err := EncodeFrame(eventFrame)
	if err != nil {
		return err
	}

	// 广播到所有连接
	for connID, conn := range bm.connections {
		// 检查订阅
		if !conn.IsSubscribed(event) && !isDefaultBroadcastEvent(event) {
			continue
		}

		// 检查 dropIfSlow
		if opts != nil && opts.DropIfSlow {
			if bm.isConnectionSlow(connID) {
				continue
			}
		}

		// 发送事件
		_ = conn.SendMessage(data)
	}

	return nil
}

// BroadcastToConnIDs 广播事件到指定连接
func (bm *BroadcastManager) BroadcastToConnIDs(connIDs []string, event string, payload interface{}) error {
	bm.mu.RLock()
	defer bm.mu.Unlock()

	seq := bm.nextSeq()
	eventFrame, err := NewEvent(event, payload, seq, nil)
	if err != nil {
		return err
	}

	data, err := EncodeFrame(eventFrame)
	if err != nil {
		return err
	}

	for _, connID := range connIDs {
		if conn, ok := bm.connections[connID]; ok {
			_ = conn.SendMessage(data)
		}
	}

	return nil
}

// BroadcastToSession 广播事件到会话相关的连接
func (bm *BroadcastManager) BroadcastToSession(sessionKey string, event string, payload interface{}) error {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	seq := bm.nextSeq()
	eventFrame, err := NewEvent(event, payload, seq, nil)
	if err != nil {
		return err
	}

	data, err := EncodeFrame(eventFrame)
	if err != nil {
		return err
	}

	// 查找会话相关的连接
	for _, conn := range bm.connections {
		// 这里应该检查连接是否与该会话相关
		// 简化实现：广播到所有连接
		_ = conn.SendMessage(data)
	}

	return nil
}

// BroadcastPresenceChange 广播在线状态变化
func (bm *BroadcastManager) BroadcastPresenceChange(presence []PresenceEntry) error {
	return bm.Broadcast("presence", presence, &BroadcastOptions{
		IncludeStateVersion: true,
	})
}

// BroadcastHealthChange 广播健康状态变化
func (bm *BroadcastManager) BroadcastHealthChange(health HealthStatus) error {
	return bm.Broadcast("health", health, &BroadcastOptions{
		IncludeStateVersion: true,
	})
}

// BroadcastTick 广播心跳事件
func (bm *BroadcastManager) BroadcastTick() error {
	payload := map[string]interface{}{
		"timestamp": time.Now().Unix(),
	}
	return bm.Broadcast("tick", payload, nil)
}

// BroadcastAgentEvent 广播 Agent 事件
func (bm *BroadcastManager) BroadcastAgentEvent(agentID string, event string, payload interface{}) error {
	data := map[string]interface{}{
		"agent_id": agentID,
		"event":    event,
		"payload":  payload,
	}
	return bm.Broadcast("agent", data, nil)
}

// BroadcastChatEvent 广播聊天事件
func (bm *BroadcastManager) BroadcastChatEvent(sessionKey string, chatEvent *ChatEvent) error {
	return bm.Broadcast("chat", chatEvent, nil)
}

// BroadcastCronEvent 广播 Cron 事件
func (bm *BroadcastManager) BroadcastCronEvent(event string, payload interface{}) error {
	data := map[string]interface{}{
		"event":   event,
		"payload": payload,
	}
	return bm.Broadcast("cron", data, nil)
}

// BroadcastNodePairRequested 广播 Node 配对请求
func (bm *BroadcastManager) BroadcastNodePairRequested(requestID string, nodeID string, metadata interface{}) error {
	payload := map[string]interface{}{
		"request_id": requestID,
		"node_id":    nodeID,
		"metadata":   metadata,
	}
	return bm.Broadcast("node.pair.requested", payload, nil)
}

// BroadcastNodePairResolved 广播 Node 配对解决
func (bm *BroadcastManager) BroadcastNodePairResolved(requestID string, approved bool, reason string) error {
	payload := map[string]interface{}{
		"request_id": requestID,
		"approved":   approved,
		"reason":     reason,
	}
	return bm.Broadcast("node.pair.resolved", payload, nil)
}

// BroadcastNodeInvokeRequest 广播 Node 调用请求
func (bm *BroadcastManager) BroadcastNodeInvokeRequest(nodeID string, invokeID string, method string, params interface{}) error {
	payload := map[string]interface{}{
		"node_id":   nodeID,
		"invoke_id": invokeID,
		"method":    method,
		"params":    params,
	}
	return bm.Broadcast("node.invoke.request", payload, nil)
}

// BroadcastDevicePairRequested 广播设备配对请求
func (bm *BroadcastManager) BroadcastDevicePairRequested(requestID string, deviceID string, metadata interface{}) error {
	payload := map[string]interface{}{
		"request_id": requestID,
		"device_id":  deviceID,
		"metadata":   metadata,
	}
	return bm.Broadcast("device.pair.requested", payload, nil)
}

// BroadcastDevicePairResolved 广播设备配对解决
func (bm *BroadcastManager) BroadcastDevicePairResolved(requestID string, approved bool, reason string) error {
	payload := map[string]interface{}{
		"request_id": requestID,
		"approved":   approved,
		"reason":     reason,
	}
	return bm.Broadcast("device.pair.resolved", payload, nil)
}

// BroadcastVoiceWakeChanged 广播语音唤醒变更
func (bm *BroadcastManager) BroadcastVoiceWakeChanged(enabled bool, mode string) error {
	payload := map[string]interface{}{
		"enabled": enabled,
		"mode":    mode,
	}
	return bm.Broadcast("voicewake.changed", payload, nil)
}

// BroadcastExecApprovalRequested 广播执行批准请求
func (bm *BroadcastManager) BroadcastExecApprovalRequested(approvalID string, command string, args []string) error {
	payload := map[string]interface{}{
		"approval_id": approvalID,
		"command":     command,
		"args":        args,
	}
	return bm.Broadcast("exec.approval.requested", payload, nil)
}

// BroadcastExecApprovalResolved 广播执行批准解决
func (bm *BroadcastManager) BroadcastExecApprovalResolved(approvalID string, approved bool, reason string) error {
	payload := map[string]interface{}{
		"approval_id": approvalID,
		"approved":    approved,
		"reason":      reason,
	}
	return bm.Broadcast("exec.approval.resolved", payload, nil)
}

// BroadcastUpdateAvailable 广播更新可用
func (bm *BroadcastManager) BroadcastUpdateAvailable(currentVersion, latestVersion, channel string) error {
	payload := map[string]interface{}{
		"current_version": currentVersion,
		"latest_version":  latestVersion,
		"channel":         channel,
	}
	return bm.Broadcast("update.available", payload, nil)
}

// BroadcastShutdown 广播关闭事件
func (bm *BroadcastManager) BroadcastShutdown(reason string) error {
	payload := map[string]interface{}{
		"reason":    reason,
		"timestamp": time.Now().Unix(),
	}
	return bm.Broadcast("shutdown", payload, nil)
}

// NodeSubscribe 订阅 Node
func (bm *BroadcastManager) NodeSubscribe(nodeID, sessionKey, connID string) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if bm.nodeSubscriptions[nodeID] == nil {
		bm.nodeSubscriptions[nodeID] = make(map[string]string)
	}
	bm.nodeSubscriptions[nodeID][sessionKey] = connID
}

// NodeUnsubscribe 取消订阅 Node
func (bm *BroadcastManager) NodeUnsubscribe(nodeID, sessionKey string) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if sessions, ok := bm.nodeSubscriptions[nodeID]; ok {
		delete(sessions, sessionKey)
		if len(sessions) == 0 {
			delete(bm.nodeSubscriptions, nodeID)
		}
	}
}

// NodeSendToSession 发送消息到订阅 Node 的会话
func (bm *BroadcastManager) NodeSendToSession(nodeID, sessionKey string, event string, payload interface{}) error {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	sessions, ok := bm.nodeSubscriptions[nodeID]
	if !ok {
		return nil // 没有订阅者
	}

	connID, ok := sessions[sessionKey]
	if !ok {
		return nil // 会话未订阅
	}

	conn, ok := bm.connections[connID]
	if !ok {
		return nil // 连接不存在
	}

	seq := bm.nextSeq()
	eventFrame, err := NewEvent(event, payload, seq, nil)
	if err != nil {
		return err
	}

	return conn.SendFrame(eventFrame)
}

// GetNodeSubscriptions 获取 Node 的订阅者
func (bm *BroadcastManager) GetNodeSubscriptions(nodeID string) []string {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	sessions, ok := bm.nodeSubscriptions[nodeID]
	if !ok {
		return nil
	}

	result := make([]string, 0, len(sessions))
	for sessionKey := range sessions {
		result = append(result, sessionKey)
	}
	return result
}

// nextSeq 获取下一个序列号
func (bm *BroadcastManager) nextSeq() int64 {
	bm.eventSeq++
	return bm.eventSeq
}

// isConnectionSlow 检查连接是否慢
func (bm *BroadcastManager) isConnectionSlow(connID string) bool {
	// 简化实现：总是返回 false
	// 实际应该检查连接的缓冲区大小
	return false
}

// isDefaultBroadcastEvent 检查是否是默认广播事件
func isDefaultBroadcastEvent(event string) bool {
	defaultEvents := []string{
		"presence", "health", "tick", "shutdown",
		"agent", "chat", "heartbeat",
	}
	for _, e := range defaultEvents {
		if e == event {
			return true
		}
	}
	return false
}

// StartTickBroadcast 启动心跳广播
func (bm *BroadcastManager) StartTickBroadcast(interval time.Duration) func() {
	ticker := time.NewTicker(interval)
	stop := make(chan struct{})

	go func() {
		for {
			select {
			case <-ticker.C:
				_ = bm.BroadcastTick()
			case <-stop:
				ticker.Stop()
				return
			case <-bm.ctx.Done():
				ticker.Stop()
				return
			}
		}
	}()

	return func() {
		close(stop)
	}
}

// Stop 停止广播管理器
func (bm *BroadcastManager) Stop() {
	bm.cancel()
}

// BroadcastOptions 广播选项
type BroadcastOptions struct {
	DropIfSlow          bool
	IncludeStateVersion bool
	StateVersion        *StateVersion
}

// GetConnectionCount 获取连接数
func (bm *BroadcastManager) GetConnectionCount() int {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return len(bm.connections)
}

// GetConnection 获取连接
func (bm *BroadcastManager) GetConnection(connID string) (*Connection, bool) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	conn, ok := bm.connections[connID]
	if !ok {
		return nil, false
	}

	return conn, true
}

// ListConnections 列出所有连接 ID
func (bm *BroadcastManager) ListConnections() []string {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	ids := make([]string, 0, len(bm.connections))
	for id := range bm.connections {
		ids = append(ids, id)
	}
	return ids
}
