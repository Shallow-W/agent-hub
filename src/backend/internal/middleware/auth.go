package middleware

import (
	"net/http"
	"strings"

	"fmt"

	"github.com/agent-hub/backend/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// JWT 中间件所需的配置
type JWTConfig struct {
	Secret string
}

// Auth 返回 JWT 鉴权中间件
func Auth(cfg JWTConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			ErrorResponse(c, http.StatusUnauthorized, 40101, "缺少 Authorization 头")
			c.Abort()
			return
		}

		// 提取 Bearer token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			ErrorResponse(c, http.StatusUnauthorized, 40102, "Authorization 格式错误，应为 Bearer <token>")
			c.Abort()
			return
		}

		token, err := jwt.Parse(parts[1], func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return []byte(cfg.Secret), nil
		})
		if err != nil || !token.Valid {
			ErrorResponse(c, http.StatusUnauthorized, 40103, "无效或过期的 token")
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			ErrorResponse(c, http.StatusUnauthorized, 40104, "token claims 解析失败")
			c.Abort()
			return
		}

		userID, ok := claims["user_id"].(string)
		if !ok {
			ErrorResponse(c, http.StatusUnauthorized, 40105, "token 中缺少 user_id")
			c.Abort()
			return
		}

		// 将用户信息注入上下文
		c.Set("user_id", userID)
		username, _ := claims["username"].(string)
	c.Set("username", username)
		c.Next()
	}
}

// GetUserID 从上下文提取用户 ID
func GetUserID(c *gin.Context) string {
	v, _ := c.Get("user_id")
	id, _ := v.(string)
	return id
}

// GetUser 从上下文提取完整用户信息
func GetUser(c *gin.Context) *model.User {
	v, _ := c.Get("user")
	user, _ := v.(*model.User)
	return user
}
