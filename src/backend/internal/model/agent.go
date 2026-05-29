package model

import "time"

// Agent 表示可被对话选择和调度的 Agent 配置
type Agent struct {
	ID               string     `json:"id" db:"id"`
	UserID           *string    `json:"user_id,omitempty" db:"user_id"`
	Name             string     `json:"name" db:"name"`
	Type             string     `json:"type" db:"type"`
	CLITool          string     `json:"cli_tool" db:"cli_tool"`
	SystemPrompt     string     `json:"system_prompt,omitempty" db:"system_prompt"`
	Avatar           string     `json:"avatar,omitempty" db:"avatar"`
	CapabilitiesJSON string     `json:"capabilities_json,omitempty" db:"capabilities_json"`
	Source           string     `json:"source" db:"source"`
	Status           string     `json:"status" db:"status"`
	Version          string     `json:"version,omitempty" db:"version"`
	MachineID        *string    `json:"machine_id,omitempty" db:"machine_id"`
	MachineName      string     `json:"machine_name,omitempty" db:"machine_name"`
	LastSeenAt       *time.Time `json:"last_seen_at,omitempty" db:"last_seen_at"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at" db:"updated_at"`
}
