/**
 * Unified Catalog API client.
 *
 * All 4 catalog domains (platform_skill, tool_definition, agent_prompt_template,
 * user_template) are accessed through this single module using the backend's
 * /api/catalog/:domain endpoints.
 */
import { get, post, put, del } from './client';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type CatalogDomain =
  | 'platform_skill'
  | 'tool_definition'
  | 'agent_prompt_template'
  | 'user_template';

export interface CatalogItem {
  id: string;
  domain: string;
  key: string;
  label: string;
  category?: string;
  description?: string;
  subtype?: string;
  payload?: string; // JSON string, domain-specific
  user_id?: string;
  created_at: string;
  updated_at: string;
}

export interface CatalogCreateInput {
  key: string;
  label: string;
  category?: string;
  description?: string;
  subtype?: string;
  payload?: string;
}

export interface CatalogUpdateInput {
  key?: string;
  label?: string;
  category?: string;
  description?: string;
  payload?: string;
}

export interface CatalogListParams {
  subtype?: string;
  category?: string;
}

// ---------------------------------------------------------------------------
// API calls
// ---------------------------------------------------------------------------

export async function listCatalog(
  domain: CatalogDomain,
  params?: CatalogListParams,
): Promise<CatalogItem[]> {
  const query = new URLSearchParams();
  if (params?.subtype) query.set('subtype', params.subtype);
  if (params?.category) query.set('category', params.category);
  const qs = query.toString();
  const path = `/api/catalog/${domain}${qs ? `?${qs}` : ''}`;
  const items = await get<CatalogItem[] | null>(path);
  return items ?? [];
}

export async function getCatalogItem(
  domain: CatalogDomain,
  id: string,
): Promise<CatalogItem> {
  return get<CatalogItem>(`/api/catalog/${domain}/${id}`);
}

export async function createCatalogItem(
  domain: CatalogDomain,
  data: CatalogCreateInput,
): Promise<CatalogItem> {
  return post<CatalogItem>(`/api/catalog/${domain}`, data);
}

export async function updateCatalogItem(
  domain: CatalogDomain,
  id: string,
  data: CatalogUpdateInput,
): Promise<CatalogItem> {
  return put<CatalogItem>(`/api/catalog/${domain}/${id}`, data);
}

export async function deleteCatalogItem(
  domain: CatalogDomain,
  id: string,
): Promise<void> {
  return del<void>(`/api/catalog/${domain}/${id}`);
}

export async function importCatalogDefaults(
  domain: CatalogDomain,
): Promise<CatalogItem[]> {
  const items = await post<CatalogItem[] | null>(
    `/api/catalog/${domain}/defaults`,
  );
  return items ?? [];
}
