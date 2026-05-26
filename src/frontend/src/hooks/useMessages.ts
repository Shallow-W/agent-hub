import { useEffect, useCallback, useRef } from 'react';
import { useMessageStore } from '@/store/messageStore';
import type { OptimisticMessage } from '@/types/message';

export function useMessages(conversationId: string | null) {
  // 使用精确 selector 避免订阅整个 store map（防止无关更新触发重渲染）
  const conversationMessages = useMessageStore(
    (s) => (conversationId ? s.messages[conversationId] : undefined),
  );
  const streaming = useMessageStore(
    (s) => (conversationId ? s.streamingContent[conversationId] : undefined),
  );
  const hasMoreEntry = useMessageStore(
    (s) => (conversationId ? s.hasMore[conversationId] : undefined),
  );
  const loading = useMessageStore((s) => s.loading);
  const optimisticEntry = useMessageStore(
    (s) => (conversationId ? s.optimisticMessages[conversationId] : undefined),
  );
  const fetchMessages = useMessageStore((s) => s.fetchMessages);
  const sendMessage = useMessageStore((s) => s.sendMessage);
  const retryOptimistic = useMessageStore((s) => s.retryOptimistic);
  const removeOptimistic = useMessageStore((s) => s.removeOptimistic);

  const messages = conversationMessages ?? [];
  const streamingContent = streaming ?? '';
  const hasMore = hasMoreEntry !== false;
  const optimisticMessages: OptimisticMessage[] = optimisticEntry ?? [];

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
    streamingContent,
    loading,
    loadMore,
    send,
    hasMore,
    optimisticMessages,
    retry,
    removeOptimistic: removeOptimisticMsg,
  };
}
