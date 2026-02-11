package gateway

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/smallnest/dogclaw/goclaw/bus"
	"github.com/smallnest/dogclaw/goclaw/channels"
	"github.com/smallnest/dogclaw/goclaw/config"
	"github.com/smallnest/dogclaw/goclaw/internal/logger"
	"github.com/smallnest/dogclaw/goclaw/session"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// 在生产环境中应该检查 Origin
		return true
	},
}

// Server HTTP 网关服务器
type Server struct {
	config        *config.GatewayConfig
	wsConfig      *WebSocketConfig
	bus           *bus.MessageBus
	channelMgr    *channels.Manager
	sessionMgr    *session.Manager
	server        *http.Server
	wsServer      *http.Server
	handler       *Handler
	mu            sync.RWMutex
	running       bool
	connections   map[string]*Connection
	connectionsMu sync.RWMutex
	enableAuth    bool
	authToken     string
}

// WebSocketConfig WebSocket 配置
type WebSocketConfig struct {
	Host           string
	Port           int
	Path           string
	EnableAuth     bool
	AuthToken      string
	PingInterval   time.Duration
	PongTimeout    time.Duration
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	MaxMessageSize int64
	// TLS 配置
	EnableTLS bool
	CertFile  string
	KeyFile   string
}

// NewServer 创建网关服务器
func NewServer(cfg *config.GatewayConfig, messageBus *bus.MessageBus, channelMgr *channels.Manager, sessionMgr *session.Manager) *Server {
	return &Server{
		config: cfg,
		wsConfig: &WebSocketConfig{
			Host:           "0.0.0.0",
			Port:           18789,
			Path:           "/ws",
			EnableAuth:     false,
			PingInterval:   30 * time.Second,
			PongTimeout:    60 * time.Second,
			ReadTimeout:    60 * time.Second,
			WriteTimeout:   10 * time.Second,
			MaxMessageSize: 10 * 1024 * 1024, // 10MB
		},
		bus:         messageBus,
		channelMgr:  channelMgr,
		sessionMgr:  sessionMgr,
		handler:     NewHandler(messageBus, sessionMgr, channelMgr),
		connections: make(map[string]*Connection),
	}
}

// SetWebSocketConfig 设置 WebSocket 配置
func (s *Server) SetWebSocketConfig(cfg *WebSocketConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.wsConfig = cfg
	s.enableAuth = cfg.EnableAuth
	s.authToken = cfg.AuthToken
}

// Start 启动服务器
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("server already running")
	}
	s.running = true
	s.mu.Unlock()

	// 启动 HTTP 服务器
	if err := s.startHTTPServer(ctx); err != nil {
		return err
	}

	// 启动 WebSocket 服务器
	if err := s.startWebSocketServer(ctx); err != nil {
		return err
	}

	// 启动出站消息广播（使用新的订阅机制）
	go s.broadcastOutbound(ctx)

	// 监听上下文取消
	go func() {
		<-ctx.Done()
		_ = s.Stop()
	}()

	return nil
}

// startHTTPServer 启动 HTTP 服务器
func (s *Server) startHTTPServer(ctx context.Context) error {
	// 创建 HTTP 路由
	mux := http.NewServeMux()

	// 健康检查端点
	mux.HandleFunc("/health", s.handleHealth)

	// 飞书 webhook 端点
	mux.HandleFunc("/webhook/feishu", s.handleFeishuWebhook)

	// 通用 webhook 端点
	mux.HandleFunc("/webhook/", s.handleGenericWebhook)

	// 创建 HTTP 服务器
	s.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", s.config.Host, s.config.Port),
		Handler:      mux,
		ReadTimeout:  time.Duration(s.config.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(s.config.WriteTimeout) * time.Second,
	}

	// 启动服务器
	go func() {
		logger.Info("HTTP gateway server started",
			zap.String("addr", s.server.Addr),
		)

		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP gateway server error", zap.Error(err))
		}
	}()

	return nil
}

// startWebSocketServer 启动 WebSocket 服务器
func (s *Server) startWebSocketServer(ctx context.Context) error {
	// 创建 WebSocket 路由
	mux := http.NewServeMux()

	// WebSocket 端点
	mux.HandleFunc(s.wsConfig.Path, s.handleWebSocket)

	// 健康检查端点
	mux.HandleFunc("/health", s.handleHealth)

	// 创建 WebSocket 服务器
	s.wsServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", s.wsConfig.Host, s.wsConfig.Port),
		Handler:      mux,
		ReadTimeout:  s.wsConfig.ReadTimeout,
		WriteTimeout: s.wsConfig.WriteTimeout,
	}

	// 启动服务器
	go func() {
		logger.Info("WebSocket gateway server started",
			zap.String("addr", s.wsServer.Addr),
			zap.String("path", s.wsConfig.Path),
		)

		if err := s.wsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("WebSocket gateway server error", zap.Error(err))
		}
	}()

	return nil
}

// Stop 停止服务器
func (s *Server) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	s.mu.Unlock()

	// 关闭所有 WebSocket 连接
	s.closeAllConnections()

	// 停止 HTTP 服务器
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := s.server.Shutdown(ctx); err != nil {
			logger.Error("Failed to shutdown HTTP gateway server", zap.Error(err))
		}
	}

	// 停止 WebSocket 服务器
	if s.wsServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := s.wsServer.Shutdown(ctx); err != nil {
			logger.Error("Failed to shutdown WebSocket gateway server", zap.Error(err))
		}
	}

	logger.Info("Gateway server stopped")
	return nil
}

// closeAllConnections 关闭所有 WebSocket 连接
func (s *Server) closeAllConnections() {
	s.connectionsMu.Lock()
	defer s.connectionsMu.Unlock()

	for id, conn := range s.connections {
		conn.Close()
		delete(s.connections, id)
	}
}

// addConnection 添加连接
func (s *Server) addConnection(conn *Connection) {
	s.connectionsMu.Lock()
	defer s.connectionsMu.Unlock()
	s.connections[conn.ID] = conn
}

// removeConnection 移除连接
func (s *Server) removeConnection(id string) {
	s.connectionsMu.Lock()
	defer s.connectionsMu.Unlock()
	delete(s.connections, id)
}

// _getConnection 获取连接 (未使用，保留供将来使用)
// nolint:unused
func (s *Server) _getConnection(id string) (*Connection, bool) {
	s.connectionsMu.RLock()
	defer s.connectionsMu.RUnlock()
	conn, ok := s.connections[id]
	return conn, ok
}

// IsRunning 检查是否运行中
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// handleHealth 健康检查处理器
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"time":   time.Now().Unix(),
	})
}

// handleFeishuWebhook 飞书 webhook 处理器
func (s *Server) handleFeishuWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 获取飞书通道
	_, ok := s.channelMgr.Get("feishu")
	if !ok {
		http.Error(w, "Feishu channel not found", http.StatusServiceUnavailable)
		return
	}

	// 读取请求体
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("Failed to read webhook body", zap.Error(err))
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// 验证签名（由通道处理）
	// 这里我们需要将请求传递给飞书通道处理
	// 由于接口限制，我们暂时记录日志

	logger.Info("Received Feishu webhook",
		zap.Int("content_length", len(body)),
	)

	// 将事件发布到消息总线（由飞书通道解析）
	// 这里简化处理，实际应该由飞书通道解析并发布

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// handleGenericWebhook 通用 webhook 处理器
func (s *Server) handleGenericWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 从 URL 路径提取通道名称
	// /webhook/{channel}
	channelName := r.URL.Path[len("/webhook/"):]
	if channelName == "" {
		http.Error(w, "Channel not specified", http.StatusBadRequest)
		return
	}

	// 获取通道
	_, ok := s.channelMgr.Get(channelName)
	if !ok {
		http.Error(w, fmt.Sprintf("Channel %s not found", channelName), http.StatusServiceUnavailable)
		return
	}

	// 读取请求体
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("Failed to read webhook body",
			zap.String("channel", channelName),
			zap.Error(err),
		)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	logger.Info("Received webhook",
		zap.String("channel", channelName),
		zap.Int("content_length", len(body)),
	)

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// handleWebSocket WebSocket 连接处理器
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// 检查认证
	if s.wsConfig.EnableAuth && !s.authenticateWebSocket(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// 升级到 WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("Failed to upgrade to WebSocket", zap.Error(err))
		return
	}

	// 创建连接对象
	connection := NewConnection(conn, s.wsConfig)
	sessionID := connection.ID

	// 添加到连接管理
	s.addConnection(connection)

	logger.Info("WebSocket connection established",
		zap.String("session_id", sessionID),
		zap.String("remote_addr", r.RemoteAddr),
	)

	// 发送欢迎消息
	welcome := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "connected",
		Params: map[string]interface{}{
			"session_id": sessionID,
			"version":    ProtocolVersion,
		},
	}
	_ = connection.SendJSON(welcome)

	// 启动心跳
	go connection.heartbeat()

	// 处理消息
	go s.handleWebSocketMessages(connection)
}

// authenticateWebSocket 验证 WebSocket 连接
func (s *Server) authenticateWebSocket(r *http.Request) bool {
	// 从查询参数获取 token
	token := r.URL.Query().Get("token")
	if token == "" {
		// 从 Authorization header 获取
		auth := r.Header.Get("Authorization")
		if auth != "" {
			// 支持 "Bearer <token>" 格式
			if len(auth) > 7 && auth[:7] == "Bearer " {
				token = auth[7:]
			}
		}
	}

	if token == "" {
		return false
	}

	// 使用恒定时间比较防止时序攻击
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.authToken)) == 1
}

// handleWebSocketMessages 处理 WebSocket 消息
func (s *Server) handleWebSocketMessages(conn *Connection) {
	defer func() {
		conn.Close()
		s.removeConnection(conn.ID)
		logger.Info("WebSocket connection closed",
			zap.String("session_id", conn.ID),
		)
	}()

	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				logger.Error("WebSocket error",
					zap.String("session_id", conn.ID),
					zap.Error(err))
			}
			break
		}

		// 只处理文本消息
		if messageType != websocket.TextMessage {
			continue
		}

		// 解析请求
		req, err := ParseRequest(data)
		if err != nil {
			logger.Error("Failed to parse WebSocket message",
				zap.String("session_id", conn.ID),
				zap.Error(err))
			errorResp := NewErrorResponse("", ErrorParseError, "Parse error")
			_ = conn.SendJSON(errorResp)
			continue
		}

		logger.Debug("WebSocket request",
			zap.String("session_id", conn.ID),
			zap.String("method", req.Method),
		)

		// 处理请求
		resp := s.handler.HandleRequest(conn.ID, req)

		// 发送响应
		if err := conn.SendJSON(resp); err != nil {
			logger.Error("Failed to send WebSocket response",
				zap.String("session_id", conn.ID),
				zap.Error(err))
		}
	}
}

// broadcastOutbound 广播出站消息到所有 WebSocket 连接
func (s *Server) broadcastOutbound(ctx context.Context) {
	logger.Info("Starting WebSocket outbound broadcaster")

	// 订阅出站消息
	subscription := s.bus.SubscribeOutbound()
	defer subscription.Unsubscribe()

	logger.Info("WebSocket broadcaster subscribed",
		zap.String("subscription_id", subscription.ID))

	busChan := subscription.Channel

	for {
		select {
		case <-ctx.Done():
			logger.Info("WebSocket outbound broadcaster stopped")
			return
		case msg, ok := <-busChan:
			if !ok {
				logger.Info("Outbound channel closed, exiting broadcaster")
				return
			}
			if msg == nil {
				continue
			}

			logger.Debug("Broadcasting to WebSocket connections",
				zap.Int("connections", len(s.connections)))

			// 广播到所有连接
			s.connectionsMu.RLock()
			for _, conn := range s.connections {
				// 创建通知
				notif, err := s.handler.BroadcastNotification("message.outbound", map[string]interface{}{
					"channel":   msg.Channel,
					"chat_id":   msg.ChatID,
					"content":   msg.Content,
					"timestamp": msg.Timestamp,
				})
				if err != nil {
					logger.Error("Failed to create notification", zap.Error(err))
					continue
				}

				// 发送通知
				if err := conn.SendMessage(websocket.TextMessage, notif); err != nil {
					logger.Error("Failed to broadcast notification",
						zap.String("session_id", conn.ID),
						zap.Error(err))
				}
			}
			s.connectionsMu.RUnlock()
		}
	}
}

// Connection WebSocket 连接
type Connection struct {
	*websocket.Conn
	ID           string
	// nolint:unused
	_sessionID   string // 保留供将来使用
	pingInterval time.Duration
	pongTimeout  time.Duration
	mu           sync.Mutex
}

// NewConnection 创建连接
func NewConnection(ws *websocket.Conn, cfg *WebSocketConfig) *Connection {
	return &Connection{
		Conn:         ws,
		ID:           uuid.New().String(),
		pingInterval: cfg.PingInterval,
		pongTimeout:  cfg.PongTimeout,
	}
}

// SendJSON 发送 JSON 消息
func (c *Connection) SendJSON(v interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.WriteJSON(v)
}

// SendMessage 发送消息
func (c *Connection) SendMessage(messageType int, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.WriteMessage(messageType, data)
}

// heartbeat 心跳
func (c *Connection) heartbeat() {
	ticker := time.NewTicker(c.pingInterval)
	defer ticker.Stop()

	c.SetPongHandler(func(string) error {
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.SetReadDeadline(time.Now().Add(c.pongTimeout))
	})

	for range ticker.C {
		c.mu.Lock()
		if err := c.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
			c.mu.Unlock()
			return
		}
		if err := c.WriteMessage(websocket.PingMessage, nil); err != nil {
			c.mu.Unlock()
			return
		}
		c.mu.Unlock()
	}
}

// Close 关闭连接
func (c *Connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 发送关闭帧
	message := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
	_ = c.WriteMessage(websocket.CloseMessage, message)

	// 关闭连接
	return c.Conn.Close()
}
