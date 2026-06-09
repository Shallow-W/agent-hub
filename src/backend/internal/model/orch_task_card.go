package model

import "time"

// OrchTaskCard 表示 Orch 编排分派的一张任务卡片，独立于 workspace_tasks。
// 创建时已写入完整的发送方/处理方信息，无需 JOIN 即可展示。
type OrchTaskCard struct {
	ID             string     `json:"id" db:"id"`
	ConversationID string     `json:"conversation_id" db:"conversation_id"`
	OrchTaskID     string     `json:"orch_task_id" db:"orch_task_id"`

	SenderID       string     `json:"sender_id" db:"sender_id"`
	SenderName     string     `json:"sender_name" db:"sender_name"`
	SenderAvatar   string     `json:"sender_avatar" db:"sender_avatar"`

	WorkerID       string     `json:"worker_id" db:"worker_id"`
	WorkerName     string     `json:"worker_name" db:"worker_name"`
	WorkerAvatar   string     `json:"worker_avatar" db:"worker_avatar"`

	TaskContent    string     `json:"task_content" db:"task_content"`
	TaskSummary    string     `json:"task_summary" db:"task_summary"`
	WorkerResult   string     `json:"worker_result" db:"worker_result"`

	Status         string     `json:"status" db:"status"`
	Priority       string     `json:"priority" db:"priority"`
	TaskHash       string     `json:"task_hash" db:"task_hash"`

	DispatchedAt   time.Time  `json:"dispatched_at" db:"dispatched_at"`
	StartedAt      *time.Time `json:"started_at,omitempty" db:"started_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty" db:"completed_at"`

	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}
