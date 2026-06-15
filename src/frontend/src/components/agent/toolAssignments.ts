import { get } from '@/api/client';

// ---------------------------------------------------------------------------
// Tool categories (sourced from GET /api/tools/categories)
// ---------------------------------------------------------------------------

export interface ToolCategory {
  name: string;
  label: string;
  color: string;
  sort_order: number;
}

let _toolCategories: ToolCategory[] = [];

export async function fetchToolCategories(): Promise<void> {
  if (_toolCategories.length > 0) return;
  try {
    const list = await get<ToolCategory[]>('/api/tools/categories');
    if (Array.isArray(list)) {
      _toolCategories = [...list].sort((a, b) => a.sort_order - b.sort_order);
    }
  } catch (e) {
    console.warn('fetchToolCategories failed', e);
  }
}

export function getToolCategoriesSync(): ToolCategory[] {
  return _toolCategories;
}

export function getCategoryLabel(name: string): string {
  const c = _toolCategories.find((c) => c.name === name);
  return c?.label ?? name;
}

export function getCategoryColor(name: string): string {
  const c = _toolCategories.find((c) => c.name === name);
  return c?.color ?? '#595959';
}

export function getCategoryOrder(): string[] {
  return _toolCategories.map((c) => c.name);
}

export interface CategoryMeta {
  label: string;
  color: string;
}

/**
 * Build a category metadata lookup (label + color) from the API-backed
 * categories cache. Returns an empty record if the cache hasn't loaded yet.
 */
export function getCategoryMeta(): Record<string, CategoryMeta> {
  const map: Record<string, CategoryMeta> = {};
  for (const c of _toolCategories) {
    map[c.name] = { label: c.label, color: c.color };
  }
  return map;
}

export interface ToolCatalogItem {
  name: string;
  label: string;
  // Category is a free-form string. The closed union was removed so new
  // categories (e.g. `deployment`) introduced by the backend don't require
  // a frontend type change.
  category: string;
  description: string;
  is_management?: boolean;
}

export interface BuiltinTemplate {
  name: string;
  label: string;
  description: string;
  tool_names: string[];
}

// ---------------------------------------------------------------------------
// Builtin skill templates (sourced from GET /api/tools/builtin-skill-templates)
// ---------------------------------------------------------------------------

export interface BuiltinSkillTemplate {
  name: string;
  label: string;
  description: string;
  skill_categories: string[];
}

// Module-level mutable cache (populated by fetch functions).
let _toolCatalog: ToolCatalogItem[] = [];
let _toolsetTemplates: Record<string, string[]> = {};
let _builtinTemplates: BuiltinTemplate[] = [];
let _skillTemplates: BuiltinSkillTemplate[] = [];

let _fetchPromise: Promise<void> | null = null;

async function ensureLoaded(): Promise<void> {
  if (_fetchPromise) return _fetchPromise;
  _fetchPromise = (async () => {
    try {
      const [definitions, templates, categories, skillTemplates] = await Promise.all([
        get<ToolCatalogItem[]>('/api/tools/definitions'),
        get<BuiltinTemplate[]>('/api/tools/builtin-templates'),
        get<ToolCategory[]>('/api/tools/categories'),
        get<BuiltinSkillTemplate[]>('/api/tools/builtin-skill-templates'),
      ]);
      _toolCatalog = definitions ?? [];
      _builtinTemplates = templates ?? [];
      _skillTemplates = skillTemplates ?? [];
      if (Array.isArray(categories)) {
        _toolCategories = [...categories].sort((a, b) => a.sort_order - b.sort_order);
      }
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

export function getSkillTemplatesSync(): BuiltinSkillTemplate[] {
  return _skillTemplates;
}

export function getSkillTemplateOptions(): { value: string; label: string }[] {
  return [
    ..._skillTemplates.map((tpl) => ({ value: tpl.name, label: tpl.label })),
    { value: 'custom', label: '自定义' },
  ];
}

export function getTemplateSkillCategories(name: string): string[] {
  const tpl = _skillTemplates.find((t) => t.name === name);
  return tpl?.skill_categories ?? [];
}

/**
 * Derive the set of management tool names from the loaded catalog.
 * Returns an empty set when the catalog has not loaded yet (callers
 * that need the value during save should ensure `fetchToolCatalog` has
 * resolved first).
 */
export function getManagementTools(): Set<string> {
  const set = new Set<string>();
  for (const item of _toolCatalog) {
    if (item.is_management) set.add(item.name);
  }
  return set;
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

export function getToolsByCategory(): Record<string, ToolCatalogItem[]> {
  const groups: Record<string, ToolCatalogItem[]> = {};
  for (const cat of getCategoryOrder()) {
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
  // Do NOT filter against _toolCatalog here — the catalog may be empty due to
  // a failed/late fetch, which would silently discard all tools on save.
  // The backend normalizeToolNames already validates against the in-memory
  // tool registry (single source of truth).
  return JSON.stringify({
    toolset: toolset === 'custom' ? '' : toolset,
    allowed_tools: allowedTools,
  });
}

// ---------------------------------------------------------------------------
// Management tools helpers (moved here from the deleted @/config/catalogConfig).
//
// Management membership comes from the API (`is_management` flag on tool
// definitions). These helpers accept an optional `managementSet` parameter so
// updated callers can pass the dynamic set returned by `getManagementTools()`.
// Callers that haven't been migrated yet get the legacy fallback set so the
// function still produces a sensible result before the catalog finishes loading.
// ---------------------------------------------------------------------------

const fallbackManagementTools = new Set([
  'create_agent',
  'update_agent',
  'delete_agent',
]);

export function hasManagementToolsInArray(
  tools: string[],
  managementSet?: Set<string>,
): boolean {
  const set = managementSet ?? fallbackManagementTools;
  return tools.some((tool) => set.has(tool));
}

export function hasManagementToolsInConfig(
  toolsConfig: string,
  managementSet?: Set<string>,
): boolean {
  try {
    const cfg = JSON.parse(toolsConfig) as { allowed_tools?: unknown };
    const set = managementSet ?? fallbackManagementTools;
    return Array.isArray(cfg.allowed_tools)
      && cfg.allowed_tools.some((tool) => typeof tool === 'string' && set.has(tool));
  } catch {
    return false;
  }
}
