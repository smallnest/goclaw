package openclaw

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// AuthMode 认证模式
type AuthMode string

const (
	AuthModeNone         AuthMode = "none"
	AuthModeToken        AuthMode = "token"
	AuthModePassword     AuthMode = "password"
	AuthModeTrustedProxy AuthMode = "trusted-proxy"
	AuthModeDevice       AuthMode = "device" // 设备签名认证
)

// AuthContext 认证上下文
type AuthContext struct {
	mu                 sync.RWMutex
	authMode           AuthMode
	authToken          string
	authPassword       string
	trustedProxies     []string
	devicePairs        map[string]*DevicePair // deviceID -> pair
	pendingDevicePairs map[string]*PendingPair
	nodePairs          map[string]*NodePair // nodeID -> pair
	pendingNodePairs   map[string]*PendingPair
	challengeNonces    map[string]time.Time // nonce -> expiry
}

// DevicePair 设备配对
type DevicePair struct {
	DeviceID   string
	PublicKey  string
	Name       string
	Roles      []string
	Scopes     []string
	Metadata   map[string]string
	PairedAt   time.Time
	LastSeenAt time.Time
}

// NodePair Node 配对
type NodePair struct {
	NodeID       string
	Name         string
	Capabilities []string
	Metadata     map[string]string
	PairedAt     time.Time
	LastSeenAt   time.Time
}

// PendingPair 待处理的配对请求
type PendingPair struct {
	ID          string
	RequesterID string
	Name        string
	RequestedAt time.Time
	ExpiresAt   time.Time
	Metadata    map[string]interface{}
}

// NewAuthContext 创建认证上下文
func NewAuthContext() *AuthContext {
	return &AuthContext{
		authMode:           AuthModeNone,
		trustedProxies:     make([]string, 0),
		devicePairs:        make(map[string]*DevicePair),
		pendingDevicePairs: make(map[string]*PendingPair),
		nodePairs:          make(map[string]*NodePair),
		pendingNodePairs:   make(map[string]*PendingPair),
		challengeNonces:    make(map[string]time.Time),
	}
}

// SetAuthMode 设置认证模式
func (ac *AuthContext) SetAuthMode(mode AuthMode) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.authMode = mode
}

// SetAuthToken 设置认证 token
func (ac *AuthContext) SetAuthToken(token string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.authToken = token
}

// SetAuthPassword 设置认证密码
func (ac *AuthContext) SetAuthPassword(password string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.authPassword = password
}

// SetTrustedProxies 设置受信任的代理
func (ac *AuthContext) SetTrustedProxies(proxies []string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.trustedProxies = proxies
}

// GetAuthMode 获取认证模式
func (ac *AuthContext) GetAuthMode() AuthMode {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.authMode
}

// AuthenticateToken 验证 token
func (ac *AuthContext) AuthenticateToken(token string) bool {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	if ac.authMode != AuthModeToken {
		return false
	}

	// 使用恒定时间比较防止时序攻击
	return subtle.ConstantTimeCompare([]byte(token), []byte(ac.authToken)) == 1
}

// AuthenticatePassword 验证密码
func (ac *AuthContext) AuthenticatePassword(password string) bool {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	if ac.authMode != AuthModePassword {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(password), []byte(ac.authPassword)) == 1
}

// IsTrustedProxy 检查是否是受信任的代理
func (ac *AuthContext) IsTrustedProxy(ip string) bool {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	if ac.authMode != AuthModeTrustedProxy {
		return false
	}

	for _, trusted := range ac.trustedProxies {
		if ip == trusted {
			return true
		}
	}
	return false
}

// GenerateChallengeNonce 生成挑战随机数
func (ac *AuthContext) GenerateChallengeNonce() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	nonce := hex.EncodeToString(bytes)
	expiry := time.Now().Add(5 * time.Minute)

	ac.mu.Lock()
	ac.challengeNonces[nonce] = expiry
	ac.mu.Unlock()

	return nonce, nil
}

// ValidateChallengeNonce 验证挑战随机数
func (ac *AuthContext) ValidateChallengeNonce(nonce string) bool {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	expiry, ok := ac.challengeNonces[nonce]
	if !ok {
		return false
	}

	if time.Now().After(expiry) {
		delete(ac.challengeNonces, nonce)
		return false
	}

	// 使用后删除
	delete(ac.challengeNonces, nonce)
	return true
}

// CleanExpiredNonces 清理过期的随机数
func (ac *AuthContext) CleanExpiredNonces() {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	now := time.Now()
	for nonce, expiry := range ac.challengeNonces {
		if now.After(expiry) {
			delete(ac.challengeNonces, nonce)
		}
	}
}

// VerifyDeviceSignature 验证设备签名 (v2/v3 协议)
func (ac *AuthContext) VerifyDeviceSignature(deviceID, publicKey, signature string, signedAt int64, nonce string) bool {
	// 检查设备是否已配对
	ac.mu.RLock()
	pair, ok := ac.devicePairs[deviceID]
	ac.mu.RUnlock()

	if !ok {
		return false
	}

	// 验证公钥
	if pair.PublicKey != publicKey {
		return false
	}

	// 验证签名: signature = H(deviceID + publicKey + signedAt + nonce + secretKey)
	expectedSig := ac.computeDeviceSignature(deviceID, publicKey, signedAt, nonce)
	return subtle.ConstantTimeCompare([]byte(signature), []byte(expectedSig)) == 1
}

// computeDeviceSignature 计算设备签名
func (ac *AuthContext) computeDeviceSignature(deviceID, publicKey string, signedAt int64, nonce string) string {
	data := fmt.Sprintf("%s:%s:%d:%s", deviceID, publicKey, signedAt, nonce)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// AddDevicePair 添加设备配对
func (ac *AuthContext) AddDevicePair(pair *DevicePair) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	ac.devicePairs[pair.DeviceID] = pair
}

// RemoveDevicePair 移除设备配对
func (ac *AuthContext) RemoveDevicePair(deviceID string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	delete(ac.devicePairs, deviceID)
}

// GetDevicePair 获取设备配对
func (ac *AuthContext) GetDevicePair(deviceID string) (*DevicePair, bool) {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	pair, ok := ac.devicePairs[deviceID]
	if !ok {
		return nil, false
	}
	// Return a copy
	copy := *pair
	return &copy, true
}

// ListDevicePairs 列出所有设备配对
func (ac *AuthContext) ListDevicePairs() []*DevicePair {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	pairs := make([]*DevicePair, 0, len(ac.devicePairs))
	for _, pair := range ac.devicePairs {
		copy := *pair
		pairs = append(pairs, &copy)
	}
	return pairs
}

// AddNodePair 添加 Node 配对
func (ac *AuthContext) AddNodePair(pair *NodePair) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	ac.nodePairs[pair.NodeID] = pair
}

// RemoveNodePair 移除 Node 配对
func (ac *AuthContext) RemoveNodePair(nodeID string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	delete(ac.nodePairs, nodeID)
}

// GetNodePair 获取 Node 配对
func (ac *AuthContext) GetNodePair(nodeID string) (*NodePair, bool) {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	pair, ok := ac.nodePairs[nodeID]
	if !ok {
		return nil, false
	}
	// Return a copy
	copy := *pair
	return &copy, true
}

// ListNodePairs 列出所有 Node 配对
func (ac *AuthContext) ListNodePairs() []*NodePair {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	pairs := make([]*NodePair, 0, len(ac.nodePairs))
	for _, pair := range ac.nodePairs {
		copy := *pair
		pairs = append(pairs, &copy)
	}
	return pairs
}

// AddPendingDevicePair 添加待处理的设备配对请求
func (ac *AuthContext) AddPendingDevicePair(pair *PendingPair) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	ac.pendingDevicePairs[pair.ID] = pair
}

// RemovePendingDevicePair 移除待处理的设备配对请求
func (ac *AuthContext) RemovePendingDevicePair(id string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	delete(ac.pendingDevicePairs, id)
}

// GetPendingDevicePair 获取待处理的设备配对请求
func (ac *AuthContext) GetPendingDevicePair(id string) (*PendingPair, bool) {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	pair, ok := ac.pendingDevicePairs[id]
	if !ok {
		return nil, false
	}
	// Return a copy
	copy := *pair
	return &copy, true
}

// ListPendingDevicePairs 列出所有待处理的设备配对请求
func (ac *AuthContext) ListPendingDevicePairs() []*PendingPair {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	pairs := make([]*PendingPair, 0, len(ac.pendingDevicePairs))
	for _, pair := range ac.pendingDevicePairs {
		copy := *pair
		pairs = append(pairs, &copy)
	}
	return pairs
}

// AddPendingNodePair 添加待处理的 Node 配对请求
func (ac *AuthContext) AddPendingNodePair(pair *PendingPair) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	ac.pendingNodePairs[pair.ID] = pair
}

// RemovePendingNodePair 移除待处理的 Node 配对请求
func (ac *AuthContext) RemovePendingNodePair(id string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	delete(ac.pendingNodePairs, id)
}

// GetPendingNodePair 获取待处理的 Node 配对请求
func (ac *AuthContext) GetPendingNodePair(id string) (*PendingPair, bool) {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	pair, ok := ac.pendingNodePairs[id]
	if !ok {
		return nil, false
	}
	// Return a copy
	copy := *pair
	return &copy, true
}

// ListPendingNodePairs 列出所有待处理的 Node 配对请求
func (ac *AuthContext) ListPendingNodePairs() []*PendingPair {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	pairs := make([]*PendingPair, 0, len(ac.pendingNodePairs))
	for _, pair := range ac.pendingNodePairs {
		copy := *pair
		pairs = append(pairs, &copy)
	}
	return pairs
}

// CleanExpiredPendingPairs 清理过期的待处理配对请求
func (ac *AuthContext) CleanExpiredPendingPairs() {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	now := time.Now()

	for id, pair := range ac.pendingDevicePairs {
		if now.After(pair.ExpiresAt) {
			delete(ac.pendingDevicePairs, id)
		}
	}

	for id, pair := range ac.pendingNodePairs {
		if now.After(pair.ExpiresAt) {
			delete(ac.pendingNodePairs, id)
		}
	}
}

// GeneratePairID 生成配对 ID
func GeneratePairID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate pair ID: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}
