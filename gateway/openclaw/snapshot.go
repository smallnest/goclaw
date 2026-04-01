package openclaw

import (
	"encoding/json"
	"sync"
	"time"
)

// Snapshot 状态快照
type Snapshot struct {
	Presence        []PresenceEntry  `json:"presence"`
	Health          HealthStatus     `json:"health"`
	StateVersion    StateVersion     `json:"stateVersion"`
	UptimeMs        int64            `json:"uptimeMs"`
	ConfigPath      string           `json:"configPath,omitempty"`
	StateDir        string           `json:"stateDir,omitempty"`
	SessionDefaults *SessionDefaults `json:"sessionDefaults,omitempty"`
	AuthMode        string           `json:"authMode,omitempty"`
	UpdateAvailable *UpdateInfo      `json:"updateAvailable,omitempty"`
	Timestamp       int64            `json:"timestamp"`
}

// SnapshotManager 快照管理器
type SnapshotManager struct {
	mu              sync.RWMutex
	presence        map[string]*PresenceEntry
	health          HealthStatus
	stateVersion    StateVersion
	startTime       time.Time
	configPath      string
	stateDir        string
	sessionDefaults *SessionDefaults
	authMode        string
	updateAvailable *UpdateInfo
	changeListeners []chan Snapshot
}

// NewSnapshotManager 创建快照管理器
func NewSnapshotManager() *SnapshotManager {
	return &SnapshotManager{
		presence:        make(map[string]*PresenceEntry),
		stateVersion:    StateVersion{Presence: ptrInt64(0), Health: ptrInt64(0)},
		startTime:       time.Now(),
		authMode:        "none",
		changeListeners: make([]chan Snapshot, 0),
	}
}

// GetSnapshot 获取快照
func (sm *SnapshotManager) GetSnapshot() *Snapshot {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	presence := make([]PresenceEntry, 0, len(sm.presence))
	for _, entry := range sm.presence {
		presence = append(presence, *entry)
	}

	return &Snapshot{
		Presence:        presence,
		Health:          sm.health,
		StateVersion:    sm.stateVersion,
		UptimeMs:        time.Since(sm.startTime).Milliseconds(),
		ConfigPath:      sm.configPath,
		StateDir:        sm.stateDir,
		SessionDefaults: sm.sessionDefaults,
		AuthMode:        sm.authMode,
		UpdateAvailable: sm.updateAvailable,
		Timestamp:       time.Now().Unix(),
	}
}

// UpdatePresence 更新在线状态
func (sm *SnapshotManager) UpdatePresence(key string, entry *PresenceEntry) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.presence[key] = entry
	sm.incrementPresenceVersion()
	sm.notifyChange()
}

// RemovePresence 移除在线状态
func (sm *SnapshotManager) RemovePresence(key string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, ok := sm.presence[key]; ok {
		delete(sm.presence, key)
		sm.incrementPresenceVersion()
		sm.notifyChange()
	}
}

// GetPresence 获取在线状态
func (sm *SnapshotManager) GetPresence(key string) (*PresenceEntry, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	entry, ok := sm.presence[key]
	if !ok {
		return nil, false
	}
	// Return a copy
	copy := *entry
	return &copy, true
}

// ListPresence 列出所有在线状态
func (sm *SnapshotManager) ListPresence() []PresenceEntry {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make([]PresenceEntry, 0, len(sm.presence))
	for _, entry := range sm.presence {
		result = append(result, *entry)
	}
	return result
}

// UpdateHealth 更新健康状态
func (sm *SnapshotManager) UpdateHealth(health HealthStatus) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.health = health
	sm.incrementHealthVersion()
	sm.notifyChange()
}

// GetHealth 获取健康状态
func (sm *SnapshotManager) GetHealth() HealthStatus {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return sm.health
}

// GetStateVersion 获取状态版本
func (sm *SnapshotManager) GetStateVersion() StateVersion {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return sm.stateVersion
}

// GetStateVersionPtr 获取状态版本指针
func (sm *SnapshotManager) GetStateVersionPtr() *StateVersion {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// 返回副本
	copy := sm.stateVersion
	return &copy
}

// SetConfigPath 设置配置路径
func (sm *SnapshotManager) SetConfigPath(path string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.configPath = path
}

// SetStateDir 设置状态目录
func (sm *SnapshotManager) SetStateDir(dir string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.stateDir = dir
}

// SetSessionDefaults 设置会话默认配置
func (sm *SnapshotManager) SetSessionDefaults(defaults *SessionDefaults) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.sessionDefaults = defaults
}

// SetAuthMode 设置认证模式
func (sm *SnapshotManager) SetAuthMode(mode string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.authMode = mode
}

// SetUpdateAvailable 设置更新信息
func (sm *SnapshotManager) SetUpdateAvailable(info *UpdateInfo) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.updateAvailable = info
}

// SubscribeChanges 订阅快照变化
func (sm *SnapshotManager) SubscribeChanges() chan Snapshot {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	ch := make(chan Snapshot, 10)
	sm.changeListeners = append(sm.changeListeners, ch)
	return ch
}

// UnsubscribeChanges 取消订阅
func (sm *SnapshotManager) UnsubscribeChanges(ch chan Snapshot) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for i, listener := range sm.changeListeners {
		if listener == ch {
			sm.changeListeners = append(sm.changeListeners[:i], sm.changeListeners[i+1:]...)
			close(ch)
			break
		}
	}
}

// notifyChange 通知变化
func (sm *SnapshotManager) notifyChange() {
	snapshot := sm.GetSnapshot()
	snapshotJSON, _ := json.Marshal(snapshot)

	for _, ch := range sm.changeListeners {
		select {
		case ch <- *snapshot:
		default:
			// Channel full, skip
		}
	}
	_ = snapshotJSON
}

// incrementPresenceVersion 增加 presence 版本
func (sm *SnapshotManager) incrementPresenceVersion() {
	if sm.stateVersion.Presence == nil {
		sm.stateVersion.Presence = ptrInt64(0)
	}
	*sm.stateVersion.Presence++
}

// incrementHealthVersion 增加 health 版本
func (sm *SnapshotManager) incrementHealthVersion() {
	if sm.stateVersion.Health == nil {
		sm.stateVersion.Health = ptrInt64(0)
	}
	*sm.stateVersion.Health++
}

// ptrInt64 返回 int64 指针
func ptrInt64(v int64) *int64 {
	return &v
}

// ptrString 返回 string 指针
func ptrString(v string) *string {
	return &v
}

// NewPresenceEntry 创建在线状态条目
func NewPresenceEntry(connID string, client ClientInfo, clientIP string) *PresenceEntry {
	now := time.Now().Unix()

	return &PresenceEntry{
		Host:     ptrString(client.ID),
		IP:       ptrString(clientIP),
		Version:  ptrString(client.Version),
		Platform: ptrString(client.Platform),
		Mode:     ptrString(client.Mode),
		Ts:       now,
		DeviceID: ptrString(client.InstanceID),
		Roles:    []string{},
		Scopes:   []string{},
	}
}
