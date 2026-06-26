import { get, post, put, patch, del } from './client';
import type { ConversationBlackboard, Message, PinnedMessage, SendMessageResult, MessageRole } from '@/types/message';
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

export async function pinMessage(
  conversationId: string,
  messageId: string,
): Promise<void> {
  return post<void>(`/api/conversations/${conversationId}/messages/${messageId}/pin`);
}

export async function unpinMessage(
  conversationId: string,
  messageId: string,
): Promise<void> {
  return del<void>(`/api/conversations/${conversationId}/messages/${messageId}/pin`);
}

export async function getPinnedContext(
  conversationId: string,
): Promise<PinnedMessage[]> {
  return get<PinnedMessage[]>(`/api/conversations/${conversationId}/pinned-context`);
}

export async function getConversationBlackboard(
  conversationId: string,
): Promise<ConversationBlackboard> {
  return get<ConversationBlackboard>(`/api/conversations/${conversationId}/blackboard`);
}

export async function updateConversationBlackboard(
  conversationId: string,
  manualContext: string,
): Promise<ConversationBlackboard> {
  return put<ConversationBlackboard>(`/api/conversations/${conversationId}/blackboard`, {
    manual_context: manualContext,
  });
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

export async function getMessageReplies(
  conversationId: string,
  messageId: string,
): Promise<Message[]> {
  return get<Message[]>(`/api/conversations/${conversationId}/messages/${messageId}/replies`);
}

export async function hideMessage(
  conversationId: string,
  messageId: string,
): Promise<void> {
  return post<void>(`/api/conversations/${conversationId}/messages/${messageId}/hide`);
}

export async function unhideMessage(
  conversationId: string,
  messageId: string,
): Promise<void> {
  return del<void>(`/api/conversations/${conversationId}/messages/${messageId}/hide`);
}

export async function updateMessageCards(
  conversationId: string,
  messageId: string,
  cardsJSON: string,
): Promise<void> {
  // 后端路由是 PATCH（见 router.go），必须用 patch 而非 put，否则 404 状态丢失。
  return patch<void>(`/api/conversations/${conversationId}/messages/${messageId}/cards`, {
    cards_json: cardsJSON,
  });
}

/**
 * 取消正在流式生成的 assistant 消息——后端异步向 daemon 发 task.cancel，
 * 返回 202 Accepted，不等 daemon 响应。
 */
export async function cancelStreamingMessage(
  conversationId: string,
  messageId: string,
  taskId?: string,
): Promise<void> {
  return post<void>(`/api/conversations/${conversationId}/messages/${messageId}/cancel`, {
    ...(taskId ? { task_id: taskId } : {}),
  });
}
