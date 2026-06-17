package model

import "time"

// ReplyToPreview 回复引用预览
type ReplyToPreview struct {
	ID        string     `json:"id"`
	Content   string     `json:"content"`
	SenderID  string     `json:"sender_id"`
	Username  string     `json:"username"`
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
	Pinned         bool                `json:"pinned" db:"pinned"`
	Attachments    []MessageAttachment `json:"attachments,omitempty" db:"-"`
	Artifacts      []Artifact          `json:"artifacts,omitempty" db:"-"` // 结构化产物（独立 artifacts 表关联加载，不占用 artifacts_json）
	ReplyToMessage *ReplyToPreview     `json:"reply_to_message,omitempty" db:"-"`
	Mentions       []string            `json:"mentions,omitempty" db:"-"`  // JSON array of user IDs, stored in mentions TEXT column
	CardsJSON      string              `json:"cards_json,omitempty" db:"cards_json"`
	Cards          []map[string]any    `json:"cards,omitempty" db:"-"` // 交互式卡片（plan/progress/confirm/result），从 cards_json 反序列化
}

// MessagePin represents a message pinned into the shared conversation context.
type MessagePin struct {
	ID             string    `json:"id" db:"id"`
	ConversationID string    `json:"conversation_id" db:"conversation_id"`
	MessageID      string    `json:"message_id" db:"message_id"`
	CreatedBy      string    `json:"created_by" db:"created_by"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}

// PinnedMessage is the API/prompt view of a pinned message.
type PinnedMessage struct {
	ID               string    `json:"id" db:"pin_id"`
	ConversationID   string    `json:"conversation_id" db:"conversation_id"`
	MessageID        string    `json:"message_id" db:"message_id"`
	Role             string    `json:"role" db:"role"`
	Content          string    `json:"content" db:"content"`
	ArtifactsJSON    string    `json:"artifacts_json,omitempty" db:"artifacts_json"`
	SenderID         *string   `json:"sender_id,omitempty" db:"sender_id"`
	Username         string    `json:"username,omitempty" db:"username"`
	MessageCreatedAt time.Time `json:"message_created_at" db:"message_created_at"`
	PinnedBy         string    `json:"pinned_by" db:"pinned_by"`
	PinnedByName     string    `json:"pinned_by_name" db:"pinned_by_name"`
	PinnedAt         time.Time `json:"pinned_at" db:"pinned_at"`
}

// ConversationBlackboard stores user-authored long-term context for a conversation.
type ConversationBlackboard struct {
	ConversationID string    `json:"conversation_id" db:"conversation_id"`
	ManualContext  string    `json:"manual_context" db:"manual_context"`
	UpdatedBy      *string   `json:"updated_by,omitempty" db:"updated_by"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}
