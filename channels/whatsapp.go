package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/smallnest/dogclaw/goclaw/bus"
	"github.com/smallnest/dogclaw/goclaw/internal/logger"
	"go.uber.org/zap"
)

// WhatsAppChannel WhatsApp 通道
type WhatsAppChannel struct {
	*BaseChannelImpl
	bridgeURL string
	client    *http.Client
	mu        sync.Mutex
}

// WhatsAppConfig WhatsApp 配置
type WhatsAppConfig struct {
	BaseChannelConfig
	BridgeURL string `mapstructure:"bridge_url" json:"bridge_url"`
}

// NewWhatsAppChannel 创建 WhatsApp 通道
func NewWhatsAppChannel(cfg WhatsAppConfig, bus *bus.MessageBus) (*WhatsAppChannel, error) {
	return &WhatsAppChannel{
		BaseChannelImpl: NewBaseChannelImpl("whatsapp", cfg.BaseChannelConfig, bus),
		bridgeURL:       cfg.BridgeURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// Start 启动 WhatsApp 通道
func (c *WhatsAppChannel) Start(ctx context.Context) error {
	if err := c.BaseChannelImpl.Start(ctx); err != nil {
		return err
	}

	if c.bridgeURL == "" {
		logger.Info("WhatsApp bridge URL not configured, channel disabled")
		return nil
	}

	logger.Info("Starting WhatsApp channel",
		zap.String("bridge_url", c.bridgeURL),
	)

	// 启动消息轮询
	go c.pollMessages(ctx)

	return nil
}

// pollMessages 轮询消息
func (c *WhatsAppChannel) pollMessages(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("WhatsApp channel stopped by context")
			return
		case <-c.WaitForStop():
			logger.Info("WhatsApp channel stopped")
			return
		case <-ticker.C:
			if err := c.fetchMessages(ctx); err != nil {
				logger.Error("Failed to fetch WhatsApp messages", zap.Error(err))
			}
		}
	}
}

// fetchMessages 获取消息
func (c *WhatsAppChannel) fetchMessages(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.bridgeURL+"/messages", nil)
	if err != nil {
		return err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var messages []WhatsAppMessage
	if err := json.Unmarshal(body, &messages); err != nil {
		return err
	}

	for _, msg := range messages {
		if err := c.handleMessage(ctx, &msg); err != nil {
			logger.Error("Failed to handle WhatsApp message",
				zap.Error(err),
				zap.String("message_id", msg.ID),
			)
		}
	}

	return nil
}

// handleMessage 处理消息
func (c *WhatsAppChannel) handleMessage(ctx context.Context, msg *WhatsAppMessage) error {
	// 检查权限
	if !c.IsAllowed(msg.From) {
		return nil
	}

	// 构建入站消息
	inboundMsg := &bus.InboundMessage{
		ID:        msg.ID,
		Channel:   c.Name(),
		SenderID:  msg.From,
		ChatID:    msg.ChatID,
		Content:   msg.Text,
		Metadata: map[string]interface{}{
			"message_type": msg.Type,
			"timestamp":    msg.Timestamp,
		},
		Timestamp: time.Now(),
	}

	return c.PublishInbound(ctx, inboundMsg)
}

// Send 发送消息
func (c *WhatsAppChannel) Send(msg *bus.OutboundMessage) error {
	if !c.IsRunning() {
		return fmt.Errorf("whatsapp channel is not running")
	}

	// 构建请求数据
	data := map[string]interface{}{
		"chat_id": msg.ChatID,
		"text":    msg.Content,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// 发送请求
	req, err := http.NewRequest("POST", c.bridgeURL+"/send", strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	// 实际发送请求
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	logger.Info("WhatsApp message sent",
		zap.String("chat_id", msg.ChatID),
		zap.Int("content_length", len(msg.Content)),
	)

	return nil
}

// WhatsAppMessage WhatsApp 消息
type WhatsAppMessage struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	Type      string `json:"type"`
	Timestamp int64  `json:"timestamp"`
}
