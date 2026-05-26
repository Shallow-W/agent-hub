package model

import "time"

// Message 消息模型
type Message struct {
	ID             string              `json:"id" db:"id"`
	ConversationID string              `json:"conversation_id" db:"conversation_id"`
	Role           string              `json:"role" db:"role"`
	Content        string              `json:"content" db:"content"`
	ArtifactsJSON  string              `json:"artifacts_json,omitempty" db:"artifacts_json"`
	CreatedAt      time.Time           `json:"created_at" db:"created_at"`
	Attachments    []MessageAttachment `json:"attachments,omitempty" db:"-"`
}
