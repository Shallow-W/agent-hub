-- Table for all available MCP tools (the master catalog)
CREATE TABLE IF NOT EXISTS tool_definitions (
    name VARCHAR(64) PRIMARY KEY,
    label VARCHAR(64) NOT NULL,
    category VARCHAR(32) NOT NULL,
    description VARCHAR(256) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Table for built-in toolset templates
CREATE TABLE IF NOT EXISTS builtin_toolset_templates (
    name VARCHAR(32) PRIMARY KEY,
    label VARCHAR(64) NOT NULL DEFAULT '',
    description VARCHAR(256) NOT NULL DEFAULT '',
    tool_names JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed tool definitions from the frontend toolCatalog
INSERT INTO tool_definitions (name, label, category, description) VALUES
('list_conversations', '会话列表', 'conversation', '读取当前用户会话列表'),
('list_conversation_agents', '会话 Agent', 'conversation', '读取指定会话内的 Agent'),
('list_group_agents', '群 Agent', 'conversation', '读取群聊可用 Agent'),
('get_messages', '读取消息', 'conversation', '读取指定会话历史消息'),
('get_agent_skill', '查看 Skill', 'skill', '读取当前 Agent 已分配平台 Skill 的详细内容'),
('create_group', '创建群聊', 'group', '创建新的群聊会话'),
('get_group_info', '群信息', 'group', '读取群聊详情'),
('list_group_members', '群成员', 'group', '读取群聊成员列表'),
('list_tasks', '任务列表', 'task', '读取任务看板'),
('create_task', '创建任务', 'task', '创建工作任务'),
('update_task', '更新任务', 'task', '更新任务内容和负责人'),
('move_task_status', '移动任务状态', 'task', '流转任务状态'),
('delete_task', '删除任务', 'task', '删除任务看板条目'),
('list_agents', 'Agent 列表', 'agent', '读取可用 Agent 列表'),
('list_agent_candidates', 'Agent 候选', 'agent', '读取本机发现的底座候选'),
('list_machines', '电脑列表', 'machine', '读取已连接电脑列表'),
('get_agent_detail', 'Agent 详情', 'agent', '查询单个 Agent 完整详情'),
('update_agent_prompt', '更新提示词', 'agent', '更新 Agent 系统提示词'),
('start_agent', '启动 Agent', 'agent', '启动指定 Agent'),
('stop_agent', '停止 Agent', 'agent', '停止指定 Agent'),
('list_knowledge_bases', '知识库列表', 'knowledge', '列出用户的知识库'),
('list_knowledge_files', '知识库文件', 'knowledge', '列出知识库中的文件'),
('search_knowledge', '搜索知识库', 'knowledge', '在知识库中按关键词搜索文件'),
('read_knowledge_file', '读取知识库文件', 'knowledge', '读取知识库文件的抽取文本'),
('create_agent', '创建 Agent', 'agent', '创建自建 Agent'),
('update_agent', '更新 Agent', 'agent', '更新 Agent 配置'),
('delete_agent', '删除 Agent', 'agent', '删除自建 Agent'),
('list_toolsets', '工具模板', 'agent', '列出工具模板'),
('list_platform_skills', '平台 Skills', 'skill', '列出所有平台 Skill 摘要')
ON CONFLICT (name) DO NOTHING;

-- Seed built-in toolset templates matching daemon-npm TOOLSET_TEMPLATES
INSERT INTO builtin_toolset_templates (name, label, description, tool_names) VALUES
('none', '无工具', '不分配任何工具', '[]'::jsonb),
('basic', '基础群聊', '基础会话工具', '["list_group_agents","get_messages","get_agent_skill"]'::jsonb),
('tasks', '任务协作', '任务管理相关工具', '["list_group_agents","get_messages","get_agent_skill","list_tasks","create_task","update_task","move_task_status"]'::jsonb),
('orchestrator', 'Orchestrator', '完整的编排器工具集', '["list_group_agents","list_conversation_agents","get_messages","get_agent_skill","list_tasks","create_task","update_task","move_task_status","list_conversations","get_group_info","list_group_members","list_knowledge_bases","list_knowledge_files","search_knowledge","read_knowledge_file","create_agent","update_agent","delete_agent","list_toolsets","list_platform_skills"]'::jsonb),
('agent_builder', 'Agent 创建', 'Agent 发现、详情查询、创建和更新工具', '["list_agents","list_group_agents","get_agent_skill","list_agent_candidates","list_machines","get_agent_detail","create_agent","update_agent","update_agent_prompt","list_platform_skills"]'::jsonb),
('agent_manager', 'Agent 管理', 'Agent 详情、配置更新、提示词修改、启停和删除', '["list_agents","get_agent_detail","update_agent","update_agent_prompt","start_agent","stop_agent","delete_agent","get_agent_skill","list_platform_skills"]'::jsonb),
('knowledge', '知识库', '知识库检索工具', '["list_knowledge_bases","list_knowledge_files","search_knowledge","read_knowledge_file"]'::jsonb)
ON CONFLICT (name) DO NOTHING;
