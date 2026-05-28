import { get, post } from './client';
import type { Message, MessageRole, SendMessageResult } from '@/types/message';

export async function sendMessage(
  conversationId: string,
  content: string,
  role: MessageRole,
  agentId?: string,
): Promise<SendMessageResult> {
  return post<SendMessageResult>(`/api/conversations/${conversationId}/messages`, {
    content,
    role,
    agent_id: agentId,
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
