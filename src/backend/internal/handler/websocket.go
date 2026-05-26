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
	authSvc        *service.AuthService
	hub            *ws.Hub
	memberChecker  MemberChecker
	logger         *slog.Logger
	allowedOrigins []string
}

// MemberChecker 校验用户是否为会话成员
type MemberChecker interface {
	IsConversationMember(ctx context.Context, conversationID, userID string) (bool, error)
}

// NewWebSocketHandler 创建 WebSocket 处理器
func NewWebSocketHandler(authSvc *service.AuthService, hub *ws.Hub, mc MemberChecker, logger *slog.Logger, allowedOrigins []string) *WebSocketHandler {
	return &WebSocketHandler{authSvc: authSvc, hub: hub, memberChecker: mc, logger: logger, allowedOrigins: allowedOrigins}
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
		OriginPatterns: h.allowedOrigins,
	})
	if err != nil {
		h.logger.Error("websocket accept failed", "error", err)
		return
	}

	client := ws.NewClient(conn, userID)
	h.hub.Register(client)

	// 为连接创建独立 context，连接关闭时取消所有子操作
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动写协程：从 sendCh 读取消息写入连接
	go client.WritePump(ctx)

	// 启动心跳检测
	go ws.StartHeartbeat(ctx, client, h.hub)

	// 消费循环，阻塞直到连接断开
	h.readLoop(ctx, client)
}

// readLoop 持续读取客户端消息
func (h *WebSocketHandler) readLoop(ctx context.Context, client *ws.Client) {
	defer h.hub.Unregister(client)

	for {
		_, data, err := client.Conn.Read(ctx)
		if err != nil {
			// 正常关闭或 context 取消
			h.logger.Debug("websocket read end", "user_id", client.UserID, "error", err)
			return
		}

		client.LastActive = time.Now()

		// 解析消息并路由
		var msg ws.WSMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			h.logger.Warn("invalid ws message", "error", err)
			continue
		}

		h.logger.Debug("ws message received", "user_id", client.UserID, "type", msg.Type)

		switch msg.Type {
		case "join_room":
			var payload struct {
				ConversationID string `json:"conversation_id"`
			}
			if raw, err := json.Marshal(msg.Data); err == nil {
				_ = json.Unmarshal(raw, &payload)
			}
			if payload.ConversationID != "" {
				if ok, _ := h.memberChecker.IsConversationMember(ctx, payload.ConversationID, client.UserID); !ok {
					h.hub.SendToUser(client.UserID, ws.WSMessage{
						Type: ws.TypeError,
						Data: map[string]string{"message": "无权加入该会话"},
					})
					continue
				}
				h.hub.JoinRoom(payload.ConversationID, client)
			}
		case "leave_room":
			var payload struct {
				ConversationID string `json:"conversation_id"`
			}
			if raw, err := json.Marshal(msg.Data); err == nil {
				_ = json.Unmarshal(raw, &payload)
			}
			if payload.ConversationID != "" {
				h.hub.LeaveRoom(payload.ConversationID, client)
			}
		case "chat":
			var payload struct {
				ConversationID string `json:"conversation_id"`
				Content        string `json:"content"`
			}
			if raw, err := json.Marshal(msg.Data); err == nil {
				_ = json.Unmarshal(raw, &payload)
			}
			if payload.ConversationID != "" {
				if ok, _ := h.memberChecker.IsConversationMember(ctx, payload.ConversationID, client.UserID); !ok {
					h.hub.SendToUser(client.UserID, ws.WSMessage{
						Type: ws.TypeError,
						Data: map[string]string{"message": "无权向该会话发送消息"},
					})
					continue
				}
				h.hub.SendToRoom(payload.ConversationID, ws.WSMessage{
					Type: ws.TypeMessageComplete,
					Data: msg.Data,
				})
			}
		default:
			h.hub.SendToUser(client.UserID, ws.WSMessage{
				Type: ws.TypeError,
				Data: map[string]string{"message": "未识别的消息类型: " + msg.Type},
			})
		}
	}
}
