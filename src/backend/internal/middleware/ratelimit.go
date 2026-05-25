package middleware

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimit 基于 IP 的令牌桶限流中间件
// rps: 每秒允许的请求数，burst: 突发上限
func RateLimit(rps float64, burst int) gin.HandlerFunc {
	var mu sync.Mutex
	visitors := make(map[string]*rate.Limiter)

	getLimiter := func(ip string) *rate.Limiter {
		mu.Lock()
		defer mu.Unlock()
		limiter, ok := visitors[ip]
		if !ok {
			limiter = rate.NewLimiter(rate.Limit(rps), burst)
			visitors[ip] = limiter
		}
		return limiter
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
