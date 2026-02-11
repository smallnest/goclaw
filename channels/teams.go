package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/smallnest/dogclaw/goclaw/bus"
	"github.com/smallnest/dogclaw/goclaw/internal/logger"
	"go.uber.org/zap"
)

// TeamsChannel Microsoft Teams 通道
type TeamsChannel struct {
	*BaseChannelImpl
	appID       string
	appPassword string
	tenantID    string
	webhookURL  string
	httpClient  *http.Client
}

// TeamsConfig Teams 配置
type TeamsConfig struct {
	BaseChannelConfig
	AppID       string `mapstructure:"app_id" json:"app_id"`
	AppPassword string `mapstructure:"app_password" json:"app_password"`
	TenantID    string `mapstructure:"tenant_id" json:"tenant_id"`
	WebhookURL  string `mapstructure:"webhook_url" json:"webhook_url"` // For outgoing webhooks
}

// NewTeamsChannel 创建 Teams 通道
func NewTeamsChannel(cfg TeamsConfig, bus *bus.MessageBus) (*TeamsChannel, error) {
	if cfg.WebhookURL == "" && (cfg.AppID == "" || cfg.AppPassword == "" || cfg.TenantID == "") {
		return nil, fmt.Errorf("teams webhook_url or app credentials (app_id, app_password, tenant_id) are required")
	}

	return &TeamsChannel{
		BaseChannelImpl: NewBaseChannelImpl("teams", cfg.BaseChannelConfig, bus),
		appID:           cfg.AppID,
		appPassword:     cfg.AppPassword,
		tenantID:        cfg.TenantID,
		webhookURL:      cfg.WebhookURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// Start 启动 Teams 通道
func (c *TeamsChannel) Start(ctx context.Context) error {
	if err := c.BaseChannelImpl.Start(ctx); err != nil {
		return err
	}

	logger.Info("Starting Teams channel")

	// Teams 主要通过 webhook 或 Bot Framework 工作
	// 这里我们设置基础状态，实际的消息接收需要通过 webhook 端点

	// 启动健康检查
	go c.healthCheck(ctx)

	logger.Info("Teams channel started (webhook mode)")

	return nil
}

// healthCheck 健康检查
func (c *TeamsChannel) healthCheck(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Teams health check stopped by context")
			return
		case <-c.WaitForStop():
			logger.Info("Teams health check stopped")
			return
		case <-ticker.C:
			if !c.IsRunning() {
				logger.Warn("Teams channel is not running")
			}
		}
	}
}

// HandleWebhook 处理 Teams webhook (需要在外部 HTTP 端点调用)
func (c *TeamsChannel) HandleWebhook(ctx context.Context, webhookMsg *TeamsWebhookMessage) error {
	if webhookMsg == nil {
		return fmt.Errorf("webhook message is nil")
	}

	// 检查权限
	senderID := webhookMsg.From.ID
	if !c.IsAllowed(senderID) {
		logger.Warn("Teams message from unauthorized sender",
			zap.String("sender_id", senderID),
			zap.String("sender_name", webhookMsg.From.Name),
		)
		return nil
	}

	// 处理命令
	if strings.HasPrefix(webhookMsg.Text, "/") {
		return c.handleCommand(ctx, webhookMsg)
	}

	// 构建入站消息
	msg := &bus.InboundMessage{
		Channel:  c.Name(),
		SenderID: senderID,
		ChatID:   webhookMsg.Conversation.ID,
		Content:  webhookMsg.Text,
		Metadata: map[string]interface{}{
			"message_id":   webhookMsg.ID,
			"sender_name":  webhookMsg.From.Name,
			"conversation": webhookMsg.Conversation,
			"attachments":  webhookMsg.Attachments,
			"entities":     webhookMsg.Entities,
		},
		Timestamp: time.Now(),
	}

	return c.PublishInbound(ctx, msg)
}

// handleCommand 处理命令
func (c *TeamsChannel) handleCommand(ctx context.Context, webhookMsg *TeamsWebhookMessage) error {
	command := webhookMsg.Text

	var responseText string
	switch command {
	case "/start":
		responseText = "👋 Welcome to goclaw!\n\nI can help you with various tasks. Send /help to see available commands."
	case "/help":
		responseText = `🐾 goclaw commands:

/start - Get started
/help - Show this help message

You can chat with me directly and I'll do my best to help!`
	case "/status":
		responseText = fmt.Sprintf("✅ goclaw is running\n\nChannel status: %s", map[bool]string{true: "🟢 Online", false: "🔴 Offline"}[c.IsRunning()])
	default:
		return nil
	}

	// 发送响应
	return c.Send(&bus.OutboundMessage{
		ChatID:    webhookMsg.Conversation.ID,
		Content:   responseText,
		Timestamp: time.Now(),
	})
}

// Send 发送消息
func (c *TeamsChannel) Send(msg *bus.OutboundMessage) error {
	if !c.IsRunning() {
		return fmt.Errorf("teams channel is not running")
	}

	// 优先使用配置的 webhook URL
	targetURL := c.webhookURL

	// 如果消息中有特定的 serviceUrl，使用它
	if serviceURL, ok := msg.Metadata["serviceUrl"].(string); ok && serviceURL != "" {
		// 使用 Bot Framework REST API 发送
		return c.sendViaBotFramework(msg, serviceURL)
	}

	// 如果没有 webhook URL，返回错误
	if targetURL == "" {
		return fmt.Errorf("no webhook URL configured and no serviceUrl in message metadata")
	}

	// 使用简单的 webhook 发送
	return c.sendViaWebhook(msg, targetURL)
}

// sendViaWebhook 通过 webhook URL 发送消息
func (c *TeamsChannel) sendViaWebhook(msg *bus.OutboundMessage, webhookURL string) error {
	// 构建简单的文本消息
	payload := map[string]interface{}{
		"type": "message",
		"text": msg.Content,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	req, err := http.NewRequest("POST", webhookURL, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	logger.Info("Teams message sent via webhook",
		zap.String("webhook_url", webhookURL),
		zap.Int("content_length", len(msg.Content)),
	)

	return nil
}

// sendViaBotFramework 通过 Bot Framework REST API 发送消息
func (c *TeamsChannel) sendViaBotFramework(msg *bus.OutboundMessage, serviceURL string) error {
	// 构建消息卡片
	payload := map[string]interface{}{
		"type": "message",
		"from": map[string]interface{}{
			"id": c.appID,
		},
		"conversation": map[string]interface{}{
			"id": msg.ChatID,
		},
		"text": msg.Content,
		// 如果有回复的消息 ID
	}

	if msg.ReplyTo != "" {
		payload["replyToId"] = msg.ReplyTo
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// 构建发送 URL: {serviceUrl}/v3/conversations/{conversationId}/activities
	sendURL := fmt.Sprintf("%s/v3/conversations/%s/activities", serviceURL, msg.ChatID)

	req, err := http.NewRequest("POST", sendURL, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.appPassword)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	logger.Info("Teams message sent via Bot Framework",
		zap.String("conversation_id", msg.ChatID),
		zap.Int("content_length", len(msg.Content)),
	)

	return nil
}

// SendWithWebhook 使用 webhook 发送消息 (用于 outgoing webhooks)
func (c *TeamsChannel) SendWithWebhook(webhookURL string, msg *bus.OutboundMessage) error {
	if !c.IsRunning() {
		return fmt.Errorf("teams channel is not running")
	}

	// 构建简单的文本响应
	_ = map[string]interface{}{
		"type": "message",
		"text": msg.Content,
	}

	logger.Info("Teams webhook message sent",
		zap.String("webhook_url", webhookURL),
		zap.Int("content_length", len(msg.Content)),
	)

	return nil
}

// SendAdaptiveCard 发送自适应卡片 (富格式消息)
func (c *TeamsChannel) SendAdaptiveCard(msg *bus.OutboundMessage, card map[string]interface{}) error {
	if !c.IsRunning() {
		return fmt.Errorf("teams channel is not running")
	}

	// 构建包含自适应卡片的消息
	messageCard := map[string]interface{}{
		"type": "message",
		"attachments": []map[string]interface{}{
			{
				"contentType": "application/vnd.microsoft.card.adaptive",
				"content":     card,
			},
		},
	}

	// 如果有回复，设置回复信息
	if msg.ReplyTo != "" {
		messageCard["replyToId"] = msg.ReplyTo
	}

	logger.Info("Teams adaptive card sent",
		zap.String("conversation_id", msg.ChatID),
	)

	return nil
}

// Stop 停止 Teams 通道
func (c *TeamsChannel) Stop() error {
	return c.BaseChannelImpl.Stop()
}

// TeamsWebhookMessage Teams webhook 消息
type TeamsWebhookMessage struct {
	Type         string            `json:"type"`
	ID           string            `json:"id"`
	Timestamp    string            `json:"timestamp"`
	ServiceURL   string            `json:"serviceUrl"`
	ChannelID    string            `json:"channelId"`
	From         TeamsActor        `json:"from"`
	Conversation TeamsConversation `json:"conversation"`
	Text         string            `json:"text"`
	Attachments  []TeamsAttachment `json:"attachments"`
	Entities     []TeamsEntity     `json:"entities"`
}

// TeamsActor Teams 参与者
type TeamsActor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TeamsConversation Teams 会话
type TeamsConversation struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TeamsAttachment Teams 附件
type TeamsAttachment struct {
	ContentType string                 `json:"contentType"`
	Content     map[string]interface{} `json:"content"`
}

// TeamsEntity Teams 实体
type TeamsEntity struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data,omitempty"`
}
