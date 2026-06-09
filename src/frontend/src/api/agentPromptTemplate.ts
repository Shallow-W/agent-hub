import { del, get, post, put } from './client';
import type { AgentPromptTemplate, AgentPromptTemplateRequest } from '@/types/agent';

const BASE = '/api/agent-prompt-templates';

export async function getAgentPromptTemplates(): Promise<AgentPromptTemplate[]> {
  const templates = await get<AgentPromptTemplate[] | null>(BASE);
  return templates ?? [];
}

export async function createAgentPromptTemplate(
  body: AgentPromptTemplateRequest,
): Promise<AgentPromptTemplate> {
  return post<AgentPromptTemplate>(BASE, body);
}

export async function importDefaultAgentPromptTemplates(): Promise<AgentPromptTemplate[]> {
  const templates = await post<AgentPromptTemplate[] | null>(`${BASE}/import-defaults`);
  return templates ?? [];
}

export async function updateAgentPromptTemplate(
  id: string,
  body: AgentPromptTemplateRequest,
): Promise<AgentPromptTemplate> {
  return put<AgentPromptTemplate>(`${BASE}/${id}`, body);
}

export async function deleteAgentPromptTemplate(id: string): Promise<void> {
  return del<void>(`${BASE}/${id}`);
}
