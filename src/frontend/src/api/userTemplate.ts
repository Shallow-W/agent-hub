/**
 * User Template API — backward-compatible wrapper over the unified catalog API.
 *
 * Exports the same function signatures and types that existing components rely on.
 */
import type { CatalogItem } from './catalog';
import {
  listCatalog,
  createCatalogItem,
  updateCatalogItem,
  deleteCatalogItem,
} from './catalog';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface ToolTemplateContent {
  tools: string[];
}

export interface SkillTemplateContent {
  skill_ids: string[];
}

export type UserTemplateContent = ToolTemplateContent | SkillTemplateContent;

export interface UserTemplate {
  id: string;
  user_id: string;
  type: 'tools' | 'skills';
  name: string;
  content: UserTemplateContent;
  created_at: string;
  updated_at: string;
}

// ---------------------------------------------------------------------------
// CatalogItem <-> UserTemplate mappers
// ---------------------------------------------------------------------------

function parseContent(raw: string | undefined, type: 'tools' | 'skills'): UserTemplateContent {
  if (!raw) return type === 'tools' ? { tools: [] } : { skill_ids: [] };
  try {
    const parsed = JSON.parse(raw);
    if (type === 'tools' && Array.isArray((parsed as Record<string, unknown>)?.tools)) {
      return { tools: (parsed as { tools: unknown }).tools as string[] };
    }
    if (type === 'skills' && Array.isArray((parsed as Record<string, unknown>)?.skill_ids)) {
      return { skill_ids: (parsed as { skill_ids: unknown }).skill_ids as string[] };
    }
    return type === 'tools' ? { tools: [] } : { skill_ids: [] };
  } catch {
    return type === 'tools' ? { tools: [] } : { skill_ids: [] };
  }
}

function itemToUserTemplate(item: CatalogItem): UserTemplate {
  const type = (item.subtype as 'tools' | 'skills') ?? 'tools';
  return {
    id: item.id,
    user_id: item.user_id ?? '',
    type,
    name: item.key,
    content: parseContent(item.payload, type),
    created_at: item.created_at,
    updated_at: item.updated_at,
  };
}

const DOMAIN = 'user_template' as const;

// ---------------------------------------------------------------------------
// Public API (same signatures as before)
// ---------------------------------------------------------------------------

export async function listUserTemplates(
  type: 'tools' | 'skills',
): Promise<UserTemplate[]> {
  const items = await listCatalog(DOMAIN, { subtype: type });
  return items.map(itemToUserTemplate);
}

export async function createUserTemplate(body: {
  type: 'tools' | 'skills';
  name: string;
  content: UserTemplateContent;
}): Promise<UserTemplate> {
  const item = await createCatalogItem(DOMAIN, {
    key: body.name,
    label: body.name,
    subtype: body.type,
    payload: JSON.stringify(body.content),
  });
  return itemToUserTemplate(item);
}

export async function updateUserTemplate(
  id: string,
  body: {
    type: 'tools' | 'skills';
    name: string;
    content: UserTemplateContent;
  },
): Promise<UserTemplate> {
  const item = await updateCatalogItem(DOMAIN, id, {
    key: body.name,
    label: body.name,
    payload: JSON.stringify(body.content),
  });
  return itemToUserTemplate(item);
}

export async function deleteUserTemplate(id: string): Promise<void> {
  return deleteCatalogItem(DOMAIN, id);
}
