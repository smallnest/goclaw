package openclaw

import "time"

// ClientInfo 客户端信息
type ClientInfo struct {
	ID           string `json:"id"`
	DisplayName  string `json:"displayName"`
	Version      string `json:"version"`
	Platform     string `json:"platform"`
	DeviceFamily string `json:"deviceFamily"`
	Mode         string `json:"mode"`
	InstanceID   string `json:"instanceId"`
}

// DeviceIdentity 设备身份
type DeviceIdentity struct {
	ID        string `json:"id"`
	PublicKey string `json:"publicKey"`
	Signature string `json:"signature"`
	SignedAt  int64  `json:"signedAt"`
	Nonce     string `json:"nonce"`
}

// ConnectParams 连接参数
type ConnectParams struct {
	MinProtocol int             `json:"minProtocol"`
	MaxProtocol int             `json:"maxProtocol"`
	Client      ClientInfo      `json:"client"`
	Caps        []string        `json:"caps,omitempty"`
	Commands    []string        `json:"commands,omitempty"`
	Role        string          `json:"role,omitempty"` // "operator" | "node"
	Scopes      []string        `json:"scopes,omitempty"`
	Device      *DeviceIdentity `json:"device,omitempty"`
	Auth        *AuthParams     `json:"auth,omitempty"`
}

// AuthParams 认证参数
type AuthParams struct {
	Token       string `json:"token,omitempty"`
	DeviceToken string `json:"deviceToken,omitempty"`
	Password    string `json:"password,omitempty"`
}

// ConnectionInfo 连接信息
type ConnectionInfo struct {
	ConnID                    string
	Client                    ClientInfo
	Role                      string
	Scopes                    []string
	PresenceKey               string
	ClientIP                  string
	CanvasHostURL             string
	CanvasCapability          string
	CanvasCapabilityExpiresAt time.Time
	Authenticated             bool
	DeviceID                  string
	Metadata                  map[string]interface{}
	CreatedAt                 time.Time
	LastActivityAt            time.Time
}

// PresenceEntry 在线状态条目
type PresenceEntry struct {
	Host             *string  `json:"host,omitempty"`
	IP               *string  `json:"ip,omitempty"`
	Version          *string  `json:"version,omitempty"`
	Platform         *string  `json:"platform,omitempty"`
	DeviceFamily     *string  `json:"deviceFamily,omitempty"`
	ModelIdentifier  *string  `json:"modelIdentifier,omitempty"`
	Mode             *string  `json:"mode,omitempty"`
	LastInputSeconds *int64   `json:"lastInputSeconds,omitempty"`
	Reason           *string  `json:"reason,omitempty"`
	Tags             []string `json:"tags,omitempty"`
	Text             *string  `json:"text,omitempty"`
	Ts               int64    `json:"ts"`
	DeviceID         *string  `json:"deviceId,omitempty"`
	Roles            []string `json:"roles,omitempty"`
	Scopes           []string `json:"scopes,omitempty"`
	InstanceID       *string  `json:"instanceId,omitempty"`
}

// HealthStatus 健康状态
type HealthStatus struct {
	Status     string                     `json:"status"`
	Timestamp  int64                      `json:"timestamp"`
	Components map[string]ComponentStatus `json:"components,omitempty"`
	Issues     []string                   `json:"issues,omitempty"`
}

// ComponentStatus 组件状态
type ComponentStatus struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// UsageStatus 使用状态
type UsageStatus struct {
	TotalRequests int64     `json:"totalRequests"`
	TotalTokens   int64     `json:"totalTokens"`
	TotalCost     float64   `json:"totalCost"`
	PeriodStart   time.Time `json:"periodStart"`
	PeriodEnd     time.Time `json:"periodEnd"`
}

// UpdateInfo 更新信息
type UpdateInfo struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	Channel        string `json:"channel"`
}

// SessionDefaults 会话默认配置
type SessionDefaults struct {
	DefaultAgentID string `json:"defaultAgentId"`
	MainKey        string `json:"mainKey"`
	MainSessionKey string `json:"mainSessionKey"`
	Scope          string `json:"scope,omitempty"`
}

// ConnectionState 连接状态
type ConnectionState struct {
	ConnID           string
	ClientID         string
	ConnectedAt      time.Time
	LastHeartbeatAt  time.Time
	StateVersion     StateVersion
	BufferedBytes    int64
	MessagesSent     int64
	MessagesReceived int64
}

// Role 角色
const (
	RoleOperator string = "operator"
	RoleNode     string = "node"
)

// Scope 权限范围
const (
	ScopeRead     string = "read"
	ScopeWrite    string = "write"
	ScopeAdmin    string = "admin"
	ScopeExecute  string = "execute"
	ScopeConfig   string = "config"
	ScopeAgents   string = "agents"
	ScopeSessions string = "sessions"
	ScopeCron     string = "cron"
	ScopeChannels string = "channels"
	ScopeNodes    string = "nodes"
	ScopeDevices  string = "devices"
)

// AllScopes 所有权限范围
var AllScopes = []string{
	ScopeRead, ScopeWrite, ScopeAdmin, ScopeExecute,
	ScopeConfig, ScopeAgents, ScopeSessions, ScopeCron,
	ScopeChannels, ScopeNodes, ScopeDevices,
}

// HasScope 检查是否有权限
func HasScope(scopes []string, scope string) bool {
	for _, s := range scopes {
		if s == scope || s == ScopeAdmin {
			return true
		}
	}
	return false
}

// HasAnyScope 检查是否有任一权限
func HasAnyScope(scopes []string, requiredScopes []string) bool {
	for _, required := range requiredScopes {
		if HasScope(scopes, required) {
			return true
		}
	}
	return false
}

// HasAllScopes 检查是否有所有权限
func HasAllScopes(scopes []string, requiredScopes []string) bool {
	for _, required := range requiredScopes {
		if !HasScope(scopes, required) {
			return false
		}
	}
	return true
}

// ClientCapabilities 客户端能力
const (
	CapChat         string = "chat"
	CapSessions     string = "sessions"
	CapAgents       string = "agents"
	CapCron         string = "cron"
	CapChannels     string = "channels"
	CapNodes        string = "nodes"
	CapDevices      string = "devices"
	CapBrowser      string = "browser"
	CapTTS          string = "tts"
	CapVoiceWake    string = "voicewake"
	CapWizard       string = "wizard"
	CapExecApproval string = "exec_approval"
	CapStreaming    string = "streaming"
	CapAttachments  string = "attachments"
)

// DefaultClientCapabilities 默认客户端能力
var DefaultClientCapabilities = []string{
	CapChat, CapSessions, CapAgents, CapCron,
	CapChannels, CapNodes, CapDevices,
}
