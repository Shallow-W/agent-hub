package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/agent-hub/backend/internal/service"
	"github.com/agent-hub/backend/pkg/ws"
	"github.com/gin-gonic/gin"
	"nhooyr.io/websocket"
)

// WebSocketHandler WebSocket 连接处理器
type WebSocketHandler struct {
	authSvc *service.AuthService
	hub     *ws.Hub
	logger  *slog.Logger
}

// NewWebSocketHandler 创建 WebSocket 处理器
func NewWebSocketHandler(authSvc *service.AuthService, hub *ws.Hub, logger *slog.Logger) *WebSocketHandler {
	return &WebSocketHandler{authSvc: authSvc, hub: hub, logger: logger}
}

// Handle 处理 WebSocket 升级请求
func (h *WebSocketHandler) Handle(c *gin.Context) {
	// 从 query 参数提取 token（浏览器 WebSocket 无法设置自定义 Header）
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40110, "message": "缺少 token 参数", "data": nil})
		return
	}

	userID, err := h.authSvc.ValidateToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40111, "message": "无效 token", "data": nil})
		return
	}

	// 升级为 WebSocket 连接
	conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
		OriginPatterns: []string{"localhost:*"},
	})
	if err != nil {
		h.logger.Error("websocket accept failed", "error", err)
		return
	}

	wsConn := &ws.Connection{Conn: conn, UserID: userID}
	h.hub.Register(wsConn)

	// 为连接创建独立 context，连接关闭时取消所有子操作
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动心跳检测
	go ws.StartPing(ctx, wsConn, h.hub, 30*time.Second)

	// 消费循环，阻塞直到连接断开
	h.readLoop(ctx, wsConn)
}

// readLoop 持续读取客户端消息
func (h *WebSocketHandler) readLoop(ctx context.Context, conn *ws.Connection) {
	defer h.hub.Unregister(conn)

	for {
		_, data, err := conn.Conn.Read(ctx)
		if err != nil {
			// 正常关闭或 context 取消
			h.logger.Debug("websocket read end", "user_id", conn.UserID, "error", err)
			return
		}

		// 解析消息并路由
		var msg map[string]interface{}
		if err := json.Unmarshal(data, &msg); err != nil {
			h.logger.Warn("invalid ws message", "error", err)
			continue
		}

		msgType, _ := msg["type"].(string)
		h.logger.Debug("ws message received", "user_id", conn.UserID, "type", msgType)

		// 当前阶段仅做消息路由预留，具体业务逻辑后续迭代
		switch msgType {
		default:
			// 未识别的消息类型，原样广播回用户
			h.hub.SendToUser(conn.UserID, ws.WSMessage{
				Type: ws.TypeError,
				Data: map[string]string{"message": "未识别的消息类型: " + msgType},
			})
		}
	}
}
