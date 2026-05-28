package model

import "time"

// Conversation 对话模型
type Conversation struct {
	ID        string    `json:"id" db:"id"`
	UserID    string    `json:"user_id" db:"user_id"`
	Type      string    `json:"type" db:"type"`
	Title     string    `json:"title" db:"title"`
	Pinned    bool      `json:"pinned" db:"pinned"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// ConversationAgent 表示某个对话中已加入的 Robot 成员。
type ConversationAgent struct {
	ID               string     `json:"id" db:"id"`
	ConversationID   string     `json:"conversation_id" db:"conversation_id"`
	AgentID          string     `json:"agent_id" db:"agent_id"`
	AddedBy          string     `json:"added_by" db:"added_by"`
	Role             string     `json:"role" db:"role"`
	JoinedAt         time.Time  `json:"joined_at" db:"joined_at"`
	Name             string     `json:"name" db:"name"`
	Type             string     `json:"type" db:"type"`
	CLITool          string     `json:"cli_tool" db:"cli_tool"`
	Avatar           string     `json:"avatar" db:"avatar"`
	Source           string     `json:"source" db:"source"`
	Status           string     `json:"status" db:"status"`
	Version          string     `json:"version" db:"version"`
	MachineID        *string    `json:"machine_id,omitempty" db:"machine_id"`
	MachineName      string     `json:"machine_name" db:"machine_name"`
	LastSeenAt       *time.Time `json:"last_seen_at,omitempty" db:"last_seen_at"`
	CapabilitiesJSON string     `json:"capabilities_json" db:"capabilities_json"`
}
