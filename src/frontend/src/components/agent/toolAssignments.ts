export interface ToolCatalogItem {
  name: string;
  label: string;
  category: 'conversation' | 'task' | 'agent' | 'machine' | 'group' | 'skill' | 'knowledge';
  description: string;
}

export const toolCatalog: ToolCatalogItem[] = [
  { name: 'list_conversations', label: '会话列表', category: 'conversation', description: '读取当前用户会话列表' },
  { name: 'list_conversation_agents', label: '会话 Agent', category: 'conversation', description: '读取指定会话内的 Agent' },
  { name: 'list_group_agents', label: '群 Agent', category: 'conversation', description: '读取群聊可用 Agent' },
  { name: 'get_messages', label: '读取消息', category: 'conversation', description: '读取指定会话历史消息' },
  { name: 'get_agent_skill', label: '查看 Skill', category: 'skill', description: '读取当前 Agent 已分配平台 Skill 的详细内容' },
  { name: 'create_group', label: '创建群聊', category: 'group', description: '创建新的群聊会话' },
  { name: 'get_group_info', label: '群信息', category: 'group', description: '读取群聊详情' },
  { name: 'list_group_members', label: '群成员', category: 'group', description: '读取群聊成员列表' },
  { name: 'list_tasks', label: '任务列表', category: 'task', description: '读取任务看板' },
  { name: 'create_task', label: '创建任务', category: 'task', description: '创建工作任务' },
  { name: 'update_task', label: '更新任务', category: 'task', description: '更新任务内容和负责人' },
  { name: 'move_task_status', label: '移动任务状态', category: 'task', description: '流转任务状态' },
  { name: 'delete_task', label: '删除任务', category: 'task', description: '删除任务看板条目' },
  { name: 'list_agents', label: 'Agent 列表', category: 'agent', description: '读取可用 Agent 列表' },
  { name: 'list_agent_candidates', label: 'Agent 候选', category: 'agent', description: '读取本机发现的底座候选' },
  { name: 'list_machines', label: '电脑列表', category: 'machine', description: '读取已连接电脑列表' },
  { name: 'get_agent_detail', label: 'Agent 详情', category: 'agent', description: '查询单个 Agent 完整详情' },
  { name: 'update_agent_prompt', label: '更新提示词', category: 'agent', description: '更新 Agent 系统提示词' },
  { name: 'start_agent', label: '启动 Agent', category: 'agent', description: '启动指定 Agent' },
  { name: 'stop_agent', label: '停止 Agent', category: 'agent', description: '停止指定 Agent' },
  { name: 'list_knowledge_bases', label: '知识库列表', category: 'knowledge', description: '列出用户的知识库' },
  { name: 'list_knowledge_files', label: '知识库文件', category: 'knowledge', description: '列出知识库中的文件' },
  { name: 'search_knowledge', label: '搜索知识库', category: 'knowledge', description: '在知识库中按关键词搜索文件' },
  { name: 'read_knowledge_file', label: '读取知识库文件', category: 'knowledge', description: '读取知识库文件的抽取文本' },
];

export const toolsetTemplates: Record<string, string[]> = {
  none: [],
  basic: ['list_group_agents', 'get_messages', 'get_agent_skill'],
  tasks: ['list_group_agents', 'get_messages', 'get_agent_skill', 'list_tasks', 'create_task', 'update_task', 'move_task_status'],
  orchestrator: [
    'list_group_agents',
    'list_conversation_agents',
    'get_messages',
    'get_agent_skill',
    'list_tasks',
    'create_task',
    'update_task',
    'move_task_status',
    'list_conversations',
    'get_group_info',
    'list_group_members',
    'list_knowledge_bases',
    'list_knowledge_files',
    'search_knowledge',
    'read_knowledge_file',
  ],
  agent_builder: ['list_agents', 'list_group_agents', 'get_agent_skill', 'list_agent_candidates', 'list_machines', 'get_agent_detail'],
  agent_manager: ['list_agents', 'get_agent_detail', 'update_agent_prompt', 'start_agent', 'stop_agent', 'get_agent_skill'],
  knowledge: ['list_knowledge_bases', 'list_knowledge_files', 'search_knowledge', 'read_knowledge_file'],
};

export const toolsetOptions = [
  { value: 'none', label: '无工具' },
  { value: 'basic', label: '基础群聊' },
  { value: 'tasks', label: '任务协作' },
  { value: 'orchestrator', label: 'Orchestrator' },
  { value: 'agent_builder', label: 'Agent 创建' },
  { value: 'agent_manager', label: 'Agent 管理' },
  { value: 'knowledge', label: '知识库' },
  { value: 'custom', label: '自定义' },
];

export function getTemplateTools(toolset: string): string[] {
  return toolsetTemplates[toolset] ?? toolsetTemplates.tasks ?? [];
}

export function parseToolsConfig(raw?: string): { toolset: string; allowedTools: string[] } {
  if (!raw) return { toolset: 'none', allowedTools: [] };
  try {
    const cfg: unknown = JSON.parse(raw);
    if (typeof cfg !== 'object' || cfg === null || Array.isArray(cfg)) {
      return { toolset: 'none', allowedTools: [] };
    }
    const record = cfg as Record<string, unknown>;
    const toolset = typeof record.toolset === 'string' && record.toolset in toolsetTemplates
      ? record.toolset
      : 'custom';
    const allowedTools = Array.isArray(record.allowed_tools)
      ? record.allowed_tools.filter((name: unknown): name is string => (
          typeof name === 'string' && toolCatalog.some((tool) => tool.name === name)
        ))
        : toolset !== 'custom' ? getTemplateTools(toolset) : [];
    return { toolset, allowedTools };
  } catch {
    return { toolset: 'none', allowedTools: [] };
  }
}

export interface CategoryMeta {
  label: string;
  color: string;
}

export const categoryMeta: Record<string, CategoryMeta> = {
  conversation: { label: '会话', color: '#1677ff' },
  task: { label: '任务', color: '#fa8c16' },
  agent: { label: 'Agent', color: '#722ed1' },
  machine: { label: '电脑', color: '#595959' },
  group: { label: '群聊', color: '#52c41a' },
  skill: { label: '技能', color: '#eb2f96' },
  knowledge: { label: '知识库', color: '#13c2c2' },
};

export const categoryOrder: string[] = [
  'conversation',
  'task',
  'agent',
  'machine',
  'group',
  'skill',
  'knowledge',
];

export function getToolsByCategory(): Record<string, ToolCatalogItem[]> {
  const groups: Record<string, ToolCatalogItem[]> = {};
  for (const cat of categoryOrder) {
    groups[cat] = [];
  }
  for (const tool of toolCatalog) {
    if (!groups[tool.category]) {
      groups[tool.category] = [];
    }
    groups[tool.category]!.push(tool);
  }
  return groups;
}

export function toolsConfigToJSON(toolset: string, allowedTools: string[]): string {
  const validTools = allowedTools.filter((name) => toolCatalog.some((tool) => tool.name === name));
  return JSON.stringify({
    toolset: toolset === 'custom' ? '' : toolset,
    allowed_tools: validTools,
  });
}
