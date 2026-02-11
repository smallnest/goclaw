package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/smallnest/dogclaw/goclaw/bus"
	"github.com/smallnest/dogclaw/goclaw/config"
	"github.com/smallnest/dogclaw/goclaw/internal/logger"
	"github.com/tencent-connect/botgo"
	"github.com/tencent-connect/botgo/dto"
	"github.com/tencent-connect/botgo/log"
	"github.com/tencent-connect/botgo/openapi"
	"github.com/tencent-connect/botgo/token"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
)

// QQChannel QQ 官方开放平台 Bot 通道
// 使用 botgo SDK 实现：https://github.com/tencent-connect/botgo
type QQChannel struct {
	*BaseChannelImpl
	appID        string
	appSecret    string
	api          openapi.OpenAPI
	tokenSource  oauth2.TokenSource
	tokenCancel  context.CancelFunc
	session      *dto.WebsocketAP
	ctx          context.Context
	cancel       context.CancelFunc
	conn         *websocket.Conn
	connMu       sync.Mutex
	mu           sync.RWMutex
	sessionID    string
	lastSeq      uint32
	heartbeatInt int
	accessToken  string
	msgSeqMap    map[string]int64 // 消息序列号管理，用于去重
}

// filteredLogger 静默 botgo SDK 的日志
type filteredLogger struct{}

func (f *filteredLogger) Debug(v ...interface{})                 {}
func (f *filteredLogger) Info(v ...interface{})                  {}
func (f *filteredLogger) Warn(v ...interface{})                  {}
func (f *filteredLogger) Error(v ...interface{})                 {}
func (f *filteredLogger) Debugf(format string, v ...interface{}) {}
func (f *filteredLogger) Infof(format string, v ...interface{})  {}
func (f *filteredLogger) Warnf(format string, v ...interface{})  {}
func (f *filteredLogger) Errorf(format string, v ...interface{}) {}
func (f *filteredLogger) Sync() error                            { return nil }

// WSPayload WebSocket 消息负载
type WSPayload struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d"`
	S  uint32          `json:"s"`
	T  string          `json:"t"`
}

// HelloData Hello 事件数据
type HelloData struct {
	HeartbeatInterval int `json:"heartbeat_interval"`
}

// ReadyData Ready 事件数据
type ReadyData struct {
	SessionID string `json:"session_id"`
	Version   int    `json:"version"`
	User      struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Bot      bool   `json:"bot"`
	} `json:"user"`
}

// C2CMessageEventData C2C 消息事件数据
type C2CMessageEventData struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
	Author    struct {
		UserOpenID string `json:"user_openid"`
	} `json:"author"`
}

// GroupATMessageEventData 群 @消息事件数据
type GroupATMessageEventData struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
	Author    struct {
		MemberOpenID string `json:"member_openid"`
	} `json:"author"`
	GroupOpenID string `json:"group_openid"`
}

// ATMessageEventData 频道 @消息事件数据
type ATMessageEventData struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
	Author    struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	} `json:"author"`
	ChannelID string `json:"channel_id"`
	GuildID   string `json:"guild_id"`
}

// NewQQChannel 创建 QQ 官方 Bot 通道
func NewQQChannel(cfg config.QQChannelConfig, bus *bus.MessageBus) (*QQChannel, error) {
	if cfg.AppID == "" || cfg.AppSecret == "" {
		return nil, fmt.Errorf("qq app_id and app_secret are required")
	}

	baseCfg := BaseChannelConfig{
		Enabled:    cfg.Enabled,
		AllowedIDs: cfg.AllowedIDs,
	}

	return &QQChannel{
		BaseChannelImpl: NewBaseChannelImpl("qq-official", baseCfg, bus),
		appID:           cfg.AppID,
		appSecret:       cfg.AppSecret,
		msgSeqMap:       make(map[string]int64),
	}, nil
}

// Start 启动 QQ 官方 Bot 通道
func (c *QQChannel) Start(ctx context.Context) error {
	if err := c.BaseChannelImpl.Start(ctx); err != nil {
		return err
	}

	logger.Info("Starting QQ Official Bot channel", zap.String("app_id", c.appID))

	// 设置自定义 logger，静默 SDK 日志
	log.DefaultLogger = &filteredLogger{}

	// 创建 token source
	credentials := &token.QQBotCredentials{
		AppID:     c.appID,
		AppSecret: c.appSecret,
	}
	c.tokenSource = token.NewQQBotTokenSource(credentials)

	// 启动 token 自动刷新
	tokenCtx, cancel := context.WithCancel(context.Background())
	c.tokenCancel = cancel
	if err := token.StartRefreshAccessToken(tokenCtx, c.tokenSource); err != nil {
		return fmt.Errorf("failed to start token refresh: %w", err)
	}

	// 初始化 OpenAPI
	c.api = botgo.NewOpenAPI(c.appID, c.tokenSource).WithTimeout(10 * time.Second).SetDebug(false)

	// 启动 WebSocket 连接
	c.ctx, c.cancel = context.WithCancel(ctx)
	go c.connectWebSocket(c.ctx)

	logger.Info("QQ Official Bot channel started (WebSocket mode)")

	return nil
}

// connectWebSocket 连接 WebSocket
func (c *QQChannel) connectWebSocket(ctx context.Context) {
	reconnectDelay := 1000 * time.Millisecond
	maxDelay := 60 * time.Second

	for {
		select {
		case <-ctx.Done():
			logger.Info("QQ WebSocket connection stopped by context")
			return
		default:
			if err := c.doConnect(ctx); err != nil {
				logger.Error("QQ WebSocket connection failed, will retry",
					zap.Error(err),
					zap.Duration("retry_after", reconnectDelay),
				)
				time.Sleep(reconnectDelay)
				// 递增延迟
				reconnectDelay *= 2
				if reconnectDelay > maxDelay {
					reconnectDelay = maxDelay
				}
			} else {
				// 连接成功，重置延迟
				reconnectDelay = 1000 * time.Millisecond
				// 等待连接关闭或上下文取消
				c.waitForConnection(ctx)
			}
		}
	}
}

// doConnect 执行单次连接
func (c *QQChannel) doConnect(ctx context.Context) error {
	// 获取 access token
	token, err := c.tokenSource.Token()
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}
	c.accessToken = token.AccessToken

	// 获取 WebSocket URL
	wsResp, err := c.api.WS(ctx, map[string]string{}, "")
	if err != nil {
		return fmt.Errorf("failed to get websocket URL: %w", err)
	}

	c.mu.Lock()
	c.session = wsResp
	c.mu.Unlock()

	logger.Info("QQ WebSocket URL obtained",
		zap.String("url", wsResp.URL),
		zap.String("shards", fmt.Sprintf("%d/%d", wsResp.Shards, wsResp.SessionStartLimit)),
	)

	// 连接 WebSocket
	c.connMu.Lock()
	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, wsResp.URL, nil)
	c.connMu.Unlock()
	if err != nil {
		return fmt.Errorf("failed to dial websocket: %w", err)
	}

	c.connMu.Lock()
	c.conn = conn
	c.connMu.Unlock()

	logger.Info("QQ WebSocket connected")

	// 等待 Hello 消息并处理
	return c.waitForHello(ctx)
}

// waitForHello 等待并处理 Hello 消息
func (c *QQChannel) waitForHello(ctx context.Context) error {
	c.connMu.Lock()
	conn := c.conn
	c.connMu.Unlock()

	if conn == nil {
		return fmt.Errorf("connection is nil")
	}

	// 读取第一条消息（应该是 Hello）
	_, message, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("failed to read Hello message: %w", err)
	}

	var payload WSPayload
	if err := json.Unmarshal(message, &payload); err != nil {
		return fmt.Errorf("failed to parse Hello message: %w", err)
	}

	// Hello 事件 (op=10)
	if payload.Op != 10 {
		return fmt.Errorf("expected Hello (op=10), got op=%d", payload.Op)
	}

	var helloData HelloData
	if err := json.Unmarshal(payload.D, &helloData); err != nil {
		return fmt.Errorf("failed to parse Hello data: %w", err)
	}

	c.heartbeatInt = helloData.HeartbeatInterval
	logger.Info("QQ Hello received", zap.Int("heartbeat_interval", c.heartbeatInt))

	// 如果有 session_id，尝试 Resume；否则发送 Identify
	if c.sessionID != "" {
		return c.sendResume()
	}
	return c.sendIdentify()
}

// sendIdentify 发送 Identify
func (c *QQChannel) sendIdentify() error {
	// 尝试完整权限（群聊+私信+频道）
	intents := (1 << 25) | (1 << 12) | (1 << 30) | (1 << 0) | (1 << 1)

	payload := map[string]interface{}{
		"op": 2,
		"d": map[string]interface{}{
			"token":   fmt.Sprintf("QQBot %s", c.accessToken),
			"intents": intents,
			"shard":   []uint32{0, 1},
		},
	}

	c.connMu.Lock()
	defer c.connMu.Unlock()

	if err := c.conn.WriteJSON(payload); err != nil {
		return fmt.Errorf("failed to send identify: %w", err)
	}

	logger.Info("QQ Identify sent", zap.Int("intents", intents))
	return nil
}

// sendResume 发送 Resume
func (c *QQChannel) sendResume() error {
	payload := map[string]interface{}{
		"op": 6,
		"d": map[string]interface{}{
			"token":      fmt.Sprintf("QQBot %s", c.accessToken),
			"session_id": c.sessionID,
			"seq":        c.lastSeq,
		},
	}

	c.connMu.Lock()
	defer c.connMu.Unlock()

	if err := c.conn.WriteJSON(payload); err != nil {
		return fmt.Errorf("failed to send resume: %w", err)
	}

	logger.Info("QQ Resume sent", zap.String("session_id", c.sessionID), zap.Uint32("seq", c.lastSeq))
	return nil
}

// waitForConnection 等待 WebSocket 连接关闭
func (c *QQChannel) waitForConnection(ctx context.Context) {
	c.connMu.Lock()
	conn := c.conn
	c.connMu.Unlock()

	if conn == nil {
		return
	}

	// 启动心跳
	heartbeatTicker := time.NewTicker(time.Duration(c.heartbeatInt) * time.Millisecond)
	defer heartbeatTicker.Stop()

	// 消息读取通道
	messageChan := make(chan []byte, 100)
	errorChan := make(chan error, 1)

	// 单独的 goroutine 读取消息
	go func() {
		for {
			c.connMu.Lock()
			currentConn := c.conn
			c.connMu.Unlock()

			if currentConn == nil {
				errorChan <- fmt.Errorf("connection closed")
				return
			}

			_, message, err := currentConn.ReadMessage()
			if err != nil {
				errorChan <- err
				return
			}
			messageChan <- message
		}
	}()

	// 消息处理循环
	for {
		select {
		case <-ctx.Done():
			logger.Info("QQ WebSocket context cancelled")
			return
		case <-heartbeatTicker.C:
			c.sendHeartbeat()
		case message := <-messageChan:
			c.handleMessage(message)
		case err := <-errorChan:
			logger.Warn("WebSocket read error", zap.Error(err))
			return
		}
	}
}

// sendMessage 发送消息到 WebSocket
func (c *QQChannel) sendMessage(op int, d interface{}) error {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("connection is nil")
	}

	payload := map[string]interface{}{
		"op": op,
		"d":  d,
	}

	return c.conn.WriteJSON(payload)
}

// sendHeartbeat 发送心跳
func (c *QQChannel) sendHeartbeat() {
	if err := c.sendMessage(1, c.lastSeq); err != nil {
		logger.Warn("Failed to send heartbeat", zap.Error(err))
	}
}

// handleMessage 处理 WebSocket 消息
func (c *QQChannel) handleMessage(message []byte) {
	var payload WSPayload
	if err := json.Unmarshal(message, &payload); err != nil {
		logger.Warn("Failed to parse WebSocket message", zap.Error(err))
		return
	}

	// 更新 seq
	if payload.S > 0 {
		c.lastSeq = payload.S
	}

	switch payload.Op {
	case 0: // Dispatch
		c.handleDispatch(payload.T, payload.D)
	case 1: // Heartbeat ACK
		logger.Debug("QQ Heartbeat ACK")
	case 7: // Reconnect
		logger.Info("QQ Reconnect requested")
	default:
		logger.Debug("QQ WebSocket message", zap.Int("op", payload.Op), zap.String("t", payload.T))
	}
}

// handleDispatch 处理 Dispatch 事件
func (c *QQChannel) handleDispatch(eventType string, data json.RawMessage) {
	switch eventType {
	case "READY":
		c.handleReady(data)
	case "RESUMED":
		logger.Info("QQ Session resumed")
	case "C2C_MESSAGE_CREATE":
		c.handleC2CMessage(data)
	case "GROUP_AT_MESSAGE_CREATE":
		c.handleGroupATMessage(data)
	case "AT_MESSAGE_CREATE":
		c.handleChannelATMessage(data)
	case "DIRECT_MESSAGE_CREATE":
		// 频道私信（暂不处理）
	default:
		logger.Debug("QQ Event", zap.String("event_type", eventType))
	}
}

// handleReady 处理 Ready 事件
func (c *QQChannel) handleReady(data json.RawMessage) {
	var readyData ReadyData
	if err := json.Unmarshal(data, &readyData); err != nil {
		logger.Warn("Failed to parse Ready data", zap.Error(err))
		return
	}

	c.sessionID = readyData.SessionID
	logger.Info("QQ Ready", zap.String("session_id", c.sessionID))
}

// handleC2CMessage 处理 C2C 消息
func (c *QQChannel) handleC2CMessage(data json.RawMessage) {
	var event C2CMessageEventData
	if err := json.Unmarshal(data, &event); err != nil {
		logger.Warn("Failed to parse C2C message", zap.Error(err))
		return
	}

	senderID := event.Author.UserOpenID
	if !c.IsAllowed(senderID) {
		return
	}

	msg := &bus.InboundMessage{
		ID:        event.ID,
		Content:   event.Content,
		SenderID:  senderID,
		ChatID:    senderID,
		Channel:   c.Name(),
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"chat_type": "c2c",
			"msg_id":    event.ID,
		},
	}

	logger.Info("QQ C2C message", zap.String("sender", senderID), zap.String("content", event.Content))
	_ = c.PublishInbound(context.Background(), msg)
}

// handleGroupATMessage 处理群 @消息
func (c *QQChannel) handleGroupATMessage(data json.RawMessage) {
	var event GroupATMessageEventData
	if err := json.Unmarshal(data, &event); err != nil {
		logger.Warn("Failed to parse Group @message", zap.Error(err))
		return
	}

	senderID := event.Author.MemberOpenID
	if !c.IsAllowed(senderID) && !c.IsAllowed(event.GroupOpenID) {
		return
	}

	msg := &bus.InboundMessage{
		ID:        event.ID,
		Content:   event.Content,
		SenderID:  senderID,
		ChatID:    event.GroupOpenID,
		Channel:   c.Name(),
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"chat_type":     "group",
			"group_id":      event.GroupOpenID,
			"member_openid": senderID,
			"msg_id":        event.ID,
		},
	}

	logger.Info("QQ Group @message", zap.String("group", event.GroupOpenID), zap.String("sender", senderID), zap.String("content", event.Content))
	_ = c.PublishInbound(context.Background(), msg)
}

// handleChannelATMessage 处理频道 @消息
func (c *QQChannel) handleChannelATMessage(data json.RawMessage) {
	var event ATMessageEventData
	if err := json.Unmarshal(data, &event); err != nil {
		logger.Warn("Failed to parse Channel @message", zap.Error(err))
		return
	}

	senderID := event.Author.ID
	if !c.IsAllowed(senderID) && !c.IsAllowed(event.ChannelID) {
		return
	}

	msg := &bus.InboundMessage{
		ID:        event.ID,
		Content:   event.Content,
		SenderID:  senderID,
		ChatID:    event.ChannelID,
		Channel:   c.Name(),
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"chat_type":  "channel",
			"channel_id": event.ChannelID,
			"group_id":   event.GuildID,
			"msg_id":     event.ID,
		},
	}

	logger.Info("QQ Channel @message", zap.String("channel", event.ChannelID), zap.String("sender", senderID), zap.String("content", event.Content))
	_ = c.PublishInbound(context.Background(), msg)
}

// Send 发送消息
func (c *QQChannel) Send(msg *bus.OutboundMessage) error {
	if c.api == nil {
		return fmt.Errorf("QQ API not initialized")
	}

	ctx := context.Background()

	// 获取或递增 msg_seq
	msgSeq := c.getNextMsgSeq(msg.ChatID)

	// 构建消息
	messageToSend := &dto.MessageToCreate{
		Content:   msg.Content,
		Timestamp: time.Now().UnixMilli(),
	}

	// 判断消息类型并调用对应 API
	var err error
	if chatType, ok := msg.Metadata["chat_type"].(string); ok {
		switch chatType {
		case "group":
			err = c.sendGroupMessage(ctx, msg.ChatID, messageToSend, msgSeq)
		case "channel":
			err = c.sendChannelMessage(ctx, msg.ChatID, messageToSend, msgSeq)
		default:
			err = c.sendC2CMessage(ctx, msg.ChatID, messageToSend, msgSeq)
		}
	} else {
		// 默认 C2C 私聊
		err = c.sendC2CMessage(ctx, msg.ChatID, messageToSend, msgSeq)
	}

	return err
}

// sendC2CMessage 发送 C2C 消息
func (c *QQChannel) sendC2CMessage(ctx context.Context, openID string, msg *dto.MessageToCreate, msgSeq int64) error {
	_, err := c.api.PostC2CMessage(ctx, openID, msg)
	return err
}

// sendGroupMessage 发送群消息
func (c *QQChannel) sendGroupMessage(ctx context.Context, groupID string, msg *dto.MessageToCreate, msgSeq int64) error {
	_, err := c.api.PostGroupMessage(ctx, groupID, msg)
	return err
}

// sendChannelMessage 发送频道消息
func (c *QQChannel) sendChannelMessage(ctx context.Context, channelID string, msg *dto.MessageToCreate, msgSeq int64) error {
	_, err := c.api.PostMessage(ctx, channelID, msg)
	return err
}

// getNextMsgSeq 获取下一个消息序列号
func (c *QQChannel) getNextMsgSeq(chatID string) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	seq := c.msgSeqMap[chatID] + 1
	c.msgSeqMap[chatID] = seq
	return seq
}

// Stop 停止 QQ 官方 Bot 通道
func (c *QQChannel) Stop() error {
	logger.Info("Stopping QQ Official Bot channel")

	// 停止 token 刷新
	if c.tokenCancel != nil {
		c.tokenCancel()
	}

	// 取消上下文，断开 WebSocket
	if c.cancel != nil {
		c.cancel()
	}

	// 关闭连接
	c.closeConnection()

	return c.BaseChannelImpl.Stop()
}

// closeConnection 关闭 WebSocket 连接
func (c *QQChannel) closeConnection() {
	c.connMu.Lock()
	conn := c.conn
	c.conn = nil
	c.connMu.Unlock()

	if conn != nil {
		conn.Close()
	}
}

// HandleWebhook 处理 QQ Webhook 回调（WebSocket 模式下不使用）
func (c *QQChannel) HandleWebhook(ctx context.Context, event []byte) error {
	return nil
}

// GetSession 获取当前会话信息（用于调试）
func (c *QQChannel) GetSession() *dto.WebsocketAP {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.session
}
