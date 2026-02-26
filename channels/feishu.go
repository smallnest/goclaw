package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/internal/logger"
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
	// bot open_id for mention checking
	botOpenId string
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

	// 获取机器人的 open_id（用于 @ 检查）
	if err := c.fetchBotOpenId(); err != nil {
		logger.Warn("Failed to fetch bot open_id, mention checking will be disabled", zap.Error(err))
	} else {
		logger.Info("Feishu bot open_id resolved", zap.String("bot_open_id", c.botOpenId))
	}

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

// fetchBotOpenId 获取机器人的 open_id
func (c *FeishuChannel) fetchBotOpenId() error {
	ctx := context.Background()

	// 1. 获取 app_access_token
	tokenReq := &larkcore.SelfBuiltAppAccessTokenReq{
		AppID:     c.appID,
		AppSecret: c.appSecret,
	}

	tokenResp, err := c.httpClient.GetAppAccessTokenBySelfBuiltApp(ctx, tokenReq)
	if err != nil {
		return fmt.Errorf("failed to get app access token: %w", err)
	}
	if !tokenResp.Success() || tokenResp.AppAccessToken == "" {
		return fmt.Errorf("app access token error: code=%d msg=%s", tokenResp.Code, tokenResp.Msg)
	}

	// 2. 使用 app_access_token 调用 bot/info API
	apiResp, err := c.httpClient.Get(ctx, "/open-apis/bot/v3/info", nil, larkcore.AccessTokenTypeApp)
	if err != nil {
		return fmt.Errorf("failed to fetch bot info: %w", err)
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Bot  struct {
			OpenId  string `json:"open_id"`
			BotName string `json:"bot_name"`
		} `json:"bot"`
	}

	if err := json.Unmarshal(apiResp.RawBody, &result); err != nil {
		return fmt.Errorf("failed to decode bot info response: %w", err)
	}

	if result.Code != 0 {
		return fmt.Errorf("bot info API error: code=%d msg=%s", result.Code, result.Msg)
	}

	c.botOpenId = result.Bot.OpenId
	logger.Info("Fetched bot info",
		zap.String("bot_name", result.Bot.BotName),
		zap.String("bot_open_id", c.botOpenId))
	return nil
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

	chatType := "unknown"
	if event.Event.Message.ChatType != nil {
		chatType = *event.Event.Message.ChatType
	}
	messageType := "unknown"
	if event.Event.Message.MessageType != nil {
		messageType = *event.Event.Message.MessageType
	}

	logger.Info("Feishu message received",
		zap.String("chat_id", chatID),
		zap.String("message_id", messageID),
		zap.String("sender_id", senderID),
		zap.String("chat_type", chatType),
		zap.String("message_type", messageType))

	// 检查发送者权限
	if senderID != "" && !c.IsAllowed(senderID) {
		logger.Info("Feishu message filtered (not allowed)",
			zap.String("sender_id", senderID))
		return
	}

	// 检查群聊消息是否 @ 了机器人
	isGroupChat := chatType == "group"

	if isGroupChat {
		if c.botOpenId == "" {
			logger.Info("Feishu group message skipped (botOpenId not resolved)",
				zap.String("chat_id", chatID))
			return
		}
		mentionedBot := c.checkBotMentioned(event.Event.Message)
		if !mentionedBot {
			logger.Info("Feishu message filtered (not mentioned bot in group)",
				zap.String("chat_id", chatID),
				zap.String("bot_open_id", c.botOpenId),
				zap.Int("mentions_count", len(event.Event.Message.Mentions)))
			return
		}
	}

	// 解析消息内容
	content := c.extractMessageContent(event.Event.Message)
	if content == "" {
		logger.Info("Feishu message has no extractable text content", zap.String("message_type", messageType))
		return
	}

	logger.Info("Processing Feishu message",
		zap.String("chat_id", chatID),
		zap.String("chat_type", chatType),
		zap.Int("content_length", len(content)))

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

	logger.Info("Processed Feishu message",
		zap.String("message_id", messageID),
		zap.String("chat_id", chatID),
		zap.String("sender_id", senderID))
}

// extractMessageContent 从消息中提取文本内容
func (c *FeishuChannel) extractMessageContent(msg *larkim.EventMessage) string {
	if msg.Content == nil {
		logger.Debug("Message content is nil")
		return ""
	}

	contentRaw := *msg.Content
	logger.Debug("Extracting message content", zap.String("message_type", getStringPtr(msg.MessageType)), zap.String("content", contentRaw))

	// 支持多种消息类型
	msgType := "text"
	if msg.MessageType != nil {
		msgType = *msg.MessageType
	}

	switch msgType {
	case "text":
		// 文本消息格式: {"text":"内容"}
		var content map[string]string
		if err := json.Unmarshal([]byte(contentRaw), &content); err != nil {
			logger.Error("Failed to parse text message content", zap.Error(err))
			return ""
		}
		return content["text"]

	case "post":
		// 富文本消息格式: {"post":{"zh_cn":[{"tag":"text","text":"内容"}]}}
		var content map[string]interface{}
		if err := json.Unmarshal([]byte(contentRaw), &content); err != nil {
			logger.Error("Failed to parse post message content", zap.Error(err))
			return ""
		}
		if post, ok := content["post"].(map[string]interface{}); ok {
			if zhCn, ok := post["zh_cn"].([]interface{}); ok && len(zhCn) > 0 {
				// 提取所有文本元素
				var result strings.Builder
				for _, elem := range zhCn {
					if elemMap, ok := elem.(map[string]interface{}); ok {
						if tag, ok := elemMap["tag"].(string); ok && tag == "text" {
							if text, ok := elemMap["text"].(string); ok {
								result.WriteString(text)
							}
						}
					}
				}
				return result.String()
			}
		}

	default:
		logger.Debug("Unsupported message type", zap.String("type", msgType))
	}

	return ""
}

// checkBotMentioned 检查消息是否 @ 了机器人
func (c *FeishuChannel) checkBotMentioned(msg *larkim.EventMessage) bool {
	mentions := msg.Mentions

	// 如果不 AT 任何机器人，就当废话
	// if len(mentions) == 0 {
	// 	logger.Debug("No mentions in message", zap.String("bot_open_id", c.botOpenId))
	// 	return false
	// }

	// 遍历 mentions，检查是否有机器人的 open_id
	for _, mention := range mentions {
		mentionOpenId := ""
		if mention.Id != nil && mention.Id.OpenId != nil {
			mentionOpenId = *mention.Id.OpenId
		}
		logger.Debug("Checking mention",
			zap.String("bot_open_id", c.botOpenId),
			zap.String("mention_open_id", mentionOpenId),
			zap.Bool("matches", mentionOpenId == c.botOpenId))

		if mention.Id != nil && mention.Id.OpenId != nil {
			if *mention.Id.OpenId == c.botOpenId {
				logger.Info("Bot mentioned in message",
					zap.String("bot_open_id", c.botOpenId),
					zap.String("mention_open_id", *mention.Id.OpenId))
				return true
			}
		}
	}

	logger.Debug("Bot not mentioned in message",
		zap.String("bot_open_id", c.botOpenId),
		zap.Int("mentions_count", len(mentions)))
	return false
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
	// 使用 schema 2.0 格式以支持完整的 markdown 渲染（包括 heading 和 code fence）
	cardContent := fmt.Sprintf(`{
		"schema": "2.0",
		"config": {
			"wide_screen_mode": true
		},
		"body": {
			"elements": [
				{
					"tag": "markdown",
					"content": %s
				}
			]
		}
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
