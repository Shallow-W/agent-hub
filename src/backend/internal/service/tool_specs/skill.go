package tool_specs

import "github.com/agent-hub/backend/internal/port"

// ── Skill tools ──

func ListPlatformSkills() port.MCPToolSpec {
	return routeSpec{
		name:        "list_platform_skills",
		label:       "平台 Skills",
		category:    "skill",
		description: "列出所有平台 Skill 摘要，包含名称、分类、描述和触发场景，用于为 Agent 分配 Skill。",
		inputSchema: noParams(),
		routeInfo:   &port.RouteInfo{Method: "GET", Path: "/mcp/platform-skills"},
	}
}
