package model

// ToolCategory 表示 MCP 工具类别（前端工具目录的分组维度）。
type ToolCategory struct {
	Name      string `json:"name" db:"name"`
	Label     string `json:"label" db:"label"`
	Color     string `json:"color" db:"color"`
	SortOrder int    `json:"sort_order" db:"sort_order"`
}
