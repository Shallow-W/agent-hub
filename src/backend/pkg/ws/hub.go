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
