package tool_specs

import "github.com/agent-hub/backend/internal/port"

// ── Task tools ──

func ListTasks() port.MCPToolSpec {
	return routeSpec{
		name:        "list_tasks",
		label:       "任务列表",
		category:    "task",
		description: "查询任务看板列表。任务以会话为单位组织，建议传入 conversation_id 查看特定会话的任务。不传则返回当前用户所有任务",
		inputSchema: schema(map[string]map[string]interface{}{
			"conversation_id": strProp("按会话ID筛选（推荐，任务以会话为单位组织）"),
			"status":          enumProp("按状态筛选", "todo", "in_progress", "blocked", "done"),
		}),
		routeInfo: &port.RouteInfo{Method: "GET", Path: "/mcp/tasks", Optional: []string{"conversation_id", "status"}},
	}
}

func CreateTask() port.MCPToolSpec {
	return routeSpec{
		name:        "create_task",
		label:       "创建任务",
		category:    "task",
		description: "在指定会话中创建新任务。任务归属到某个会话（群聊），建议必传 conversation_id",
		inputSchema: schema(map[string]map[string]interface{}{
			"title":           strProp("任务标题（必填，1-120字符）"),
			"description":     strProp("任务描述"),
			"status":          enumProp("初始状态，默认 todo", "todo", "in_progress", "blocked", "done"),
			"priority":        enumProp("优先级，默认 medium", "low", "medium", "high"),
			"conversation_id": strProp("所属会话ID（推荐必传，任务以会话为单位）"),
			"assignee_id":     strProp("负责人ID（用户ID）"),
			"agent_id":        strProp("分配的 Agent ID（可用 list_agents 查询）"),
		}, "title"),
		routeInfo: &port.RouteInfo{Method: "POST", Path: "/mcp/tasks", Required: []string{"title"}, Optional: []string{"description", "status", "priority", "conversation_id", "assignee_id", "agent_id"}},
	}
}

func UpdateTask() port.MCPToolSpec {
	return routeSpec{
		name:        "update_task",
		label:       "更新任务",
		category:    "task",
		description: "更新任务内容（标题、描述、优先级、负责人、分配的 Agent）",
		inputSchema: schema(map[string]map[string]interface{}{
			"id":          strProp("任务ID（必填）"),
			"title":       strProp("新标题"),
			"description": strProp("新描述"),
			"priority":    enumProp("新优先级", "low", "medium", "high"),
			"assignee_id": strProp("新负责人ID"),
			"agent_id":    strProp("新分配的 Agent ID"),
		}, "id"),
		routeInfo: &port.RouteInfo{Method: "PUT", Path: "/mcp/tasks/{id}", Required: []string{"id"}, Optional: []string{"title", "description", "priority", "assignee_id", "agent_id"}},
	}
}

func MoveTaskStatus() port.MCPToolSpec {
	return routeSpec{
		name:        "move_task_status",
		label:       "流转任务状态",
		category:    "task",
		description: "流转任务状态。可选值：todo、in_progress、blocked、done",
		inputSchema: schema(map[string]map[string]interface{}{
			"id":     strProp("任务ID（必填）"),
			"status": enumProp("目标状态（必填）", "todo", "in_progress", "blocked", "done"),
		}, "id", "status"),
		routeInfo: &port.RouteInfo{Method: "POST", Path: "/mcp/tasks/{id}/status", Required: []string{"id", "status"}},
	}
}

func DeleteTask() port.MCPToolSpec {
	return routeSpec{
		name:        "delete_task",
		label:       "删除任务",
		category:    "task",
		description: "删除任务",
		inputSchema: schema(map[string]map[string]interface{}{
			"id": strProp("任务ID（必填）"),
		}, "id"),
		routeInfo: &port.RouteInfo{Method: "DELETE", Path: "/mcp/tasks/{id}", Required: []string{"id"}},
	}
}
