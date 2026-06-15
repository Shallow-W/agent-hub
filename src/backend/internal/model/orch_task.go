package model

import "time"

// OrchTaskStatus 枚举
const (
	OrchTaskPlanning       = "planning"
	OrchTaskDispatching    = "dispatching"
	OrchTaskWorkersRunning = "workers_running"
	OrchTaskSummarizing    = "summarizing"
	OrchTaskEvaluating     = "evaluating"
	OrchTaskCompleted      = "completed"
	OrchTaskFailed         = "failed"

	MaxOrchRounds = 5
)

// OrchTask 表示一次编排任务的完整生命周期。
type OrchTask struct {
	ID                string    `json:"id" db:"id"`
	ConversationID    string    `json:"conversation_id" db:"conversation_id"`
	UserID            string    `json:"user_id" db:"user_id"`
	OrchAgentID       string    `json:"orch_agent_id" db:"orch_agent_id"`
	Status            string    `json:"status" db:"status"`
	DispatchPlan      string    `json:"dispatch_plan,omitempty" db:"dispatch_plan"`
	WorkerStatus      string    `json:"worker_status,omitempty" db:"worker_status"`
	WorkerResults     string    `json:"worker_results,omitempty" db:"worker_results"`
	Summary           string    `json:"summary,omitempty" db:"summary"`
	OriginalMessage   string    `json:"original_message,omitempty" db:"original_message"`
	SourceMessageID   string    `json:"source_message_id,omitempty" db:"source_message_id"`
	DispatchMessageID string    `json:"dispatch_message_id,omitempty" db:"dispatch_message_id"`
	KBPreload         string    `json:"kb_preload,omitempty" db:"kb_preload"`
	Round             int       `json:"round" db:"round"`
	RoundHistory      string    `json:"round_history,omitempty" db:"round_history"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time `json:"updated_at" db:"updated_at"`
}
