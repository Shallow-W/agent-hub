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
}
