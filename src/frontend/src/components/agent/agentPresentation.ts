import type { Agent } from '@/types/agent';

export interface Skill {
  name: string;
  description?: string;
  detail?: string;
  source_path?: string;
  auto?: boolean;
}

export function parseCapabilities(value?: string): string[] {
  if (!value) return [];
  try {
    const parsed: unknown = JSON.parse(value);
    if (Array.isArray(parsed)) {
      return parsed.map((item): string => {
        if (typeof item === 'string') return item;
        if (item && typeof item === 'object' && 'name' in item && typeof (item as Skill).name === 'string') {
          return (item as Skill).name;
        }
        return '';
      }).filter(Boolean);
    }
  } catch {
    return value.split(',').map((item) => item.trim()).filter(Boolean);
  }
  return [];
}

export function parseSkills(value?: string): Skill[] {
  if (!value) return [];
  try {
    const parsed: unknown = JSON.parse(value);
    if (Array.isArray(parsed)) {
      return parsed
        .map((item): Skill | null => {
          if (typeof item === 'string') return { name: item };
          if (item && typeof item === 'object' && 'name' in item && typeof (item as Skill).name === 'string') {
            return item as Skill;
          }
          return null;
        })
        .filter((item): item is Skill => item !== null && item.name.trim().length > 0);
    }
  } catch {
    return value.split(',').map((item) => item.trim()).filter(Boolean).map((name) => ({ name }));
  }
  return [];
}

export function autoGenerateSkills(agent: Agent): Skill[] {
  const tool = agent.cli_tool.toLowerCase();
  const prompt = (agent.system_prompt || '').toLowerCase();
  const skills: Skill[] = [];

  if (tool.includes('claude') || tool.includes('claude-code')) {
    skills.push({ name: '代码生成', description: '根据需求生成高质量代码', auto: true });
    skills.push({ name: '代码审查', description: '分析代码质量、找 bug 和改进点', auto: true });
    skills.push({ name: '调试', description: '定位并修复代码错误', auto: true });
    skills.push({ name: '重构', description: '优化代码结构和可读性', auto: true });
  }
  if (tool.includes('codex') || tool.includes('opencode') || tool.includes('openai')) {
    skills.push({ name: '代码补全', description: '智能补全代码片段', auto: true });
    skills.push({ name: '代码生成', description: '根据注释或描述生成代码', auto: true });
  }

  if (prompt.includes('test') || prompt.includes('测试')) {
    skills.push({ name: '测试编写', description: '编写单元测试和集成测试', auto: true });
  }
  if (prompt.includes('document') || prompt.includes('文档')) {
    skills.push({ name: '文档撰写', description: '生成代码注释和技术文档', auto: true });
  }
  if (prompt.includes('review') || prompt.includes('审查')) {
    skills.push({ name: '代码审查', description: '审查代码变更', auto: true });
  }
  if (prompt.includes('architect') || prompt.includes('架构')) {
    skills.push({ name: '架构设计', description: '系统和组件架构规划', auto: true });
  }

  if (skills.length === 0) {
    skills.push({ name: agent.name, description: `${agent.cli_tool} CLI 工具`, auto: true });
  }

  const seen = new Set<string>();
  return skills.filter((s) => {
    if (seen.has(s.name)) return false;
    seen.add(s.name);
    return true;
  });
}

export function getAgentDescription(agent: Agent): string {
  if (agent.system_prompt) return agent.system_prompt;

  switch (agent.cli_tool) {
    case 'claude':
      return 'Claude Code 本地 CLI Agent，适合代码生成、项目理解、重构、评审与 Orchestrator 意图拆解。';
    case 'codex':
      return 'Codex 本地 CLI Agent，适合代码实现、补丁生成、测试修复和工程化任务。';
    case 'opencode':
      return 'OpenCode 本地 CLI Agent，适合通用代码任务和命令行开发工作流。';
    default:
      return '通过本地守护进程或用户配置接入的 Agent，可在后续对话链路中承担任务执行。';
  }
}

export function getRuntimeLabel(agent: Agent): string {
  switch (agent.cli_tool) {
    case 'claude':
      return 'Claude Code CLI';
    case 'codex':
      return 'Codex CLI';
    case 'opencode':
      return 'OpenCode CLI';
    default:
      return agent.cli_tool;
  }
}

export function getModelLabel(agent: Agent): string {
  switch (agent.cli_tool) {
    case 'claude':
      return 'Claude Code Default';
    case 'codex':
      return 'Codex CLI Default';
    case 'opencode':
      return 'OpenCode Default';
    default:
      return 'CLI Default';
  }
}

export function formatDateTime(value?: string): string {
  if (!value) return '暂无';
  return new Date(value).toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  });
}
