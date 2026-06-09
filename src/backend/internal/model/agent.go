package model

import "time"

// Agent 表示可被对话选择和调度的 Agent 配置
type Agent struct {
	ID                    string     `json:"id" db:"id"`
	UserID                *string    `json:"user_id,omitempty" db:"user_id"`
	Name                  string     `json:"name" db:"name"`
	Type                  string     `json:"type" db:"type"`
	CLITool               string     `json:"cli_tool" db:"cli_tool"`
	SystemPrompt          string     `json:"system_prompt,omitempty" db:"system_prompt"`
	ToolsConfig           string     `json:"tools_config,omitempty" db:"tools_config"`
	Avatar                string     `json:"avatar,omitempty" db:"avatar"`
	CapabilitiesJSON      string     `json:"capabilities_json,omitempty" db:"capabilities_json"`
	CustomSkills          string     `json:"custom_skills,omitempty" db:"custom_skills"`
	Tags                  string     `json:"tags,omitempty" db:"tags"`
	Source                string     `json:"source" db:"source"`
	Status                string     `json:"status" db:"status"`
	Version               string     `json:"version,omitempty" db:"version"`
	MachineID             *string    `json:"machine_id,omitempty" db:"machine_id"`
	MachineName           string     `json:"machine_name,omitempty" db:"machine_name"`
	EnableManagementTools bool       `json:"enable_management_tools" db:"enable_management_tools"`
	LastSeenAt            *time.Time `json:"last_seen_at,omitempty" db:"last_seen_at"`
	CreatedAt             time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at" db:"updated_at"`
}

// PlatformSkill 是用户在平台 Skill 库中维护、可分配给不同 Agent 的技能。
type PlatformSkill struct {
	ID          string    `json:"id" db:"id"`
	UserID      string    `json:"user_id" db:"user_id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description,omitempty" db:"description"`
	Trigger     string    `json:"trigger,omitempty" db:"trigger"`
	Detail      string    `json:"detail,omitempty" db:"detail"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}
