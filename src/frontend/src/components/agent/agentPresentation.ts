import type { Agent } from '@/types/agent';

export function parseCapabilities(value?: string): string[] {
  if (!value) return [];
  try {
    const parsed: unknown = JSON.parse(value);
    if (Array.isArray(parsed)) {
      return parsed.filter((item): item is string => typeof item === 'string');
    }
  } catch {
    return value.split(',').map((item) => item.trim()).filter(Boolean);
  }
  return [];
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
