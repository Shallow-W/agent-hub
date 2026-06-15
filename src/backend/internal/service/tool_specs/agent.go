package tool_specs

import "github.com/agent-hub/backend/internal/port"

// ── Agent tools ──

// Route-proxy agent tools.

func ListAgents() port.MCPToolSpec {
	return routeSpec{
		name:        "list_agents",
		label:       "Agent 列表",
		category:    "agent",
		description: "查询当前用户可用的 Agent 列表（包括系统 Agent 和自建 Agent），返回名称、类型、状态、能力等信息",
		inputSchema: noParams(),
		routeInfo:   &port.RouteInfo{Method: "GET", Path: "/mcp/agents"},
	}
}

func ListAgentCandidates() port.MCPToolSpec {
	return routeSpec{
		name:        "list_agent_candidates",
		label:       "Agent 候选列表",
		category:    "agent",
		description: "查询本机已发现的 Agent 候选列表（来自 daemon 扫描），包含 CLI 路径、版本、能力（skills）等信息。尚未添加到平台的 Agent 会出现在这里",
		inputSchema: noParams(),
		routeInfo:   &port.RouteInfo{Method: "GET", Path: "/mcp/daemon/agent-candidates"},
	}
}

func GetAgentDetail() port.MCPToolSpec {
	return routeSpec{
		name:        "get_agent_detail",
		label:       "Agent 详情",
		category:    "agent",
		description: "查询单个 Agent 的完整详情，包括名称、类型、CLI 工具、系统提示词、工具配置、状态、版本、机器名称、能力、技能、标签等",
		inputSchema: schema(map[string]map[string]interface{}{
			"agent_id": strProp("Agent ID（必填）"),
		}, "agent_id"),
		routeInfo: &port.RouteInfo{Method: "GET", Path: "/mcp/agents/{agent_id}", Required: []string{"agent_id"}},
	}
}

func StartAgent() port.MCPToolSpec {
	return routeSpec{
		name:        "start_agent",
		label:       "启动 Agent",
		category:    "agent",
		description: "启动指定的 Agent",
		inputSchema: schema(map[string]map[string]interface{}{
			"agent_id": strProp("Agent ID（必填）"),
		}, "agent_id"),
		routeInfo: &port.RouteInfo{Method: "POST", Path: "/mcp/agents/{agent_id}/start", Required: []string{"agent_id"}},
	}
}

func StopAgent() port.MCPToolSpec {
	return routeSpec{
		name:        "stop_agent",
		label:       "停止 Agent",
		category:    "agent",
		description: "停止指定的 Agent",
		inputSchema: schema(map[string]map[string]interface{}{
			"agent_id": strProp("Agent ID（必填）"),
		}, "agent_id"),
		routeInfo: &port.RouteInfo{Method: "POST", Path: "/mcp/agents/{agent_id}/stop", Required: []string{"agent_id"}},
	}
}

func DeleteAgent() port.MCPToolSpec {
	return routeSpec{
		name:        "delete_agent",
		label:       "删除 Agent",
		category:    "agent",
		description: "删除自建 Agent",
		inputSchema: schema(map[string]map[string]interface{}{
			"agent_id": strProp("Agent ID（必填）"),
		}, "agent_id"),
		routeInfo: &port.RouteInfo{Method: "DELETE", Path: "/mcp/agents/{agent_id}", Required: []string{"agent_id"}},
	}
}

// ── Complex agent tools (RouteInfo = nil; require daemon custom handlers) ──

func GetAgentSkill() port.MCPToolSpec {
	return routeSpec{
		name:        "get_agent_skill",
		label:       "获取 Agent Skill",
		category:    "skill",
		description: "查看当前 Agent 已分配平台 Skill 的详细内容。先根据提示词中的 Skill 索引选择 name，再调用本工具渐进加载 detail",
		inputSchema: schema(map[string]map[string]interface{}{
			"name": strProp("平台 Skill 名称（必填，必须属于当前 Agent）"),
		}, "name"),
		routeInfo: nil,
	}
}

func UpdateAgentPrompt() port.MCPToolSpec {
	return routeSpec{
		name:        "update_agent_prompt",
		label:       "更新 Agent 提示词",
		category:    "agent",
		description: "更新 Agent 的系统提示词。会先获取当前完整信息，再只修改 system_prompt 字段",
		inputSchema: schema(map[string]map[string]interface{}{
			"agent_id":      strProp("Agent ID（必填）"),
			"system_prompt": strProp("新的系统提示词（必填）"),
		}, "agent_id", "system_prompt"),
		routeInfo: nil,
	}
}

func CreateAgent() port.MCPToolSpec {
	return routeSpec{
		name:        "create_agent",
		label:       "创建 Agent",
		category:    "agent",
		description: "创建自建 Agent。需要提供名称和系统提示词，可选指定工具模板、CLI 工具和标签",
		inputSchema: schema(map[string]map[string]interface{}{
			"name":          strProp("Agent 名称（必填）"),
			"system_prompt": strProp("系统提示词（必填）"),
			"toolset":       strProp("工具模板名（none/basic/tasks/orchestrator/agent_builder/agent_manager/knowledge），默认 none"),
			"cli_tool":      strProp("CLI 工具名，默认 claude"),
			"tags":          strProp("标签"),
		}, "name", "system_prompt"),
		routeInfo: nil,
	}
}

func UpdateAgent() port.MCPToolSpec {
	return routeSpec{
		name:        "update_agent",
		label:       "更新 Agent 配置",
		category:    "agent",
		description: "更新 Agent 配置，只改传入的字段。可修改名称、系统提示词、工具模板、自定义工具列表和标签",
		inputSchema: schema(map[string]map[string]interface{}{
			"agent_id":      strProp("Agent ID（必填）"),
			"name":          strProp("新名称"),
			"system_prompt": strProp("新系统提示词"),
			"toolset":       strProp("切换工具模板"),
			"allowed_tools": arrayProp("自定义工具列表"),
			"tags":          strProp("新标签"),
		}, "agent_id"),
		routeInfo: nil,
	}
}

func ListToolsets() port.MCPToolSpec {
	return routeSpec{
		name:        "list_toolsets",
		label:       "工具模板列表",
		category:    "agent",
		description: "列出可用的工具模板及其描述，用于创建或更新 Agent 时选择合适的工具配置",
		inputSchema: noParams(),
		routeInfo:   nil,
	}
}
