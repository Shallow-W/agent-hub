/**
 * Agent Prompt Template API — backward-compatible wrapper over the unified catalog API.
 *
 * Exports the same function signatures that existing components rely on.
 */
import type { CatalogItem } from './catalog';
import {
  listCatalog,
  createCatalogItem,
  updateCatalogItem,
  deleteCatalogItem,
  importCatalogDefaults,
} from './catalog';
import type {
  AgentPromptTemplate,
  AgentPromptTemplateRequest,
} from '@/types/agent';

const DOMAIN = 'agent_prompt_template' as const;

// ---------------------------------------------------------------------------
// CatalogItem <-> AgentPromptTemplate mappers
// ---------------------------------------------------------------------------

interface AgentPromptPayload {
  system_prompt?: string;
}

export function itemToTemplate(item: CatalogItem): AgentPromptTemplate {
  let payload: AgentPromptPayload = {};
  if (item.payload) {
    try {
      payload = JSON.parse(item.payload) as AgentPromptPayload;
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
    system_prompt: payload.system_prompt,
    created_at: item.created_at,
    updated_at: item.updated_at,
  };
}

function templateToCreateInput(body: AgentPromptTemplateRequest) {
  return {
    key: body.name,
    label: body.name,
    category: body.category,
    description: body.description,
    payload: JSON.stringify({
      system_prompt: body.system_prompt,
    } satisfies AgentPromptPayload),
  };
}

// ---------------------------------------------------------------------------
// Public API (same signatures as before)
// ---------------------------------------------------------------------------

export async function getAgentPromptTemplates(): Promise<AgentPromptTemplate[]> {
  const items = await listCatalog(DOMAIN);
  return items.map(itemToTemplate);
}

export async function createAgentPromptTemplate(
  body: AgentPromptTemplateRequest,
): Promise<AgentPromptTemplate> {
  const item = await createCatalogItem(DOMAIN, templateToCreateInput(body));
  return itemToTemplate(item);
}

export async function importDefaultAgentPromptTemplates(): Promise<AgentPromptTemplate[]> {
  const items = await importCatalogDefaults(DOMAIN);
  return items.map(itemToTemplate);
}

export async function updateAgentPromptTemplate(
  id: string,
  body: AgentPromptTemplateRequest,
): Promise<AgentPromptTemplate> {
  const payload: AgentPromptPayload = {
    system_prompt: body.system_prompt,
  };
  const item = await updateCatalogItem(DOMAIN, id, {
    key: body.name,
    label: body.name,
    category: body.category,
    description: body.description,
    payload: JSON.stringify(payload),
  });
  return itemToTemplate(item);
}

export async function deleteAgentPromptTemplate(id: string): Promise<void> {
  return deleteCatalogItem(DOMAIN, id);
}
