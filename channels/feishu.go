package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/internal/logger"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	"go.uber.org/zap"
)

// FeishuChannel 飞书通道 - WebSocket 模式
type FeishuChannel struct {
	*BaseChannelImpl
	appID             string
	appSecret         string
	domain            string
	encryptKey        string
	verificationToken string
	wsClient          *larkws.Client
	eventDispatcher   *dispatcher.EventDispatcher
	httpClient        *lark.Client
}

// NewFeishuChannel 创建飞书通道
func NewFeishuChannel(cfg config.FeishuChannelConfig, bus *bus.MessageBus) (*FeishuChannel, error) {
	if cfg.AppID == "" || cfg.AppSecret == "" {
		return nil, fmt.Errorf("feishu app_id and app_secret are required")
	}

	// 创建 HTTP client for sending messages
	client := lark.NewClient(
		cfg.AppID,
		cfg.AppSecret,
		lark.WithAppType(larkcore.AppTypeSelfBuilt),
		lark.WithOpenBaseUrl(resolveDomain(cfg.Domain)),
	)

	baseCfg := BaseChannelConfig{
		Enabled:    cfg.Enabled,
		AllowedIDs: cfg.AllowedIDs,
	}

	return &FeishuChannel{
		BaseChannelImpl:   NewBaseChannelImpl("feishu", "default", baseCfg, bus),
		appID:             cfg.AppID,
		appSecret:         cfg.AppSecret,
		domain:            cfg.Domain,
		encryptKey:        cfg.EncryptKey,
		verificationToken: cfg.VerificationToken,
		httpClient:        client,
	}, nil
}

// Start 启动飞书通道
func (c *FeishuChannel) Start(ctx context.Context) error {
	if err := c.BaseChannelImpl.Start(ctx); err != nil {
		return err
	}

	logger.Info("Starting Feishu channel (WebSocket mode)",
		zap.String("app_id", c.appID),
		zap.String("domain", c.domain))

	// 创建事件分发器
	c.eventDispatcher = dispatcher.NewEventDispatcher(
		c.verificationToken,
		c.encryptKey,
	)

	// 注册事件处理器
	c.registerEventHandlers(ctx)

	// 创建 WebSocket 客户端
	c.wsClient = larkws.NewClient(
		c.appID,
		c.appSecret,
		larkws.WithEventHandler(c.eventDispatcher),
		larkws.WithDomain(resolveDomain(c.domain)),
		larkws.WithLogLevel(larkcore.LogLevelInfo),
	)

	// 启动 WebSocket 连接
	go c.startWebSocket(ctx)

	return nil
}

// resolveDomain 解析域名
func resolveDomain(domain string) string {
	if domain == "lark" {
		return lark.LarkBaseUrl
	}
	return lark.FeishuBaseUrl
}

// registerEventHandlers 注册事件处理器
func (c *FeishuChannel) registerEventHandlers(ctx context.Context) {
	// 处理接收消息事件
	c.eventDispatcher.OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
		c.handleMessageReceived(ctx, event)
		return nil
	})

	// 处理机器人被添加到群聊事件
	c.eventDispatcher.OnP2ChatMemberBotAddedV1(func(ctx context.Context, event *larkim.P2ChatMemberBotAddedV1) error {
		logger.Info("Feishu bot added to chat",
			zap.String("chat_id", *event.Event.ChatId))
		return nil
	})

	// 处理机器人被移出群聊事件
	c.eventDispatcher.OnP2ChatMemberBotDeletedV1(func(ctx context.Context, event *larkim.P2ChatMemberBotDeletedV1) error {
		logger.Info("Feishu bot removed from chat",
			zap.String("chat_id", *event.Event.ChatId))
		return nil
	})
}

// startWebSocket 启动 WebSocket 连接
func (c *FeishuChannel) startWebSocket(ctx context.Context) {
	logger.Info("Starting Feishu WebSocket connection")

	// Start blocks forever, so run it in the goroutine
	// The wsClient will handle reconnection automatically
	if err := c.wsClient.Start(ctx); err != nil {
		logger.Error("Feishu WebSocket error", zap.Error(err))
	}

	logger.Info("Feishu WebSocket connection stopped")
}

// handleMessageReceived 处理接收到的消息
func (c *FeishuChannel) handleMessageReceived(ctx context.Context, event *larkim.P2MessageReceiveV1) {
	if event.Event == nil || event.Event.Sender == nil || event.Event.Message == nil {
		logger.Debug("Feishu message event has nil fields")
		return
	}

	senderID := ""
	if event.Event.Sender.SenderId != nil {
		if event.Event.Sender.SenderId.OpenId != nil {
			senderID = *event.Event.Sender.SenderId.OpenId
		} else if event.Event.Sender.SenderId.UserId != nil {
			senderID = *event.Event.Sender.SenderId.UserId
		}
	}

	chatID := ""
	if event.Event.Message.ChatId != nil {
		chatID = *event.Event.Message.ChatId
	}

	messageID := ""
	if event.Event.Message.MessageId != nil {
		messageID = *event.Event.Message.MessageId
	}

	logger.Debug("Feishu message received",
		zap.String("chat_id", chatID),
		zap.String("message_id", messageID),
		zap.String("sender_id", senderID))

	// 检查发送者权限
	if senderID != "" && !c.IsAllowed(senderID) {
		logger.Debug("Feishu message filtered (not allowed)",
			zap.String("sender_id", senderID))
		return
	}

	// 解析消息内容
	content := c.extractMessageContent(event.Event.Message)
	if content == "" {
		logger.Debug("Feishu message has no extractable text content")
		return
	}

	// 解析时间戳
	var timestamp time.Time
	if event.Event.Message.CreateTime != nil {
		if ms, err := strconv.ParseInt(*event.Event.Message.CreateTime, 10, 64); err == nil {
			timestamp = time.UnixMilli(ms)
		} else {
			timestamp = time.Now()
		}
	} else {
		timestamp = time.Now()
	}

	// 发布到消息总线
	inbound := &bus.InboundMessage{
		ID:        messageID,
		Content:   content,
		SenderID:  senderID,
		ChatID:    chatID,
		Channel:   c.Name(),
		AccountID: "default",
		Timestamp: timestamp,
		Metadata: map[string]interface{}{
			"msg_type": getStringPtr(event.Event.Message.MessageType),
		},
	}

	if err := c.PublishInbound(ctx, inbound); err != nil {
		logger.Error("Failed to publish inbound message",
			zap.String("message_id", messageID),
			zap.Error(err))
		return
	}

	logger.Debug("Processed Feishu message",
		zap.String("message_id", messageID),
		zap.String("chat_id", chatID),
		zap.String("sender_id", senderID))
}

// extractMessageContent 从消息中提取文本内容
func (c *FeishuChannel) extractMessageContent(msg *larkim.EventMessage) string {
	if msg.MessageType == nil || *msg.MessageType != "text" {
		return ""
	}

	if msg.Content == nil {
		return ""
	}

	// 解析 content JSON
	var content map[string]string
	if err := json.Unmarshal([]byte(*msg.Content), &content); err != nil {
		logger.Error("Failed to parse message content", zap.Error(err))
		return ""
	}

	return content["text"]
}

// Send 发送消息
func (c *FeishuChannel) Send(msg *bus.OutboundMessage) error {
	// 构建 content JSON 对象
	contentMap := map[string]string{"text": msg.Content}
	contentBytes, err := json.Marshal(contentMap)
	if err != nil {
		return fmt.Errorf("failed to marshal content: %w", err)
	}

	// 构建消息请求
	resp, err := c.httpClient.Im.Message.Create(context.Background(),
		larkim.NewCreateMessageReqBuilder().
			ReceiveIdType(larkim.ReceiveIdTypeChatId).
			Body(larkim.NewCreateMessageReqBodyBuilder().
				ReceiveId(msg.ChatID).
				MsgType(larkim.MsgTypeText).
				Content(string(contentBytes)).
				Build()).
			Build())

	if err != nil {
		return fmt.Errorf("failed to create message request: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("feishu api error: %d %s", resp.Code, resp.Msg)
	}

	logger.Debug("Sent Feishu message",
		zap.String("chat_id", msg.ChatID),
		zap.Int("content_length", len(msg.Content)))

	return nil
}

// Stop 停止飞书通道
func (c *FeishuChannel) Stop() error {
	logger.Info("Stopping Feishu channel")

	// WebSocket 客户端没有 explicit Stop 方法
	// 当 context 被 cancel 时，Start 方法会自动返回

	return c.BaseChannelImpl.Stop()
}

// Helper function
func getStringPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
