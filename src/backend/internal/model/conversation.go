package model

import "time"

// Conversation 对话模型
type Conversation struct {
	ID         string     `json:"id" db:"id"`
	UserID     string     `json:"user_id" db:"user_id"`
	Type       string     `json:"type" db:"type"`
	Title      string     `json:"title" db:"title"`
	Pinned     bool       `json:"pinned" db:"pinned"`
	ArchivedAt *time.Time `json:"archived_at,omitempty" db:"archived_at"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at" db:"updated_at"`

	// 计算字段，非 DB 列
	PeerName    string `json:"peer_name,omitempty" db:"peer_name"`
	LastMessage string `json:"last_message,omitempty" db:"last_message"`
	MemberCount int    `json:"member_count,omitempty" db:"member_count"`
}
