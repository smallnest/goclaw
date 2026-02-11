package channels

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/smallnest/dogclaw/goclaw/bus"
	"github.com/smallnest/dogclaw/goclaw/config"
	"github.com/smallnest/dogclaw/goclaw/internal/logger"
	"go.uber.org/zap"
)

// WeWorkChannel 企业微信通道
type WeWorkChannel struct {
	*BaseChannelImpl
	corpID         string
	agentID        string
	secret         string
	token          string
	encodingAESKey string
	webhookPort    int
	recvMsg        bool // 是否使用加密模式

	accessToken    string
	tokenExpiresAt int64
	mu             sync.Mutex
	httpClient     *http.Client
}

// NewWeWorkChannel 创建企业微信通道
func NewWeWorkChannel(cfg config.WeWorkChannelConfig, bus *bus.MessageBus) (*WeWorkChannel, error) {
	if cfg.CorpID == "" || cfg.Secret == "" || cfg.AgentID == "" {
		return nil, fmt.Errorf("wework corp_id, secret and agent_id are required")
	}

	baseCfg := BaseChannelConfig{
		Enabled:    cfg.Enabled,
		AllowedIDs: cfg.AllowedIDs,
	}

	port := cfg.WebhookPort
	if port == 0 {
		port = 8766
	}

	return &WeWorkChannel{
		BaseChannelImpl: NewBaseChannelImpl("wework", baseCfg, bus),
		corpID:          cfg.CorpID,
		agentID:         cfg.AgentID,
		secret:          cfg.Secret,
		token:           cfg.Token,
		encodingAESKey:  cfg.EncodingAESKey,
		webhookPort:     port,
		recvMsg:         cfg.EncodingAESKey != "", // 有加密密钥则使用加密模式
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

// Start 启动企业微信通道
func (c *WeWorkChannel) Start(ctx context.Context) error {
	if err := c.BaseChannelImpl.Start(ctx); err != nil {
		return err
	}

	logger.Info("Starting WeWork channel")

	// 启动 Webhook 服务器
	go c.startWebhookServer(ctx)

	return nil
}

func (c *WeWorkChannel) startWebhookServer(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/wework/event", c.handleWebhook)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", c.webhookPort),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("WeWork webhook server started", zap.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("WeWork webhook server error", zap.Error(err))
		}
	}()

	<-ctx.Done()
	_ = server.Shutdown(ctx)
}

func (c *WeWorkChannel) handleWebhook(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	signature := query.Get("msg_signature")
	timestamp := query.Get("timestamp")
	nonce := query.Get("nonce")
	echostr := query.Get("echostr")

	if r.Method == http.MethodGet {
		// 验证回调 URL
		if !c.verifySignature(c.token, timestamp, nonce, echostr, signature) {
			logger.Warn("Invalid signature for GET", zap.String("expected", c.computeSignature(c.token, timestamp, nonce, echostr)))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// 如果有加密密钥，需要解密 echostr
		if c.recvMsg && c.encodingAESKey != "" {
			decrypted, err := c.decryptMsg(echostr)
			if err != nil {
				logger.Error("Failed to decrypt echostr", zap.Error(err))
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write(decrypted)
		} else {
			_, _ = w.Write([]byte(echostr))
		}
		return
	}

	// 处理 POST 请求
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("Failed to read body", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// 解析 XML 提取 Encrypt 字段用于签名验证
	var encryptedMsg struct {
		XMLName    xml.Name `xml:"xml"`
		ToUserName string   `xml:"ToUserName"`
		Encrypt    string   `xml:"Encrypt"`
		AgentID    string   `xml:"AgentID"`
	}

	// 尝试解析为加密格式
	if err := xml.Unmarshal(body, &encryptedMsg); err == nil && encryptedMsg.Encrypt != "" {
		if !c.verifySignature(c.token, timestamp, nonce, encryptedMsg.Encrypt, signature) {
			logger.Warn("Invalid signature for POST",
				zap.String("received", signature),
				zap.String("expected", c.computeSignature(c.token, timestamp, nonce, encryptedMsg.Encrypt)))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// 解密 body
		decryptedBody, err := c.decryptMsg(encryptedMsg.Encrypt)
		if err != nil {
			logger.Error("Failed to decrypt message", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		body = decryptedBody
	}

	// 解析明文 XML (或者解密后的内容)
	var msg struct {
		XMLName      xml.Name `xml:"xml"`
		ToUserName   string   `xml:"ToUserName"`
		FromUserName string   `xml:"FromUserName"`
		CreateTime   int64    `xml:"CreateTime"`
		MsgType      string   `xml:"MsgType"`
		Content      string   `xml:"Content"`
		MsgId        string   `xml:"MsgId"`
		AgentID      string   `xml:"AgentID"`
	}

	if err := xml.Unmarshal(body, &msg); err != nil {
		logger.Error("Failed to unmarshal XML", zap.Error(err), zap.String("body", string(body)))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// 检查权限
	if !c.IsAllowed(msg.FromUserName) {
		w.WriteHeader(http.StatusOK)
		return
	}

	if msg.MsgType == "text" {
		inMsg := &bus.InboundMessage{
			ID:        msg.MsgId,
			Content:   msg.Content,
			SenderID:  msg.FromUserName,
			ChatID:    msg.FromUserName,
			Channel:   c.Name(),
			Timestamp: time.Unix(msg.CreateTime, 0),
			Metadata: map[string]interface{}{
				"agent_id": msg.AgentID,
			},
		}
		_ = c.PublishInbound(context.Background(), inMsg)
	}

	w.WriteHeader(http.StatusOK)
}

func (c *WeWorkChannel) decryptMsg(encrypted string) ([]byte, error) {
	// 企业微信消息加解密流程：
	// 1. 对密文进行 Base64 解码
	// 2. 对解码后的结果进行 AES-CBC 解密 (使用 EncodingAESKey + "=")
	// 3. 去掉前 16 字节随机字符串
	// 4. 读取 4 字节的长度，得到消息体长度
	// 5. 提取消息体
	// 6. 验证消息体末尾的 CorpID

	if c.encodingAESKey == "" {
		return nil, fmt.Errorf("encoding_aes_key not configured")
	}

	// Base64 解码
	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return nil, fmt.Errorf("base64 decode failed: %w", err)
	}

	// EncodingAESKey 需要填充为 32 字节 (AES-256)
	key := c.encodingAESKey + "="
	keyBytes, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return nil, fmt.Errorf("key decode failed: %w", err)
	}

	// AES-CBC 解密
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("aes cipher failed: %w", err)
	}

	if len(ciphertext) < aes.BlockSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]

	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext not a multiple of block size")
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(ciphertext, ciphertext)

	// 去掉 PKCS7 填充
	padding := int(ciphertext[len(ciphertext)-1])
	if padding < 1 || padding > aes.BlockSize {
		return nil, fmt.Errorf("invalid padding")
	}
	ciphertext = ciphertext[:len(ciphertext)-padding]

	// 去掉前 16 字节随机字符串
	if len(ciphertext) < 16 {
		return nil, fmt.Errorf("decrypted text too short")
	}
	content := ciphertext[16:]

	// 读取 4 字节消息长度
	if len(content) < 4 {
		return nil, fmt.Errorf("content too short for length header")
	}
	msgLen := int(content[0])<<24 | int(content[1])<<16 | int(content[2])<<8 | int(content[3])

	if len(content) < 4+msgLen {
		return nil, fmt.Errorf("content too short for message")
	}

	// 提取消息体
	message := content[4 : 4+msgLen]

	// 验证 CorpID
	if len(content) < 4+msgLen+len(c.corpID) {
		return nil, fmt.Errorf("content too short for corp_id")
	}
	receivedCorpID := string(content[4+msgLen:])
	if receivedCorpID != c.corpID {
		return nil, fmt.Errorf("corp_id mismatch: expected %s, got %s", c.corpID, receivedCorpID)
	}

	return message, nil
}

func (c *WeWorkChannel) computeSignature(token, timestamp, nonce, data string) string {
	// 排序
	strs := []string{token, timestamp, nonce, data}
	sort.Strings(strs)

	// 拼接
	str := strings.Join(strs, "")

	// SHA1
	h := sha1.New()
	h.Write([]byte(str))
	bs := h.Sum(nil)
	return hex.EncodeToString(bs)
}

func (c *WeWorkChannel) verifySignature(token, timestamp, nonce, data, signature string) bool {
	expected := c.computeSignature(token, timestamp, nonce, data)
	if expected != signature {
		logger.Debug("Signature mismatch",
			zap.String("expected", expected),
			zap.String("received", signature))
		return false
	}
	return true
}

func (c *WeWorkChannel) getAccessToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.accessToken != "" && time.Now().Unix() < c.tokenExpiresAt {
		return c.accessToken, nil
	}

	url := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpid=%s&corpsecret=%s", c.corpID, c.secret)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("http get failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("json decode failed: %w", err)
	}

	if result.ErrCode != 0 {
		return "", fmt.Errorf("wechat api error: %s", result.ErrMsg)
	}

	c.accessToken = result.AccessToken
	c.tokenExpiresAt = time.Now().Unix() + result.ExpiresIn - 200
	return c.accessToken, nil
}

// Send 发送消息
func (c *WeWorkChannel) Send(msg *bus.OutboundMessage) error {
	token, err := c.getAccessToken()
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/message/send?access_token=%s", token)

	payload := map[string]interface{}{
		"touser":  msg.ChatID,
		"msgtype": "text",
		"agentid": c.agentID,
		"text": map[string]string{
			"content": msg.Content,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("json marshal failed: %w", err)
	}

	resp, err := c.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("http post failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("json decode failed: %w", err)
	}

	if result.ErrCode != 0 {
		return fmt.Errorf("failed to send message: %s", result.ErrMsg)
	}

	logger.Info("WeWork message sent",
		zap.String("chat_id", msg.ChatID),
		zap.Int("content_length", len(msg.Content)),
	)

	return nil
}
