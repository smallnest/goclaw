package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
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
	// typing indicator state: messageID -> reactionID mapping
	typingReactions   map[string]string
	typingReactionsMu sync.RWMutex
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
		typingReactions:   make(map[string]string),
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

	// 发布到消息总线前，先添加 typing indicator
	// 使用 messageID 来匹配用户消息
	if err := c.addTypingIndicator(messageID); err != nil {
		logger.Debug("Failed to add typing indicator (non-critical)", zap.Error(err))
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
		// 清除 typing indicator
		c.removeTypingIndicator(messageID)
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
	logger.Info("Feishu sending message",
		zap.String("chat_id", msg.ChatID),
		zap.String("reply_to", msg.ReplyTo),
		zap.Int("content_length", len(msg.Content)))

	// 判断接收者类型
	receiveIDType := larkim.ReceiveIdTypeChatId
	if len(msg.ChatID) > 3 && msg.ChatID[:3] == "ou_" {
		receiveIDType = larkim.ReceiveIdTypeOpenId
	}

	err := c.sendCardMessage(msg, receiveIDType)

	// 清除 typing indicator（无论成功或失败）
	if msg.ReplyTo != "" {
		rmErr := c.removeTypingIndicator(msg.ReplyTo)
		if rmErr != nil {
			logger.Debug("Failed to remove typing indicator (non-critical)", zap.Error(rmErr))
		}
	}

	return err
}

// addTypingIndicator 添加 typing indicator（使用 "Typing" emoji reaction）
func (c *FeishuChannel) addTypingIndicator(messageID string) error {
	emojiType := "Typing"
	req := larkim.NewCreateMessageReactionReqBuilder().
		MessageId(messageID).
		Body(larkim.NewCreateMessageReactionReqBodyBuilder().
			ReactionType(&larkim.Emoji{EmojiType: &emojiType}).
			Build()).
		Build()

	resp, err := c.httpClient.Im.MessageReaction.Create(context.Background(), req)
	if err != nil {
		logger.Debug("Feishu add typing indicator error", zap.Error(err))
		return err
	}

	if !resp.Success() {
		logger.Debug("Feishu API error for typing indicator",
			zap.Int("code", int(resp.Code)),
			zap.String("msg", resp.Msg))
		return fmt.Errorf("feishu api error: %d %s", resp.Code, resp.Msg)
	}

	if resp.Data.ReactionId != nil {
		reactionID := *resp.Data.ReactionId
		c.typingReactionsMu.Lock()
		c.typingReactions[messageID] = reactionID
		c.typingReactionsMu.Unlock()
		logger.Debug("Added typing indicator",
			zap.String("message_id", messageID),
			zap.String("reaction_id", reactionID))
	}

	return nil
}

// removeTypingIndicator 移除 typing indicator
func (c *FeishuChannel) removeTypingIndicator(messageID string) error {
	c.typingReactionsMu.Lock()
	reactionID, ok := c.typingReactions[messageID]
	if !ok {
		c.typingReactionsMu.Unlock()
		return nil
	}
	delete(c.typingReactions, messageID)
	c.typingReactionsMu.Unlock()

	req := larkim.NewDeleteMessageReactionReqBuilder().
		MessageId(messageID).
		ReactionId(reactionID).
		Build()

	resp, err := c.httpClient.Im.MessageReaction.Delete(context.Background(), req)
	if err != nil {
		logger.Debug("Feishu remove typing indicator error", zap.Error(err))
		return err
	}

	if !resp.Success() {
		logger.Debug("Feishu API error for removing typing indicator",
			zap.Int("code", int(resp.Code)),
			zap.String("msg", resp.Msg))
		return fmt.Errorf("feishu api error: %d %s", resp.Code, resp.Msg)
	}

	logger.Debug("Removed typing indicator",
		zap.String("message_id", messageID),
		zap.String("reaction_id", reactionID))

	return nil
}

// sendCardMessage 发送卡片消息（使用 markdown 格式）
func (c *FeishuChannel) sendCardMessage(msg *bus.OutboundMessage, receiveIDType string) error {
	// 构建交互式卡片，使用 markdown 元素渲染内容
	cardContent := fmt.Sprintf(`{
		"elements": [
			{
				"tag": "markdown",
				"content": %s
			}
		]
	}`, jsonEscape(msg.Content))

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(receiveIDType).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(msg.ChatID).
			MsgType(larkim.MsgTypeInteractive).
			Content(cardContent).
			Build()).
		Build()

	resp, err := c.httpClient.Im.Message.Create(context.Background(), req)
	if err != nil {
		logger.Error("Feishu send message error", zap.Error(err), zap.String("chat_id", msg.ChatID))
		return err
	}

	if !resp.Success() {
		logger.Error("Feishu API error",
			zap.Int("code", int(resp.Code)),
			zap.String("msg", resp.Msg),
			zap.String("chat_id", msg.ChatID),
		)
		return fmt.Errorf("feishu api error: %d %s", resp.Code, resp.Msg)
	}

	logger.Debug("Sent Feishu card message",
		zap.String("chat_id", msg.ChatID),
		zap.Int("content_length", len(msg.Content)))

	return nil
}

// jsonEscape 转义 JSON 字符串
func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
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
