/**
 * Centralised catalog configuration constants.
 *
 * Domain knowledge that was previously hardcoded across multiple components
 * is consolidated here so it can be maintained in a single place.
 */

// ---------------------------------------------------------------------------
// Tool category metadata (previously in toolAssignments.ts)
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Management tools set (previously in AgentProfile.tsx / ComputerProfile.tsx)
// ---------------------------------------------------------------------------

export const managementTools = new Set([
  'create_agent',
  'update_agent',
  'delete_agent',
]);

export function hasManagementToolsInArray(tools: string[]): boolean {
  return tools.some((tool) => managementTools.has(tool));
}

export function hasManagementToolsInConfig(toolsConfig: string): boolean {
  try {
    const cfg = JSON.parse(toolsConfig) as { allowed_tools?: unknown };
    return Array.isArray(cfg.allowed_tools)
      && cfg.allowed_tools.some((tool) => typeof tool === 'string' && managementTools.has(tool));
  } catch {
    return false;
  }
}

// ---------------------------------------------------------------------------
// Skill category shortcuts (previously inline in AgentSkillsPanel.tsx)
// ---------------------------------------------------------------------------

export const defaultSkillCategories = ['产品经理', '开发人员'];

// ---------------------------------------------------------------------------
// Quick templates (previously in AgentCreateModal.tsx)
// ---------------------------------------------------------------------------

export interface QuickTemplate {
  key: string;
  label: string;
  toolset: string;
  skillCategories: string[];
}

export const quickTemplates: QuickTemplate[] = [
  { key: 'pm', label: '产品经理', toolset: 'tasks', skillCategories: ['产品经理'] },
  { key: 'dev', label: '开发人员', toolset: 'full', skillCategories: ['开发人员'] },
  { key: 'manager', label: '管理助手', toolset: 'full', skillCategories: [] },
  { key: 'empty', label: '空白', toolset: 'none', skillCategories: [] },
];
