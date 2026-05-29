import { get } from './client';
import type { Message } from '@/types/message';

export async function searchMessages(conversationId: string, keyword: string): Promise<Message[]> {
  const params = new URLSearchParams();
  params.set('keyword', keyword);
  const path = `/api/conversations/${conversationId}/messages/search?${params.toString()}`;
  return get<Message[]>(path);
}
