package channels

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/smallnest/dogclaw/goclaw/bus"
	"github.com/smallnest/dogclaw/goclaw/internal/logger"
	"go.uber.org/zap"
)

// DiscordChannel Discord 通道
type DiscordChannel struct {
	*BaseChannelImpl
	session *discordgo.Session
	token   string
}

// DiscordConfig Discord 配置
type DiscordConfig struct {
	BaseChannelConfig
	Token string `mapstructure:"token" json:"token"`
}

// NewDiscordChannel 创建 Discord 通道
func NewDiscordChannel(cfg DiscordConfig, bus *bus.MessageBus) (*DiscordChannel, error) {
	if cfg.Token == "" {
		return nil, fmt.Errorf("discord token is required")
	}

	return &DiscordChannel{
		BaseChannelImpl: NewBaseChannelImpl("discord", cfg.BaseChannelConfig, bus),
		token:           cfg.Token,
	}, nil
}

// Start 启动 Discord 通道
func (c *DiscordChannel) Start(ctx context.Context) error {
	if err := c.BaseChannelImpl.Start(ctx); err != nil {
		return err
	}

	logger.Info("Starting Discord channel")

	// 创建 Discord 会话
	session, err := discordgo.New("Bot " + c.token)
	if err != nil {
		return fmt.Errorf("failed to create discord session: %w", err)
	}

	c.session = session

	// 注册消息处理
	session.AddHandler(c.handleMessage)

	// 连接到 Discord
	if err := session.Open(); err != nil {
		return fmt.Errorf("failed to open discord connection: %w", err)
	}

	// 获取 bot 信息
	botUser, err := session.User("@me")
	if err != nil {
		session.Close()
		return fmt.Errorf("failed to get bot info: %w", err)
	}

	logger.Info("Discord bot started",
		zap.String("bot_name", botUser.Username),
		zap.String("bot_id", botUser.ID),
	)

	// 启动健康检查
	go c.healthCheck(ctx)

	return nil
}

// healthCheck 健康检查
func (c *DiscordChannel) healthCheck(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Discord health check stopped by context")
			return
		case <-c.WaitForStop():
			logger.Info("Discord health check stopped")
			return
		case <-ticker.C:
			if c.session == nil || c.session.State == nil {
				logger.Warn("Discord session is not healthy")
				continue
			}

			// 尝试获取用户信息来验证连接
			if _, err := c.session.User("@me"); err != nil {
				logger.Error("Discord health check failed", zap.Error(err))
			}
		}
	}
}

// handleMessage 处理 Discord 消息
func (c *DiscordChannel) handleMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	// 忽略机器人自己的消息
	if m.Author.Bot {
		return
	}

	// 检查权限
	senderID := m.Author.ID
	if !c.IsAllowed(senderID) {
		logger.Warn("Discord message from unauthorized sender",
			zap.String("sender_id", senderID),
			zap.String("sender_name", m.Author.Username),
		)
		return
	}

	// 处理命令
	if strings.HasPrefix(m.Content, "/") {
		c.handleCommand(context.Background(), m)
		return
	}

	// 提取内容
	content := m.Content
	var media []bus.Media

	// 处理附件
	if len(m.Attachments) > 0 {
		for _, att := range m.Attachments {
			mediaType := "document"
			if strings.HasPrefix(att.ContentType, "image/") {
				mediaType = "image"
			} else if strings.HasPrefix(att.ContentType, "video/") {
				mediaType = "video"
			} else if strings.HasPrefix(att.ContentType, "audio/") {
				mediaType = "audio"
			}

			media = append(media, bus.Media{
				Type:     mediaType,
				URL:      att.URL,
				MimeType: att.ContentType,
			})
		}
	}

	// 构建入站消息
	msg := &bus.InboundMessage{
		Channel:  c.Name(),
		SenderID: senderID,
		ChatID:   m.ChannelID,
		Content:  content,
		Media:    media,
		Metadata: map[string]interface{}{
			"message_id":       m.ID,
			"guild_id":         m.GuildID,
			"author":           m.Author.Username,
			"discriminator":    m.Author.Discriminator,
			"mention_everyone": m.MentionEveryone,
		},
		Timestamp: time.Now(),
	}

	if err := c.PublishInbound(context.Background(), msg); err != nil {
		logger.Error("Failed to publish Discord message", zap.Error(err))
	}
}

// handleCommand 处理命令
func (c *DiscordChannel) handleCommand(ctx context.Context, m *discordgo.MessageCreate) {
	command := m.Content

	switch command {
	case "/start":
		_, err := c.session.ChannelMessageSend(m.ChannelID, "👋 Welcome to goclaw!\n\nI can help you with various tasks. Send /help to see available commands.")
		if err != nil {
			logger.Error("Failed to send Discord message", zap.Error(err))
		}
	case "/help":
		helpText := `🐾 goclaw commands:

/start - Get started
/help - Show this help message

You can chat with me directly and I'll do my best to help!`
		_, err := c.session.ChannelMessageSend(m.ChannelID, helpText)
		if err != nil {
			logger.Error("Failed to send Discord message", zap.Error(err))
		}
	case "/status":
		statusText := fmt.Sprintf("✅ goclaw is running\n\nChannel status: %s", map[bool]string{true: "🟢 Online", false: "🔴 Offline"}[c.IsRunning()])
		_, err := c.session.ChannelMessageSend(m.ChannelID, statusText)
		if err != nil {
			logger.Error("Failed to send Discord message", zap.Error(err))
		}
	}
}

// Send 发送消息
func (c *DiscordChannel) Send(msg *bus.OutboundMessage) error {
	if !c.IsRunning() {
		return fmt.Errorf("discord channel is not running")
	}

	if c.session == nil {
		return fmt.Errorf("discord session is not initialized")
	}

	// 创建消息发送
	discordMsg := &discordgo.MessageSend{
		Content: msg.Content,
	}

	// 处理回复
	if msg.ReplyTo != "" {
		discordMsg.Reference = &discordgo.MessageReference{
			MessageID: msg.ReplyTo,
		}
	}

	// 处理媒体
	if len(msg.Media) > 0 {
		for _, media := range msg.Media {
			if media.Type == "image" && media.URL != "" {
				discordMsg.Files = append(discordMsg.Files, &discordgo.File{
					Name: "image",
				})
			}
		}
	}

	// 发送消息
	_, err := c.session.ChannelMessageSendComplex(msg.ChatID, discordMsg)
	if err != nil {
		return fmt.Errorf("failed to send discord message: %w", err)
	}

	logger.Info("Discord message sent",
		zap.String("channel_id", msg.ChatID),
		zap.Int("content_length", len(msg.Content)),
	)

	return nil
}

// Stop 停止 Discord 通道
func (c *DiscordChannel) Stop() error {
	if err := c.BaseChannelImpl.Stop(); err != nil {
		return err
	}

	if c.session != nil {
		if err := c.session.Close(); err != nil {
			logger.Error("Failed to close Discord session", zap.Error(err))
		}
	}

	return nil
}
