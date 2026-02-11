package channels

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	telegrambot "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/smallnest/dogclaw/goclaw/bus"
	"github.com/smallnest/dogclaw/goclaw/internal/logger"
	"go.uber.org/zap"
)

// TelegramChannel Telegram 通道
type TelegramChannel struct {
	*BaseChannelImpl
	bot   *telegrambot.BotAPI
	token string
}

// TelegramConfig Telegram 配置
type TelegramConfig struct {
	BaseChannelConfig
	Token string `mapstructure:"token" json:"token"`
}

// NewTelegramChannel 创建 Telegram 通道
func NewTelegramChannel(cfg TelegramConfig, bus *bus.MessageBus) (*TelegramChannel, error) {
	if cfg.Token == "" {
		return nil, fmt.Errorf("telegram token is required")
	}

	bot, err := telegrambot.NewBotAPI(cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	return &TelegramChannel{
		BaseChannelImpl: NewBaseChannelImpl("telegram", cfg.BaseChannelConfig, bus),
		bot:             bot,
		token:           cfg.Token,
	}, nil
}

// Start 启动 Telegram 通道
func (c *TelegramChannel) Start(ctx context.Context) error {
	if err := c.BaseChannelImpl.Start(ctx); err != nil {
		return err
	}

	logger.Info("Starting Telegram channel")

	// 获取 bot 信息
	bot, err := c.bot.GetMe()
	if err != nil {
		return fmt.Errorf("failed to get bot info: %w", err)
	}

	logger.Info("Telegram bot started",
		zap.String("bot_name", bot.UserName),
		zap.String("bot_id", strconv.FormatInt(bot.ID, 10)),
	)

	// 启动消息处理
	go c.receiveUpdates(ctx)

	return nil
}

// receiveUpdates 接收更新
func (c *TelegramChannel) receiveUpdates(ctx context.Context) {
	u := telegrambot.NewUpdate(0)
	u.Timeout = 60

	updates := c.bot.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			logger.Info("Telegram channel stopped by context")
			return
		case <-c.WaitForStop():
			logger.Info("Telegram channel stopped")
			return
		case update := <-updates:
			if err := c.handleUpdate(ctx, &update); err != nil {
				logger.Error("Failed to handle update",
					zap.Error(err),
				)
			}
		}
	}
}

// handleUpdate 处理更新
func (c *TelegramChannel) handleUpdate(ctx context.Context, update *telegrambot.Update) error {
	if update.Message == nil {
		return nil
	}

	message := update.Message

	// 检查权限
	senderID := strconv.FormatInt(message.From.ID, 10)
	if !c.IsAllowed(senderID) {
		logger.Warn("Telegram message from unauthorized sender",
			zap.Int64("sender_id", message.From.ID),
			zap.String("sender_name", message.From.UserName),
		)
		return nil
	}

	// 提取文本内容
	content := ""
	if message.Text != "" {
		content = message.Text
	} else if message.Caption != "" {
		content = message.Caption
	}

	// 处理命令
	if strings.HasPrefix(content, "/") {
		return c.handleCommand(ctx, message, content)
	}

	// 构建入站消息
	msg := &bus.InboundMessage{
		Channel:  c.Name(),
		SenderID: senderID,
		ChatID:   strconv.FormatInt(message.Chat.ID, 10),
		Content:  content,
		Media:    c.extractMedia(message),
		Metadata: map[string]interface{}{
			"message_id": message.MessageID,
			"from_user":  message.From.UserName,
			"from_name":  message.From.FirstName,
			"chat_type":  message.Chat.Type,
			"reply_to":   message.ReplyToMessage,
		},
		Timestamp: time.Now(),
	}

	return c.PublishInbound(ctx, msg)
}

// handleCommand 处理命令
func (c *TelegramChannel) handleCommand(ctx context.Context, message *telegrambot.Message, command string) error {
	chatID := message.Chat.ID

	switch command {
	case "/start":
		msg := telegrambot.NewMessage(chatID, "👋 欢迎使用 goclaw!\n\n我可以帮助你完成各种任务。发送 /help 查看可用命令。")
		if _, err := c.bot.Send(msg); err != nil {
			return err
		}
	case "/help":
		helpText := `🐾 goclaw 命令列表：

/start - 开始使用
/help - 显示帮助

你可以直接与我对话，我会尽力帮助你！`
		msg := telegrambot.NewMessage(chatID, helpText)
		if _, err := c.bot.Send(msg); err != nil {
			return err
		}
	case "/status":
		statusText := fmt.Sprintf("✅ goclaw 运行中\n\n通道状态: %s", map[bool]string{true: "🟢 在线", false: "🔴 离线"}[c.IsRunning()])
		msg := telegrambot.NewMessage(chatID, statusText)
		if _, err := c.bot.Send(msg); err != nil {
			return err
		}
	}

	return nil
}

// extractMedia 提取媒体
func (c *TelegramChannel) extractMedia(message *telegrambot.Message) []bus.Media {
	var media []bus.Media

	if len(message.Photo) > 0 {
		// 获取最大尺寸的照片
		_ = message.Photo[len(message.Photo)-1]
		media = append(media, bus.Media{
			Type:     "image",
			MimeType: "image/jpeg",
		})
	}

	if message.Document != nil {
		media = append(media, bus.Media{
			Type:     "document",
			MimeType: message.Document.MimeType,
		})
	}

	if message.Voice != nil {
		media = append(media, bus.Media{
			Type:     "audio",
			MimeType: message.Voice.MimeType,
		})
	}

	if message.Video != nil {
		media = append(media, bus.Media{
			Type:     "video",
			MimeType: message.Video.MimeType,
		})
	}

	return media
}

// Send 发送消息
func (c *TelegramChannel) Send(msg *bus.OutboundMessage) error {
	if !c.IsRunning() {
		return fmt.Errorf("telegram channel is not running")
	}

	// 解析 ChatID
	chatID, err := strconv.ParseInt(msg.ChatID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid chat id: %w", err)
	}

	// 创建消息
	tgMsg := telegrambot.NewMessage(chatID, msg.Content)

	// 解析回复
	if msg.ReplyTo != "" {
		replyToID, err := strconv.Atoi(msg.ReplyTo)
		if err == nil {
			tgMsg.ReplyToMessageID = replyToID
		} else {
			logger.Warn("Invalid reply_to id for telegram", zap.String("id", msg.ReplyTo), zap.Error(err))
		}
	}

	// 发送消息
	_, err = c.bot.Send(tgMsg)
	if err != nil {
		return fmt.Errorf("failed to send telegram message: %w", err)
	}

	logger.Info("Telegram message sent",
		zap.Int64("chat_id", chatID),
		zap.Int("content_length", len(msg.Content)),
	)

	return nil
}
