package channels

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/internal/logger"
	infoflow "github.com/smallnest/infoflow"
	"go.uber.org/zap"
)

// InfoflowChannel 百度如流通道
type InfoflowChannel struct {
	*BaseChannelImpl
	webhookURL  string
	token       string
	aesKey      string
	webhookPort int
	robot       *infoflow.Robot
	sender      *infoflow.Sender
}

// InfoflowConfig 如流通道配置
type InfoflowConfig struct {
	BaseChannelConfig
	WebhookURL  string `json:"webhook_url" mapstructure:"webhook_url"`
	Token       string `json:"token" mapstructure:"token"`
	AESKey      string `json:"aes_key" mapstructure:"aes_key"`
	WebhookPort int    `json:"webhook_port" mapstructure:"webhook_port"`
}

// NewInfoflowChannel 创建如流通道
func NewInfoflowChannel(accountID string, cfg InfoflowConfig, bus *bus.MessageBus) (*InfoflowChannel, error) {
	if cfg.WebhookURL == "" || cfg.Token == "" {
		return nil, fmt.Errorf("infoflow webhook_url and token are required")
	}

	port := cfg.WebhookPort
	if port == 0 {
		port = 8767
	}

	return &InfoflowChannel{
		BaseChannelImpl: NewBaseChannelImpl("infoflow", accountID, cfg.BaseChannelConfig, bus),
		webhookURL:      cfg.WebhookURL,
		token:           cfg.Token,
		aesKey:          cfg.AESKey,
		webhookPort:     port,
	}, nil
}

// Start 启动如流通道
func (c *InfoflowChannel) Start(ctx context.Context) error {
	if err := c.BaseChannelImpl.Start(ctx); err != nil {
		return err
	}

	logger.Info("Starting Infoflow channel",
		zap.String("account_id", c.AccountID()),
		zap.String("webhook_url", c.webhookURL),
		zap.Int("port", c.webhookPort))

	// 创建发送器
	c.sender = infoflow.NewSender(c.webhookURL)

	// 创建机器人用于接收消息
	c.robot = infoflow.NewRobot(
		c.webhookURL,
		fmt.Sprintf(":%d", c.webhookPort),
		c.token,
		c.aesKey,
	)

	// 注册命令处理器
	c.robot.SetUnknownHandler(c.handleDefault)
	c.robot.AddHandler("status", c.handleStatus)
	c.robot.AddHandler("help", c.handleHelp)

	// 启动机器人接收服务
	go c.runReceiver(ctx)

	return nil
}

// runReceiver 运行接收器
func (c *InfoflowChannel) runReceiver(ctx context.Context) {
	// 在单独的 goroutine 中启动机器人
	go func() {
		c.robot.Run()
	}()

	// 等待上下文取消
	<-ctx.Done()
	logger.Info("Infoflow channel stopped by context")
}

// handleDefault 处理默认消息（非命令消息）
func (c *InfoflowChannel) handleDefault(cmd, fromUserID string, body infoflow.Body, msg infoflow.HiMessage) error {
	// 检查权限
	if !c.IsAllowed(fromUserID) {
		logger.Debug("Message from non-allowed user, ignoring",
			zap.String("user_id", fromUserID))
		return nil
	}

	// 解析消息内容
	var content string
	if body.Content != "" {
		content = body.Content
	}

	// 获取群组ID
	chatID := fmt.Sprintf("%d", msg.GroupID)

	// 发布入站消息
	inboundMsg := &bus.InboundMessage{
		ID:        fmt.Sprintf("%d", msg.Message.Header.MessageID),
		Channel:   c.Name(),
		AccountID: c.AccountID(),
		SenderID:  fromUserID,
		ChatID:    chatID,
		Content:   content,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"group_id":   msg.GroupID,
			"message_id": msg.Message.Header.MessageID,
			"msg_type":   msg.Message.Header.MsgType,
		},
	}

	_ = c.PublishInbound(context.Background(), inboundMsg)

	// 返回 nil 表示成功处理
	return nil
}

// handleStatus 处理 status 命令
func (c *InfoflowChannel) handleStatus(cmd, fromUserID string, body infoflow.Body, msg infoflow.HiMessage) error {
	if !c.IsAllowed(fromUserID) {
		return c.sender.SendMsg2Group(msg.GroupID,
			infoflow.CreateText("你无权查看状态"),
		)
	}

	statusText := fmt.Sprintf("Infoflow Channel Status:\n运行状态: 在线\n账号ID: %s", c.AccountID())
	return c.sender.SendMsg2Group(msg.GroupID,
		infoflow.CreateText(statusText),
	)
}

// handleHelp 处理 help 命令
func (c *InfoflowChannel) handleHelp(cmd, fromUserID string, body infoflow.Body, msg infoflow.HiMessage) error {
	helpText := `Infoflow 机器人帮助:

可用命令:
  /help - 显示此帮助信息
  /status - 查看机器人状态

直接发送消息即可与 AI 助手进行对话。`
	return c.sender.SendMsg2Group(msg.GroupID,
		infoflow.CreateText(helpText),
	)
}

// Send 发送消息
func (c *InfoflowChannel) Send(msg *bus.OutboundMessage) error {
	if !c.IsRunning() {
		return fmt.Errorf("infoflow channel is not running")
	}

	if c.sender == nil {
		return fmt.Errorf("infoflow sender is not initialized")
	}

	// 解析群组ID
	groupID, err := strconv.ParseInt(msg.ChatID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid chat_id: %w", err)
	}

	// 发送文本消息
	// 如流支持 Markdown 格式，所以直接发送
	err = c.sender.SendMsg2Group(groupID, infoflow.CreateText(msg.Content))
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	logger.Info("Message sent via Infoflow",
		zap.String("account_id", c.AccountID()),
		zap.String("chat_id", msg.ChatID),
		zap.Int("content_length", len(msg.Content)))

	return nil
}

// SendStream 发送流式消息（如流不支持，使用普通发送）
func (c *InfoflowChannel) SendStream(chatID string, stream <-chan *bus.StreamMessage) error {
	// 收集所有流式消息
	var messages []string
	for streamMsg := range stream {
		messages = append(messages, streamMsg.Content)

		if streamMsg.IsComplete {
			break
		}
	}

	// 合并发送
	if len(messages) > 0 {
		fullContent := strings.Join(messages, "")
		msg := &bus.OutboundMessage{
			Channel: c.Name(),
			ChatID:  chatID,
			Content: fullContent,
		}
		return c.Send(msg)
	}

	return nil
}

// Stop 停止通道
func (c *InfoflowChannel) Stop() error {
	logger.Info("Stopping Infoflow channel",
		zap.String("account_id", c.AccountID()))

	// 如流的 robot 会自动关闭 HTTP 服务器
	return c.BaseChannelImpl.Stop()
}
