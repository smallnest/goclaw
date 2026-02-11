package channels

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/smallnest/dogclaw/goclaw/bus"
	"github.com/smallnest/dogclaw/goclaw/config"
	"github.com/smallnest/dogclaw/goclaw/internal/logger"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"go.uber.org/zap"
)

// FeishuChannel 飞书通道
type FeishuChannel struct {
	*BaseChannelImpl
	appID             string
	appSecret         string
	encryptKey        string
	verificationToken string
	webhookPort       int
	client            *lark.Client
}

// NewFeishuChannel 创建飞书通道
func NewFeishuChannel(cfg config.FeishuChannelConfig, bus *bus.MessageBus) (*FeishuChannel, error) {
	if cfg.AppID == "" || cfg.AppSecret == "" {
		return nil, fmt.Errorf("feishu app_id and app_secret are required")
	}

	client := lark.NewClient(cfg.AppID, cfg.AppSecret)

	baseCfg := BaseChannelConfig{
		Enabled:    cfg.Enabled,
		AllowedIDs: cfg.AllowedIDs,
	}

	port := cfg.WebhookPort
	if port == 0 {
		port = 8765
	}

	return &FeishuChannel{
		BaseChannelImpl:   NewBaseChannelImpl("feishu", baseCfg, bus),
		appID:             cfg.AppID,
		appSecret:         cfg.AppSecret,
		encryptKey:        cfg.EncryptKey,
		verificationToken: cfg.VerificationToken,
		webhookPort:       port,
		client:            client,
	}, nil
}

// Start 启动飞书通道
func (c *FeishuChannel) Start(ctx context.Context) error {
	if err := c.BaseChannelImpl.Start(ctx); err != nil {
		return err
	}

	logger.Info("Starting Feishu channel", zap.String("app_id", c.appID))

	// 启动 HTTP 服务器监听事件
	go c.startEventServer(ctx)

	return nil
}

func (c *FeishuChannel) startEventServer(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/feishu/webhook", c.handleWebhook)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", c.webhookPort),
		Handler: mux,
	}

	go func() {
		logger.Info("Feishu webhook server started", zap.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Feishu webhook server error", zap.Error(err))
		}
	}()

	<-ctx.Done()
	_ = server.Shutdown(ctx)
}

func (c *FeishuChannel) handleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("Failed to read body", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// 验证签名
	if !c.verifySignature(r, body) {
		logger.Warn("Invalid signature")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var event map[string]interface{}
	if err := json.Unmarshal(body, &event); err != nil {
		logger.Error("Failed to unmarshal JSON", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// URL 验证
	if challenge, ok := event["challenge"].(string); ok {
		// 检查 token
		if token, ok := event["token"].(string); ok && token != c.verificationToken {
			logger.Warn("Invalid verification token")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{"challenge":"%s"}`, challenge)))
		return
	}

	// 处理事件
	header, _ := event["header"].(map[string]interface{})
	eventType, _ := header["event_type"].(string)

	if eventType == "im.message.receive_v1" {
		c.handleMessage(event)
	}

	w.WriteHeader(http.StatusOK)
}

func (c *FeishuChannel) handleMessage(event map[string]interface{}) {
	evt, _ := event["event"].(map[string]interface{})
	message, _ := evt["message"].(map[string]interface{})
	sender, _ := evt["sender"].(map[string]interface{})

	senderIDMap, _ := sender["sender_id"].(map[string]interface{})
	senderID, _ := senderIDMap["user_id"].(string)

	// 检查权限
	if !c.IsAllowed(senderID) {
		return
	}

	contentStr, _ := message["content"].(string)
	msgType, _ := message["message_type"].(string)

	// 解析内容
	var contentText string
	var contentJSON map[string]interface{}
	if err := json.Unmarshal([]byte(contentStr), &contentJSON); err == nil {
		if text, ok := contentJSON["text"].(string); ok {
			contentText = text
		} else if imageKey, ok := contentJSON["image_key"].(string); ok {
			contentText = fmt.Sprintf("[Image: %s]", imageKey)
		} else if fileKey, ok := contentJSON["file_key"].(string); ok {
			contentText = fmt.Sprintf("[File: %s]", fileKey)
		} else {
			contentText = contentStr
		}
	} else {
		contentText = contentStr
	}

	if msgType != "text" {
		contentText = fmt.Sprintf("[%s] %s", msgType, contentText)
	}

	msgID, _ := message["message_id"].(string)
	chatID, _ := message["chat_id"].(string)
	chatType, _ := message["chat_type"].(string)

	msg := &bus.InboundMessage{
		ID:        msgID,
		Content:   contentText,
		SenderID:  senderID,
		ChatID:    chatID,
		Channel:   c.Name(),
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"chat_type": chatType,
			"msg_type":  msgType,
		},
	}

	_ = c.PublishInbound(context.Background(), msg)
}

func (c *FeishuChannel) verifySignature(r *http.Request, body []byte) bool {
	if c.encryptKey == "" {
		return true
	}

	timestamp := r.Header.Get("X-Lark-Request-Timestamp")
	nonce := r.Header.Get("X-Lark-Request-Nonce")
	signature := r.Header.Get("X-Lark-Signature")

	if timestamp == "" || nonce == "" || signature == "" {
		return false
	}

	// 计算签名
	b := bytes.NewBufferString(timestamp)
	b.WriteString(nonce)
	b.WriteString(c.encryptKey)
	b.Write(body)

	h := sha256.New()
	h.Write(b.Bytes())

	target := fmt.Sprintf("%x", h.Sum(nil))
	return target == signature
}

// Send 发送消息
func (c *FeishuChannel) Send(msg *bus.OutboundMessage) error {
	// 构建请求
	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(msg.ChatID).
			MsgType(larkim.MsgTypeText).
			Content(fmt.Sprintf(`{"text":"%s"}`, msg.Content)).
			Build()).
		Build()

	// 发送消息
	resp, err := c.client.Im.Message.Create(context.Background(), req)
	if err != nil {
		return err
	}

	if !resp.Success() {
		return fmt.Errorf("feishu api error: %d %s", resp.Code, resp.Msg)
	}

	return nil
}
