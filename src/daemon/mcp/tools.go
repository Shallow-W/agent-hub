package mcp

// Per-category registration functions.
//
// Each RegisterXxx function defines its tools inline (using schema helpers)
// and registers them with the registry — either as route-based proxies via
// RegisterRoutes, or with custom handlers via Register.
//
// To add a new tool: add an entry in the appropriate RegisterXxx function.
// To add a new category: create a RegisterXxx function, add one line to BuildRegistry.

// RegisterConversationTools registers conversation/group-creation tools.
func RegisterConversationTools(r *Registry, api *APIClient) {
	RegisterRoutes(r, api,
		RouteEntry{
			T("list_conversations", "查询当前用户的会话列表（单聊/群聊）。任务看板以会话为基本单位，通常需要先获取会话列表再操作对应任务", NoParams()),
			RouteDef{Method: "GET", Path: "/mcp/conversations"},
		},
		RouteEntry{
			T("list_conversation_agents", "查询指定会话中参与的 Agent 列表，用于了解群聊中有哪些 Agent 可用",
				Schema(map[string]map[string]interface{}{"conversation_id": Prop("会话ID（必填）")}, "conversation_id")),
			RouteDef{Method: "GET", Path: "/mcp/conversations/{conversation_id}/agents", Required: []string{"conversation_id"}},
		},
		RouteEntry{
			T("get_messages", "读取指定会话的历史消息，用于获取上下文",
				Schema(map[string]map[string]interface{}{
					"conversation_id": Prop("会话ID（必填）"),
					"limit":           IntProp("返回条数，默认 50"),
				}, "conversation_id")),
			RouteDef{Method: "GET", Path: "/mcp/conversations/{conversation_id}/messages", Required: []string{"conversation_id"}, Optional: []string{"limit"}},
		},
		RouteEntry{
			T("create_group", "创建一个群聊，可指定初始成员用户 ID 列表",
				Schema(map[string]map[string]interface{}{
					"name":       Prop("群聊名称（必填）"),
					"member_ids": ArrayProp("初始成员用户 ID 列表"),
				}, "name")),
			RouteDef{Method: "POST", Path: "/mcp/groups", Required: []string{"name"}, Optional: []string{"member_ids"}},
		},
	)
}

// RegisterTaskTools registers task-board tools.
func RegisterTaskTools(r *Registry, api *APIClient) {
	RegisterRoutes(r, api,
		RouteEntry{
			T("list_tasks", "查询任务看板列表。任务以会话为单位组织，建议传入 conversation_id 查看特定会话的任务。不传则返回当前用户所有任务",
				Schema(map[string]map[string]interface{}{
					"conversation_id": Prop("按会话ID筛选（推荐，任务以会话为单位组织）"),
					"status":          EnumProp("按状态筛选", "todo", "in_progress", "blocked", "done"),
				})),
			RouteDef{Method: "GET", Path: "/mcp/tasks", Optional: []string{"conversation_id", "status"}},
		},
		RouteEntry{
			T("create_task", "在指定会话中创建新任务。任务归属到某个会话（群聊），建议必传 conversation_id",
				Schema(map[string]map[string]interface{}{
					"title":           Prop("任务标题（必填，1-120字符）"),
					"description":     Prop("任务描述"),
					"status":          EnumProp("初始状态，默认 todo", "todo", "in_progress", "blocked", "done"),
					"priority":        EnumProp("优先级，默认 medium", "low", "medium", "high"),
					"conversation_id": Prop("所属会话ID（推荐必传，任务以会话为单位）"),
					"assignee_id":     Prop("负责人ID（用户ID）"),
					"agent_id":        Prop("分配的 Agent ID（可用 list_agents 查询）"),
				}, "title")),
			RouteDef{Method: "POST", Path: "/mcp/tasks", Required: []string{"title"}, Optional: []string{"description", "status", "priority", "conversation_id", "assignee_id", "agent_id"}},
		},
		RouteEntry{
			T("update_task", "更新任务内容（标题、描述、优先级、负责人、分配的 Agent）",
				Schema(map[string]map[string]interface{}{
					"id":          Prop("任务ID（必填）"),
					"title":       Prop("新标题"),
					"description": Prop("新描述"),
					"priority":    EnumProp("新优先级", "low", "medium", "high"),
					"assignee_id": Prop("新负责人ID"),
					"agent_id":    Prop("新分配的 Agent ID"),
				}, "id")),
			RouteDef{Method: "PUT", Path: "/mcp/tasks/{id}", Required: []string{"id"}, Optional: []string{"title", "description", "priority", "assignee_id", "agent_id"}},
		},
		RouteEntry{
			T("move_task_status", "流转任务状态。可选值：todo、in_progress、blocked、done",
				Schema(map[string]map[string]interface{}{
					"id":     Prop("任务ID（必填）"),
					"status": EnumProp("目标状态（必填）", "todo", "in_progress", "blocked", "done"),
				}, "id", "status")),
			RouteDef{Method: "POST", Path: "/mcp/tasks/{id}/status", Required: []string{"id", "status"}},
		},
		RouteEntry{
			T("delete_task", "删除任务",
				Schema(map[string]map[string]interface{}{"id": Prop("任务ID（必填）")}, "id")),
			RouteDef{Method: "DELETE", Path: "/mcp/tasks/{id}", Required: []string{"id"}},
		},
	)
}

// RegisterAgentTools registers agent query tools.
func RegisterAgentTools(r *Registry, api *APIClient, agentID string) {
	RegisterRoutes(r, api,
		RouteEntry{
			T("list_agents", "查询当前用户可用的 Agent 列表（包括系统 Agent 和自建 Agent），返回名称、类型、状态、能力等信息", NoParams()),
			RouteDef{Method: "GET", Path: "/mcp/agents"},
		},
		RouteEntry{
			T("list_agent_candidates", "查询本机已发现的 Agent 候选列表（来自 daemon 扫描），包含 CLI 路径、版本、能力（skills）等信息。尚未添加到平台的 Agent 会出现在这里", NoParams()),
			RouteDef{Method: "GET", Path: "/mcp/daemon/agent-candidates"},
		},
	)
}

// RegisterMachineTools registers machine/device query tools.
func RegisterMachineTools(r *Registry, api *APIClient) {
	RegisterRoutes(r, api,
		RouteEntry{
			T("list_machines", "查询当前用户已连接的电脑（daemon 机器）列表，包含机器名称、在线状态、最后心跳时间等信息", NoParams()),
			RouteDef{Method: "GET", Path: "/mcp/daemon/machines"},
		},
	)
}

// RegisterGroupTools registers group-management tools.
func RegisterGroupTools(r *Registry, api *APIClient) {
	RegisterRoutes(r, api,
		RouteEntry{
			T("get_group_info", "查询群聊详情，包含群名称、成员数量、创建时间等。需要传入群聊ID（group_id）",
				Schema(map[string]map[string]interface{}{"group_id": Prop("群聊ID（必填）")}, "group_id")),
			RouteDef{Method: "GET", Path: "/mcp/groups/{group_id}", Required: []string{"group_id"}},
		},
		RouteEntry{
			T("list_group_members", "查询群聊成员列表，包含每个成员的用户名、角色（owner/admin/member）、加入时间等信息",
				Schema(map[string]map[string]interface{}{"group_id": Prop("群聊ID（必填）")}, "group_id")),
			RouteDef{Method: "GET", Path: "/mcp/groups/{group_id}/members", Required: []string{"group_id"}},
		},
	)
}

// RegisterAgentManagementTools registers agent detail/prompt/start/stop tools.
func RegisterAgentManagementTools(r *Registry, api *APIClient) {
	RegisterRoutes(r, api,
		RouteEntry{
			T("get_agent_detail", "查询单个 Agent 的完整详情，包括名称、类型、CLI 工具、系统提示词、工具配置、状态、版本、机器名称、能力、技能、标签等",
				Schema(map[string]map[string]interface{}{"agent_id": Prop("Agent ID（必填）")}, "agent_id")),
			RouteDef{Method: "GET", Path: "/mcp/agents/{agent_id}", Required: []string{"agent_id"}},
		},
		RouteEntry{
			T("start_agent", "启动指定的 Agent",
				Schema(map[string]map[string]interface{}{"agent_id": Prop("Agent ID（必填）")}, "agent_id")),
			RouteDef{Method: "POST", Path: "/mcp/agents/{agent_id}/start", Required: []string{"agent_id"}},
		},
		RouteEntry{
			T("stop_agent", "停止指定的 Agent",
				Schema(map[string]map[string]interface{}{"agent_id": Prop("Agent ID（必填）")}, "agent_id")),
			RouteDef{Method: "POST", Path: "/mcp/agents/{agent_id}/stop", Required: []string{"agent_id"}},
		},
	)
	r.Register(
		T("update_agent_prompt", "更新 Agent 的系统提示词。会先获取当前完整信息，再只修改 system_prompt 字段",
			Schema(map[string]map[string]interface{}{
				"agent_id":      Prop("Agent ID（必填）"),
				"system_prompt": Prop("新的系统提示词（必填）"),
			}, "agent_id", "system_prompt")),
		makeUpdateAgentPromptHandler(api),
	)
}

// RegisterKnowledgeTools registers knowledge-base tools.
func RegisterKnowledgeTools(r *Registry, api *APIClient) {
	RegisterRoutes(r, api,
		RouteEntry{
			T("list_knowledge_bases", "列出当前用户的知识库，包含 ID、名称、描述、可见性、文件数量、创建时间等信息", NoParams()),
			RouteDef{Method: "GET", Path: "/mcp/knowledge-bases"},
		},
		RouteEntry{
			T("list_knowledge_files", "列出指定知识库中的文件，包含文件名、大小、类型、预览文本等信息",
				Schema(map[string]map[string]interface{}{"knowledge_base_id": Prop("知识库 ID（必填）")}, "knowledge_base_id")),
			RouteDef{Method: "GET", Path: "/mcp/knowledge-bases/{knowledge_base_id}/files", Required: []string{"knowledge_base_id"}},
		},
		RouteEntry{
			T("search_knowledge", "在指定知识库中按关键词搜索文件，基于文件的 preview_text 字段进行匹配过滤",
				Schema(map[string]map[string]interface{}{
					"knowledge_base_id": Prop("知识库 ID（必填）"),
					"keyword":           Prop("搜索关键词（必填）"),
					"limit":             IntProp("最多返回结果数（可选，默认 20）"),
				}, "knowledge_base_id", "keyword")),
			RouteDef{Method: "GET", Path: "/mcp/knowledge-bases/{knowledge_base_id}/search", Required: []string{"knowledge_base_id", "keyword"}, Optional: []string{"limit"}},
		},
		RouteEntry{
			T("read_knowledge_file", "读取指定知识库文件已抽取的文本内容。适合在搜索命中文件后按 file_id 获取完整可用上下文",
				Schema(map[string]map[string]interface{}{
					"knowledge_base_id": Prop("知识库 ID（必填）"),
					"file_id":           Prop("文件 ID（必填）"),
				}, "knowledge_base_id", "file_id")),
			RouteDef{Method: "GET", Path: "/mcp/knowledge-bases/{knowledge_base_id}/files/{file_id}/text", Required: []string{"knowledge_base_id", "file_id"}},
		},
	)
}

// RegisterAgentCreationTools registers agent create/update/delete + toolset listing.
func RegisterAgentCreationTools(r *Registry, api *APIClient, ts *ToolsetStore) {
	RegisterRoutes(r, api,
		RouteEntry{
			T("delete_agent", "删除自建 Agent",
				Schema(map[string]map[string]interface{}{"agent_id": Prop("Agent ID（必填）")}, "agent_id")),
			RouteDef{Method: "DELETE", Path: "/mcp/agents/{agent_id}", Required: []string{"agent_id"}},
		},
	)
	r.Register(
		T("create_agent", "创建自建 Agent。需要提供名称和系统提示词，可选指定工具模板、CLI 工具和标签",
			Schema(map[string]map[string]interface{}{
				"name":          Prop("Agent 名称（必填）"),
				"system_prompt": Prop("系统提示词（必填）"),
				"toolset":       Prop("工具模板名（none/basic/tasks/orchestrator/agent_builder/agent_manager/knowledge），默认 none"),
				"cli_tool":      Prop("CLI 工具名，默认 claude"),
				"tags":          Prop("标签"),
			}, "name", "system_prompt")),
		makeCreateAgentHandler(api, ts),
	)
	r.Register(
		T("update_agent", "更新 Agent 配置，只改传入的字段。可修改名称、系统提示词、工具模板、自定义工具列表和标签",
			Schema(map[string]map[string]interface{}{
				"agent_id":      Prop("Agent ID（必填）"),
				"name":          Prop("新名称"),
				"system_prompt": Prop("新系统提示词"),
				"toolset":       Prop("切换工具模板"),
				"allowed_tools": ArrayProp("自定义工具列表"),
				"tags":          Prop("新标签"),
			}, "agent_id")),
		makeUpdateAgentHandler(api, ts),
	)
	r.Register(
		T("list_toolsets", "列出可用的工具模板及其描述，用于创建或更新 Agent 时选择合适的工具配置", NoParams()),
		makeListToolsetsHandler(ts),
	)
}

// RegisterSkillTools registers platform-skill query tools.
func RegisterSkillTools(r *Registry, api *APIClient, agentID string) {
	r.Register(
		T("get_agent_skill", "查看当前 Agent 已分配平台 Skill 的详细内容。先根据提示词中的 Skill 索引选择 name，再调用本工具渐进加载 detail",
			Schema(map[string]map[string]interface{}{"name": Prop("平台 Skill 名称（必填，必须属于当前 Agent）")}, "name")),
		makeGetAgentSkillHandler(api, agentID),
	)
	RegisterRoutes(r, api,
		RouteEntry{
			T("list_platform_skills", "列出所有平台 Skill 摘要，包含名称、分类、描述和触发场景，用于为 Agent 分配 Skill", NoParams()),
			RouteDef{Method: "GET", Path: "/mcp/platform-skills"},
		},
	)
}
