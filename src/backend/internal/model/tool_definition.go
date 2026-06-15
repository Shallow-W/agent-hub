package model

import (
	"encoding/json"
	"time"
)

// ToolDefinition 表示 MCP 工具定义（全局工具目录）。
type ToolDefinition struct {
	Name         string    `json:"name" db:"name"`
	Label        string    `json:"label" db:"label"`
	Category     string    `json:"category" db:"category"`
	Description  string    `json:"description" db:"description"`
	IsManagement bool      `json:"is_management" db:"is_management"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// BuiltinToolsetTemplate 表示内置工具集模板。
type BuiltinToolsetTemplate struct {
	Name        string          `json:"name" db:"name"`
	Label       string          `json:"label" db:"label"`
	Description string          `json:"description" db:"description"`
	ToolNames   json.RawMessage `json:"tool_names" db:"tool_names"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
}

// BuiltinSkillTemplate 表示内置技能模板（与 BuiltinToolsetTemplate 对称）。
type BuiltinSkillTemplate struct {
	Name            string          `json:"name" db:"name"`
	Label           string          `json:"label" db:"label"`
	Description     string          `json:"description" db:"description"`
	SkillCategories json.RawMessage `json:"skill_categories" db:"skill_categories"`
}
