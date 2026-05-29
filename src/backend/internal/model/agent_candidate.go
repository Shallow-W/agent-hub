package model

import "time"

// AgentCandidate 表示 daemon 在某台电脑上检测到但尚未添加的 Agent。
type AgentCandidate struct {
	ID               string     `json:"id" db:"id"`
	MachineID        string     `json:"machine_id" db:"machine_id"`
	MachineName      string     `json:"machine_name" db:"machine_name"`
	Name             string     `json:"name" db:"name"`
	CLITool          string     `json:"cli_tool" db:"cli_tool"`
	Version          string     `json:"version,omitempty" db:"version"`
	CapabilitiesJSON string     `json:"capabilities_json,omitempty" db:"capabilities_json"`
	LastSeenAt       *time.Time `json:"last_seen_at,omitempty" db:"last_seen_at"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at" db:"updated_at"`
}
