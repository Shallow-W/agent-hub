package model

import "time"

// ReplyToPreview 回复引用预览
type ReplyToPreview struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	SenderID  string `json:"sender_id"`
	Username  string `json:"username"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

// Message 消息模型
type Message struct {
	ID             string              `json:"id" db:"id"`
	ConversationID string              `json:"conversation_id" db:"conversation_id"`
	Role           string              `json:"role" db:"role"`
	Content        string              `json:"content" db:"content"`
	ArtifactsJSON  string              `json:"artifacts_json,omitempty" db:"artifacts_json"`
	ReplyTo        *string             `json:"reply_to,omitempty" db:"reply_to"`
	DeletedAt      *time.Time          `json:"deleted_at,omitempty" db:"deleted_at"`
	CreatedAt      time.Time           `json:"created_at" db:"created_at"`
	SenderID       *string             `json:"sender_id,omitempty" db:"sender_id"`
	Username       string              `json:"username,omitempty" db:"username"`
	Attachments      []MessageAttachment `json:"attachments,omitempty" db:"-"`
	ReplyToMessage   *ReplyToPreview     `json:"reply_to_message,omitempty" db:"-"`
	Mentions         []string            `json:"mentions,omitempty" db:"-"` // JSON array of user IDs, stored in mentions TEXT column
}
