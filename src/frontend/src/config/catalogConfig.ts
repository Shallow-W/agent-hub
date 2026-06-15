/**
 * Centralised catalog configuration constants.
 *
 * Tool category metadata and management-tool membership are now sourced
 * from the backend (see `components/agent/toolAssignments.ts`); this file
 * only retains UX-only constants (skill category shortcuts, quick
 * templates) and the backward-compatible `hasManagementTools*` helpers.
 */

// ---------------------------------------------------------------------------
// Management tools helpers (backward-compatible)
//
// The hardcoded Set is gone — management membership now comes from the
// API (`is_management` flag on tool definitions). These helpers accept an
// optional `managementSet` parameter so updated callers can pass the
// dynamic set returned by `getManagementTools()`. Callers that haven't
// been migrated yet get the legacy fallback set so the function still
// produces a sensible result before the catalog finishes loading.
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

// ---------------------------------------------------------------------------
// Skill category shortcuts (UX-only data — no backend equivalent)
// ---------------------------------------------------------------------------

export const defaultSkillCategories = ['产品经理', '开发人员'];

// ---------------------------------------------------------------------------
// Quick templates (UX-only data — labels for the "快速模板" pill row)
// ---------------------------------------------------------------------------

export interface QuickTemplate {
  key: string;
  label: string;
  toolset: string;
  skillCategories: string[];
}

export const quickTemplates: QuickTemplate[] = [
  { key: 'pm', label: '产品经理', toolset: 'tasks', skillCategories: ['产品经理'] },
  { key: 'dev', label: '开发人员', toolset: 'orchestrator', skillCategories: ['开发人员'] },
  { key: 'manager', label: '管理助手', toolset: 'orchestrator', skillCategories: [] },
  { key: 'empty', label: '空白', toolset: 'none', skillCategories: [] },
];
