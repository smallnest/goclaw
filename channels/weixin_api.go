package channels

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

// Default Weixin API endpoints
const (
	DefaultWeixinBaseURL = "https://ilinkai.weixin.qq.com"
	DefaultWeixinCDNURL  = "https://novac2c.cdn.weixin.qq.com/c2c"
	DefaultILinkBotType  = "3"
)

// WeixinAPIClient is the HTTP client for Weixin API
type WeixinAPIClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewWeixinAPIClient creates a new Weixin API client
func NewWeixinAPIClient(baseURL, token string) *WeixinAPIClient {
	if baseURL == "" {
		baseURL = DefaultWeixinBaseURL
	}
	return &WeixinAPIClient{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// SetToken sets the authentication token
func (c *WeixinAPIClient) SetToken(token string) {
	c.token = token
}

// GetToken returns the current token
func (c *WeixinAPIClient) GetToken() string {
	return c.token
}

// generateWechatUIN generates a random X-WECHAT-UIN header value
func generateWechatUIN() string {
	b := make([]byte, 4)
	rand.Read(b)
	uint32Val := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", uint32Val)))
}

// buildHeaders builds the common headers for API requests
func buildHeaders(token string, bodyLength int) map[string]string {
	headers := map[string]string{
		"Content-Type":      "application/json",
		"AuthorizationType": "ilink_bot_token",
		"X-WECHAT-UIN":      generateWechatUIN(),
	}
	if bodyLength > 0 {
		headers["Content-Length"] = fmt.Sprintf("%d", bodyLength)
	}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	return headers
}

// ensureTrailingSlash ensures URL ends with /
func ensureTrailingSlash(u string) string {
	if len(u) > 0 && u[len(u)-1] != '/' {
		return u + "/"
	}
	return u
}

// doJSONRequest performs a POST JSON request to the Weixin API
func (c *WeixinAPIClient) doJSONRequest(ctx context.Context, endpoint string, reqBody, respBody interface{}) error {
	var bodyReader io.Reader
	var bodyLen int

	if reqBody != nil {
		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		bodyReader = NewReaderAt(jsonData)
		bodyLen = len(jsonData)

		// Add base_info
		type requestWithBaseInfo struct {
			BaseInfo map[string]string `json:"base_info"`
		}
	}

	baseURL := ensureTrailingSlash(c.baseURL)
	fullURL := baseURL + endpoint

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	headers := buildHeaders(c.token, bodyLen)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	logger.Debug("Weixin API request",
		zap.String("method", http.MethodPost),
		zap.String("url", fullURL),
		zap.String("endpoint", endpoint))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	logger.Debug("Weixin API response",
		zap.Int("status_code", resp.StatusCode),
		zap.Int("response_size", len(respData)))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(respData))
	}

	if respBody != nil {
		if err := json.Unmarshal(respData, respBody); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}

	return nil
}

// ReaderAt wraps a byte slice to implement io.Reader
type ReaderAt struct {
	data []byte
	pos  int
}

// NewReaderAt creates a new ReaderAt
func NewReaderAt(data []byte) *ReaderAt {
	return &ReaderAt{data: data}
}

// Read implements io.Reader
func (r *ReaderAt) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// GetUpdates fetches new messages using long polling
func (c *WeixinAPIClient) GetUpdates(ctx context.Context, req *GetUpdatesReq) (*GetUpdatesResp, error) {
	// Add base_info to request
	body := map[string]interface{}{
		"get_updates_buf": req.GetUpdatesBuf,
		"base_info": map[string]string{
			"channel_version": "1.0.0",
		},
	}

	resp := &GetUpdatesResp{}
	if err := c.doJSONRequest(ctx, "ilink/bot/getupdates", body, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// SendMessage sends a message to a user
func (c *WeixinAPIClient) SendMessage(ctx context.Context, req *SendMessageReq) error {
	// Wrap in msg field as per API
	body := map[string]interface{}{
		"msg": req,
		"base_info": map[string]string{
			"channel_version": "1.0.0",
		},
	}
	return c.doJSONRequest(ctx, "ilink/bot/sendmessage", body, nil)
}

// GetUploadURL gets a pre-signed URL for CDN upload
func (c *WeixinAPIClient) GetUploadURL(ctx context.Context, req *GetUploadUrlReq) (*GetUploadUrlResp, error) {
	// Add base_info
	body := map[string]interface{}{
		"filekey":          req.FileKey,
		"media_type":       req.MediaType,
		"to_user_id":       req.ToUserID,
		"rawsize":          req.RawSize,
		"rawfilemd5":       req.RawFileMD5,
		"filesize":         req.FileSize,
		"thumb_rawsize":    req.ThumbRawSize,
		"thumb_rawfilemd5": req.ThumbRawFileMD5,
		"thumb_filesize":   req.ThumbFileSize,
		"no_need_thumb":    req.NoNeedThumb,
		"aeskey":           req.AESKey,
		"base_info": map[string]string{
			"channel_version": "1.0.0",
		},
	}

	resp := &GetUploadUrlResp{}
	if err := c.doJSONRequest(ctx, "ilink/bot/getuploadurl", body, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetConfig gets account configuration including typing ticket
func (c *WeixinAPIClient) GetConfig(ctx context.Context, ilinkUserID, contextToken string) (*GetConfigResp, error) {
	body := map[string]interface{}{
		"ilink_user_id": ilinkUserID,
		"context_token": contextToken,
		"base_info": map[string]string{
			"channel_version": "1.0.0",
		},
	}

	resp := &GetConfigResp{}
	if err := c.doJSONRequest(ctx, "ilink/bot/getconfig", body, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// SendTyping sends typing indicator
func (c *WeixinAPIClient) SendTyping(ctx context.Context, ilinkUserID, typingTicket string, status int) error {
	body := map[string]interface{}{
		"ilink_user_id": ilinkUserID,
		"typing_ticket": typingTicket,
		"status":        status,
		"base_info": map[string]string{
			"channel_version": "1.0.0",
		},
	}
	return c.doJSONRequest(ctx, "ilink/bot/sendtyping", body, nil)
}

// QRCodeResponse is the response from get_bot_qrcode API (GET request)
type QRCodeResponse struct {
	QRCode           string `json:"qrcode"`             // Unique identifier for polling
	QRCodeImgContent string `json:"qrcode_img_content"` // URL to display as QR code
}

// QRCodeStatusResponse is the response from get_qrcode_status API (GET request)
type QRCodeStatusResponse struct {
	Status      string `json:"status"`        // "wait", "scaned", "confirmed", "expired"
	BotToken    string `json:"bot_token"`     // Available when confirmed
	ILinkBotID  string `json:"ilink_bot_id"`  // Bot ID
	BaseURL     string `json:"baseurl"`       // API base URL
	ILinkUserID string `json:"ilink_user_id"` // User ID who scanned
}

// GetBotQRCode gets the QR code for bot login (GET request)
func (c *WeixinAPIClient) GetBotQRCode(ctx context.Context) (*QRCodeResponse, error) {
	baseURL := ensureTrailingSlash(c.baseURL)
	endpoint := fmt.Sprintf("ilink/bot/get_bot_qrcode?bot_type=%s", url.QueryEscape(DefaultILinkBotType))
	fullURL := baseURL + endpoint

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers (no auth needed for QR code request)
	req.Header.Set("Content-Type", "application/json")

	logger.Info("Fetching QR code", zap.String("url", fullURL))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch QR code: %w", err)
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("QR code request failed with status %d: %s", resp.StatusCode, string(respData))
	}

	var qrResp QRCodeResponse
	if err := json.Unmarshal(respData, &qrResp); err != nil {
		return nil, fmt.Errorf("failed to parse QR code response: %w", err)
	}

	logger.Info("QR code received",
		zap.Int("qrcode_len", len(qrResp.QRCode)),
		zap.Int("qrcode_url_len", len(qrResp.QRCodeImgContent)))

	return &qrResp, nil
}

// GetQRCodeStatus checks the QR code scan status (GET request)
func (c *WeixinAPIClient) GetQRCodeStatus(ctx context.Context, qrcode string) (*QRCodeStatusResponse, error) {
	baseURL := ensureTrailingSlash(c.baseURL)
	endpoint := fmt.Sprintf("ilink/bot/get_qrcode_status?qrcode=%s", url.QueryEscape(qrcode))
	fullURL := baseURL + endpoint

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("iLink-App-ClientVersion", "1")

	logger.Debug("Polling QR status", zap.String("url", fullURL))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to poll QR status: %w", err)
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("QR status request failed with status %d: %s", resp.StatusCode, string(respData))
	}

	var statusResp QRCodeStatusResponse
	if err := json.Unmarshal(respData, &statusResp); err != nil {
		return nil, fmt.Errorf("failed to parse QR status response: %w", err)
	}

	return &statusResp, nil
}

// UploadToCDN uploads encrypted data to CDN
func (c *WeixinAPIClient) UploadToCDN(ctx context.Context, uploadURL string, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, NewReaderAt(data))
	if err != nil {
		return fmt.Errorf("failed to create upload request: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload to CDN: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("CDN upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// DownloadFromCDN downloads encrypted data from CDN
func (c *WeixinAPIClient) DownloadFromCDN(ctx context.Context, encryptQueryParam string) ([]byte, error) {
	// Build CDN URL
	cdnURL := DefaultWeixinCDNURL + "/" + encryptQueryParam

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cdnURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create download request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download from CDN: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("CDN download failed with status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read CDN response: %w", err)
	}

	return data, nil
}
