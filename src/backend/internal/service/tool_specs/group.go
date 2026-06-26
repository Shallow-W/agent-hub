package tool_specs

import "github.com/agent-hub/backend/internal/port"

// ── Group tools ──

func GetGroupInfo() port.MCPToolSpec {
	return newRouteSpec(
		"get_group_info",
		"群聊详情",
		"group",
		"查询群聊详情，包含群名称、成员数量、创建时间等。需要传入群聊ID（group_id）",
		schema(map[string]map[string]interface{}{
			"group_id": strProp("群聊ID（必填）"),
		}, "group_id"),
		&port.RouteInfo{Method: "GET", Path: "/mcp/groups/{group_id}", Required: []string{"group_id"}},
	)
}

func ListGroupMembers() port.MCPToolSpec {
	return newRouteSpec(
		"list_group_members",
		"群聊成员列表",
		"group",
		"查询群聊成员列表，包含每个成员的用户名、角色（owner/admin/member）、加入时间等信息",
		schema(map[string]map[string]interface{}{
			"group_id": strProp("群聊ID（必填）"),
		}, "group_id"),
		&port.RouteInfo{Method: "GET", Path: "/mcp/groups/{group_id}/members", Required: []string{"group_id"}},
	)
}
