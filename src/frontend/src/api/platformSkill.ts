/**
 * Platform Skill API — backward-compatible wrapper over the unified catalog API.
 *
 * Existing components import { getPlatformSkills, createPlatformSkill, ... } and
 * receive the same legacy shape { id, name, category, description, trigger, detail }.
 * Internally every call is routed through the catalog module.
 */
import type { CatalogItem } from './catalog';
import {
  listCatalog,
  createCatalogItem,
  updateCatalogItem,
  deleteCatalogItem,
  importCatalogDefaults,
} from './catalog';
import type { PlatformSkill, PlatformSkillRequest } from '@/types/agent';

const DOMAIN = 'platform_skill' as const;

// ---------------------------------------------------------------------------
// CatalogItem <-> PlatformSkill mappers
// ---------------------------------------------------------------------------

interface PlatformSkillPayload {
  trigger?: string;
  detail?: string;
}

function itemToSkill(item: CatalogItem): PlatformSkill {
  let payload: PlatformSkillPayload = {};
  if (item.payload) {
    try {
      payload = JSON.parse(item.payload) as PlatformSkillPayload;
    } catch {
      // keep empty payload
    }
  }
  return {
    id: item.id,
    user_id: item.user_id ?? '',
    name: item.key,
    category: item.category,
    description: item.description,
    trigger: payload.trigger,
    detail: payload.detail,
    created_at: item.created_at,
    updated_at: item.updated_at,
  };
}

function skillToCreateInput(body: PlatformSkillRequest) {
  return {
    key: body.name,
    label: body.name,
    category: body.category,
    description: body.description,
    payload: JSON.stringify({
      trigger: body.trigger,
      detail: body.detail,
    } satisfies PlatformSkillPayload),
  };
}

// ---------------------------------------------------------------------------
// Public API (same signatures as before)
// ---------------------------------------------------------------------------

export async function getPlatformSkills(): Promise<PlatformSkill[]> {
  const items = await listCatalog(DOMAIN);
  return items.map(itemToSkill);
}

export async function createPlatformSkill(
  body: PlatformSkillRequest,
): Promise<PlatformSkill> {
  const item = await createCatalogItem(DOMAIN, skillToCreateInput(body));
  return itemToSkill(item);
}

export async function importDefaultPlatformSkills(): Promise<PlatformSkill[]> {
  const items = await importCatalogDefaults(DOMAIN);
  return items.map(itemToSkill);
}

export async function updatePlatformSkill(
  id: string,
  body: PlatformSkillRequest,
): Promise<PlatformSkill> {
  const payload: PlatformSkillPayload = {
    trigger: body.trigger,
    detail: body.detail,
  };
  const item = await updateCatalogItem(DOMAIN, id, {
    key: body.name,
    label: body.name,
    category: body.category,
    description: body.description,
    payload: JSON.stringify(payload),
  });
  return itemToSkill(item);
}

export async function deletePlatformSkill(id: string): Promise<void> {
  return deleteCatalogItem(DOMAIN, id);
}
