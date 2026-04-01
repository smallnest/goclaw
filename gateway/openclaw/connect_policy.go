package openclaw

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ConnectPolicy 连接策略
type ConnectPolicy struct {
	// 配置
	MaxPayload       int64
	MaxBufferedBytes int64
	TickIntervalMs   int64

	// 安全设置
	CheckOrigin    bool
	AllowedOrigins []string
	AllowedIPs     []string
	BlockedIPs     []string

	// 速率限制
	EnableRateLimit           bool
	MaxConnectionsPerIP       int
	ConnectionCleanupInterval time.Duration

	// 状态
	mu                sync.RWMutex
	ipConnectionCount map[string]int
	connections       map[string]*ConnectionStats
}

// ConnectionStats 连接统计
type ConnectionStats struct {
	ConnID           string
	RemoteAddr       string
	ConnectedAt      time.Time
	LastActivityAt   time.Time
	MessagesSent     int64
	MessagesReceived int64
	BytesSent        int64
	BytesReceived    int64
	BufferedBytes    int64
}

// NewConnectPolicy 创建连接策略
func NewConnectPolicy() *ConnectPolicy {
	return &ConnectPolicy{
		MaxPayload:       10 * 1024 * 1024,  // 10MB
		MaxBufferedBytes: 100 * 1024 * 1024, // 100MB
		TickIntervalMs:   30000,             // 30秒

		CheckOrigin:    true,
		AllowedOrigins: []string{},
		AllowedIPs:     []string{},
		BlockedIPs:     []string{},

		EnableRateLimit:           true,
		MaxConnectionsPerIP:       10,
		ConnectionCleanupInterval: 5 * time.Minute,

		ipConnectionCount: make(map[string]int),
		connections:       make(map[string]*ConnectionStats),
	}
}

// SetMaxPayload 设置最大负载
func (cp *ConnectPolicy) SetMaxPayload(max int64) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.MaxPayload = max
}

// SetMaxBufferedBytes 设置最大缓冲字节数
func (cp *ConnectPolicy) SetMaxBufferedBytes(max int64) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.MaxBufferedBytes = max
}

// SetTickInterval 设置心跳间隔
func (cp *ConnectPolicy) SetTickInterval(ms int64) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.TickIntervalMs = ms
}

// SetAllowedOrigins 设置允许的来源
func (cp *ConnectPolicy) SetAllowedOrigins(origins []string) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.AllowedOrigins = origins
}

// SetAllowedIPs 设置允许的 IP
func (cp *ConnectPolicy) SetAllowedIPs(ips []string) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.AllowedIPs = ips
}

// SetBlockedIPs 设置阻止的 IP
func (cp *ConnectPolicy) SetBlockedIPs(ips []string) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.BlockedIPs = ips
}

// SetMaxConnectionsPerIP 设置每个 IP 最大连接数
func (cp *ConnectPolicy) SetMaxConnectionsPerIP(max int) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.MaxConnectionsPerIP = max
}

// CheckOriginFunc 检查 Origin 的函数
func (cp *ConnectPolicy) CheckOriginFunc(r *http.Request) bool {
	if !cp.CheckOrigin {
		return true
	}

	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // 允许没有 Origin 的请求（如直接连接）
	}

	cp.mu.RLock()
	defer cp.mu.RUnlock()

	// 如果没有配置允许的来源，则拒绝所有跨域请求
	if len(cp.AllowedOrigins) == 0 {
		return false
	}

	for _, allowed := range cp.AllowedOrigins {
		if origin == allowed || strings.HasSuffix(origin, allowed) {
			return true
		}
	}

	return false
}

// CheckIP 检查 IP 是否被允许
func (cp *ConnectPolicy) CheckIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	cp.mu.RLock()
	defer cp.mu.RUnlock()

	// 检查阻止列表
	for _, blocked := range cp.BlockedIPs {
		if _, blockedIP, err := net.ParseCIDR(blocked); err == nil {
			if blockedIP.Contains(ip) {
				return false
			}
		} else if blocked == ipStr {
			return false
		}
	}

	// 如果没有配置允许列表，则允许所有
	if len(cp.AllowedIPs) == 0 {
		return true
	}

	// 检查允许列表
	for _, allowed := range cp.AllowedIPs {
		if _, allowedIP, err := net.ParseCIDR(allowed); err == nil {
			if allowedIP.Contains(ip) {
				return true
			}
		} else if allowed == ipStr {
			return true
		}
	}

	return false
}

// CanConnect 检查是否允许连接
func (cp *ConnectPolicy) CanConnect(ip string) bool {
	if !cp.EnableRateLimit {
		return true
	}

	cp.mu.Lock()
	defer cp.mu.Unlock()

	count := cp.ipConnectionCount[ip]
	if count >= cp.MaxConnectionsPerIP {
		return false
	}

	cp.ipConnectionCount[ip]++
	return true
}

// RegisterConnection 注册连接
func (cp *ConnectPolicy) RegisterConnection(connID, remoteAddr string) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	cp.connections[connID] = &ConnectionStats{
		ConnID:         connID,
		RemoteAddr:     remoteAddr,
		ConnectedAt:    time.Now(),
		LastActivityAt: time.Now(),
	}
}

// UnregisterConnection 注销连接
func (cp *ConnectPolicy) UnregisterConnection(connID string) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	if conn, ok := cp.connections[connID]; ok {
		ip := conn.RemoteAddr
		if count, ok := cp.ipConnectionCount[ip]; ok && count > 0 {
			cp.ipConnectionCount[ip]--
		}
		delete(cp.connections, connID)
	}
}

// UpdateActivity 更新连接活动
func (cp *ConnectPolicy) UpdateActivity(connID string) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	if conn, ok := cp.connections[connID]; ok {
		conn.LastActivityAt = time.Now()
	}
}

// IncrementMessagesReceived 增加接收消息计数
func (cp *ConnectPolicy) IncrementMessagesReceived(connID string, bytes int64) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	if conn, ok := cp.connections[connID]; ok {
		conn.MessagesReceived++
		conn.BytesReceived += bytes
		conn.LastActivityAt = time.Now()
	}
}

// IncrementMessagesSent 增加发送消息计数
func (cp *ConnectPolicy) IncrementMessagesSent(connID string, bytes int64) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	if conn, ok := cp.connections[connID]; ok {
		conn.MessagesSent++
		conn.BytesSent += bytes
	}
}

// UpdateBufferedBytes 更新缓冲字节数
func (cp *ConnectPolicy) UpdateBufferedBytes(connID string, bytes int64) bool {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	if conn, ok := cp.connections[connID]; ok {
		conn.BufferedBytes = bytes
		return conn.BufferedBytes <= cp.MaxBufferedBytes
	}
	return true
}

// GetConnectionStats 获取连接统计
func (cp *ConnectPolicy) GetConnectionStats(connID string) (*ConnectionStats, bool) {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	conn, ok := cp.connections[connID]
	if !ok {
		return nil, false
	}

	// Return a copy
	copy := *conn
	return &copy, true
}

// GetAllConnectionStats 获取所有连接统计
func (cp *ConnectPolicy) GetAllConnectionStats() []*ConnectionStats {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	stats := make([]*ConnectionStats, 0, len(cp.connections))
	for _, conn := range cp.connections {
		copy := *conn
		stats = append(stats, &copy)
	}
	return stats
}

// GetConnectionCount 获取连接总数
func (cp *ConnectPolicy) GetConnectionCount() int {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	return len(cp.connections)
}

// GetIPConnectionCount 获取 IP 连接数
func (cp *ConnectPolicy) GetIPConnectionCount(ip string) int {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	return cp.ipConnectionCount[ip]
}

// CleanupIdleConnections 清理空闲连接
func (cp *ConnectPolicy) CleanupIdleConnections(idleTimeout time.Duration) []string {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	now := time.Now()
	var toRemove []string

	for connID, conn := range cp.connections {
		if now.Sub(conn.LastActivityAt) > idleTimeout {
			toRemove = append(toRemove, connID)
		}
	}

	return toRemove
}

// StartCleanupTask 启动清理任务
func (cp *ConnectPolicy) StartCleanupTask(idleTimeout time.Duration) func() {
	ticker := time.NewTicker(cp.ConnectionCleanupInterval)
	stop := make(chan struct{})

	go func() {
		for {
			select {
			case <-ticker.C:
				// 清理过期的配对请求由 AuthContext 处理
				// 这里可以添加其他清理逻辑
			case <-stop:
				ticker.Stop()
				return
			}
		}
	}()

	return func() {
		close(stop)
	}
}

// ExtractIP 从 RemoteAddr 提取 IP
func ExtractIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

// IsLocalIP 检查是否是本地 IP
func IsLocalIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	// 检查 IPv4 loopback
	if ip.To4() != nil {
		return ip.IsLoopback()
	}

	// 检查 IPv6 loopback
	return ip.IsLoopback()
}

// IsPrivateIP 检查是否是私有 IP
func IsPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	// 检查私有 IP 范围
	privateBlocks := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"fc00::/7", // IPv6 私有地址
	}

	for _, block := range privateBlocks {
		_, cidr, err := net.ParseCIDR(block)
		if err == nil && cidr.Contains(ip) {
			return true
		}
	}

	return false
}
