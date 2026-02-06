package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/smallnest/dogclaw/goclaw/bus"
	"github.com/smallnest/dogclaw/goclaw/config"
	"github.com/smallnest/dogclaw/goclaw/internal/logger"
	"go.uber.org/zap"
)

// QQChannel QQ 官方开放平台 Bot 通道
// 基于 QQ 开放平台 API v2: https://bot.q.qq.com/wiki
type QQChannel struct {
	*BaseChannelImpl
	appID        string
	appSecret    string
	accessToken  string
	httpClient   *http.Client
	mu           sync.RWMutex
	msgSeqMap    map[string]int64 // 消息序列号管理，用于去重
}

// NewQQChannel 创建 QQ 官方 Bot 通道
func NewQQChannel(cfg config.QQChannelConfig, bus *bus.MessageBus) (*QQChannel, error) {
	if cfg.AppID == "" {
		return nil, fmt.Errorf("qq app_id is required")
	}

	baseCfg := BaseChannelConfig{
		Enabled:    cfg.Enabled,
		AllowedIDs: cfg.AllowedIDs,
	}

	return &QQChannel{
		BaseChannelImpl: NewBaseChannelImpl("qq-official", baseCfg, bus),
		appID:           cfg.AppID,
		appSecret:       cfg.AppSecret,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		msgSeqMap: make(map[string]int64),
	}, nil
}

// Start 启动 QQ 官方 Bot 通道
func (c *QQChannel) Start(ctx context.Context) error {
	if err := c.BaseChannelImpl.Start(ctx); err != nil {
		return err
	}

	logger.Info("Starting QQ Official Bot channel", zap.String("app_id", c.appID))

	// 获取访问令牌
	if err := c.refreshAccessToken(); err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	// 启动 WebSocket 监听（用于接收消息推送）
	go c.listenWebSocket(ctx)

	return nil
}

// QQAccessTokenResponse QQ 访问令牌响应
type QQAccessTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

// refreshAccessToken 刷新访问令牌
func (c *QQChannel) refreshAccessToken() error {
	url := "https://bot.q.qq.com/openapi/oauth2/token"

	req := map[string]string{
		"app_id":     c.appID,
		"client_secret": c.appSecret,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to get access token: status %d, response: %s", resp.StatusCode, string(data))
	}

	var tokenResp QQAccessTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return err
	}

	c.mu.Lock()
	c.accessToken = tokenResp.AccessToken
	c.mu.Unlock()

	logger.Info("QQ access token refreshed", zap.String("expires_in", fmt.Sprintf("%d seconds", tokenResp.ExpiresIn)))

	return nil
}

// QQMessageEvent QQ 消息事件
type QQMessageEvent struct {
	PostType     string `json:"post_type"`
	MessageType  string `json:"message_type"`
	MessageID    string `json:"message_id"`
	Message      struct {
		ID      string `json:"id"`
		Content string `json:"content"`
	} `json:"message"`
	Author       struct {
		ID       string `json:"id"`
		Nickname string `json:"nickname"`
	} `json:"author"`
	GroupID      string `json:"group_id,omitempty"`
	ChannelID   string `json:"channel_id,omitempty"`
	GuildID     string `json:"guild_id,omitempty"`
	Timestamp    int64  `json:"timestamp"`
}

// listenWebSocket 监听 WebSocket 消息（待实现）
// QQ 官方 API 支持 Webhook 推送，这里需要设置 Webhook 服务器
func (c *QQChannel) listenWebSocket(ctx context.Context) {
	// QQ 官方 API 主要通过 Webhook 推送事件
	// 需要设置一个公网可访问的 Webhook 服务器
	// 这里暂时不实现，可以作为后续功能
	logger.Info("QQ Official Bot uses Webhook for event delivery, WebSocket not implemented yet")
}

// Send 发送消息
func (c *QQChannel) Send(msg *bus.OutboundMessage) error {
	c.mu.RLock()
	token := c.accessToken
	c.mu.RUnlock()

	if token == "" {
		return fmt.Errorf("no access token")
	}

	// 根据消息类型调用不同的 API
	// 支持：私聊消息、群消息、频道消息

	// 获取或递增 msg_seq
	msgSeq := c.getNextMsgSeq(msg.ChatID)

	payload := map[string]interface{}{
		"content":   msg.Content,
		"msg_id":    msg.ID,
		"msg_seq":   msgSeq,
		"timestamp": time.Now().Unix(),
	}

	// 判断消息类型并调用对应 API
	var url string
	if chatType, ok := msg.Metadata["chat_type"].(string); ok {
		switch chatType {
		case "group":
			url = fmt.Sprintf("https://bot.q.qq.com/openapi/v2/groups/%s/messages", msg.ChatID)
		case "channel":
			url = fmt.Sprintf("https://bot.q.qq.com/openapi/v2/channels/%s/messages", msg.ChatID)
		default:
			url = fmt.Sprintf("https://bot.q.qq.com/openapi/v2/users/%s/messages", msg.ChatID)
		}
	} else {
		// 默认私聊
		url = fmt.Sprintf("https://bot.q.qq.com/openapi/v2/users/%s/messages", msg.ChatID)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "QQBot "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to send message: status %d, response: %s", resp.StatusCode, string(data))
	}

	return nil
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
	return c.BaseChannelImpl.Stop()
}

// HandleWebhook 处理 QQ Webhook 回调
func (c *QQChannel) HandleWebhook(ctx context.Context, event []byte) error {
	var msgEvent QQMessageEvent
	if err := json.Unmarshal(event, &msgEvent); err != nil {
		return fmt.Errorf("failed to parse webhook event: %w", err)
	}

	// 只处理消息事件
	if msgEvent.PostType != "message" {
		return nil
	}

	// 解析用户 ID
	userID := msgEvent.Author.ID
	if !c.IsAllowed(userID) {
		return nil
	}

	// 解析聊天 ID
	chatID := userID
	chatType := "private"

	if msgEvent.GroupID != "" {
		chatID = msgEvent.GroupID
		chatType = "group"
	} else if msgEvent.ChannelID != "" {
		chatID = msgEvent.ChannelID
		chatType = "channel"
	}

	// 构造消息
	msg := &bus.InboundMessage{
		ID:        msgEvent.Message.ID,
		Content:   msgEvent.Message.Content,
		SenderID:  userID,
		ChatID:    chatID,
		Channel:   c.Name(),
		Timestamp: time.Unix(msgEvent.Timestamp, 0),
		Metadata: map[string]interface{}{
			"sender_name": msgEvent.Author.Nickname,
			"chat_type":   chatType,
			"guild_id":    msgEvent.GuildID,
			"channel_id":  msgEvent.ChannelID,
		},
	}

	c.PublishInbound(ctx, msg)

	return nil
}
