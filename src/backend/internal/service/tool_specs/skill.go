package tool_specs

import "github.com/agent-hub/backend/internal/port"

// ── Skill tools ──

// GetAgentSkill 返回查看当前 Agent 已分配平台 Skill 详情的工具规格。
// RouteInfo 为 nil，需要 daemon 侧自定义 handler（按当前 agent 过滤 skill 归属）。
func GetAgentSkill() port.MCPToolSpec {
	return newRouteSpec(
		"get_agent_skill",
		"获取 Agent Skill",
		"skill",
		"查看当前 Agent 已分配平台 Skill 的详细内容。先根据提示词中的 Skill 索引选择 name，再调用本工具渐进加载 detail",
		schema(map[string]map[string]interface{}{
			"name": strProp("平台 Skill 名称（必填，必须属于当前 Agent）"),
		}, "name"),
		nil,
	)
}

func ListPlatformSkills() port.MCPToolSpec {
	return newRouteSpec(
		"list_platform_skills",
		"平台 Skills",
		"skill",
		"列出所有平台 Skill 摘要，包含名称、分类、描述和触发场景，用于为 Agent 分配 Skill。",
		noParams(),
		&port.RouteInfo{Method: "GET", Path: "/mcp/platform-skills"},
	)
}
