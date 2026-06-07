package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/agent-hub/backend/internal/model"
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
	msgSender      WSMessageSender
	logger         *slog.Logger
	allowedOrigins []string
}

// MemberChecker 校验用户是否为会话成员
type MemberChecker interface {
	IsConversationMember(ctx context.Context, conversationID, userID string) (bool, error)
}

// WSMessageSender WS 消息持久化接口
type WSMessageSender interface {
	SendMessage(ctx context.Context, convID, userID, role, content, artifactsJSON string, attachments []model.MessageAttachment) (*service.SendMessageResult, error)
	SendMessageWithMentions(ctx context.Context, convID, userID, role, content, artifactsJSON string, attachments []model.MessageAttachment, mentions []string) (*service.SendMessageResult, error)
}

// NewWebSocketHandler 创建 WebSocket 处理器
func NewWebSocketHandler(authSvc *service.AuthService, hub *ws.Hub, mc MemberChecker, ms WSMessageSender, logger *slog.Logger, allowedOrigins []string) *WebSocketHandler {
	return &WebSocketHandler{authSvc: authSvc, hub: hub, memberChecker: mc, msgSender: ms, logger: logger, allowedOrigins: allowedOrigins}
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
	if u, err := h.authSvc.GetUserByID(c.Request.Context(), userID); err == nil && u != nil {
		client.SetUsername(u.Username)
	}
	conn.SetReadLimit(1 << 17) // 128KB max WS frame
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

		client.UpdateLastActive()

		// 解析消息并路由
		var msg ws.WSMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			h.logger.Warn("invalid ws message", "error", err)
			h.hub.SendToUser(client.UserID, ws.WSMessage{
				Type: ws.TypeError,
				Data: map[string]string{"message": "消息格式错误"},
			})
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
				ConversationID string   `json:"conversation_id"`
				Content        string   `json:"content"`
				Mentions       []string `json:"mentions"`
			}
			if raw, err := json.Marshal(msg.Data); err == nil {
				if err := json.Unmarshal(raw, &payload); err != nil {
					h.hub.SendToUser(client.UserID, ws.WSMessage{
						Type: ws.TypeError,
						Data: map[string]string{"message": "chat 消息格式错误"},
					})
					continue
				}
			}
			if payload.ConversationID == "" || payload.Content == "" {
				h.hub.SendToUser(client.UserID, ws.WSMessage{
					Type: ws.TypeError,
					Data: map[string]string{"message": "缺少 conversation_id 或 content"},
				})
				continue
			}
			if len(payload.Content) > 10000 {
				h.hub.SendToUser(client.UserID, ws.WSMessage{
					Type: ws.TypeError,
					Data: map[string]string{"message": "消息内容过长"},
				})
				continue
			}
			if ok, _ := h.memberChecker.IsConversationMember(ctx, payload.ConversationID, client.UserID); !ok {
				h.hub.SendToUser(client.UserID, ws.WSMessage{
					Type: ws.TypeError,
					Data: map[string]string{"message": "无权向该会话发送消息"},
				})
				continue
			}
			// 通过 Service 持久化（内部触发 Hub 推送 + Redis 缓存）
			if h.msgSender != nil {
				if _, err := h.msgSender.SendMessageWithMentions(ctx, payload.ConversationID, client.UserID, "user", payload.Content, "", nil, payload.Mentions); err != nil {
					h.logger.Error("ws message persist failed", "conversation_id", payload.ConversationID, "user_id", client.UserID, "error", err)
					h.hub.SendToUser(client.UserID, ws.WSMessage{
						Type: ws.TypeError,
						Data: map[string]string{"message": "消息发送失败，请重试"},
					})
				}
			}
		case "typing_start":
			var payload struct {
				ConversationID string `json:"conversation_id"`
			}
			if raw, err := json.Marshal(msg.Data); err == nil {
				_ = json.Unmarshal(raw, &payload)
			}
			if payload.ConversationID != "" {
				if ok, _ := h.memberChecker.IsConversationMember(ctx, payload.ConversationID, client.UserID); !ok {
					continue
				}
				h.hub.SendToRoomExcept(payload.ConversationID, client, ws.WSMessage{
					Type: "user.typing_start",
					Data: map[string]string{
						"user_id":         client.UserID,
						"username":        client.Username,
						"conversation_id": payload.ConversationID,
					},
				})
			}
		case "typing_stop":
			var payload struct {
				ConversationID string `json:"conversation_id"`
			}
			if raw, err := json.Marshal(msg.Data); err == nil {
				_ = json.Unmarshal(raw, &payload)
			}
			if payload.ConversationID != "" {
				if ok, _ := h.memberChecker.IsConversationMember(ctx, payload.ConversationID, client.UserID); !ok {
					continue
				}
				h.hub.SendToRoomExcept(payload.ConversationID, client, ws.WSMessage{
					Type: "user.typing_stop",
					Data: map[string]string{
						"user_id":         client.UserID,
						"username":        client.Username,
						"conversation_id": payload.ConversationID,
					},
				})
			}
		case "user.stop_stream":
			// 前端停止生成按钮：清除流式状态并通知房间
			var payload struct {
				ConversationID string `json:"conversation_id"`
			}
			if raw, err := json.Marshal(msg.Data); err == nil {
				_ = json.Unmarshal(raw, &payload)
			}
			if payload.ConversationID != "" {
				h.hub.SendToRoom(payload.ConversationID, ws.WSMessage{
					Type: "stream.stopped",
					Data: map[string]string{
						"conversation_id": payload.ConversationID,
						"user_id":         client.UserID,
					},
				})
			}
		default:
			h.hub.SendToUser(client.UserID, ws.WSMessage{
				Type: ws.TypeError,
				Data: map[string]string{"message": "未识别的消息类型"},
			})
		}
	}
}
