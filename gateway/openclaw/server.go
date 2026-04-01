package openclaw

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Server OpenClaw Gateway 服务器
type Server struct {
	mu               sync.RWMutex
	config           *ServerConfig
	authContext      *AuthContext
	snapshotMgr      *SnapshotManager
	connectPolicy    *ConnectPolicy
	broadcastMgr     *BroadcastManager
	messageHandler   *MessageHandler
	chatMgr          *ChatManager
	nodePairingMgr   *NodePairingManager
	devicePairingMgr *DevicePairingManager
	nodeInvokeMgr    *NodeInvokeManager

	connections map[string]*Connection
	upgrader    *websocket.Upgrader
	running     bool
	ctx         context.Context
	cancel      context.CancelFunc

	tickStopper func()
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Addr           string
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	PingInterval   time.Duration
	PongTimeout    time.Duration
	MaxMessageSize int64

	// 认证配置
	AuthMode       AuthMode
	AuthToken      string
	AuthPassword   string
	TrustedProxies []string

	// 策略配置
	CheckOrigin    bool
	AllowedOrigins []string
	AllowedIPs     []string
	BlockedIPs     []string
	MaxConnPerIP   int
}

// DefaultServerConfig 默认服务器配置
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Addr:           "0.0.0.0:18789",
		ReadTimeout:    60 * time.Second,
		WriteTimeout:   10 * time.Second,
		PingInterval:   30 * time.Second,
		PongTimeout:    60 * time.Second,
		MaxMessageSize: 10 * 1024 * 1024, // 10MB

		AuthMode:     AuthModeNone,
		CheckOrigin:  true,
		MaxConnPerIP: 10,
	}
}

// NewServer 创建 OpenClaw Gateway 服务器
func NewServer(config *ServerConfig) *Server {
	if config == nil {
		config = DefaultServerConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	s := &Server{
		config:           config,
		authContext:      NewAuthContext(),
		snapshotMgr:      NewSnapshotManager(),
		connectPolicy:    NewConnectPolicy(),
		broadcastMgr:     nil, // 将在下面设置
		messageHandler:   nil, // 将在下面设置
		chatMgr:          NewChatManager(),
		nodePairingMgr:   nil, // 将在下面设置
		devicePairingMgr: nil, // 将在下面设置
		nodeInvokeMgr:    nil, // 将在下面设置
		connections:      make(map[string]*Connection),
		ctx:              ctx,
		cancel:           cancel,
	}

	// 设置依赖关系
	s.broadcastMgr = NewBroadcastManager(s.snapshotMgr)
	s.messageHandler = NewMessageHandler(s.authContext, s.snapshotMgr)
	s.nodePairingMgr = NewNodePairingManager(s.authContext, s.broadcastMgr)
	s.devicePairingMgr = NewDevicePairingManager(s.authContext, s.broadcastMgr)
	s.nodeInvokeMgr = NewNodeInvokeManager(s.authContext, s.broadcastMgr)

	// 设置连接策略
	s.connectPolicy.SetAllowedOrigins(config.AllowedOrigins)
	s.connectPolicy.SetAllowedIPs(config.AllowedIPs)
	s.connectPolicy.SetBlockedIPs(config.BlockedIPs)
	s.connectPolicy.SetMaxConnectionsPerIP(config.MaxConnPerIP)

	// 设置认证
	s.authContext.SetAuthMode(config.AuthMode)
	s.authContext.SetAuthToken(config.AuthToken)
	s.authContext.SetAuthPassword(config.AuthPassword)
	s.authContext.SetTrustedProxies(config.TrustedProxies)

	// 创建 WebSocket 升级器
	s.upgrader = &websocket.Upgrader{
		ReadBufferSize:   1024,
		WriteBufferSize:  1024,
		CheckOrigin:      s.connectPolicy.CheckOriginFunc,
		HandshakeTimeout: 10 * time.Second,
	}

	// 注册所有方法
	s.registerAllMethods()

	return s
}

// registerAllMethods 注册所有方法
func (s *Server) registerAllMethods() {
	// 系统方法
	s.messageHandler.RegisterSystemMethods()
	s.messageHandler.RegisterConfigMethods()

	// Agent 方法
	RegisterAgentMethods(s.messageHandler)

	// Session 方法
	RegisterSessionMethods(s.messageHandler)

	// 工具和技能方法
	RegisterToolsSkillsMethods(s.messageHandler)

	// Wizard 和语音方法
	RegisterWizardVoiceMethods(s.messageHandler)

	// 执行批准方法
	RegisterExecApprovalMethods(s.messageHandler)

	// 日志和监控方法
	RegisterLoggingMonitoringMethods(s.messageHandler)

	// 浏览器方法
	RegisterBrowserMethods(s.messageHandler)

	// 通道方法
	RegisterChannelsMethods(s.messageHandler)

	// Node 配对方法
	RegisterNodePairingMethods(s.messageHandler, s.nodePairingMgr)

	// 设备配对方法
	RegisterDevicePairingMethods(s.messageHandler, s.devicePairingMgr)

	// Node 调用方法
	RegisterNodeInvokeMethods(s.messageHandler, s.nodeInvokeMgr)

	// 聊天方法
	RegisterChatMethods(s.messageHandler, s.chatMgr)
}

// Start 启动服务器
func (s *Server) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true
	s.mu.Unlock()

	// 启动心跳广播
	s.tickStopper = s.broadcastMgr.StartTickBroadcast(30 * time.Second)

	// 启动 HTTP 服务器
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/health", s.handleHealth)

	srv := &http.Server{
		Addr:         s.config.Addr,
		Handler:      mux,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
	}

	// 启动清理任务
	go s.cleanupTask()

	// 监听上下文取消
	go func() {
		<-s.ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()

	return srv.ListenAndServe()
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

	s.cancel()

	if s.tickStopper != nil {
		s.tickStopper()
	}

	// 关闭所有连接
	s.closeAllConnections()

	s.broadcastMgr.Stop()

	return nil
}

// handleWebSocket 处理 WebSocket 连接
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// 检查 IP
	ip := ExtractIP(r.RemoteAddr)
	if !s.connectPolicy.CheckIP(ip) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// 检查连接限制
	if !s.connectPolicy.CanConnect(ip) {
		http.Error(w, "Too many connections", http.StatusTooManyRequests)
		return
	}

	// 升级到 WebSocket
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	// 创建连接
	connectionConfig := &ConnectionConfig{
		AuthContext:     s.authContext,
		ConnectPolicy:   s.connectPolicy,
		SnapshotManager: s.snapshotMgr,
		SendBufferSize:  256,
		ReadTimeout:     s.config.ReadTimeout,
		WriteTimeout:    s.config.WriteTimeout,
	}

	connection := NewConnection(conn, r.RemoteAddr, connectionConfig)

	// 注册连接
	s.broadcastMgr.RegisterConnection(connection)
	s.connectPolicy.RegisterConnection(connection.ID(), r.RemoteAddr)

	// 添加到连接列表
	s.mu.Lock()
	s.connections[connection.ID()] = connection
	s.mu.Unlock()

	// 发送连接挑战
	nonce, _ := s.authContext.GenerateChallengeNonce()
	challenge := NewConnectChallenge(nonce, time.Now().Unix())
	_ = connection.SendFrame(challenge)

	// 启动读写协程
	connection.StartWriter()
	connection.StartReader(s.handleConnectionMessage)

	// 等待连接关闭
	<-s.ctx.Done()
	_ = connection.Close()
}

// handleConnectionMessage 处理连接消息
func (s *Server) handleConnectionMessage(conn *Connection, req *Request) error {
	// 处理 connect 方法
	if req.Method == "connect" {
		return s.handleConnect(conn, req)
	}

	// 处理其他方法
	result, errInfo := s.messageHandler.Handle(conn, req)

	if errInfo != nil {
		return conn.SendErrorResponse(req.ID, ErrorCode(errInfo.Code), errInfo.Message, errInfo.Details)
	}

	return conn.SendSuccessResponse(req.ID, result)
}

// handleConnect 处理 connect 方法
func (s *Server) handleConnect(conn *Connection, req *Request) error {
	params, err := ParseConnectParams(req.Params)
	if err != nil {
		return conn.SendErrorResponse(req.ID, ErrorParseError, err.Error(), nil)
	}

	// 验证连接参数
	if err := conn.ValidateConnect(params); err != nil {
		return conn.SendErrorResponse(req.ID, ErrorProtocolMismatch, err.Error(), nil)
	}

	// 认证
	if err := conn.AuthenticateConnect(params); err != nil {
		return conn.SendErrorResponse(req.ID, ErrorUnauthorized, err.Error(), nil)
	}

	// 设置客户端信息
	conn.SetClientInfo(params.Client)
	conn.SetRole(params.Role)
	conn.SetScopes(params.Scopes)
	conn.SetProtocol(ProtocolVersion)

	// 更新在线状态
	presence := NewPresenceEntry(conn.ID(), params.Client, conn.RemoteAddr())
	s.snapshotMgr.UpdatePresence(conn.ID(), presence)

	// 发送 hello-ok
	if err := conn.SendHelloOK(); err != nil {
		return err
	}

	// 订阅默认事件
	for _, event := range GatewayEvents {
		if isDefaultBroadcastEvent(event) {
			conn.Subscribe(event)
		}
	}

	return nil
}

// handleHealth 处理健康检查
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// 简单的 JSON 响应
	w.Write([]byte(`{"status":"ok"}`))
}

// closeAllConnections 关闭所有连接
func (s *Server) closeAllConnections() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, conn := range s.connections {
		_ = conn.Close()
	}

	s.connections = make(map[string]*Connection)
}

// cleanupTask 清理任务
func (s *Server) cleanupTask() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.authContext.CleanExpiredNonces()
			s.authContext.CleanExpiredPendingPairs()
			s.nodePairingMgr.CleanupExpired()
			s.nodeInvokeMgr.CleanupExpired()
		case <-s.ctx.Done():
			return
		}
	}
}

// GetSnapshot 获取快照
func (s *Server) GetSnapshot() *Snapshot {
	return s.snapshotMgr.GetSnapshot()
}

// GetConnectionCount 获取连接数
func (s *Server) GetConnectionCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.connections)
}

// GetBroadcastManager 获取广播管理器
func (s *Server) GetBroadcastManager() *BroadcastManager {
	return s.broadcastMgr
}

// IsRunning 检查是否运行中
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}
