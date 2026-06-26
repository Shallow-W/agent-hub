import { get, post, put, del } from './client';
import type { Conversation, ConversationAgent, ConversationType } from '@/types/conversation';
import type { ConversationAgentRole } from '@/types/role';

export async function getConversations(): Promise<Conversation[]> {
  return get<Conversation[]>('/api/conversations?limit=100');
}

export async function createConversation(
  type: ConversationType,
  title: string,
): Promise<Conversation> {
  return post<Conversation>('/api/conversations', { type, title });
}

export async function getOrCreatePrivateChat(
  friendId: string,
): Promise<Conversation> {
  return post<Conversation>('/api/conversations/private', { friend_id: friendId });
}

export async function getOrCreateAgentChat(agentId: string): Promise<Conversation> {
  return post<Conversation>('/api/conversations/agent', { agent_id: agentId });
}

export async function deleteConversation(id: string): Promise<void> {
  return del<void>(`/api/conversations/${id}`);
}

export async function togglePin(id: string): Promise<void> {
  return post<void>(`/api/conversations/${id}/pin`);
}

export async function archiveConversation(id: string): Promise<void> {
  return post<void>(`/api/conversations/${id}/archive`);
}

export async function unarchiveConversation(id: string): Promise<void> {
  return post<void>(`/api/conversations/${id}/unarchive`);
}

export async function getArchivedConversations(): Promise<Conversation[]> {
  return get<Conversation[]>('/api/conversations/archived');
}

export async function renameConversation(
  id: string,
  title: string,
): Promise<void> {
  return put<void>(`/api/conversations/${id}`, { title });
}

export async function markConversationRead(id: string): Promise<void> {
  return put<void>(`/api/conversations/${id}/read`);
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

export async function setConversationAgentRole(
  id: string,
  agentId: string,
  role: ConversationAgentRole,
): Promise<void> {
  return put<void>(`/api/conversations/${id}/agents/${agentId}/role`, { role });
}
