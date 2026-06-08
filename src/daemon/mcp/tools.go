package mcp

// Tool 定义一个 MCP tool
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]interface{}
}

// ToolHandlerFunc 处理 tool 调用
type ToolHandlerFunc func(toolName string, arguments map[string]interface{}) (interface{}, error)

// AllTools 返回所有 MCP tool 定义
func AllTools() []Tool {
	var tools []Tool
	tools = append(tools, ConversationTools()...)
	tools = append(tools, TaskTools()...)
	tools = append(tools, AgentTools()...)
	tools = append(tools, MachineTools()...)
	tools = append(tools, GroupTools()...)
	return tools
}

// ConversationTools 会话相关工具
func ConversationTools() []Tool {
	return []Tool{
		{
			Name:        "list_conversations",
			Description: "查询当前用户的会话列表（单聊/群聊）。任务看板以会话为基本单位，通常需要先获取会话列表再操作对应任务",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "list_conversation_agents",
			Description: "查询指定会话中参与的 Agent 列表，用于了解群聊中有哪些 Agent 可用",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"conversation_id": map[string]interface{}{
						"type":        "string",
						"description": "会话ID（必填）",
					},
				},
				"required": []string{"conversation_id"},
			},
		},
		{
			Name:        "list_group_agents",
			Description: "查询指定群聊中参与的 Agent 列表，用于了解群聊中有哪些 Agent 可用",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"conversation_id": map[string]interface{}{
						"type":        "string",
						"description": "群聊会话ID（必填）",
					},
				},
				"required": []string{"conversation_id"},
			},
		},
		{
			Name:        "get_messages",
			Description: "读取指定会话的历史消息，用于获取上下文",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"conversation_id": map[string]interface{}{
						"type":        "string",
						"description": "会话ID（必填）",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "返回条数，默认 50",
					},
				},
				"required": []string{"conversation_id"},
			},
		},
		{
			Name:        "create_group",
			Description: "创建一个群聊，可指定初始成员用户 ID 列表",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "群聊名称（必填）",
					},
					"member_ids": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "初始成员用户 ID 列表",
					},
				},
				"required": []string{"name"},
			},
		},
	}
}

// TaskTools 任务看板工具——任务以会话（群聊）为基本单位
func TaskTools() []Tool {
	return []Tool{
		{
			Name:        "list_tasks",
			Description: "查询任务看板列表。任务以会话为单位组织，建议传入 conversation_id 查看特定会话的任务。不传则返回当前用户所有任务",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"conversation_id": map[string]interface{}{
						"type":        "string",
						"description": "按会话ID筛选（推荐，任务以会话为单位组织）",
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
			Description: "在指定会话中创建新任务。任务归属到某个会话（群聊），建议必传 conversation_id",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"conversation_id": map[string]interface{}{
						"type":        "string",
						"description": "所属会话ID（推荐必传，任务以会话为单位）",
					},
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
					"assignee_id": map[string]interface{}{
						"type":        "string",
						"description": "负责人ID（用户ID）",
					},
					"agent_id": map[string]interface{}{
						"type":        "string",
						"description": "分配的 Agent ID（可用 list_agents 查询）",
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

// AgentTools 智能体相关工具——查询当前用户可用的 Agent
func AgentTools() []Tool {
	return []Tool{
		{
			Name:        "list_agents",
			Description: "查询当前用户可用的 Agent 列表（包括系统 Agent 和自建 Agent），返回名称、类型、状态、能力等信息",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "list_agent_candidates",
			Description: "查询本机已发现的 Agent 候选列表（来自 daemon 扫描），包含 CLI 路径、版本、能力（skills）等信息。尚未添加到平台的 Agent 会出现在这里",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
	}
}

// MachineTools 机器/设备管理工具——查询当前用户连接的电脑
func MachineTools() []Tool {
	return []Tool{
		{
			Name:        "list_machines",
			Description: "查询当前用户已连接的电脑（daemon 机器）列表，包含机器名称、在线状态、最后心跳时间等信息",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
	}
}

// GroupTools 群聊管理工具——查询群聊详情和成员信息
func GroupTools() []Tool {
	return []Tool{
		{
			Name:        "get_group_info",
			Description: "查询群聊详情，包含群名称、成员数量、创建时间等。需要传入群聊ID（group_id）",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"group_id": map[string]interface{}{
						"type":        "string",
						"description": "群聊ID（必填）",
					},
				},
				"required": []string{"group_id"},
			},
		},
		{
			Name:        "list_group_members",
			Description: "查询群聊成员列表，包含每个成员的用户名、角色（owner/admin/member）、加入时间等信息",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"group_id": map[string]interface{}{
						"type":        "string",
						"description": "群聊ID（必填）",
					},
				},
				"required": []string{"group_id"},
			},
		},
	}
}
