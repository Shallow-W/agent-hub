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
// Types (preserved from the original module)
// ---------------------------------------------------------------------------

export interface UserTemplate {
  id: string;
  user_id: string;
  type: 'tools' | 'skills';
  name: string;
  content: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

// ---------------------------------------------------------------------------
// CatalogItem <-> UserTemplate mappers
// ---------------------------------------------------------------------------

function itemToUserTemplate(item: CatalogItem): UserTemplate {
  let content: Record<string, unknown> = {};
  if (item.payload) {
    try {
      content = JSON.parse(item.payload) as Record<string, unknown>;
    } catch {
      // keep empty content
    }
  }
  return {
    id: item.id,
    user_id: item.user_id ?? '',
    type: (item.subtype as 'tools' | 'skills') ?? 'tools',
    name: item.key,
    content,
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
  content: Record<string, unknown>;
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
    content: Record<string, unknown>;
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
