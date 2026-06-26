package tool_specs

import "github.com/agent-hub/backend/internal/port"

// ── Conversation tools ──

func ListConversations() port.MCPToolSpec {
	return newRouteSpec(
		"list_conversations",
		"会话列表",
		"conversation",
		"查询当前用户的会话列表（单聊/群聊）。任务看板以会话为基本单位，通常需要先获取会话列表再操作对应任务",
		noParams(),
		&port.RouteInfo{Method: "GET", Path: "/mcp/conversations"},
	)
}

func ListConversationAgents() port.MCPToolSpec {
	return newRouteSpec(
		"list_conversation_agents",
		"会话 Agent 列表",
		"conversation",
		"查询指定会话中参与的 Agent 列表，用于了解群聊中有哪些 Agent 可用",
		schema(map[string]map[string]interface{}{
			"conversation_id": strProp("会话ID（必填）"),
		}, "conversation_id"),
		&port.RouteInfo{Method: "GET", Path: "/mcp/conversations/{conversation_id}/agents", Required: []string{"conversation_id"}},
	)
}

func GetMessages() port.MCPToolSpec {
	return newRouteSpec(
		"get_messages",
		"获取消息",
		"conversation",
		"读取指定会话的历史消息，用于获取上下文",
		schema(map[string]map[string]interface{}{
			"conversation_id": strProp("会话ID（必填）"),
			"limit":           intProp("返回条数，默认 50"),
		}, "conversation_id"),
		&port.RouteInfo{Method: "GET", Path: "/mcp/conversations/{conversation_id}/messages", Required: []string{"conversation_id"}, Optional: []string{"limit"}},
	)
}

func CreateGroup() port.MCPToolSpec {
	return newRouteSpec(
		"create_group",
		"创建群聊",
		"conversation",
		"创建一个群聊，可指定初始成员用户 ID 列表",
		schema(map[string]map[string]interface{}{
			"name":       strProp("群聊名称（必填）"),
			"member_ids": arrayProp("初始成员用户 ID 列表"),
		}, "name"),
		&port.RouteInfo{Method: "POST", Path: "/mcp/groups", Required: []string{"name"}, Optional: []string{"member_ids"}},
	)
}
