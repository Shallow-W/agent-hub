package tool_specs

import "github.com/agent-hub/backend/internal/port"

// RenderCard 返回 render_card 平台内置工具 spec。
// 这是平台基础设施工具：所有 Agent 默认可用，RouteInfo 为 nil
// （daemon 本地执行，不是 REST 代理）。
func RenderCard() port.MCPToolSpec {
	return routeSpec{
		name:        "render_card",
		label:       "卡片渲染",
		description: "在聊天中渲染交互式卡片（方案选择、审批确认、任务进度、信息展示）。调用后卡片自动出现在聊天界面，用户可直接交互。",
		category:    "platform",
		routeInfo:   nil, // daemon 本地工具，不走 REST 代理
	}
}
