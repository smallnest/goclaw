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

	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/internal/logger"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	"go.uber.org/zap"
)

type FeishuChannel struct {
	*BaseChannelImpl
	appID             string
	appSecret         string
	encryptKey        string
	verificationToken string
	webhookPort       int
	useWebSocket      bool
	client            *lark.Client
	wsClient          *larkws.Client
	stopWS            chan struct{}
}

func NewFeishuChannel(cfg config.FeishuChannelConfig, msgBus *bus.MessageBus) (*FeishuChannel, error) {
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
	fmt.Println("___+++++_", cfg.UseWebSocket)
	return &FeishuChannel{
		BaseChannelImpl:   NewBaseChannelImpl("feishu", "default", baseCfg, msgBus),
		appID:             cfg.AppID,
		appSecret:         cfg.AppSecret,
		encryptKey:        cfg.EncryptKey,
		verificationToken: cfg.VerificationToken,
		webhookPort:       port,
		useWebSocket:      cfg.UseWebSocket,
		client:            client,
		stopWS:            make(chan struct{}),
	}, nil
}

func (c *FeishuChannel) Start(ctx context.Context) error {
	if err := c.BaseChannelImpl.Start(ctx); err != nil {
		return err
	}

	logger.Info("Starting Feishu channel", zap.String("app_id", c.appID), zap.Bool("use_websocket", c.useWebSocket))

	if c.useWebSocket {
		go c.startWebSocket(ctx)
	} else {
		go c.startEventServer(ctx)
	}

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

func (c *FeishuChannel) startWebSocket(ctx context.Context) {
	eventHandler := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
			logger.Debug("Received message via WebSocket", zap.String("sender", *event.Event.Sender.SenderId.UserId))
			c.handleP2Message(event)
			return nil
		})

	c.wsClient = larkws.NewClient(c.appID, c.appSecret,
		larkws.WithEventHandler(eventHandler),
		larkws.WithLogLevel(larkcore.LogLevelDebug),
	)

	err := c.wsClient.Start(ctx)
	if err != nil {
		logger.Error("WebSocket client start failed", zap.Error(err))
		go c.reconnectWebSocket(ctx)
		return
	}

	logger.Info("Feishu WebSocket client started")

	<-ctx.Done()
}

func (c *FeishuChannel) reconnectWebSocket(ctx context.Context) {
	for {
		select {
		case <-c.stopWS:
			return
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}

		logger.Info("Reconnecting WebSocket...")

		eventHandler := dispatcher.NewEventDispatcher("", "").
			OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
				c.handleP2Message(event)
				return nil
			})

		c.wsClient = larkws.NewClient(c.appID, c.appSecret,
			larkws.WithEventHandler(eventHandler),
		)

		err := c.wsClient.Start(ctx)
		if err != nil {
			logger.Error("WebSocket reconnection failed", zap.Error(err))
			continue
		}

		logger.Info("WebSocket reconnected")
		return
	}
}

func (c *FeishuChannel) handleP2Message(event *larkim.P2MessageReceiveV1) {
	fmt.Println("+++++++++++++++++++++++++", event)
	senderID := ""
	if event.Event.Sender != nil && event.Event.Sender.SenderId != nil {
		senderID = *event.Event.Sender.SenderId.UserId
	}

	if !c.IsAllowed(senderID) {
		return
	}

	msgType := ""
	if event.Event.Message != nil {
		msgType = *event.Event.Message.MessageType
	}

	contentText := ""
	if event.Event.Message != nil && event.Event.Message.Content != nil {
		content := *event.Event.Message.Content
		var contentJSON map[string]interface{}
		if err := json.Unmarshal([]byte(content), &contentJSON); err == nil {
			if text, ok := contentJSON["text"].(string); ok {
				contentText = text
			} else if imageKey, ok := contentJSON["image_key"].(string); ok {
				contentText = fmt.Sprintf("[Image: %s]", imageKey)
			} else if fileKey, ok := contentJSON["file_key"].(string); ok {
				contentText = fmt.Sprintf("[File: %s]", fileKey)
			} else {
				contentText = content
			}
		} else {
			contentText = content
		}
	}

	if msgType != "text" {
		contentText = fmt.Sprintf("[%s] %s", msgType, contentText)
	}

	msgID := ""
	chatID := ""
	chatType := ""
	if event.Event.Message != nil {
		if event.Event.Message.MessageId != nil {
			msgID = *event.Event.Message.MessageId
		}
		if event.Event.Message.ChatId != nil {
			chatID = *event.Event.Message.ChatId
		}
		if event.Event.Message.ChatType != nil {
			chatType = *event.Event.Message.ChatType
		}
	}

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
	logger.Debug("PublishInbound", zap.String("id", msg.ID))
	_ = c.PublishInbound(context.Background(), msg)
}

func (c *FeishuChannel) handleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("Failed to read body", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

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

	if challenge, ok := event["challenge"].(string); ok {
		if token, ok := event["token"].(string); ok && token != c.verificationToken {
			logger.Warn("Invalid verification token")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{"challenge":"%s"}`, challenge)))
		return
	}

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

	if !c.IsAllowed(senderID) {
		return
	}

	contentStr, _ := message["content"].(string)
	msgType, _ := message["message_type"].(string)

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

	b := bytes.NewBufferString(timestamp)
	b.WriteString(nonce)
	b.WriteString(c.encryptKey)
	b.Write(body)

	h := sha256.New()
	h.Write(b.Bytes())

	target := fmt.Sprintf("%x", h.Sum(nil))
	return target == signature
}

func (c *FeishuChannel) Send(msg *bus.OutboundMessage) error {
	logger.Debug("Feishu sending message",
		zap.String("chat_id", msg.ChatID),
		zap.String("content", msg.Content),
		zap.Bool("use_websocket", c.useWebSocket),
	)

	receiveIDType := larkim.ReceiveIdTypeChatId
	if len(msg.ChatID) > 3 && msg.ChatID[:3] == "ou_" {
		receiveIDType = larkim.ReceiveIdTypeOpenId
	}

	/*contentMap := map[string]string{"text": msg.Content}
	contentBytes, _ := json.Marshal(contentMap)
	content := string(contentBytes)*/
	content := larkim.NewTextMsgBuilder().
		TextLine(msg.Content).
		Build()
	logger.Debug("Feishu message details",
		zap.String("receive_id", msg.ChatID),
		zap.String("receive_id_type", string(receiveIDType)),
		zap.String("content", content),
	)

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(receiveIDType).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(msg.ChatID).
			MsgType(larkim.MsgTypeText).
			Content(content).
			Build()).
		Build()

	resp, err := c.client.Im.Message.Create(context.Background(), req)
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

	return nil
}

func (c *FeishuChannel) Stop() error {
	close(c.stopWS)
	return c.BaseChannelImpl.Stop()
}
