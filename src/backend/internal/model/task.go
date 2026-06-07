package model

import "time"

// WorkspaceTask 表示任务看板中的一张任务卡片。
type WorkspaceTask struct {
	ID             string    `json:"id" db:"id"`
	UserID         *string   `json:"user_id,omitempty" db:"user_id"`
	ConversationID *string   `json:"conversation_id,omitempty" db:"conversation_id"`
	AssigneeID     *string   `json:"assignee_id,omitempty" db:"assignee_id"`
	AgentID        *string   `json:"agent_id,omitempty" db:"agent_id"`
	Title          string    `json:"title" db:"title"`
	Description    string    `json:"description" db:"description"`
	Status         string    `json:"status" db:"status"`
	Priority       string    `json:"priority" db:"priority"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
	AssigneeName   string    `json:"assignee_name,omitempty" db:"assignee_name"`
	AgentName      string    `json:"agent_name,omitempty" db:"agent_name"`
	OrchTaskID     *string   `json:"orch_task_id,omitempty" db:"orch_task_id"`
	WorkerName     *string   `json:"worker_name,omitempty" db:"worker_name"`
}

// TaskFilter 表示任务列表查询条件。
type TaskFilter struct {
	ConversationID string
	Status         string
}

// TaskCreateInput 表示创建任务的输入。
type TaskCreateInput struct {
	ConversationID *string
	AssigneeID     *string
	AgentID        *string
	Title          string
	Description    string
	Status         string
	Priority       string
	OrchTaskID     *string
	WorkerName     *string
}

// TaskUpdateInput 表示更新任务的输入。
type TaskUpdateInput struct {
	Title       *string
	Description *string
	Priority    *string
	AssigneeID  *string
	AgentID     *string
}
