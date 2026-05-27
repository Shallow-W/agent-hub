import { get, post, put, del } from './client';
import type { Message, MessageRole } from '@/types/message';
import type { AttachmentPayload } from '@/types/attachment';

export async function sendMessage(
  conversationId: string,
  content: string,
  role: MessageRole,
  attachments?: AttachmentPayload[],
  replyToId?: string,
): Promise<Message> {
  return post<Message>(`/api/conversations/${conversationId}/messages`, {
    content,
    role,
    attachments: attachments ?? [],
    ...(replyToId ? { reply_to: replyToId } : {}),
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

export async function recallMessage(
  conversationId: string,
  messageId: string,
): Promise<void> {
  return del<void>(`/api/conversations/${conversationId}/messages/${messageId}`);
}

export async function markAsRead(conversationId: string): Promise<void> {
  return put<void>(`/api/conversations/${conversationId}/read`);
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
