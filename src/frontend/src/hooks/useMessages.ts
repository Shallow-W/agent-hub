import { useEffect, useCallback, useRef } from 'react';
import { useMessageStore } from '@/store/messageStore';
import type { OptimisticMessage } from '@/types/message';

export function useMessages(conversationId: string | null) {
  const allMessages = useMessageStore((s) => s.messages);
  const streamingContent = useMessageStore((s) => s.streamingContent);
  const hasMoreMap = useMessageStore((s) => s.hasMore);
  const loading = useMessageStore((s) => s.loading);
  const optimisticMap = useMessageStore((s) => s.optimisticMessages);
  const fetchMessages = useMessageStore((s) => s.fetchMessages);
  const sendMessage = useMessageStore((s) => s.sendMessage);
  const retryOptimistic = useMessageStore((s) => s.retryOptimistic);
  const removeOptimistic = useMessageStore((s) => s.removeOptimistic);

  const messages = conversationId ? (allMessages[conversationId] ?? []) : [];
  const streaming = conversationId
    ? (streamingContent[conversationId] ?? '')
    : '';
  const hasMore = conversationId ? (hasMoreMap[conversationId] !== false) : false;
  const optimisticMessages: OptimisticMessage[] = conversationId
    ? (optimisticMap[conversationId] ?? [])
    : [];

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
    async (content: string) => {
      if (!conversationId) return;
      await sendMessage(conversationId, content);
    },
    [conversationId, sendMessage],
  );

  const retry = useCallback(
    (tempId: string) => {
      if (!conversationId) return;
      retryOptimistic(conversationId, tempId);
    },
    [conversationId, retryOptimistic],
  );

  const removeOptimisticMsg = useCallback(
    (tempId: string) => {
      if (!conversationId) return;
      removeOptimistic(conversationId, tempId);
    },
    [conversationId, removeOptimistic],
  );

  return {
    messages,
    streamingContent: streaming,
    loading,
    loadMore,
    send,
    hasMore,
    optimisticMessages,
    retry,
    removeOptimistic: removeOptimisticMsg,
  };
}
