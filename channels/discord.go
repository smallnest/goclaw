package channels

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/internal/logger"
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
		BaseChannelImpl: NewBaseChannelImpl("discord", "default", cfg.BaseChannelConfig, bus),
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

// SendStream sends streaming messages (edits original message progressively)
func (c *DiscordChannel) SendStream(chatID string, stream <-chan *bus.StreamMessage) error {
	if !c.IsRunning() {
		return fmt.Errorf("discord channel is not running")
	}

	if c.session == nil {
		return fmt.Errorf("discord session is not initialized")
	}

	var messageID string
	var content strings.Builder

	for msg := range stream {
		if msg.Error != "" {
			return fmt.Errorf("stream error: %s", msg.Error)
		}

		if !msg.IsThinking && !msg.IsFinal {
			content.WriteString(msg.Content)
		}

		if messageID == "" && content.Len() > 0 {
			// Send initial message
			discordMsg := &discordgo.MessageSend{
				Content: content.String(),
			}

			sentMsg, err := c.session.ChannelMessageSendComplex(chatID, discordMsg)
			if err != nil {
				return fmt.Errorf("failed to send initial discord message: %w", err)
			}
			messageID = sentMsg.ID
		} else if messageID != "" && content.Len() > 0 {
			// Edit the message
			contentStr := content.String()
			edit := &discordgo.MessageEdit{
				ID:      messageID,
				Channel: chatID,
				Content: &contentStr,
			}

			if _, err := c.session.ChannelMessageEditComplex(edit); err != nil {
				return fmt.Errorf("failed to update discord message: %w", err)
			}
		}

		if msg.IsComplete {
			logger.Info("Discord streaming completed",
				zap.String("channel_id", chatID),
				zap.String("message_id", messageID),
				zap.Int("content_length", content.Len()),
			)
			return nil
		}
	}

	return nil
}

// ============================================
// Discord Reactions Support
// ============================================

// DiscordReaction represents a Discord reaction
type DiscordReaction struct {
	EmojiID   string `json:"emoji_id"`
	EmojiName string `json:"emoji_name"`
	Animated  bool   `json:"animated"`
	Count     int    `json:"count"`
}

// DiscordReactionUser represents a user who reacted
type DiscordReactionUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Tag      string `json:"tag"`
}

// AddReaction adds a reaction to a message
func (c *DiscordChannel) AddReaction(channelID, messageID, emoji string) error {
	if !c.IsRunning() {
		return fmt.Errorf("discord channel is not running")
	}
	if c.session == nil {
		return fmt.Errorf("discord session is not initialized")
	}

	// Normalize emoji format
	emojiAPI := c.normalizeEmoji(emoji)

	err := c.session.MessageReactionAdd(channelID, messageID, emojiAPI)
	if err != nil {
		return fmt.Errorf("failed to add reaction: %w", err)
	}

	logger.Debug("Discord reaction added",
		zap.String("channel_id", channelID),
		zap.String("message_id", messageID),
		zap.String("emoji", emoji),
	)

	return nil
}

// RemoveReaction removes a reaction from a message
func (c *DiscordChannel) RemoveReaction(channelID, messageID, emoji string) error {
	if !c.IsRunning() {
		return fmt.Errorf("discord channel is not running")
	}
	if c.session == nil {
		return fmt.Errorf("discord session is not initialized")
	}

	emojiAPI := c.normalizeEmoji(emoji)

	err := c.session.MessageReactionRemove(channelID, messageID, emojiAPI, "@me")
	if err != nil {
		return fmt.Errorf("failed to remove reaction: %w", err)
	}

	logger.Debug("Discord reaction removed",
		zap.String("channel_id", channelID),
		zap.String("message_id", messageID),
		zap.String("emoji", emoji),
	)

	return nil
}

// RemoveAllReactions removes all reactions from a message (only bot's own reactions)
func (c *DiscordChannel) RemoveOwnReactions(channelID, messageID string) ([]string, error) {
	if !c.IsRunning() {
		return nil, fmt.Errorf("discord channel is not running")
	}
	if c.session == nil {
		return nil, fmt.Errorf("discord session is not initialized")
	}

	// Get message to see current reactions
	msg, err := c.session.ChannelMessage(channelID, messageID)
	if err != nil {
		return nil, fmt.Errorf("failed to get message: %w", err)
	}

	var removed []string

	// Remove each of the bot's reactions
	for _, reaction := range msg.Reactions {
		// Build emoji identifier for API call
		emojiAPI := c.buildEmojiAPI(reaction.Emoji)

		err := c.session.MessageReactionRemove(channelID, messageID, emojiAPI, "@me")
		if err != nil {
			logger.Warn("Failed to remove reaction",
				zap.String("emoji", reaction.Emoji.Name),
				zap.Error(err),
			)
			continue
		}

		removed = append(removed, c.formatEmoji(reaction.Emoji))
	}

	logger.Debug("Discord reactions removed",
		zap.String("channel_id", channelID),
		zap.String("message_id", messageID),
		zap.Int("count", len(removed)),
	)

	return removed, nil
}

// GetReactions fetches all reactions for a message with user details
func (c *DiscordChannel) GetReactions(channelID, messageID string, limit int) (map[string]*DiscordReactionDetail, error) {
	if !c.IsRunning() {
		return nil, fmt.Errorf("discord channel is not running")
	}
	if c.session == nil {
		return nil, fmt.Errorf("discord session is not initialized")
	}

	if limit <= 0 || limit > 100 {
		limit = 100
	}

	// Get message to see current reactions
	msg, err := c.session.ChannelMessage(channelID, messageID)
	if err != nil {
		return nil, fmt.Errorf("failed to get message: %w", err)
	}

	result := make(map[string]*DiscordReactionDetail)

	for _, reaction := range msg.Reactions {
		emojiKey := c.formatEmoji(reaction.Emoji)

		// For each reaction, we'll store the summary info
		// Note: DiscordGo doesn't have a direct API to get reaction users with our signature
		// We'll store what we can get from the reaction itself

		detail := &DiscordReactionDetail{
			Emoji: DiscordReaction{
				EmojiID:   reaction.Emoji.ID,
				EmojiName: reaction.Emoji.Name,
				Animated:  reaction.Emoji.Animated,
				Count:     reaction.Count,
			},
			Users: []DiscordReactionUser{}, // Would need separate API call to populate
		}

		result[emojiKey] = detail
	}

	return result, nil
}

// DiscordReactionDetail contains reaction details with users
type DiscordReactionDetail struct {
	Emoji DiscordReaction
	Users []DiscordReactionUser `json:"users"`
}

// normalizeEmoji converts emoji to API format
func (c *DiscordChannel) normalizeEmoji(emoji string) string {
	// Custom emoji format: <name:id>
	if strings.HasPrefix(emoji, "<") && strings.HasSuffix(emoji, ">") {
		return emoji
	}

	// Unicode emoji - use as-is
	return emoji
}

// buildEmojiAPI builds API format emoji from Discord Emoji struct
func (c *DiscordChannel) buildEmojiAPI(emoji *discordgo.Emoji) string {
	if emoji.ID != "" {
		// Custom emoji: name:id (emoji.ID is string in discordgo)
		return emoji.Name + ":" + emoji.ID
	}
	// Unicode emoji
	return emoji.Name
}

// formatEmoji formats emoji for display
func (c *DiscordChannel) formatEmoji(emoji *discordgo.Emoji) string {
	if emoji.ID != "" {
		// Custom emoji format
		if emoji.Animated {
			return fmt.Sprintf("<a:%s:%s>", emoji.Name, emoji.ID)
		}
		return fmt.Sprintf("<:%s:%s>", emoji.Name, emoji.ID)
	}
	// Unicode emoji
	return emoji.Name
}
