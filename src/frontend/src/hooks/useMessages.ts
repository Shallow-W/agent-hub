import { useEffect, useCallback, useRef } from 'react';
import { useMessageStore } from '@/store/messageStore';

export function useMessages(conversationId: string | null) {
  const allMessages = useMessageStore((s) => s.messages);
  const streamingContent = useMessageStore((s) => s.streamingContent);
  const hasMoreMap = useMessageStore((s) => s.hasMore);
  const loading = useMessageStore((s) => s.loading);
  const fetchMessages = useMessageStore((s) => s.fetchMessages);
  const sendMessage = useMessageStore((s) => s.sendMessage);

  const messages = conversationId ? (allMessages[conversationId] ?? []) : [];
  const streaming = conversationId
    ? (streamingContent[conversationId] ?? '')
    : '';
  const hasMore = conversationId ? (hasMoreMap[conversationId] !== false) : false;

  // 防止重复拉取
  const fetchedRef = useRef<Set<string>>(new Set());

  useEffect(() => {
    if (!conversationId) return;
    if (fetchedRef.current.has(conversationId)) return;
    fetchedRef.current.add(conversationId);
    fetchMessages(conversationId);
  }, [conversationId, fetchMessages]);

  const loadMore = useCallback(() => {
    if (!conversationId || !hasMore || loading) return;
    const oldest = messages[0];
    if (oldest) {
      fetchMessages(conversationId, oldest.created_at);
    }
  }, [conversationId, hasMore, loading, messages, fetchMessages]);

  const send = useCallback(
    async (content: string, agentId?: string) => {
      if (!conversationId) return;
      await sendMessage(conversationId, content, agentId);
    },
    [conversationId, sendMessage],
  );

  return {
    messages,
    streamingContent: streaming,
    loading,
    loadMore,
    send,
    hasMore,
  };
}
