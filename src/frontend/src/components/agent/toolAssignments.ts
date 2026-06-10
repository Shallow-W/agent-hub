import { get } from '@/api/client';

export interface ToolCatalogItem {
  name: string;
  label: string;
  category: 'conversation' | 'task' | 'agent' | 'machine' | 'group' | 'skill' | 'knowledge';
  description: string;
}

export interface BuiltinTemplate {
  name: string;
  label: string;
  description: string;
  tool_names: string[];
}

// Module-level mutable cache (populated by fetch functions).
let _toolCatalog: ToolCatalogItem[] = [];
let _toolsetTemplates: Record<string, string[]> = {};
let _builtinTemplates: BuiltinTemplate[] = [];

let _fetchPromise: Promise<void> | null = null;

async function ensureLoaded(): Promise<void> {
  if (_fetchPromise) return _fetchPromise;
  _fetchPromise = (async () => {
    try {
      const [definitions, templates] = await Promise.all([
        get<ToolCatalogItem[]>('/api/tools/definitions'),
        get<BuiltinTemplate[]>('/api/tools/builtin-templates'),
      ]);
      _toolCatalog = definitions ?? [];
      _builtinTemplates = templates ?? [];
      _toolsetTemplates = {};
      for (const tpl of _builtinTemplates) {
        _toolsetTemplates[tpl.name] = tpl.tool_names ?? [];
      }
    } catch {
      // Silently use empty catalog if fetch fails
    }
  })();
  return _fetchPromise;
}

export function getToolCatalogSync(): ToolCatalogItem[] {
  return _toolCatalog;
}

export function getToolsetTemplatesSync(): Record<string, string[]> {
  return _toolsetTemplates;
}

export function getBuiltinTemplatesSync(): BuiltinTemplate[] {
  return _builtinTemplates;
}

/**
 * fetchToolCatalog fetches the tool catalog and toolset templates from the API,
 * populating the module-level cache. Call this once on app mount (or early in the page lifecycle).
 */
export async function fetchToolCatalog(): Promise<ToolCatalogItem[]> {
  await ensureLoaded();
  return _toolCatalog;
}

/**
 * fetchBuiltinTemplates fetches the built-in toolset templates from the API.
 */
export async function fetchBuiltinTemplates(): Promise<Record<string, string[]>> {
  await ensureLoaded();
  return { ..._toolsetTemplates };
}

/**
 * fetchTemplateOptions returns toolset template options suitable for Select components.
 */
export async function fetchTemplateOptions(): Promise<{ value: string; label: string }[]> {
  await ensureLoaded();
  return [
    ..._builtinTemplates.map((tpl) => ({ value: tpl.name, label: tpl.label })),
    { value: 'custom', label: '自定义' },
  ];
}

export function getToolsetOptions(): { value: string; label: string }[] {
  return [
    ..._builtinTemplates.map((tpl) => ({ value: tpl.name, label: tpl.label })),
    { value: 'custom', label: '自定义' },
  ];
}

export function getTemplateTools(toolset: string): string[] {
  return _toolsetTemplates[toolset] ?? [];
}

export function parseToolsConfig(raw?: string): { toolset: string; allowedTools: string[] } {
  if (!raw) return { toolset: 'none', allowedTools: [] };
  try {
    const cfg: unknown = JSON.parse(raw);
    if (typeof cfg !== 'object' || cfg === null || Array.isArray(cfg)) {
      return { toolset: 'none', allowedTools: [] };
    }
    const record = cfg as Record<string, unknown>;
    // Trust the toolset name from the backend directly; do not require it to exist in
    // _toolsetTemplates because the catalog fetch may not have completed yet.
    const toolset = typeof record.toolset === 'string' && record.toolset !== ''
      ? record.toolset
      : 'custom';
    // Trust tool names from the backend directly — do not filter against _toolCatalog,
    // because _toolCatalog may still be empty (async fetch not yet completed) when this
    // function runs during component mount, which would incorrectly discard all tools.
    const allowedTools = Array.isArray(record.allowed_tools)
      ? record.allowed_tools.filter((name: unknown): name is string => typeof name === 'string')
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
  for (const tool of _toolCatalog) {
    if (!groups[tool.category]) {
      groups[tool.category] = [];
    }
    groups[tool.category]!.push(tool);
  }
  return groups;
}

export function toolsConfigToJSON(toolset: string, allowedTools: string[]): string {
  const validTools = allowedTools.filter((name) => _toolCatalog.some((tool) => tool.name === name));
  return JSON.stringify({
    toolset: toolset === 'custom' ? '' : toolset,
    allowed_tools: validTools,
  });
}
