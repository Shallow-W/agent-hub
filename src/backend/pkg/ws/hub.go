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
	TypeMessageRecall    = "message.recall"
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
	BusCustomEvent                         // 自定义事件推送
)

// WSMessage WebSocket 消息格式
type WSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

const (
	writeBufSize     = 256
	pingInterval     = 30 * time.Second
	pongTimeout      = 60 * time.Second
	cleanupInterval  = 15 * time.Second
	maxConnsPerUser  = 50 // per-user max WS connections (dev tool, multiple tabs expected)
)

// Client 封装单个 WebSocket 连接及其元数据
type Client struct {
	Conn        *websocket.Conn
	UserID      string
	Username    string
	ConnectedAt time.Time
	lastPong    atomic.Int64 // unix nanos，原子操作避免数据竞争
	lastActive  atomic.Int64 // unix timestamp，原子操作避免数据竞争

	sendCh    chan []byte   // 独立写缓冲通道
	mu        sync.Mutex    // 保护 Conn 的写操作（close 时排空用）
	closeOnce sync.Once     // 保证 sendCh 只关闭一次
}

// NewClient 创建 Client 实例（导出供 handler 调用）
func NewClient(conn *websocket.Conn, userID string) *Client {
	now := time.Now()
	c := &Client{
		Conn:        conn,
		UserID:      userID,
		ConnectedAt: now,
		sendCh:      make(chan []byte, writeBufSize),
	}
	c.lastPong.Store(now.UnixNano())
	c.lastActive.Store(now.Unix())
	return c
}

// SetUsername 设置用户名（导出供 handler 在连接建立后调用）
func (c *Client) SetUsername(name string) {
	c.Username = name
}

// UpdateLastActive 原子更新最后活跃时间
func (c *Client) UpdateLastActive() {
	c.lastActive.Store(time.Now().Unix())
}

// LastActiveTime 返回最后活跃时间
func (c *Client) LastActiveTime() time.Time {
	return time.Unix(c.lastActive.Load(), 0)
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
		// 背压：丢弃最旧消息腾出一个位置
		select {
		case <-c.sendCh:
		default:
		}
		select {
		case c.sendCh <- data:
		default:
			// 二次写入仍失败则直接丢弃
			snippet := string(data)
			if len(snippet) > 80 {
				snippet = snippet[:80] + "..."
			}
			slog.Warn("write buffer full after drain, dropping message", "user_id", c.UserID, "msg", snippet)
		}
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

// Hub 管理所有 WebSocket 连接，基于消息总线模式。
//
// 注意：Register/Unregister 通过 bus channel 异步处理。当客户端注册后立即发送消息时，
// 消息可能在 Register 处理之前到达 bus。这是已知的设计权衡：使用单 goroutine 事件循环
// 避免了锁竞争，且不影响正确性——未注册的客户端不在任何房间中，消息不会路由到该客户端。
// 客户端应先等待服务端确认（如 online 事件），再发送业务消息。
type Hub struct {
	// sync.Map 替代 map+mutex，优化高并发读写
	clients sync.Map // string -> *[]*Client（同一用户多设备）

	bus    chan BusMessage
	rooms  map[string]map[*Client]bool
	roomMu sync.RWMutex

	wg           sync.WaitGroup // 跟踪活跃连接数
	draining     atomic.Bool    // shutdown drain 阶段为 true，阻止新 wg.Add
	shutdownOnce sync.Once
	logger       *slog.Logger

	lastEvict sync.Map // userID -> time.Time of last eviction (rate limit)
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
	case BusCustomEvent:
		h.handleCustomEvent(msg)
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
	if len(*list) >= maxConnsPerUser {
		// Rate-limit eviction: if we evicted for this user <10s ago, reject instead
		if last, ok := h.lastEvict.Load(client.UserID); ok {
			if time.Since(last.(time.Time)) < 30*time.Second {
				h.logger.Warn("max connections, eviction rate-limited",
					"user_id", client.UserID, "current", len(*list), "max", maxConnsPerUser)
				client.closeOnce.Do(func() { close(client.sendCh) })
				client.Conn.Close(websocket.StatusPolicyViolation, "too many connections")
				if !h.draining.Load() {
					h.wg.Done()
				}
				return
			}
		}
		// Evict oldest connection — newest tab always wins
		oldest := (*list)[0]
		*list = append((*list)[:0], (*list)[1:]...)
		oldest.closeOnce.Do(func() { close(oldest.sendCh) })
		oldest.Conn.Close(websocket.StatusPolicyViolation, "evicted: newer connection")
		if !h.draining.Load() {
			h.wg.Done()
		}
		h.lastEvict.Store(client.UserID, time.Now())
		h.logger.Info("evicted oldest connection for user",
			"user_id", client.UserID, "evicted_connected_at", oldest.ConnectedAt)
	}
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
	found := false
	for i, c := range *list {
		if c == client {
			*list = append((*list)[:i], (*list)[i+1:]...)
			found = true
			break
		}
	}

	// client 不在列表中（可能是被 max-conns 踢出后重复 unregister），直接跳过
	if !found {
		return
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

	client.closeOnce.Do(func() { close(client.sendCh) })
	client.Conn.Close(websocket.StatusNormalClosure, "disconnect")
	if !h.draining.Load() {
		h.wg.Done()
	}
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
		if c == payload.Exclude {
			continue
		}
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
		// 设置 draining 标志，阻止 drain 期间新连接进入 wg
		h.draining.Store(true)
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
				c.closeOnce.Do(func() { close(c.sendCh) })
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
	if !h.draining.Load() {
		h.wg.Add(1)
	}
	select {
	case h.bus <- BusMessage{Type: BusRegister, Payload: client}:
	default:
		if !h.draining.Load() {
			h.wg.Done()
		}
		h.logger.Warn("hub bus full, dropping register", "user_id", client.UserID)
	}
}

// Unregister 注销连接
func (h *Hub) Unregister(client *Client) {
	select {
	case h.bus <- BusMessage{Type: BusUnregister, Payload: client}:
	default:
		h.logger.Warn("hub bus full, force-closing connection", "user_id", client.UserID)
		// bus 满时直接关闭连接，使 readLoop 退出并重试 Unregister
		client.closeOnce.Do(func() { close(client.sendCh) })
		client.Conn.Close(websocket.StatusNormalClosure, "disconnect")
	}
}

// SendToUser 向指定用户的所有连接发送消息
func (h *Hub) SendToUser(userID string, msg WSMessage) {
	select {
	case h.bus <- BusMessage{
		Type:    BusDirectMsg,
		Target:  userID,
		Payload: &directMsgPayload{UserID: userID, Msg: msg},
	}:
	default:
		h.logger.Warn("hub bus full, dropping send-to-user", "user_id", userID)
	}
}

// Broadcast 向所有在线用户广播消息
func (h *Hub) Broadcast(msg WSMessage) {
	select {
	case h.bus <- BusMessage{Type: BusBroadcast, Payload: msg}:
	default:
		h.logger.Warn("hub bus full, dropping broadcast")
	}
}

// IsOnline 检查用户是否在线
func (h *Hub) IsOnline(userID string) bool {
	_, ok := h.clients.Load(userID)
	return ok
}

// IsUserOnline 检查用户是否在线（IsOnline 的别名，供外部调用）
func (h *Hub) IsUserOnline(userID string) bool {
	return h.IsOnline(userID)
}

// JoinRoom 将连接加入指定会话房间
func (h *Hub) JoinRoom(conversationID string, client *Client) {
	select {
	case h.bus <- BusMessage{
		Type:    BusJoinRoom,
		Target:  conversationID,
		Payload: &roomAction{ConversationID: conversationID, Conn: client},
	}:
	default:
		h.logger.Warn("hub bus full, dropping join-room", "conversation_id", conversationID)
	}
}

// LeaveRoom 将连接移出指定会话房间
func (h *Hub) LeaveRoom(conversationID string, client *Client) {
	select {
	case h.bus <- BusMessage{
		Type:    BusLeaveRoom,
		Target:  conversationID,
		Payload: &roomAction{ConversationID: conversationID, Conn: client},
	}:
	default:
		h.logger.Warn("hub bus full, dropping leave-room", "conversation_id", conversationID)
	}
}

// SendToRoom 向指定会话房间的所有连接发送消息
func (h *Hub) SendToRoom(conversationID string, msg WSMessage) {
	select {
	case h.bus <- BusMessage{
		Type:    BusRoomMsg,
		Target:  conversationID,
		Payload: &roomMessage{ConversationID: conversationID, Message: msg},
	}:
	default:
		h.logger.Warn("hub bus full, dropping send-to-room", "conversation_id", conversationID)
	}
}

// SendToRoomExcept 向房间内除 exclude 外的所有连接发送消息
func (h *Hub) SendToRoomExcept(conversationID string, exclude *Client, msg WSMessage) {
	select {
	case h.bus <- BusMessage{
		Type:   BusRoomMsg,
		Target: conversationID,
		Payload: &roomMessage{ConversationID: conversationID, Message: msg, Exclude: exclude},
	}:
	default:
		h.logger.Warn("hub bus full, dropping send-to-room-except", "conversation_id", conversationID)
	}
}

// PushToConversation 推送持久化消息给会话所有成员（在线房间推送 + 按用户推送）
func (h *Hub) PushToConversation(conversationID string, memberIDs []string, message interface{}) {
	select {
	case h.bus <- BusMessage{
		Type:   BusPersistedMsg,
		Target: conversationID,
		Payload: &persistedMsgPayload{
			ConversationID: conversationID,
			MemberIDs:      memberIDs,
			Message:        message,
		},
	}:
	default:
		h.logger.Warn("hub bus full, dropping push-to-conversation", "conversation_id", conversationID)
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

// customEventPayload 自定义事件载体
type customEventPayload struct {
	ConversationID string
	MemberIDs      []string
	EventType      string
	Data           interface{}
}

// handleCustomEvent 处理自定义事件推送（在 bus 事件循环中执行，线程安全）
func (h *Hub) handleCustomEvent(msg BusMessage) {
	payload := msg.Payload.(*customEventPayload)
	wsMsg := WSMessage{Type: payload.EventType, Data: payload.Data}
	for _, uid := range payload.MemberIDs {
		val, ok := h.clients.Load(uid)
		if !ok {
			continue
		}
		list := val.(*[]*Client)
		for _, c := range *list {
			if err := c.Send(wsMsg); err != nil {
				h.logger.Warn("push custom event failed", "conversation_id", payload.ConversationID, "user_id", uid, "error", err)
			}
		}
	}
}

// PushCustomEvent 向会话成员推送自定义事件（通过 bus 保证线程安全）
func (h *Hub) PushCustomEvent(conversationID string, memberIDs []string, eventType string, data interface{}) {
	select {
	case h.bus <- BusMessage{
		Type:   BusCustomEvent,
		Target: conversationID,
		Payload: &customEventPayload{
			ConversationID: conversationID,
			MemberIDs:      memberIDs,
			EventType:      eventType,
			Data:           data,
		},
	}:
	default:
		h.logger.Warn("hub bus full, dropping custom-event", "conversation_id", conversationID, "event_type", eventType)
	}
}
