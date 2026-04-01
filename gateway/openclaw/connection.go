package openclaw

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Connection WebSocket 连接
type Connection struct {
	mu              sync.RWMutex
	conn            *websocket.Conn
	id              string
	clientInfo      ClientInfo
	authContext     *AuthContext
	connectPolicy   *ConnectPolicy
	snapshotManager *SnapshotManager

	// 连接状态
	authenticated bool
	role          string
	scopes        []string
	deviceID      string

	// 协议
	protocol   int
	remoteAddr string

	// 订阅
	subscriptions map[string]bool

	// 状态
	connectedAt    time.Time
	lastActivityAt time.Time
	sendChan       chan []byte
	closeChan      chan struct{}
	onceClose      sync.Once

	// 上下文
	ctx    context.Context
	cancel context.CancelFunc
}

// ConnectionConfig 连接配置
type ConnectionConfig struct {
	AuthContext     *AuthContext
	ConnectPolicy   *ConnectPolicy
	SnapshotManager *SnapshotManager
	SendBufferSize  int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
}

// NewConnection 创建连接
func NewConnection(conn *websocket.Conn, remoteAddr string, config *ConnectionConfig) *Connection {
	ctx, cancel := context.WithCancel(context.Background())

	if config == nil {
		config = &ConnectionConfig{
			SendBufferSize: 256,
			ReadTimeout:    60 * time.Second,
			WriteTimeout:   10 * time.Second,
		}
	}

	return &Connection{
		conn:            conn,
		id:              uuid.New().String(),
		authContext:     config.AuthContext,
		connectPolicy:   config.ConnectPolicy,
		snapshotManager: config.SnapshotManager,
		remoteAddr:      remoteAddr,
		subscriptions:   make(map[string]bool),
		connectedAt:     time.Now(),
		lastActivityAt:  time.Now(),
		sendChan:        make(chan []byte, config.SendBufferSize),
		closeChan:       make(chan struct{}),
		ctx:             ctx,
		cancel:          cancel,
		protocol:        ProtocolVersion,
	}
}

// ID 获取连接 ID
func (c *Connection) ID() string {
	return c.id
}

// RemoteAddr 获取远程地址
func (c *Connection) RemoteAddr() string {
	return c.remoteAddr
}

// ClientInfo 获取客户端信息
func (c *Connection) ClientInfo() ClientInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.clientInfo
}

// SetClientInfo 设置客户端信息
func (c *Connection) SetClientInfo(info ClientInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clientInfo = info
}

// IsAuthenticated 检查是否已认证
func (c *Connection) IsAuthenticated() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.authenticated
}

// SetAuthenticated 设置认证状态
func (c *Connection) SetAuthenticated(authenticated bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.authenticated = authenticated
}

// Role 获取角色
func (c *Connection) Role() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.role
}

// SetRole 设置角色
func (c *Connection) SetRole(role string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.role = role
}

// Scopes 获取权限范围
func (c *Connection) Scopes() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.scopes
}

// SetScopes 设置权限范围
func (c *Connection) SetScopes(scopes []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.scopes = scopes
}

// DeviceID 获取设备 ID
func (c *Connection) DeviceID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.deviceID
}

// SetDeviceID 设置设备 ID
func (c *Connection) SetDeviceID(deviceID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deviceID = deviceID
}

// Protocol 获取协议版本
func (c *Connection) Protocol() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.protocol
}

// SetProtocol 设置协议版本
func (c *Connection) SetProtocol(protocol int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.protocol = protocol
}

// HasScope 检查是否有权限
func (c *Connection) HasScope(scope string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return HasScope(c.scopes, scope)
}

// HasAnyScope 检查是否有任一权限
func (c *Connection) HasAnyScope(scopes []string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return HasAnyScope(c.scopes, scopes)
}

// Subscribe 订阅事件
func (c *Connection) Subscribe(event string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.subscriptions[event] = true
}

// Unsubscribe 取消订阅
func (c *Connection) Unsubscribe(event string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.subscriptions, event)
}

// IsSubscribed 检查是否订阅
func (c *Connection) IsSubscribed(event string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.subscriptions[event]
}

// SendFrame 发送帧
func (c *Connection) SendFrame(frame Frame) error {
	data, err := EncodeFrame(frame)
	if err != nil {
		return fmt.Errorf("failed to encode frame: %w", err)
	}

	return c.SendMessage(data)
}

// SendMessage 发送消息
func (c *Connection) SendMessage(data []byte) error {
	select {
	case c.sendChan <- data:
		c.mu.Lock()
		if c.connectPolicy != nil {
			c.connectPolicy.UpdateBufferedBytes(c.id, int64(len(c.sendChan)))
		}
		c.mu.Unlock()
		return nil
	case <-c.ctx.Done():
		return fmt.Errorf("connection closed")
	default:
		return fmt.Errorf("send buffer full")
	}
}

// SendEvent 发送事件
func (c *Connection) SendEvent(event string, payload interface{}, seq int64, stateVersion *StateVersion) error {
	frame, err := NewEvent(event, payload, seq, stateVersion)
	if err != nil {
		return err
	}
	return c.SendFrame(frame)
}

// SendResponse 发送响应
func (c *Connection) SendResponse(id string, ok bool, payload interface{}, detail *ErrorDetail) error {
	frame, err := NewResponse(id, ok, payload, detail)
	if err != nil {
		return err
	}
	return c.SendFrame(frame)
}

// SendSuccessResponse 发送成功响应
func (c *Connection) SendSuccessResponse(id string, payload interface{}) error {
	return c.SendResponse(id, true, payload, nil)
}

// SendErrorResponse 发送错误响应
func (c *Connection) SendErrorResponse(id string, code ErrorCode, message string, details interface{}) error {
	frame, err := NewErrorResponse(id, string(code), message, details)
	if err != nil {
		return err
	}
	return c.SendFrame(frame)
}

// UpdateActivity 更新活动时间
func (c *Connection) UpdateActivity() {
	c.mu.Lock()
	c.lastActivityAt = time.Now()
	if c.connectPolicy != nil {
		c.connectPolicy.UpdateActivity(c.id)
	}
	c.mu.Unlock()
}

// LastActivityAt 最后活动时间
func (c *Connection) LastActivityAt() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastActivityAt
}

// Close 关闭连接
func (c *Connection) Close() error {
	var err error
	c.onceClose.Do(func() {
		c.cancel()

		// 注销连接策略
		if c.connectPolicy != nil {
			c.connectPolicy.UnregisterConnection(c.id)
		}

		// 移除在线状态
		if c.snapshotManager != nil {
			c.snapshotManager.RemovePresence(c.id)
		}

		close(c.sendChan)
		close(c.closeChan)

		if c.conn != nil {
			err = c.conn.Close()
		}
	})
	return err
}

// IsClosed 检查是否已关闭
func (c *Connection) IsClosed() bool {
	select {
	case <-c.closeChan:
		return true
	default:
		return false
	}
}

// StartWriter 启动写入协程
func (c *Connection) StartWriter() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case data := <-c.sendChan:
				if c.conn != nil {
					if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
						// 写入失败，关闭连接
						_ = c.Close()
						return
					}
				}
			case <-ticker.C:
				// 发送心跳
				if c.conn != nil {
					if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
						_ = c.Close()
						return
					}
				}
			case <-c.ctx.Done():
				return
			}
		}
	}()
}

// StartReader 启动读取协程
func (c *Connection) StartReader(handler func(*Connection, *Request) error) {
	go func() {
		if c.conn != nil {
			c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			c.conn.SetPongHandler(func(string) error {
				c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
				return nil
			})
		}

		for {
			select {
			case <-c.ctx.Done():
				return
			default:
			}

			if c.conn == nil {
				return
			}

			op, data, err := c.conn.ReadMessage()
			if err != nil {
				_ = c.Close()
				return
			}

			if op == websocket.CloseMessage {
				_ = c.Close()
				return
			}

			if op != websocket.TextMessage {
				continue
			}

			c.UpdateActivity()

			// 解析请求
			frame, err := ParseFrame(data)
			if err != nil {
				// 发送错误响应
				_ = c.SendErrorResponse("", ErrorParseError, err.Error(), nil)
				continue
			}

			req, ok := frame.(*Request)
			if !ok {
				_ = c.SendErrorResponse("", ErrorInvalidRequest, "expected request frame", nil)
				continue
			}

			// 处理请求
			if handler != nil {
				if err := handler(c, req); err != nil {
					_ = c.SendErrorResponse(req.ID, ErrorInternalError, err.Error(), nil)
				}
			}
		}
	}()
}

// ValidateConnect 验证连接参数
func (c *Connection) ValidateConnect(params *ConnectParams) error {
	// 检查协议版本
	if params.MinProtocol > ProtocolVersion || params.MaxProtocol < ProtocolVersion {
		return fmt.Errorf("protocol version mismatch: client %d-%d, server %d",
			params.MinProtocol, params.MaxProtocol, ProtocolVersion)
	}

	// 检查客户端信息
	if params.Client.ID == "" {
		return fmt.Errorf("client ID is required")
	}

	// 检查角色
	if params.Role != "" && params.Role != RoleOperator && params.Role != RoleNode {
		return fmt.Errorf("invalid role: %s", params.Role)
	}

	return nil
}

// ParseConnectParams 解析连接参数
func ParseConnectParams(params json.RawMessage) (*ConnectParams, error) {
	var connectParams ConnectParams
	if err := json.Unmarshal(params, &connectParams); err != nil {
		return nil, fmt.Errorf("failed to parse connect params: %w", err)
	}
	return &connectParams, nil
}

// AuthenticateConnect 认证连接
func (c *Connection) AuthenticateConnect(params *ConnectParams) error {
	authMode := c.authContext.GetAuthMode()

	// 无认证模式
	if authMode == AuthModeNone {
		c.SetAuthenticated(true)
		c.SetRole(params.Role)
		c.SetScopes(params.Scopes)
		return nil
	}

	// Token 认证
	if authMode == AuthModeToken && params.Auth != nil && params.Auth.Token != "" {
		if c.authContext.AuthenticateToken(params.Auth.Token) {
			c.SetAuthenticated(true)
			c.SetRole(params.Role)
			c.SetScopes(params.Scopes)
			return nil
		}
		return fmt.Errorf("invalid token")
	}

	// 密码认证
	if authMode == AuthModePassword && params.Auth != nil && params.Auth.Password != "" {
		if c.authContext.AuthenticatePassword(params.Auth.Password) {
			c.SetAuthenticated(true)
			c.SetRole(params.Role)
			c.SetScopes(params.Scopes)
			return nil
		}
		return fmt.Errorf("invalid password")
	}

	// 设备签名认证
	if authMode == AuthModeDevice && params.Device != nil {
		if c.authContext.VerifyDeviceSignature(
			params.Device.ID,
			params.Device.PublicKey,
			params.Device.Signature,
			params.Device.SignedAt,
			params.Device.Nonce,
		) {
			c.SetAuthenticated(true)
			c.SetDeviceID(params.Device.ID)
			c.SetRole(params.Role)

			// 从配对获取权限
			if pair, ok := c.authContext.GetDevicePair(params.Device.ID); ok {
				c.SetScopes(pair.Scopes)
			} else {
				c.SetScopes(params.Scopes)
			}
			return nil
		}
		return fmt.Errorf("invalid device signature")
	}

	return fmt.Errorf("authentication required")
}

// SendHelloOK 发送 HelloOK 响应
func (c *Connection) SendHelloOK() error {
	snapshot := c.snapshotManager.GetSnapshot()
	snapshotJSON, _ := json.Marshal(snapshot)

	helloOK := &HelloOK{
		TypeVal:  FrameTypeHelloOK,
		Protocol: ProtocolVersion,
		Server: ServerInfo{
			Version: "1.0.0",
			ConnID:  c.id,
		},
		Features: GetFeatures(),
		Snapshot: snapshotJSON,
		Policy: &HelloConnectPolicy{
			MaxPayload:       c.connectPolicy.MaxPayload,
			MaxBufferedBytes: c.connectPolicy.MaxBufferedBytes,
			TickIntervalMs:   c.connectPolicy.TickIntervalMs,
		},
	}

	// 添加认证信息（如果有）
	if c.IsAuthenticated() && c.DeviceID() != "" {
		if pair, ok := c.authContext.GetDevicePair(c.DeviceID()); ok {
			helloOK.Auth = &AuthInfo{
				DeviceToken: pair.DeviceID,
				Role:        c.Role(),
				Scopes:      pair.Scopes,
			}
		}
	}

	return c.SendFrame(helloOK)
}
