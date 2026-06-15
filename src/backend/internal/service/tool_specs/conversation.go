package tool_specs

import "github.com/agent-hub/backend/internal/port"

// ── Conversation tools ──

func ListConversations() port.MCPToolSpec {
	return routeSpec{
		name:        "list_conversations",
		label:       "会话列表",
		category:    "conversation",
		description: "查询当前用户的会话列表（单聊/群聊）。任务看板以会话为基本单位，通常需要先获取会话列表再操作对应任务",
		inputSchema: noParams(),
		routeInfo:   &port.RouteInfo{Method: "GET", Path: "/mcp/conversations"},
	}
}

func ListConversationAgents() port.MCPToolSpec {
	return routeSpec{
		name:        "list_conversation_agents",
		label:       "会话 Agent 列表",
		category:    "conversation",
		description: "查询指定会话中参与的 Agent 列表，用于了解群聊中有哪些 Agent 可用",
		inputSchema: schema(map[string]map[string]interface{}{
			"conversation_id": strProp("会话ID（必填）"),
		}, "conversation_id"),
		routeInfo: &port.RouteInfo{Method: "GET", Path: "/mcp/conversations/{conversation_id}/agents", Required: []string{"conversation_id"}},
	}
}

func ListGroupAgents() port.MCPToolSpec {
	return routeSpec{
		name:        "list_group_agents",
		label:       "群聊 Agent 列表",
		category:    "conversation",
		description: "查询指定群聊中参与的 Agent 列表，用于了解群聊中有哪些 Agent 可用",
		inputSchema: schema(map[string]map[string]interface{}{
			"conversation_id": strProp("群聊会话ID（必填）"),
		}, "conversation_id"),
		routeInfo: &port.RouteInfo{Method: "GET", Path: "/mcp/conversations/{conversation_id}/agents", Required: []string{"conversation_id"}},
	}
}

func GetMessages() port.MCPToolSpec {
	return routeSpec{
		name:        "get_messages",
		label:       "获取消息",
		category:    "conversation",
		description: "读取指定会话的历史消息，用于获取上下文",
		inputSchema: schema(map[string]map[string]interface{}{
			"conversation_id": strProp("会话ID（必填）"),
			"limit":           intProp("返回条数，默认 50"),
		}, "conversation_id"),
		routeInfo: &port.RouteInfo{Method: "GET", Path: "/mcp/conversations/{conversation_id}/messages", Required: []string{"conversation_id"}, Optional: []string{"limit"}},
	}
}

func CreateGroup() port.MCPToolSpec {
	return routeSpec{
		name:        "create_group",
		label:       "创建群聊",
		category:    "conversation",
		description: "创建一个群聊，可指定初始成员用户 ID 列表",
		inputSchema: schema(map[string]map[string]interface{}{
			"name":       strProp("群聊名称（必填）"),
			"member_ids": arrayProp("初始成员用户 ID 列表"),
		}, "name"),
		routeInfo: &port.RouteInfo{Method: "POST", Path: "/mcp/groups", Required: []string{"name"}, Optional: []string{"member_ids"}},
	}
}
