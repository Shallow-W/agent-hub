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

var rateLimiters struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	done     chan struct{}
	once     sync.Once
}

// RateLimit 基于 IP 的令牌桶限流中间件
func RateLimit(rps float64, burst int) gin.HandlerFunc {
	rateLimiters.mu.Lock()
	defer rateLimiters.mu.Unlock()

	if rateLimiters.visitors == nil {
		rateLimiters.visitors = make(map[string]*visitor)
		rateLimiters.done = make(chan struct{})

		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-rateLimiters.done:
					return
				case <-ticker.C:
					rateLimiters.mu.Lock()
					for ip, v := range rateLimiters.visitors {
						if time.Since(v.lastSeen) > 3*time.Minute {
							delete(rateLimiters.visitors, ip)
						}
					}
					rateLimiters.mu.Unlock()
				}
			}
		}()
	}

	getLimiter := func(ip string) *rate.Limiter {
		rateLimiters.mu.Lock()
		defer rateLimiters.mu.Unlock()
		v, ok := rateLimiters.visitors[ip]
		if !ok {
			v = &visitor{limiter: rate.NewLimiter(rate.Limit(rps), burst)}
			rateLimiters.visitors[ip] = v
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

// StopRateLimiter 停止限流器后台清理协程（shutdown 时调用）
func StopRateLimiter() {
	rateLimiters.once.Do(func() {
		if rateLimiters.done != nil {
			close(rateLimiters.done)
		}
	})
}
