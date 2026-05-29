package model

// ConversationMember 群成员模型
type ConversationMember struct {
	ID             string `db:"id" json:"id"`
	ConversationID string `db:"conversation_id" json:"conversation_id"`
	UserID         string `db:"user_id" json:"user_id"`
	Role           string `db:"role" json:"role"`
	JoinedAt       string `db:"joined_at" json:"joined_at"`
	LastReadAt     *string `db:"last_read_at" json:"last_read_at,omitempty"`
	Username       string `db:"username" json:"username"` // JOIN users
}
