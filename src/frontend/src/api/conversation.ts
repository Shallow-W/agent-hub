import { get, post, put, del } from './client';
import type { Conversation, ConversationAgent, ConversationType } from '@/types/conversation';

export async function getConversations(): Promise<Conversation[]> {
  return get<Conversation[]>('/api/conversations');
}

export async function createConversation(
  type: ConversationType,
  title: string,
): Promise<Conversation> {
  return post<Conversation>('/api/conversations', { type, title });
}

export async function deleteConversation(id: string): Promise<void> {
  return del<void>(`/api/conversations/${id}`);
}

export async function togglePin(
  id: string,
  pinned: boolean,
): Promise<void> {
  return put<void>(`/api/conversations/${id}/pin`, { pinned });
}

export async function getConversationAgents(id: string): Promise<ConversationAgent[]> {
  const agents = await get<ConversationAgent[] | null>(`/api/conversations/${id}/agents`);
  return agents ?? [];
}

export async function addConversationAgent(
  id: string,
  agentId: string,
): Promise<ConversationAgent> {
  return post<ConversationAgent>(`/api/conversations/${id}/agents`, { agent_id: agentId });
}

export async function removeConversationAgent(
  id: string,
  agentId: string,
): Promise<void> {
  return del<void>(`/api/conversations/${id}/agents/${agentId}`);
}
