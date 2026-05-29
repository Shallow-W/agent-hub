package model

import "time"

// DaemonTask 表示投递给远端电脑 daemon 的一次真实 CLI 执行。
type DaemonTask struct {
	ID             string     `json:"id" db:"id"`
	UserID         string     `json:"user_id" db:"user_id"`
	ConversationID string     `json:"conversation_id" db:"conversation_id"`
	AgentID        string     `json:"agent_id" db:"agent_id"`
	MachineID      string     `json:"machine_id" db:"machine_id"`
	CLITool        string     `json:"cli_tool" db:"cli_tool"`
	Prompt         string     `json:"prompt" db:"prompt"`
	Status         string     `json:"status" db:"status"`
	Result         string     `json:"result" db:"result"`
	Error          string     `json:"error" db:"error"`
	ClaimedAt      *time.Time `json:"claimed_at,omitempty" db:"claimed_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty" db:"completed_at"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}
