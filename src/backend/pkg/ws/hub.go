package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"nhooyr.io/websocket"
)

// 消息类型常量
const (
	TypeMessageStreaming = "message.streaming"
	TypeMessageComplete  = "message.complete"
	TypeAgentStatus      = "agent.status"
	TypeError            = "error"
	TypeUserOnline       = "user.online"
	TypeUserOffline      = "user.offline"
)

// BusMessage 消息总线统一消息结构
type BusMessage struct {
	Type    BusMessageType
	Payload interface{}
	Target  string // 目标：userID、conversationID 或空（全局广播）
}

// BusMessageType 消息总线消息类型
type BusMessageType int

const (
	BusRegister      BusMessageType = iota // 注册连接
	BusUnregister                          // 注销连接
	BusBroadcast                           // 全局广播
	BusRoomMsg                             // 房间消息
	BusDirectMsg                           // 私聊消息
	BusJoinRoom                            // 加入房间
	BusLeaveRoom                           // 离开房间
	BusPersistedMsg                        // 持久化消息推送（DB 已写入，需推送给会话成员）
)

// WSMessage WebSocket 消息格式
type WSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

const (
	writeBufSize       = 256
	pingInterval       = 30 * time.Second
	pongTimeout        = 60 * time.Second
	cleanupInterval    = 15 * time.Second
)

// Client 封装单个 WebSocket 连接及其元数据
type Client struct {
	Conn        *websocket.Conn
	UserID      string
	ConnectedAt time.Time
	lastPong    atomic.Int64 // unix nanos，原子操作避免数据竞争
	LastActive  time.Time

	sendCh chan []byte // 独立写缓冲通道
	mu     sync.Mutex  // 保护 Conn 的写操作（close 时排空用）
}

// NewClient 创建 Client 实例（导出供 handler 调用）
func NewClient(conn *websocket.Conn, userID string) *Client {
	now := time.Now()
	c := &Client{
		Conn:        conn,
		UserID:      userID,
		ConnectedAt: now,
		LastActive:  now,
		sendCh:      make(chan []byte, writeBufSize),
	}
	c.lastPong.Store(now.UnixNano())
	return c
}

// UpdateLastPong 原子更新最后 pong 时间
func (c *Client) UpdateLastPong() {
	c.lastPong.Store(time.Now().UnixNano())
}

// LastPongTime 返回最后 pong 时间
func (c *Client) LastPongTime() time.Time {
	return time.Unix(0, c.lastPong.Load())
}

// enqueue 将消息入队写缓冲区；满时丢弃最旧消息（背压）
func (c *Client) enqueue(data []byte) {
	select {
	case c.sendCh <- data:
	default:
		// 背压：丢弃最旧消息
		select {
		case <-c.sendCh:
		default:
		}
		c.sendCh <- data
		// 复制一份用于日志，避免引用底层缓冲区
		snippet := string(data)
		if len(snippet) > 80 {
			snippet = snippet[:80] + "..."
		}
		slog.Warn("write buffer full, dropped oldest message", "user_id", c.UserID, "msg", snippet)
	}
}

// WritePump 从 sendCh 读取消息写入连接（导出供 handler 启动）
func (c *Client) WritePump(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-c.sendCh:
			if !ok {
				return
			}
			c.mu.Lock()
			err := c.Conn.Write(ctx, websocket.MessageText, data)
			c.mu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

// Send 向客户端发送 JSON 消息（通过写缓冲区）
func (c *Client) Send(msg WSMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	c.enqueue(data)
	return nil
}

// Hub 管理所有 WebSocket 连接，基于消息总线模式
type Hub struct {
	// sync.Map 替代 map+mutex，优化高并发读写
	clients sync.Map // string -> *[]*Client（同一用户多设备）

	bus    chan BusMessage
	rooms  map[string]map[*Client]bool
	roomMu sync.RWMutex

	wg           sync.WaitGroup // 跟踪活跃连接数
	shutdownOnce sync.Once
	logger       *slog.Logger
}

// NewHub 创建 Hub 实例
func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		bus:    make(chan BusMessage, 256),
		rooms:  make(map[string]map[*Client]bool),
		logger: logger,
	}
}

// Run 启动 Hub 消息总线事件循环，应在独立 goroutine 中调用
func (h *Hub) Run(ctx context.Context) {
	cleanupTicker := time.NewTicker(cleanupInterval)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			h.shutdown()
			return

		case msg := <-h.bus:
			h.dispatch(msg)

		case <-cleanupTicker.C:
			h.cleanStaleConnections()
		}
	}
}

// dispatch 根据消息类型分发处理
func (h *Hub) dispatch(msg BusMessage) {
	switch msg.Type {
	case BusRegister:
		h.handleRegister(msg)
	case BusUnregister:
		h.handleUnregister(msg)
	case BusBroadcast:
		h.handleBroadcast(msg)
	case BusDirectMsg:
		h.handleDirectMsg(msg)
	case BusJoinRoom:
		h.handleJoinRoom(msg)
	case BusLeaveRoom:
		h.handleLeaveRoom(msg)
	case BusRoomMsg:
		h.handleRoomMsg(msg)
	case BusPersistedMsg:
		h.handlePersistedMsg(msg)
	}
}

// --- 处理逻辑（分离关注点） ---

// broadcastOnlineStatus 广播用户上/下线状态给所有连接
func (h *Hub) broadcastOnlineStatus(userID string, msgType string) {
	wsMsg := WSMessage{
		Type: msgType,
		Data: map[string]string{"user_id": userID},
	}
	h.clients.Range(func(key, value interface{}) bool {
		list := value.(*[]*Client)
		for _, c := range *list {
			if err := c.Send(wsMsg); err != nil {
				h.logger.Warn("online status broadcast failed", "target_user_id", key, "error", err)
			}
		}
		return true
	})
}

func (h *Hub) handleRegister(msg BusMessage) {
	client := msg.Payload.(*Client)
	isFirst := false
	val, loaded := h.clients.LoadOrStore(client.UserID, &[]*Client{})
	if !loaded {
		isFirst = true
	}
	list := val.(*[]*Client)
	*list = append(*list, client)
	h.logger.Info("websocket connected", "user_id", client.UserID)

	// 首次上线时广播上线状态
	if isFirst {
		h.broadcastOnlineStatus(client.UserID, TypeUserOnline)
	}
}

func (h *Hub) handleUnregister(msg BusMessage) {
	client := msg.Payload.(*Client)
	val, ok := h.clients.Load(client.UserID)
	if !ok {
		return
	}
	list := val.(*[]*Client)
	for i, c := range *list {
		if c == client {
			*list = append((*list)[:i], (*list)[i+1:]...)
			break
		}
	}

	// 所有连接断开时广播离线状态
	if len(*list) == 0 {
		h.clients.Delete(client.UserID)
		h.broadcastOnlineStatus(client.UserID, TypeUserOffline)
	}

	// 清理该连接所在的所有房间
	h.roomMu.Lock()
	for roomID, members := range h.rooms {
		if members[client] {
			delete(members, client)
			if len(members) == 0 {
				delete(h.rooms, roomID)
			}
		}
	}
	h.roomMu.Unlock()

	close(client.sendCh)
	client.Conn.Close(websocket.StatusNormalClosure, "disconnect")
	h.wg.Done()
	h.logger.Info("websocket disconnected", "user_id", client.UserID)
}

func (h *Hub) handleBroadcast(msg BusMessage) {
	wsMsg := msg.Payload.(WSMessage)
	h.clients.Range(func(key, value interface{}) bool {
		list := value.(*[]*Client)
		for _, c := range *list {
			if err := c.Send(wsMsg); err != nil {
				h.logger.Warn("broadcast send failed", "error", err)
			}
		}
		return true
	})
}

func (h *Hub) handleDirectMsg(msg BusMessage) {
	payload := msg.Payload.(*directMsgPayload)
	val, ok := h.clients.Load(payload.UserID)
	if !ok {
		return
	}
	list := val.(*[]*Client)
	for _, c := range *list {
		if err := c.Send(payload.Msg); err != nil {
			h.logger.Warn("send to user failed", "user_id", payload.UserID, "error", err)
		}
	}
}

func (h *Hub) handleJoinRoom(msg BusMessage) {
	payload := msg.Payload.(*roomAction)
	h.roomMu.Lock()
	if h.rooms[payload.ConversationID] == nil {
		h.rooms[payload.ConversationID] = make(map[*Client]bool)
	}
	h.rooms[payload.ConversationID][payload.Conn] = true
	h.roomMu.Unlock()
	h.logger.Info("joined room", "conversation_id", payload.ConversationID, "user_id", payload.Conn.UserID)
}

func (h *Hub) handleLeaveRoom(msg BusMessage) {
	payload := msg.Payload.(*roomAction)
	h.roomMu.Lock()
	if members, ok := h.rooms[payload.ConversationID]; ok {
		delete(members, payload.Conn)
		if len(members) == 0 {
			delete(h.rooms, payload.ConversationID)
		}
	}
	h.roomMu.Unlock()
	h.logger.Info("left room", "conversation_id", payload.ConversationID, "user_id", payload.Conn.UserID)
}

func (h *Hub) handleRoomMsg(msg BusMessage) {
	payload := msg.Payload.(*roomMessage)
	h.roomMu.RLock()
	members := h.rooms[payload.ConversationID]
	toSend := make([]*Client, 0, len(members))
	for c := range members {
		toSend = append(toSend, c)
	}
	h.roomMu.RUnlock()
	for _, c := range toSend {
		if err := c.Send(payload.Message); err != nil {
			h.logger.Warn("send to room failed", "conversation_id", payload.ConversationID, "user_id", c.UserID, "error", err)
		}
	}
}

// handlePersistedMsg 处理持久化消息推送：向所有会话成员推送
func (h *Hub) handlePersistedMsg(msg BusMessage) {
	payload := msg.Payload.(*persistedMsgPayload)
	wsMsg := WSMessage{Type: TypeMessageComplete, Data: payload.Message}

	// 统一通过 SendToUser 推送，避免房间+用户列表双重推送
	for _, uid := range payload.MemberIDs {
		val, ok := h.clients.Load(uid)
		if !ok {
			continue
		}
		list := val.(*[]*Client)
		for _, c := range *list {
			if err := c.Send(wsMsg); err != nil {
				h.logger.Warn("push persisted msg failed", "conversation_id", payload.ConversationID, "user_id", uid, "error", err)
			}
		}
	}
}

// cleanStaleConnections 检查超时连接并清理
func (h *Hub) cleanStaleConnections() {
	now := time.Now()
	h.clients.Range(func(key, value interface{}) bool {
		list := value.(*[]*Client)
		for _, c := range *list {
			lastPong := time.Unix(0, c.lastPong.Load())
			if now.Sub(lastPong) > pongTimeout {
				h.logger.Warn("connection pong timeout, disconnecting", "user_id", c.UserID)
				select {
				case h.bus <- BusMessage{Type: BusUnregister, Payload: c}:
				default:
					h.logger.Warn("bus full, dropping stale unregister", "user_id", c.UserID)
				}
			}
		}
		return true
	})
}

// shutdown 优雅关闭：排空消息、关闭所有连接、清理房间（通过 sync.Once 保证只执行一次）
func (h *Hub) shutdown() {
	h.shutdownOnce.Do(func() {
		h.logger.Info("hub shutting down, draining messages")

		// 排空待发消息（最多等待 2 秒）
		drainTimer := time.NewTimer(2 * time.Second)
		drainDone := false
		for !drainDone {
			select {
			case msg := <-h.bus:
				h.dispatch(msg)
			case <-drainTimer.C:
				drainDone = true
			}
		}

		// 关闭所有连接
		h.clients.Range(func(key, value interface{}) bool {
			list := value.(*[]*Client)
			for _, c := range *list {
				close(c.sendCh)
				c.Conn.Close(websocket.StatusNormalClosure, "server shutdown")
				h.wg.Done()
			}
			return true
		})

		// 等待所有连接 goroutine 结束
		h.wg.Wait()

		// 清理房间
		h.roomMu.Lock()
		h.rooms = make(map[string]map[*Client]bool)
		h.roomMu.Unlock()

		h.logger.Info("hub shutdown complete")
	})
}

// Shutdown 外部调用关闭 Hub（委托给内部 shutdown，sync.Once 保证幂等）
func (h *Hub) Shutdown(ctx context.Context) {
	h.shutdown()
}

// --- 公开 API ---

// Register 注册连接
func (h *Hub) Register(client *Client) {
	h.wg.Add(1)
	h.bus <- BusMessage{Type: BusRegister, Payload: client}
}

// Unregister 注销连接
func (h *Hub) Unregister(client *Client) {
	h.bus <- BusMessage{Type: BusUnregister, Payload: client}
}

// SendToUser 向指定用户的所有连接发送消息
func (h *Hub) SendToUser(userID string, msg WSMessage) {
	h.bus <- BusMessage{
		Type:   BusDirectMsg,
		Target: userID,
		Payload: &directMsgPayload{UserID: userID, Msg: msg},
	}
}

// Broadcast 向所有在线用户广播消息
func (h *Hub) Broadcast(msg WSMessage) {
	h.bus <- BusMessage{Type: BusBroadcast, Payload: msg}
}

// IsOnline 检查用户是否在线
func (h *Hub) IsOnline(userID string) bool {
	val, ok := h.clients.Load(userID)
	if !ok {
		return false
	}
	list := val.(*[]*Client)
	return len(*list) > 0
}

// IsUserOnline 检查用户是否在线（IsOnline 的别名，供外部调用）
func (h *Hub) IsUserOnline(userID string) bool {
	return h.IsOnline(userID)
}

// JoinRoom 将连接加入指定会话房间
func (h *Hub) JoinRoom(conversationID string, client *Client) {
	h.bus <- BusMessage{
		Type:   BusJoinRoom,
		Target: conversationID,
		Payload: &roomAction{ConversationID: conversationID, Conn: client},
	}
}

// LeaveRoom 将连接移出指定会话房间
func (h *Hub) LeaveRoom(conversationID string, client *Client) {
	h.bus <- BusMessage{
		Type:   BusLeaveRoom,
		Target: conversationID,
		Payload: &roomAction{ConversationID: conversationID, Conn: client},
	}
}

// SendToRoom 向指定会话房间的所有连接发送消息
func (h *Hub) SendToRoom(conversationID string, msg WSMessage) {
	h.bus <- BusMessage{
		Type:   BusRoomMsg,
		Target: conversationID,
		Payload: &roomMessage{ConversationID: conversationID, Message: msg},
	}
}

// PushToConversation 推送持久化消息给会话所有成员（在线房间推送 + 按用户推送）
func (h *Hub) PushToConversation(conversationID string, memberIDs []string, message interface{}) {
	h.bus <- BusMessage{
		Type:   BusPersistedMsg,
		Target: conversationID,
		Payload: &persistedMsgPayload{
			ConversationID: conversationID,
			MemberIDs:      memberIDs,
			Message:        message,
		},
	}
}

// StartHeartbeat 启动心跳检测，每 30s 发 ping，超时则断开
func StartHeartbeat(ctx context.Context, client *Client, hub *Hub) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// nhooyr/io WebSocket 的 Ping 会在收到 pong 后返回 nil
			pingCtx, cancel := context.WithTimeout(ctx, pongTimeout)
			err := client.Conn.Ping(pingCtx)
			cancel()
			if err != nil {
				hub.Unregister(client)
				return
			}
			client.UpdateLastPong()
		}
	}
}

// directMsgPayload 私聊消息载体
type directMsgPayload struct {
	UserID string
	Msg    WSMessage
}

