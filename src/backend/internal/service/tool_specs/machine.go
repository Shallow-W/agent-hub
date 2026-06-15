package tool_specs

import "github.com/agent-hub/backend/internal/port"

// ── Machine tools ──

func ListMachines() port.MCPToolSpec {
	return routeSpec{
		name:        "list_machines",
		label:       "机器列表",
		category:    "machine",
		description: "查询当前用户已连接的电脑（daemon 机器）列表，包含机器名称、在线状态、最后心跳时间等信息",
		inputSchema: noParams(),
		routeInfo:   &port.RouteInfo{Method: "GET", Path: "/mcp/daemon/machines"},
	}
}
