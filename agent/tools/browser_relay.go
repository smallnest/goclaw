package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

// RelayClient OpenClaw Browser Relay 客户端
// OpenClaw 使用 WebSocket + JSON 文本帧协议进行通信
type RelayClient struct {
	mu              sync.RWMutex
	conn            *websocket.Conn
	url             string
	connected       bool
	pendingRequests map[string]chan *RelayResponse
}

// RelayRequest OpenClaw Relay 请求格式
type RelayRequest struct {
	Type   string                 `json:"type"`   // "req"
	ID     string                 `json:"id"`     // 唯一请求ID
	Method string                 `json:"method"` // CDP 方法名 (如 "Page.navigate")
	Params map[string]interface{} `json:"params"` // CDP 参数
}

// RelayResponse OpenClaw Relay 响应格式
type RelayResponse struct {
	Type    string                 `json:"type"` // "res"
	ID      string                 `json:"id"`   // 对应的请求ID
	OK      bool                   `json:"ok"`   // 是否成功
	Payload map[string]interface{} `json:"payload"`
	Error   *RelayError            `json:"error,omitempty"`
}

// RelayError OpenClaw Relay 错误格式
type RelayError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// NewRelayClient 创建新的 Relay 客户端
func NewRelayClient(url string) *RelayClient {
	if url == "" {
		// 默认使用 OpenClaw Gateway 的默认端口
		url = "ws://127.0.0.1:18789"
	}
	return &RelayClient{
		url:             url,
		pendingRequests: make(map[string]chan *RelayResponse),
	}
}

// Connect 连接到 OpenClaw Relay 服务器
func (r *RelayClient) Connect(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.connected {
		return nil
	}

	logger.Debug("Connecting to OpenClaw Browser Relay",
		zap.String("url", r.url))

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, r.url, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to relay server: %w", err)
	}

	r.conn = conn
	r.connected = true

	// 启动消息处理循环
	go r.messageLoop()

	logger.Debug("Connected to OpenClaw Browser Relay successfully")

	return nil
}

// messageLoop 处理来自 Relay 服务器的消息
func (r *RelayClient) messageLoop() {
	for {
		_, message, err := r.conn.ReadMessage()
		if err != nil {
			r.mu.Lock()
			r.connected = false
			r.mu.Unlock()
			logger.Error("Relay connection error", zap.Error(err))
			return
		}

		// 解析 JSON 响应
		var response RelayResponse
		if err := json.Unmarshal(message, &response); err != nil {
			logger.Error("Failed to parse relay response",
				zap.Error(err),
				zap.String("message", string(message)))
			continue
		}

		// 查找对应的等待通道
		r.mu.RLock()
		ch, ok := r.pendingRequests[response.ID]
		r.mu.RUnlock()

		if ok {
			select {
			case ch <- &response:
				// 成功发送响应
			case <-time.After(5 * time.Second):
				logger.Warn("Timeout sending response to handler",
					zap.String("id", response.ID))
			}

			r.mu.Lock()
			delete(r.pendingRequests, response.ID)
			r.mu.Unlock()
		}
	}
}

// Execute 执行 CDP 命令通过 Relay
func (r *RelayClient) Execute(ctx context.Context, method string, params map[string]interface{}) (map[string]interface{}, error) {
	r.mu.RLock()
	if !r.connected {
		r.mu.RUnlock()
		return nil, fmt.Errorf("relay client not connected")
	}
	r.mu.RUnlock()

	// 生成唯一请求ID
	requestID := fmt.Sprintf("%d", time.Now().UnixNano())
	responseCh := make(chan *RelayResponse, 1)

	r.mu.Lock()
	r.pendingRequests[requestID] = responseCh
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		delete(r.pendingRequests, requestID)
		r.mu.Unlock()
	}()

	// 构建请求
	req := RelayRequest{
		Type:   "req",
		ID:     requestID,
		Method: method,
		Params: params,
	}

	reqJSON, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// 发送请求
	r.mu.Lock()
	err = r.conn.WriteMessage(websocket.TextMessage, reqJSON)
	r.mu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// 等待响应
	select {
	case resp := <-responseCh:
		if !resp.OK {
			if resp.Error != nil {
				return nil, fmt.Errorf("relay error (code %d): %s", resp.Error.Code, resp.Error.Message)
			}
			return nil, fmt.Errorf("relay request failed")
		}
		return resp.Payload, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("request timeout")
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("request timeout after 30s")
	}
}

// IsConnected 检查是否已连接
func (r *RelayClient) IsConnected() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.connected
}

// Close 关闭连接
func (r *RelayClient) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.conn != nil {
		err := r.conn.Close()
		r.conn = nil
		r.connected = false
		return err
	}

	return nil
}

// RelaySessionManager OpenClaw Relay 会话管理器
type RelaySessionManager struct {
	mu     sync.RWMutex
	client *RelayClient
	ready  bool
}

var relaySessionManager *RelaySessionManager

// GetRelaySession 获取 Relay 会话管理器（单例）
func GetRelaySession() *RelaySessionManager {
	if relaySessionManager == nil {
		relaySessionManager = &RelaySessionManager{}
	}
	return relaySessionManager
}

// Start 启动 Relay 会话
func (r *RelaySessionManager) Start(relayURL string, timeout time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.ready {
		return nil
	}

	logger.Debug("Starting OpenClaw Browser Relay session")

	client := NewRelayClient(relayURL)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to relay server: %w", err)
	}

	r.client = client
	r.ready = true

	logger.Debug("OpenClaw Browser Relay session started successfully")

	return nil
}

// Execute 执行 CDP 命令
func (r *RelaySessionManager) Execute(ctx context.Context, method string, params map[string]interface{}) (map[string]interface{}, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !r.ready {
		return nil, fmt.Errorf("relay session not ready")
	}

	return r.client.Execute(ctx, method, params)
}

// IsReady 检查会话是否就绪
func (r *RelaySessionManager) IsReady() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.ready
}

// GetClient 获取 Relay 客户端
func (r *RelaySessionManager) GetClient() *RelayClient {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.client
}

// Stop 停止 Relay 会话
func (r *RelaySessionManager) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.ready {
		logger.Debug("Stopping OpenClaw Browser Relay session")

		if r.client != nil {
			_ = r.client.Close()
		}

		r.ready = false
		r.client = nil
	}
}
