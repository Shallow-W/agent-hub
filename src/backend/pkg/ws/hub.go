package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"nhooyr.io/websocket"
)

// 消息类型常量
const (
	TypeMessageStreaming = "message.streaming"
	TypeMessageComplete  = "message.complete"
	TypeAgentStatus      = "agent.status"
	TypeError            = "error"
)

// WSMessage WebSocket 消息格式
type WSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// Connection 封装单个 WebSocket 连接
type Connection struct {
	Conn   *websocket.Conn
	UserID string
	mu     sync.Mutex
}

// Send 向连接发送 JSON 消息（线程安全）
func (c *Connection) Send(ctx context.Context, msg WSMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return c.Conn.Write(ctx, websocket.MessageText, data)
}

// Hub 管理所有 WebSocket 连接
type Hub struct {
	// userID -> 该用户的所有连接（支持多设备）
	connections map[string][]*Connection
	mu          sync.RWMutex

	register   chan *Connection
	unregister chan *Connection
	broadcast  chan WSMessage

	// 向特定用户发送消息的通道
	sendToUser chan userMessage

	// 房间（会话）支持：conversationID -> 连接集合
	rooms     map[string]map[*Connection]bool
	joinRoom  chan roomAction
	leaveRoom chan roomAction
	// 向房间内所有连接发送消息
	sendToRoom chan roomMessage

	logger *slog.Logger
}

type userMessage struct {
	userID string
	msg    WSMessage
}

// NewHub 创建 Hub 实例
func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		connections: make(map[string][]*Connection),
		register:    make(chan *Connection),
		unregister:  make(chan *Connection),
		broadcast:   make(chan WSMessage, 64),
		sendToUser:  make(chan userMessage, 64),
		rooms:       make(map[string]map[*Connection]bool),
		joinRoom:    make(chan roomAction, 64),
		leaveRoom:   make(chan roomAction, 64),
		sendToRoom:  make(chan roomMessage, 64),
		logger:      logger,
	}
}

// Run 启动 Hub 事件循环，应在独立 goroutine 中调用
func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			h.logger.Info("hub shutting down")
			return

		case conn := <-h.register:
			h.mu.Lock()
			h.connections[conn.UserID] = append(h.connections[conn.UserID], conn)
			h.mu.Unlock()
			h.logger.Info("websocket connected", "user_id", conn.UserID)

		case conn := <-h.unregister:
			h.mu.Lock()
			conns := h.connections[conn.UserID]
			for i, c := range conns {
				if c == conn {
					h.connections[conn.UserID] = append(conns[:i], conns[i+1:]...)
					break
				}
			}
			if len(h.connections[conn.UserID]) == 0 {
				delete(h.connections, conn.UserID)
			}
			// 清理该连接所在的所有房间
			for roomID, members := range h.rooms {
				if members[conn] {
					delete(members, conn)
					if len(members) == 0 {
						delete(h.rooms, roomID)
					}
				}
			}
			h.mu.Unlock()
			conn.Conn.Close(websocket.StatusNormalClosure, "disconnect")
			h.logger.Info("websocket disconnected", "user_id", conn.UserID)

		case um := <-h.sendToUser:
			h.mu.RLock()
			conns := h.connections[um.userID]
			// 复制引用避免长持锁
			toSend := make([]*Connection, len(conns))
			copy(toSend, conns)
			h.mu.RUnlock()
			for _, c := range toSend {
				if err := c.Send(ctx, um.msg); err != nil {
					h.logger.Warn("send to user failed", "user_id", um.userID, "error", err)
				}
			}

		case msg := <-h.broadcast:
			h.mu.RLock()
			for _, conns := range h.connections {
				for _, c := range conns {
					if err := c.Send(ctx, msg); err != nil {
						h.logger.Warn("broadcast send failed", "error", err)
					}
				}
			}
			h.mu.RUnlock()

		case ra := <-h.joinRoom:
			h.mu.Lock()
			if h.rooms[ra.ConversationID] == nil {
				h.rooms[ra.ConversationID] = make(map[*Connection]bool)
			}
			h.rooms[ra.ConversationID][ra.Conn] = true
			h.mu.Unlock()
			h.logger.Info("joined room", "conversation_id", ra.ConversationID, "user_id", ra.Conn.UserID)

		case ra := <-h.leaveRoom:
			h.mu.Lock()
			if members, ok := h.rooms[ra.ConversationID]; ok {
				delete(members, ra.Conn)
				if len(members) == 0 {
					delete(h.rooms, ra.ConversationID)
				}
			}
			h.mu.Unlock()
			h.logger.Info("left room", "conversation_id", ra.ConversationID, "user_id", ra.Conn.UserID)

		case rm := <-h.sendToRoom:
			h.mu.RLock()
			members := h.rooms[rm.ConversationID]
			toSend := make([]*Connection, 0, len(members))
			for c := range members {
				toSend = append(toSend, c)
			}
			h.mu.RUnlock()
			for _, c := range toSend {
				if err := c.Send(ctx, rm.Message); err != nil {
					h.logger.Warn("send to room failed", "conversation_id", rm.ConversationID, "user_id", c.UserID, "error", err)
				}
			}
		}
	}
}

// Register 注册连接
func (h *Hub) Register(conn *Connection) {
	h.register <- conn
}

// Unregister 注销连接
func (h *Hub) Unregister(conn *Connection) {
	h.unregister <- conn
}

// SendToUser 向指定用户的所有连接发送消息
func (h *Hub) SendToUser(userID string, msg WSMessage) {
	h.sendToUser <- userMessage{userID: userID, msg: msg}
}

// Broadcast 向所有在线用户广播消息
func (h *Hub) Broadcast(msg WSMessage) {
	h.broadcast <- msg
}

// IsOnline 检查用户是否在线
func (h *Hub) IsOnline(userID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.connections[userID]) > 0
}

// JoinRoom 将连接加入指定会话房间
func (h *Hub) JoinRoom(conversationID string, conn *Connection) {
	h.joinRoom <- roomAction{ConversationID: conversationID, Conn: conn}
}

// LeaveRoom 将连接移出指定会话房间
func (h *Hub) LeaveRoom(conversationID string, conn *Connection) {
	h.leaveRoom <- roomAction{ConversationID: conversationID, Conn: conn}
}

// SendToRoom 向指定会话房间的所有连接发送消息
func (h *Hub) SendToRoom(conversationID string, msg WSMessage) {
	h.sendToRoom <- roomMessage{ConversationID: conversationID, Message: msg}
}

// StartPing 启动心跳检测，每 30s 发送 ping，超时断开
func StartPing(ctx context.Context, conn *Connection, hub *Hub, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := conn.Conn.Ping(ctx); err != nil {
				hub.Unregister(conn)
				return
			}
		}
	}
}
