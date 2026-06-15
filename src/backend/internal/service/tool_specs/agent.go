package tool_specs

import "github.com/agent-hub/backend/internal/port"

// ── Agent tools ──

// Route-proxy agent tools.

func ListAgents() port.MCPToolSpec {
	return newRouteSpec(
		"list_agents",
		"Agent 列表",
		"agent",
		"查询当前用户可用的 Agent 列表（包括系统 Agent 和自建 Agent），返回名称、类型、状态、能力等信息",
		noParams(),
		&port.RouteInfo{Method: "GET", Path: "/mcp/agents"},
	)
}

func ListAgentCandidates() port.MCPToolSpec {
	return newRouteSpec(
		"list_agent_candidates",
		"Agent 候选列表",
		"agent",
		"查询本机已发现的 Agent 候选列表（来自 daemon 扫描），包含 CLI 路径、版本、能力（skills）等信息。尚未添加到平台的 Agent 会出现在这里",
		noParams(),
		&port.RouteInfo{Method: "GET", Path: "/mcp/daemon/agent-candidates"},
	)
}

func GetAgentDetail() port.MCPToolSpec {
	return newRouteSpec(
		"get_agent_detail",
		"Agent 详情",
		"agent",
		"查询单个 Agent 的完整详情，包括名称、类型、CLI 工具、系统提示词、工具配置、状态、版本、机器名称、能力、技能、标签等",
		schema(map[string]map[string]interface{}{
			"agent_id": strProp("Agent ID（必填）"),
		}, "agent_id"),
		&port.RouteInfo{Method: "GET", Path: "/mcp/agents/{agent_id}", Required: []string{"agent_id"}},
	)
}

func StartAgent() port.MCPToolSpec {
	return newRouteSpec(
		"start_agent",
		"启动 Agent",
		"agent",
		"启动指定的 Agent",
		schema(map[string]map[string]interface{}{
			"agent_id": strProp("Agent ID（必填）"),
		}, "agent_id"),
		&port.RouteInfo{Method: "POST", Path: "/mcp/agents/{agent_id}/start", Required: []string{"agent_id"}},
	)
}

func StopAgent() port.MCPToolSpec {
	return newRouteSpec(
		"stop_agent",
		"停止 Agent",
		"agent",
		"停止指定的 Agent",
		schema(map[string]map[string]interface{}{
			"agent_id": strProp("Agent ID（必填）"),
		}, "agent_id"),
		&port.RouteInfo{Method: "POST", Path: "/mcp/agents/{agent_id}/stop", Required: []string{"agent_id"}},
	)
}

func DeleteAgent() port.MCPToolSpec {
	return newRouteSpec(
		"delete_agent",
		"删除 Agent",
		"agent",
		"删除自建 Agent",
		schema(map[string]map[string]interface{}{
			"agent_id": strProp("Agent ID（必填）"),
		}, "agent_id"),
		&port.RouteInfo{Method: "DELETE", Path: "/mcp/agents/{agent_id}", Required: []string{"agent_id"}},
	)
}

// ── Complex agent tools (RouteInfo = nil; require daemon custom handlers) ──

func UpdateAgentPrompt() port.MCPToolSpec {
	return newRouteSpec(
		"update_agent_prompt",
		"更新 Agent 提示词",
		"agent",
		"更新 Agent 的系统提示词。会先获取当前完整信息，再只修改 system_prompt 字段",
		schema(map[string]map[string]interface{}{
			"agent_id":      strProp("Agent ID（必填）"),
			"system_prompt": strProp("新的系统提示词（必填）"),
		}, "agent_id", "system_prompt"),
		nil,
	)
}

func CreateAgent() port.MCPToolSpec {
	return newRouteSpec(
		"create_agent",
		"创建 Agent",
		"agent",
		"创建自建 Agent。需要提供名称和系统提示词，可选指定工具模板、CLI 工具和标签",
		schema(map[string]map[string]interface{}{
			"name":          strProp("Agent 名称（必填）"),
			"system_prompt": strProp("系统提示词（必填）"),
			"toolset":       strProp("工具模板名（none/basic/tasks/orchestrator/agent_builder/agent_manager/knowledge），默认 none"),
			"cli_tool":      strProp("CLI 工具名，默认 claude"),
			"tags":          strProp("标签"),
		}, "name", "system_prompt"),
		nil,
	)
}

func UpdateAgent() port.MCPToolSpec {
	return newRouteSpec(
		"update_agent",
		"更新 Agent 配置",
		"agent",
		"更新 Agent 配置，只改传入的字段。可修改名称、系统提示词、工具模板、自定义工具列表和标签",
		schema(map[string]map[string]interface{}{
			"agent_id":      strProp("Agent ID（必填）"),
			"name":          strProp("新名称"),
			"system_prompt": strProp("新系统提示词"),
			"toolset":       strProp("切换工具模板"),
			"allowed_tools": arrayProp("自定义工具列表"),
			"tags":          strProp("新标签"),
		}, "agent_id"),
		nil,
	)
}

func ListToolsets() port.MCPToolSpec {
	return newRouteSpec(
		"list_toolsets",
		"工具模板列表",
		"agent",
		"列出可用的工具模板及其描述，用于创建或更新 Agent 时选择合适的工具配置",
		noParams(),
		nil,
	)
}
