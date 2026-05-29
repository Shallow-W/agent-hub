import { get, post, del } from './client';
import type { Message, SendMessageResult, MessageRole } from '@/types/message';
import type { AttachmentPayload } from '@/types/attachment';

export async function sendMessage(
  conversationId: string,
  content: string,
  role: MessageRole,
  attachments?: AttachmentPayload[],
  replyToId?: string,
  mentions?: string[],
  agentId?: string,
): Promise<SendMessageResult> {
  return post<SendMessageResult>(`/api/conversations/${conversationId}/messages`, {
    content,
    role,
    attachments: attachments ?? [],
    ...(replyToId ? { reply_to: replyToId } : {}),
    ...(mentions && mentions.length > 0 ? { mentions } : {}),
    ...(agentId ? { agent_id: agentId } : {}),
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
