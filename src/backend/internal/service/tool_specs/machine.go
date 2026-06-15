package tool_specs

import "github.com/agent-hub/backend/internal/port"

// ── Machine tools ──

func ListMachines() port.MCPToolSpec {
	return newRouteSpec(
		"list_machines",
		"机器列表",
		"machine",
		"查询当前用户已连接的电脑（daemon 机器）列表，包含机器名称、在线状态、最后心跳时间等信息",
		noParams(),
		&port.RouteInfo{Method: "GET", Path: "/mcp/daemon/machines"},
	)
}
