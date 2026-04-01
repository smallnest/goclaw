package channels

// Message item types for Weixin (proto: MessageItemType)
const (
	MessageItemTypeNone  = 0
	MessageItemTypeText  = 1
	MessageItemTypeImage = 2
	MessageItemTypeVoice = 3
	MessageItemTypeFile  = 4
	MessageItemTypeVideo = 5
)

// Message types (proto: MessageType)
const (
	MessageTypeNone = 0
	MessageTypeUser = 1
	MessageTypeBot  = 2
)

// Message states (proto: MessageState)
const (
	MessageStateNew        = 0
	MessageStateGenerating = 1
	MessageStateFinish     = 2
)

// Typing status
const (
	TypingStatusTyping = 1
	TypingStatusCancel = 2
)

// Upload media types
const (
	UploadMediaTypeImage = 1
	UploadMediaTypeVideo = 2
	UploadMediaTypeFile  = 3
	UploadMediaTypeVoice = 4
)

// WeixinMessage represents a message from Weixin (proto: WeixinMessage)
type WeixinMessage struct {
	Seq          int64         `json:"seq,omitempty"`
	MessageID    int64         `json:"message_id,omitempty"`
	FromUserID   string        `json:"from_user_id,omitempty"`
	ToUserID     string        `json:"to_user_id,omitempty"`
	ClientID     string        `json:"client_id,omitempty"`
	CreateTimeMs int64         `json:"create_time_ms,omitempty"`
	UpdateTimeMs int64         `json:"update_time_ms,omitempty"`
	DeleteTimeMs int64         `json:"delete_time_ms,omitempty"`
	SessionID    string        `json:"session_id,omitempty"`
	GroupID      string        `json:"group_id,omitempty"`
	MessageType  int           `json:"message_type,omitempty"`
	MessageState int           `json:"message_state,omitempty"`
	ItemList     []MessageItem `json:"item_list,omitempty"`
	ContextToken string        `json:"context_token,omitempty"`
}

// MessageItem represents an item in a Weixin message
type MessageItem struct {
	Type         int         `json:"type,omitempty"` // 1=TEXT, 2=IMAGE, 3=VOICE, 4=FILE, 5=VIDEO
	CreateTimeMs int64       `json:"create_time_ms,omitempty"`
	UpdateTimeMs int64       `json:"update_time_ms,omitempty"`
	IsCompleted  bool        `json:"is_completed,omitempty"`
	MsgID        string      `json:"msg_id,omitempty"`
	RefMsg       *RefMessage `json:"ref_msg,omitempty"`
	TextItem     *TextItem   `json:"text_item,omitempty"`
	ImageItem    *ImageItem  `json:"image_item,omitempty"`
	VoiceItem    *VoiceItem  `json:"voice_item,omitempty"`
	FileItem     *FileItem   `json:"file_item,omitempty"`
	VideoItem    *VideoItem  `json:"video_item,omitempty"`
}

// RefMessage represents a referenced/quoted message
type RefMessage struct {
	MessageItem *MessageItem `json:"message_item,omitempty"`
	Title       string       `json:"title,omitempty"`
}

// TextItem represents a text message item
type TextItem struct {
	Text string `json:"text,omitempty"`
}

// CDNMedia represents a CDN media reference (AES key is base64-encoded)
type CDNMedia struct {
	EncryptQueryParam string `json:"encrypt_query_param,omitempty"`
	AESKey            string `json:"aes_key,omitempty"`
	EncryptType       int    `json:"encrypt_type,omitempty"`
}

// ImageItem represents an image message item
type ImageItem struct {
	Media       *CDNMedia `json:"media,omitempty"`
	ThumbMedia  *CDNMedia `json:"thumb_media,omitempty"`
	AESKey      string    `json:"aeskey,omitempty"` // Raw AES-128 key as hex string
	URL         string    `json:"url,omitempty"`
	MidSize     int       `json:"mid_size,omitempty"`
	ThumbSize   int       `json:"thumb_size,omitempty"`
	ThumbHeight int       `json:"thumb_height,omitempty"`
	ThumbWidth  int       `json:"thumb_width,omitempty"`
	HDSize      int       `json:"hd_size,omitempty"`
}

// VoiceItem represents a voice message item
type VoiceItem struct {
	Media         *CDNMedia `json:"media,omitempty"`
	EncodeType    int       `json:"encode_type,omitempty"` // 1=pcm, 2=adpcm, 3=feature, 4=speex, 5=amr, 6=silk, 7=mp3, 8=ogg-speex
	BitsPerSample int       `json:"bits_per_sample,omitempty"`
	SampleRate    int       `json:"sample_rate,omitempty"`
	Playtime      int       `json:"playtime,omitempty"` // Duration in ms
	Text          string    `json:"text,omitempty"`     // Voice-to-text content
}

// FileItem represents a file message item
type FileItem struct {
	Media    *CDNMedia `json:"media,omitempty"`
	FileName string    `json:"file_name,omitempty"`
	MD5      string    `json:"md5,omitempty"`
	Len      string    `json:"len,omitempty"`
}

// VideoItem represents a video message item
type VideoItem struct {
	Media       *CDNMedia `json:"media,omitempty"`
	VideoSize   int       `json:"video_size,omitempty"`
	PlayLength  int       `json:"play_length,omitempty"`
	VideoMD5    string    `json:"video_md5,omitempty"`
	ThumbMedia  *CDNMedia `json:"thumb_media,omitempty"`
	ThumbSize   int       `json:"thumb_size,omitempty"`
	ThumbHeight int       `json:"thumb_height,omitempty"`
	ThumbWidth  int       `json:"thumb_width,omitempty"`
}

// GetUpdatesReq is the request for getUpdates API
type GetUpdatesReq struct {
	SyncBuf       string `json:"sync_buf,omitempty"`        // Deprecated
	GetUpdatesBuf string `json:"get_updates_buf,omitempty"` // Full context buf
}

// GetUpdatesResp is the response from getUpdates API
type GetUpdatesResp struct {
	Ret                  int64            `json:"ret,omitempty"`     // 0 = success
	ErrCode              int              `json:"errcode,omitempty"` // Error code (e.g., -14 = session timeout)
	ErrMsg               string           `json:"errmsg,omitempty"`
	Msgs                 []*WeixinMessage `json:"msgs,omitempty"`
	SyncBuf              string           `json:"sync_buf,omitempty"`        // Deprecated
	GetUpdatesBuf        string           `json:"get_updates_buf,omitempty"` // New sync cursor
	LongPollingTimeoutMs int              `json:"longpolling_timeout_ms,omitempty"`
}

// SendMessageReq is the request for sendMessage API
type SendMessageReq struct {
	ToUserID     string        `json:"to_user_id,omitempty"`
	ContextToken string        `json:"context_token,omitempty"`
	ItemList     []MessageItem `json:"item_list,omitempty"`
}

// GetUploadUrlReq is the request for getUploadUrl API
type GetUploadUrlReq struct {
	FileKey         string `json:"filekey,omitempty"`
	MediaType       int    `json:"media_type,omitempty"` // 1=IMAGE, 2=VIDEO, 3=FILE
	ToUserID        string `json:"to_user_id,omitempty"`
	RawSize         int64  `json:"rawsize,omitempty"`          // Plaintext size
	RawFileMD5      string `json:"rawfilemd5,omitempty"`       // Plaintext MD5
	FileSize        int64  `json:"filesize,omitempty"`         // Encrypted size (after AES-128-ECB)
	ThumbRawSize    int64  `json:"thumb_rawsize,omitempty"`    // Thumbnail plaintext size
	ThumbRawFileMD5 string `json:"thumb_rawfilemd5,omitempty"` // Thumbnail plaintext MD5
	ThumbFileSize   int64  `json:"thumb_filesize,omitempty"`   // Thumbnail encrypted size
	NoNeedThumb     bool   `json:"no_need_thumb,omitempty"`
	AESKey          string `json:"aeskey,omitempty"` // Encryption key
}

// GetUploadUrlResp is the response from getUploadUrl API
type GetUploadUrlResp struct {
	UploadParam      string `json:"upload_param,omitempty"`       // Encrypted upload param
	ThumbUploadParam string `json:"thumb_upload_param,omitempty"` // Thumbnail upload param
}

// GetConfigResp is the response from getConfig API
type GetConfigResp struct {
	Ret          int    `json:"ret,omitempty"`
	ErrMsg       string `json:"errmsg,omitempty"`
	TypingTicket string `json:"typing_ticket,omitempty"` // Base64-encoded typing ticket
}

// WeixinConfig is the configuration for Weixin channel
type WeixinConfig struct {
	BaseChannelConfig
	BaseURL    string `mapstructure:"base_url" json:"base_url"`
	CDNBaseURL string `mapstructure:"cdn_base_url" json:"cdn_base_url"`
	Token      string `mapstructure:"token" json:"token"`
}

// TokenInfo stores token information
type TokenInfo struct {
	Token       string `json:"token,omitempty"`
	ILinkBotID  string `json:"ilink_bot_id,omitempty"`
	ILinkUserID string `json:"ilink_user_id,omitempty"`
	BaseURL     string `json:"base_url,omitempty"`
	ExpiresAt   int64  `json:"expires_at,omitempty"`
}
