package openclaw

import (
	"sync"
	"time"
)

// RateLimiter 速率限制器
type RateLimiter struct {
	mu              sync.RWMutex
	limiters        map[string]*tokenBucketLimiter
	globalLimit     *tokenBucketLimiter
	cleanupInterval time.Duration
	lastCleanup     time.Time
}

// tokenBucketLimiter 令牌桶限流器
type tokenBucketLimiter struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

// NewRateLimiter 创建速率限制器
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		limiters:        make(map[string]*tokenBucketLimiter),
		globalLimit:     newTokenBucket(1000, 100), // 全局：1000 tokens，100/s
		cleanupInterval: 5 * time.Minute,
		lastCleanup:     time.Now(),
	}
}

// newTokenBucket 创建令牌桶
func newTokenBucket(maxTokens, refillRate float64) *tokenBucketLimiter {
	return &tokenBucketLimiter{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Allow 检查是否允许请求（控制平面写操作）
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// 定期清理
	rl.cleanupIfNeeded()

	// 检查全局限制
	if !rl.globalLimit.allow() {
		return false
	}

	// 获取或创建限流器
	limiter, ok := rl.limiters[key]
	if !ok {
		// 每个连接：10 tokens，1/s（控制平面写操作）
		limiter = newTokenBucket(10, 1)
		rl.limiters[key] = limiter
	}

	return limiter.allow()
}

// allow 令牌桶检查
func (tb *tokenBucketLimiter) allow() bool {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()

	// 补充令牌
	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
	tb.lastRefill = now

	// 检查是否有令牌
	if tb.tokens >= 1 {
		tb.tokens -= 1
		return true
	}

	return false
}

// cleanupIfNeeded 清理过期的限流器
func (rl *RateLimiter) cleanupIfNeeded() {
	if time.Since(rl.lastCleanup) < rl.cleanupInterval {
		return
	}

	now := time.Now()
	for key, limiter := range rl.limiters {
		// 超过10分钟没有活动的限流器可以被清理
		if now.Sub(limiter.lastRefill) > 10*time.Minute {
			delete(rl.limiters, key)
		}
	}
	rl.lastCleanup = now
}

// Remove 移除限流器
func (rl *RateLimiter) Remove(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.limiters, key)
}

// GetStats 获取统计信息
func (rl *RateLimiter) GetStats(key string) (tokens, maxTokens float64, ok bool) {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	limiter, exists := rl.limiters[key]
	if !exists {
		return 0, 0, false
	}

	return limiter.tokens, limiter.maxTokens, true
}

// Reset 重置限流器
func (rl *RateLimiter) Reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if limiter, ok := rl.limiters[key]; ok {
		limiter.tokens = limiter.maxTokens
		limiter.lastRefill = time.Now()
	}
}

// SetLimit 设置限流器限制
func (rl *RateLimiter) SetLimit(key string, maxTokens, refillRate float64) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.limiters[key] = newTokenBucket(maxTokens, refillRate)
}

// GetGlobalStats 获取全局统计
func (rl *RateLimiter) GetGlobalStats() (tokens, maxTokens float64) {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	return rl.globalLimit.tokens, rl.globalLimit.maxTokens
}

// SetGlobalLimit 设置全局限制
func (rl *RateLimiter) SetGlobalLimit(maxTokens, refillRate float64) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.globalLimit = newTokenBucket(maxTokens, refillRate)
}
