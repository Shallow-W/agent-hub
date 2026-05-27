import { get, post, put, del } from './client';
import type { Conversation, ConversationType } from '@/types/conversation';

export async function getConversations(): Promise<Conversation[]> {
  return get<Conversation[]>('/api/conversations');
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

export async function deleteConversation(id: string): Promise<void> {
  return del<void>(`/api/conversations/${id}`);
}

export async function togglePin(
  id: string,
  pinned: boolean,
): Promise<void> {
  return put<void>(`/api/conversations/${id}/pin`, { pinned });
}

export async function markConversationRead(id: string): Promise<void> {
  return put<void>(`/api/conversations/${id}/read`);
}
