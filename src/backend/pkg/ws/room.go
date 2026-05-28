package ws

// roomAction 房间操作请求
type roomAction struct {
	ConversationID string
	Conn           *Client
}

// roomMessage 房间广播消息
type roomMessage struct {
	ConversationID string
	Message        WSMessage
	Exclude        *Client // 排除的连接（如 typing 通知排除发送者）
}

// persistedMsgPayload 持久化消息推送载体
type persistedMsgPayload struct {
	ConversationID string
	MemberIDs      []string
	Message        interface{}
}
