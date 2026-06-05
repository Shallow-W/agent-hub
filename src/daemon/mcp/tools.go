package mcp

// Tool 定义一个 MCP tool
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]interface{}
}

// ToolHandlerFunc 处理 tool 调用
type ToolHandlerFunc func(toolName string, arguments map[string]interface{}) (interface{}, error)

// TaskTools 返回任务面板相关的 MCP tool 定义
func TaskTools() []Tool {
	return []Tool{
		{
			Name:        "list_tasks",
			Description: "查询任务看板列表。可按会话ID、状态筛选。状态可选值：todo、in_progress、blocked、done",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"conversation_id": map[string]interface{}{
						"type":        "string",
						"description": "按会话ID筛选",
					},
					"status": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"todo", "in_progress", "blocked", "done"},
						"description": "按状态筛选",
					},
				},
			},
		},
		{
			Name:        "create_task",
			Description: "创建新任务。状态可选值：todo（默认）、in_progress、blocked、done。优先级可选值：low、medium（默认）、high",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"title": map[string]interface{}{
						"type":        "string",
						"description": "任务标题（必填，1-120字符）",
					},
					"description": map[string]interface{}{
						"type":        "string",
						"description": "任务描述",
					},
					"status": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"todo", "in_progress", "blocked", "done"},
						"description": "初始状态，默认 todo",
					},
					"priority": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"low", "medium", "high"},
						"description": "优先级，默认 medium",
					},
					"conversation_id": map[string]interface{}{
						"type":        "string",
						"description": "关联的会话ID",
					},
					"assignee_id": map[string]interface{}{
						"type":        "string",
						"description": "负责人ID",
					},
					"agent_id": map[string]interface{}{
						"type":        "string",
						"description": "分配的 Agent ID",
					},
				},
				"required": []string{"title"},
			},
		},
		{
			Name:        "update_task",
			Description: "更新任务内容（标题、描述、优先级、负责人、分配的 Agent）",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type":        "string",
						"description": "任务ID（必填）",
					},
					"title": map[string]interface{}{
						"type":        "string",
						"description": "新标题",
					},
					"description": map[string]interface{}{
						"type":        "string",
						"description": "新描述",
					},
					"priority": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"low", "medium", "high"},
						"description": "新优先级",
					},
					"assignee_id": map[string]interface{}{
						"type":        "string",
						"description": "新负责人ID",
					},
					"agent_id": map[string]interface{}{
						"type":        "string",
						"description": "新分配的 Agent ID",
					},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "move_task_status",
			Description: "流转任务状态。可选值：todo、in_progress、blocked、done",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type":        "string",
						"description": "任务ID（必填）",
					},
					"status": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"todo", "in_progress", "blocked", "done"},
						"description": "目标状态（必填）",
					},
				},
				"required": []string{"id", "status"},
			},
		},
		{
			Name:        "delete_task",
			Description: "删除任务",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type":        "string",
						"description": "任务ID（必填）",
					},
				},
				"required": []string{"id"},
			},
		},
	}
}
