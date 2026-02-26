package channels

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

	// 处理消息已读事件（忽略）
	c.eventDispatcher.OnP2MessageReadV1(func(ctx context.Context, event *larkim.P2MessageReadV1) error {
		logger.Debug("Feishu message read event ignored", zap.Strings("message_ids", event.Event.MessageIdList))
		return nil
	})

	// 处理机器人进入私聊事件（忽略）
	c.eventDispatcher.OnP2ChatAccessEventBotP2pChatEnteredV1(func(ctx context.Context, event *larkim.P2ChatAccessEventBotP2pChatEnteredV1) error {
		logger.Debug("Feishu bot p2p chat entered event ignored")
		return nil
	})

	// 处理消息撤回事件（忽略）
	c.eventDispatcher.OnP2MessageRecalledV1(func(ctx context.Context, event *larkim.P2MessageRecalledV1) error {
		logger.Debug("Feishu message recalled event ignored", zap.String("message_id", *event.Event.MessageId))
		return nil
	})

	// 处理消息表情反应创建事件（忽略）
	c.eventDispatcher.OnP2MessageReactionCreatedV1(func(ctx context.Context, event *larkim.P2MessageReactionCreatedV1) error {
		logger.Debug("Feishu message reaction created event ignored")
		return nil
	})

	// 处理消息表情反应删除事件（忽略）
	c.eventDispatcher.OnP2MessageReactionDeletedV1(func(ctx context.Context, event *larkim.P2MessageReactionDeletedV1) error {
		logger.Debug("Feishu message reaction deleted event ignored")
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

	// 检查发送者权限
	if senderID != "" && !c.IsAllowed(senderID) {
		return
	}

	// 检查群聊消息是否 @ 了机器人
	isGroupChat := chatType == "group"

	if isGroupChat {
		if c.botOpenId == "" {
			return
		}
		mentionedBot := c.checkBotMentioned(event.Event.Message)
		if !mentionedBot {
			return
		}
	}

	// 解析消息内容和媒体
	content, media := c.extractMessageContentAndMedia(event.Event.Message)
	if content == "" && len(media) == 0 {
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
		Media: media,
	}

	if err := c.PublishInbound(ctx, inbound); err != nil {
		logger.Error("Failed to publish inbound message",
			zap.String("message_id", messageID),
			zap.Error(err))
		// 清除 typing indicator
		c.removeTypingIndicator(messageID)
		return
	}
}

// extractMessageContent 从消息中提取文本内容
func (c *FeishuChannel) extractMessageContent(msg *larkim.EventMessage) string {
	content, _ := c.extractMessageContentAndMedia(msg)
	return content
}

// extractMessageContentAndMedia 从消息中提取文本内容和媒体文件
func (c *FeishuChannel) extractMessageContentAndMedia(msg *larkim.EventMessage) (string, []bus.Media) {
	if msg.Content == nil {
		logger.Debug("Message content is nil")
		return "", nil
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
			return "", nil
		}
		return content["text"], nil

	case "image":
		// 图片消息格式: {"image_key":"img_xxx"}
		var content map[string]string
		if err := json.Unmarshal([]byte(contentRaw), &content); err != nil {
			logger.Error("Failed to parse image message content", zap.Error(err))
			return "", nil
		}
		imageKey := content["image_key"]
		if imageKey == "" {
			return "", nil
		}
		// 使用 feishu: 前缀格式存储 image_key，用于后续通过 GetImage API 获取
		media := []bus.Media{
			{
				Type: "image",
				URL:  "feishu:" + imageKey,
			},
		}
		return "[图片]", media

	case "post":
		// 富文本消息格式: {"post":{"zh_cn":[{"tag":"text","text":"内容"}]}}
		var content map[string]interface{}
		if err := json.Unmarshal([]byte(contentRaw), &content); err != nil {
			logger.Error("Failed to parse post message content", zap.Error(err))
			return "", nil
		}
		if post, ok := content["post"].(map[string]interface{}); ok {
			if zhCn, ok := post["zh_cn"].([]interface{}); ok && len(zhCn) > 0 {
				// 提取所有文本元素和图片
				var result strings.Builder
				var media []bus.Media
				for _, elem := range zhCn {
					if elemMap, ok := elem.(map[string]interface{}); ok {
						if tag, ok := elemMap["tag"].(string); ok {
							if tag == "text" {
								if text, ok := elemMap["text"].(string); ok {
									result.WriteString(text)
								}
							} else if tag == "img" {
								// 提取富文本中的图片
								if imageKey, ok := elemMap["image_key"].(string); ok && imageKey != "" {
									media = append(media, bus.Media{
										Type: "image",
										URL:  "feishu:" + imageKey,
									})
									result.WriteString("[图片]")
								}
							}
						}
					}
				}
				return result.String(), media
			}
		}

	default:
		logger.Debug("Unsupported message type", zap.String("type", msgType))
	}

	return "", nil
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
	logger.Debug("Feishu sending message",
		zap.String("chat_id", msg.ChatID),
		zap.String("reply_to", msg.ReplyTo),
		zap.Int("content_length", len(msg.Content)),
		zap.Int("media_count", len(msg.Media)))

	// 判断接收者类型
	receiveIDType := larkim.ReceiveIdTypeChatId
	if len(msg.ChatID) > 3 && msg.ChatID[:3] == "ou_" {
		receiveIDType = larkim.ReceiveIdTypeOpenId
	}

	var err error
	// 优先发送图片消息
	if len(msg.Media) > 0 {
		for _, media := range msg.Media {
			if media.Type == "image" {
				if err = c.sendImageMessage(msg, media, receiveIDType); err != nil {
					logger.Error("Failed to send image message", zap.Error(err))
				}
			}
		}
	}

	// 如果有文本内容，发送卡片消息
	if msg.Content != "" {
		if err = c.sendCardMessage(msg, receiveIDType); err != nil {
			logger.Error("Failed to send card message", zap.Error(err))
		}
	}

	// 清除 typing indicator（无论成功或失败）
	if msg.ReplyTo != "" {
		rmErr := c.removeTypingIndicator(msg.ReplyTo)
		if rmErr != nil {
			logger.Debug("Failed to remove typing indicator (non-critical)", zap.Error(rmErr))
		}
	}

	return err
}

// downloadFeishuImage 从飞书下载图片，返回 io.ReadCloser
func (c *FeishuChannel) downloadFeishuImage(imageKey string) (io.ReadCloser, error) {
	req := larkim.NewGetImageReqBuilder().
		ImageKey(imageKey).
		Build()

	resp, err := c.httpClient.Im.Image.Get(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("failed to get image: %w", err)
	}

	if !resp.Success() || resp.File == nil {
		return nil, fmt.Errorf("get image failed: code=%d msg=%s", resp.Code, resp.Msg)
	}

	// resp.File 是 io.Reader，读取所有数据
	data, err := io.ReadAll(resp.File)
	if err != nil {
		return nil, fmt.Errorf("failed to read image data: %w", err)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// uploadImage 上传图片到飞书，返回 image_key
func (c *FeishuChannel) uploadImage(imageData io.Reader) (string, error) {
	imageType := "message" // message 类型的图片可以用于发送消息
	req := larkim.NewCreateImageReqBuilder().
		Body(
			larkim.NewCreateImageReqBodyBuilder().
				ImageType(imageType).
				Image(imageData).
				Build(),
		).
		Build()

	resp, err := c.httpClient.Im.Image.Create(context.Background(), req)
	if err != nil {
		return "", fmt.Errorf("failed to upload image: %w", err)
	}

	if !resp.Success() || resp.Data == nil {
		return "", fmt.Errorf("upload image failed: code=%d msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data.ImageKey == nil {
		return "", fmt.Errorf("upload image response missing image_key")
	}

	logger.Debug("Image uploaded successfully", zap.String("image_key", *resp.Data.ImageKey))
	return *resp.Data.ImageKey, nil
}

// sendImageMessage 发送图片消息
func (c *FeishuChannel) sendImageMessage(msg *bus.OutboundMessage, media bus.Media, receiveIDType string) error {
	var imageReader io.Reader

	// 根据不同来源获取图片数据
	if media.URL != "" {
		var imageBody io.ReadCloser
		var err error

		// 检查是否是 Feishu 图片 (feishu:image_key 格式)
		if strings.HasPrefix(media.URL, "feishu:") {
			imageKey := strings.TrimPrefix(media.URL, "feishu:")
			imageBody, err = c.downloadFeishuImage(imageKey)
			if err != nil {
				return fmt.Errorf("failed to download feishu image: %w", err)
			}
		} else {
			// 从普通 URL 下载图片
			req, err := http.NewRequest("GET", media.URL, nil)
			if err != nil {
				return fmt.Errorf("failed to create download request: %w", err)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("failed to download image from URL: %w", err)
			}
			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				return fmt.Errorf("failed to download image, status: %d", resp.StatusCode)
			}
			imageBody = resp.Body
		}
		defer imageBody.Close()
		imageReader = imageBody

	} else if media.Base64 != "" {
		// 从 Base64 解码图片
		data, err := base64.StdEncoding.DecodeString(media.Base64)
		if err != nil {
			return fmt.Errorf("failed to decode base64 image: %w", err)
		}
		imageReader = bytes.NewReader(data)
	} else {
		return fmt.Errorf("no valid image data (URL or Base64) provided")
	}

	// 上传图片获取 image_key
	imageKey, err := c.uploadImage(imageReader)
	if err != nil {
		return fmt.Errorf("failed to upload image: %w", err)
	}

	// 构建图片消息内容: {"image_key":"xxx"}
	content := fmt.Sprintf(`{"image_key":"%s"}`, imageKey)

	imageMsgReq := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(receiveIDType).
		Body(
			larkim.NewCreateMessageReqBodyBuilder().
				ReceiveId(msg.ChatID).
				MsgType(larkim.MsgTypeImage).
				Content(content).
				Build(),
		).
		Build()

	resp, err := c.httpClient.Im.Message.Create(context.Background(), imageMsgReq)
	if err != nil {
		logger.Error("Feishu send image message error", zap.Error(err), zap.String("chat_id", msg.ChatID), zap.String("image_key", imageKey))
		return err
	}

	if !resp.Success() {
		logger.Error("Feishu API error for image message",
			zap.Int("code", int(resp.Code)),
			zap.String("msg", resp.Msg),
			zap.String("chat_id", msg.ChatID),
			zap.String("image_key", imageKey),
		)
		return fmt.Errorf("feishu api error: %d %s", resp.Code, resp.Msg)
	}

	logger.Debug("Sent Feishu image message",
		zap.String("chat_id", msg.ChatID),
		zap.String("image_key", imageKey))

	return nil
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
