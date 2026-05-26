import { get, post } from './client';
import type { Message, MessageRole } from '@/types/message';

export async function sendMessage(
  conversationId: string,
  content: string,
  role: MessageRole,
): Promise<Message> {
  return post<Message>(`/api/conversations/${conversationId}/messages`, {
    content,
    role,
  });
}

export async function getMessages(
  conversationId: string,
  before?: string,
  limit?: number,
): Promise<Message[]> {
  const params = new URLSearchParams();
  if (before) params.set('before', before);
  if (limit) params.set('limit', String(limit));

  const qs = params.toString();
  const path = `/api/conversations/${conversationId}/messages${qs ? `?${qs}` : ''}`;
  return get<Message[]>(path);
}

export async function getUnreadMessages(
  conversationId: string,
  limit?: number,
): Promise<Message[]> {
  const params = new URLSearchParams();
  if (limit) params.set('limit', String(limit));
  const qs = params.toString();
  const path = `/api/conversations/${conversationId}/messages/unread${qs ? `?${qs}` : ''}`;
  return get<Message[]>(path);
}
