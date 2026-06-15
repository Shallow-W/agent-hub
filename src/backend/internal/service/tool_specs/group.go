package tool_specs

import "github.com/agent-hub/backend/internal/port"

// ── Group tools ──

func GetGroupInfo() port.MCPToolSpec {
	return routeSpec{
		name:        "get_group_info",
		label:       "群聊详情",
		category:    "group",
		description: "查询群聊详情，包含群名称、成员数量、创建时间等。需要传入群聊ID（group_id）",
		inputSchema: schema(map[string]map[string]interface{}{
			"group_id": strProp("群聊ID（必填）"),
		}, "group_id"),
		routeInfo: &port.RouteInfo{Method: "GET", Path: "/mcp/groups/{group_id}", Required: []string{"group_id"}},
	}
}

func ListGroupMembers() port.MCPToolSpec {
	return routeSpec{
		name:        "list_group_members",
		label:       "群聊成员列表",
		category:    "group",
		description: "查询群聊成员列表，包含每个成员的用户名、角色（owner/admin/member）、加入时间等信息",
		inputSchema: schema(map[string]map[string]interface{}{
			"group_id": strProp("群聊ID（必填）"),
		}, "group_id"),
		routeInfo: &port.RouteInfo{Method: "GET", Path: "/mcp/groups/{group_id}/members", Required: []string{"group_id"}},
	}
}
