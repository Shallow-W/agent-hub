package model

import "time"

// Friend 好友关系模型
type Friend struct {
	ID        string    `json:"id" db:"id"`
	UserID    string    `json:"user_id" db:"user_id"`
	FriendID  string    `json:"friend_id" db:"friend_id"`
	Status    string    `json:"status" db:"status"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
	// 关联字段，用于 API 响应
	FriendName string `json:"friend_name,omitempty" db:"friend_name"`
}
