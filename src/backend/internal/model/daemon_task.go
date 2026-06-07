package model

import "time"

// DaemonTask 表示投递给远端电脑 daemon 的一次真实 CLI 执行。
type DaemonTask struct {
	ID              string     `json:"id" db:"id"`
	UserID          string     `json:"user_id" db:"user_id"`
	ConversationID  string     `json:"conversation_id" db:"conversation_id"`
	AgentID         string     `json:"agent_id" db:"agent_id"`
	MachineID       string     `json:"machine_id" db:"machine_id"`
	CLITool         string     `json:"cli_tool" db:"cli_tool"`
	Prompt          string     `json:"prompt" db:"prompt"`
	// ContextMessages 是 Layer 2 上下文：编排调度时为群聊背景+调度指令+依赖输出的纯文本，
	// 直接 dispatch 时为 agentHandoff 的 JSON 数组（最多 5 条）。
	// 空字符串表示无历史上下文（首条消息）。
	ContextMessages string     `json:"context_messages" db:"context_messages"`
	OrchTaskID      string     `json:"orch_task_id,omitempty" db:"orch_task_id"`
	WorkerName      string     `json:"worker_name,omitempty" db:"worker_name"`
	Status          string     `json:"status" db:"status"`
	Result          string     `json:"result" db:"result"`
	Error           string     `json:"error" db:"error"`
	ClaimedAt       *time.Time `json:"claimed_at,omitempty" db:"claimed_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty" db:"completed_at"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

// ContextMessage 是传给 Agent 的单条对话上下文消息。
type ContextMessage struct {
	Role    string `json:"role"`              // "user" 或 "assistant"
	Name    string `json:"name,omitempty"`    // 用户名 或 Agent 名
	AgentID string `json:"agent_id,omitempty"` // 仅 assistant 消息有值
	Content string `json:"content"`
}
