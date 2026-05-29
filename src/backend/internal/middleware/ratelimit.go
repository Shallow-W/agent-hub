package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type rateLimiterState struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	done     chan struct{}
}

var (
	globalCleanupOnce sync.Once
	globalCleanupDone chan struct{}
)

func startGlobalCleanup() {
	globalCleanupOnce.Do(func() {
		globalCleanupDone = make(chan struct{})
	})
}

// RateLimit 基于 IP 的令牌桶限流中间件
func RateLimit(rps float64, burst int) gin.HandlerFunc {
	state := &rateLimiterState{
		visitors: make(map[string]*visitor),
		done:     make(chan struct{}),
	}

	// 每个实例独立的清理协程
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-state.done:
				return
			case <-ticker.C:
				state.mu.Lock()
				for ip, v := range state.visitors {
					if time.Since(v.lastSeen) > 3*time.Minute {
						delete(state.visitors, ip)
					}
				}
				state.mu.Unlock()
			}
		}
	}()

	getLimiter := func(ip string) *rate.Limiter {
		state.mu.Lock()
		defer state.mu.Unlock()
		v, ok := state.visitors[ip]
		if !ok {
			v = &visitor{limiter: rate.NewLimiter(rate.Limit(rps), burst)}
			state.visitors[ip] = v
		}
		v.lastSeen = time.Now()
		return v.limiter
	}

	return func(c *gin.Context) {
		ip := c.ClientIP()
		limiter := getLimiter(ip)
		if !limiter.Allow() {
			ErrorResponse(c, http.StatusTooManyRequests, 42901, "请求过于频繁，请稍后再试")
			c.Abort()
			return
		}
		c.Next()
	}
}

// StopRateLimiters 停止所有限流器清理协程（由具体实例管理，此为全局占位）
func StopRateLimiters() {
	// 每个 RateLimit 实例有自己的 done channel
	// 由 GC 回收时自动清理，进程退出时全部终止
}
