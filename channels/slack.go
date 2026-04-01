package channels

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/slack-go/slack"
	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

// SlackChannel Slack 通道
type SlackChannel struct {
	*BaseChannelImpl
	client        *slack.Client
	token         string
	signingSecret string
}

// SlackConfig Slack 配置
type SlackConfig struct {
	BaseChannelConfig
	Token         string `mapstructure:"token" json:"token"`
	SigningSecret string `mapstructure:"signing_secret" json:"signing_secret"`
}

// NewSlackChannel 创建 Slack 通道
func NewSlackChannel(cfg SlackConfig, bus *bus.MessageBus) (*SlackChannel, error) {
	if cfg.Token == "" {
		return nil, fmt.Errorf("slack token is required")
	}

	return &SlackChannel{
		BaseChannelImpl: NewBaseChannelImpl("slack", "default", cfg.BaseChannelConfig, bus),
		token:           cfg.Token,
		signingSecret:   cfg.SigningSecret,
	}, nil
}

// Start 启动 Slack 通道
func (c *SlackChannel) Start(ctx context.Context) error {
	if err := c.BaseChannelImpl.Start(ctx); err != nil {
		return err
	}

	logger.Info("Starting Slack channel")

	// 创建 Slack 客户端
	c.client = slack.New(c.token)

	// 获取 bot 信息
	authResp, err := c.client.AuthTest()
	if err != nil {
		return fmt.Errorf("failed to authenticate with slack: %w", err)
	}

	logger.Info("Slack bot started",
		zap.String("bot_name", authResp.User),
		zap.String("team_name", authResp.Team),
		zap.String("bot_id", authResp.UserID),
	)

	// 启动消息处理 (RTM 模式)
	rtm := c.client.NewRTM()
	go rtm.ManageConnection()

	// 启动消息处理
	go c.handleRTM(ctx, rtm)

	// 启动健康检查
	go c.healthCheck(ctx)

	return nil
}

// handleRTM 处理 RTM 消息
func (c *SlackChannel) handleRTM(ctx context.Context, rtm *slack.RTM) {
	for {
		select {
		case <-ctx.Done():
			logger.Info("Slack RTM handler stopped by context")
			return
		case <-c.WaitForStop():
			logger.Info("Slack RTM handler stopped")
			return
		case msg := <-rtm.IncomingEvents:
			switch ev := msg.Data.(type) {
			case *slack.MessageEvent:
				c.handleMessage(ctx, ev)
			case *slack.ConnectedEvent:
				logger.Info("Slack RTM connected")
			case *slack.ConnectionErrorEvent:
				logger.Error("Slack connection error", zap.Error(ev))
			case *slack.RTMError:
				logger.Error("Slack RTM error", zap.Error(ev))
			}
		}
	}
}

// handleMessage 处理 Slack 消息
func (c *SlackChannel) handleMessage(ctx context.Context, ev *slack.MessageEvent) {
	// 忽略 bot 消息
	if ev.BotID != "" || ev.SubType != "" {
		return
	}

	// 获取用户信息
	user, err := c.client.GetUserInfo(ev.User)
	if err != nil {
		logger.Error("Failed to get Slack user info", zap.Error(err))
		return
	}

	// 检查权限
	senderID := ev.User
	if !c.IsAllowed(senderID) {
		logger.Warn("Slack message from unauthorized sender",
			zap.String("sender_id", senderID),
			zap.String("sender_name", user.Name),
		)
		return
	}

	// 处理命令
	if strings.HasPrefix(ev.Text, "/") {
		c.handleCommand(ctx, ev)
		return
	}

	// 构建入站消息
	msg := &bus.InboundMessage{
		Channel:  c.Name(),
		SenderID: senderID,
		ChatID:   ev.Channel,
		Content:  ev.Text,
		Media:    c.extractMedia(ev),
		Metadata: map[string]interface{}{
			"message_id":     ev.Timestamp,
			"user_name":      user.Name,
			"user_real_name": user.RealName,
			"team":           ev.Team,
		},
		Timestamp: time.Now(),
	}

	if err := c.PublishInbound(ctx, msg); err != nil {
		logger.Error("Failed to publish Slack message", zap.Error(err))
	}
}

// handleCommand 处理命令
func (c *SlackChannel) handleCommand(ctx context.Context, ev *slack.MessageEvent) {
	command := ev.Text

	// Use shared command response helper
	response := GetDefaultCommandResponse(command, c.IsRunning())
	if response == "" {
		return
	}

	_, _, err := c.client.PostMessage(ev.Channel, slack.MsgOptionText(response, false))
	if err != nil {
		logger.Error("Failed to send Slack message", zap.Error(err))
	}
}

// extractMedia 提取媒体
func (c *SlackChannel) extractMedia(ev *slack.MessageEvent) []bus.Media {
	var media []bus.Media

	// 处理附件
	if len(ev.Attachments) > 0 {
		for _, att := range ev.Attachments {
			mediaType := "document"
			// Check attachment type based on available fields
			if att.Title != "" && strings.Contains(strings.ToLower(att.Title), "image") {
				mediaType = "image"
			}
			if att.Title != "" && strings.Contains(strings.ToLower(att.Title), "video") {
				mediaType = "video"
			}

			media = append(media, bus.Media{
				Type:     mediaType,
				URL:      att.TitleLink,
				MimeType: "",
			})
		}
	}

	// 处理文件
	if len(ev.Files) > 0 {
		for _, file := range ev.Files {
			mediaType := "document"
			if strings.HasPrefix(file.Mimetype, "image/") {
				mediaType = "image"
			} else if strings.HasPrefix(file.Mimetype, "video/") {
				mediaType = "video"
			} else if strings.HasPrefix(file.Mimetype, "audio/") {
				mediaType = "audio"
			}

			media = append(media, bus.Media{
				Type:     mediaType,
				URL:      file.URLPrivate,
				MimeType: file.Mimetype,
			})
		}
	}

	return media
}

// Send 发送消息
func (c *SlackChannel) Send(msg *bus.OutboundMessage) error {
	if !c.IsRunning() {
		return fmt.Errorf("slack channel is not running")
	}

	if c.client == nil {
		return fmt.Errorf("slack client is not initialized")
	}

	// 构建消息选项
	options := []slack.MsgOption{
		slack.MsgOptionText(msg.Content, false),
	}

	// 处理回复
	if msg.ReplyTo != "" {
		options = append(options, slack.MsgOptionTS(msg.ReplyTo))
	}

	// 处理媒体
	if len(msg.Media) > 0 {
		for _, media := range msg.Media {
			if media.Type == "image" && media.URL != "" {
				options = append(options, slack.MsgOptionAttachments(slack.Attachment{
					ImageURL: media.URL,
				}))
			}
		}
	}

	// 发送消息
	_, _, err := c.client.PostMessage(msg.ChatID, options...)
	if err != nil {
		return fmt.Errorf("failed to send slack message: %w", err)
	}

	logger.Info("Slack message sent",
		zap.String("channel_id", msg.ChatID),
		zap.Int("content_length", len(msg.Content)),
	)

	return nil
}

// healthCheck 健康检查
func (c *SlackChannel) healthCheck(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Slack health check stopped by context")
			return
		case <-c.WaitForStop():
			logger.Info("Slack health check stopped")
			return
		case <-ticker.C:
			if c.client == nil {
				logger.Warn("Slack client is not initialized")
				continue
			}

			// 尝试进行认证测试
			if _, err := c.client.AuthTest(); err != nil {
				logger.Error("Slack health check failed", zap.Error(err))
			}
		}
	}
}

// Stop 停止 Slack 通道
func (c *SlackChannel) Stop() error {
	return c.BaseChannelImpl.Stop()
}

// SendStream sends streaming messages (posts initial message for updates)
func (c *SlackChannel) SendStream(chatID string, stream <-chan *bus.StreamMessage) error {
	if !c.IsRunning() {
		return fmt.Errorf("slack channel is not running")
	}

	if c.client == nil {
		return fmt.Errorf("slack client is not initialized")
	}

	var timestamp string
	var content strings.Builder

	for msg := range stream {
		if msg.Error != "" {
			return fmt.Errorf("stream error: %s", msg.Error)
		}

		if !msg.IsThinking && !msg.IsFinal {
			content.WriteString(msg.Content)
		}

		if timestamp == "" && content.Len() > 0 {
			// Send initial message
			options := []slack.MsgOption{
				slack.MsgOptionText(content.String(), false),
			}

			_, ts, err := c.client.PostMessage(chatID, options...)
			if err != nil {
				return fmt.Errorf("failed to send initial slack message: %w", err)
			}
			timestamp = ts
		} else if timestamp != "" && content.Len() > 0 {
			// Update the message
			options := []slack.MsgOption{
				slack.MsgOptionText(content.String(), false),
			}

			if _, _, _, err := c.client.UpdateMessage(chatID, timestamp, options...); err != nil {
				return fmt.Errorf("failed to update slack message: %w", err)
			}
		}

		if msg.IsComplete {
			logger.Info("Slack streaming completed",
				zap.String("channel_id", chatID),
				zap.String("message_timestamp", timestamp),
				zap.Int("content_length", content.Len()),
			)
			return nil
		}
	}

	return nil
}
