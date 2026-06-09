export interface ToolCatalogItem {
  name: string;
  label: string;
  category: 'conversation' | 'task' | 'agent' | 'machine' | 'group' | 'skill';
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
  ],
  agent_builder: ['list_agents', 'list_group_agents', 'get_agent_skill', 'list_agent_candidates', 'list_machines'],
};

export const toolsetOptions = [
  { value: 'none', label: '无工具' },
  { value: 'basic', label: '基础群聊' },
  { value: 'tasks', label: '任务协作' },
  { value: 'orchestrator', label: 'Orchestrator' },
  { value: 'agent_builder', label: 'Agent 创建' },
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

export function toolsConfigToJSON(toolset: string, allowedTools: string[]): string {
  const validTools = allowedTools.filter((name) => toolCatalog.some((tool) => tool.name === name));
  return JSON.stringify({
    toolset: toolset === 'custom' ? '' : toolset,
    allowed_tools: validTools,
  });
}
