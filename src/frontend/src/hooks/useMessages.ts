import { useEffect, useCallback, useRef } from 'react';
import { useMessageStore } from '@/store/messageStore';
import { getUnreadMessages } from '@/api/message';
import { markConversationRead } from '@/api/conversation';
import type { OptimisticMessage } from '@/types/message';
import type { AttachmentPayload } from '@/types/attachment';

export function useMessages(conversationId: string | null) {
  const conversationMessages = useMessageStore(
    (s) => (conversationId ? s.messages[conversationId] : undefined),
  );
  const streaming = useMessageStore(
    (s) => (conversationId ? s.streamingContent[conversationId] : undefined),
  );
  const hasMoreEntry = useMessageStore(
    (s) => (conversationId ? s.hasMore[conversationId] : undefined),
  );
  const loadingEntry = useMessageStore((s) => (conversationId ? s.loading[conversationId] : undefined));
  const optimisticEntry = useMessageStore(
    (s) => (conversationId ? s.optimisticMessages[conversationId] : undefined),
  );
  const fetchMessages = useMessageStore((s) => s.fetchMessages);
  const sendMessage = useMessageStore((s) => s.sendMessage);
  const retryOptimistic = useMessageStore((s) => s.retryOptimistic);
  const removeOptimistic = useMessageStore((s) => s.removeOptimistic);

  const messages = conversationMessages ?? [];
  const streamingContent = streaming ?? '';
  const hasMore = hasMoreEntry === true;
  const optimisticMessages: OptimisticMessage[] = optimisticEntry ?? [];

  // 追踪当前活跃的 conversationId，用于 stale check
  const activeIdRef = useRef<string | null>(null);

  useEffect(() => {
    if (!conversationId) return;
    activeIdRef.current = conversationId;
    const currentId = conversationId;

    fetchMessages(currentId);

    // 标记已读
    markConversationRead(currentId).catch(() => {});

    // 拉取离线/未读消息并合并
    getUnreadMessages(currentId, 100).then((unread) => {
      // stale check：如果用户已切换到其他对话，丢弃结果
      if (activeIdRef.current !== currentId) return;
      if (unread && unread.length > 0) {
        const store = useMessageStore.getState();
        const existing = store.messages[currentId] ?? [];
        const existingIds = new Set(existing.map((m) => m.id));
        const newMsgs = unread.filter((m) => !existingIds.has(m.id));
        if (newMsgs.length > 0) {
          const merged = [...existing, ...newMsgs].sort(
            (a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
          );
          useMessageStore.setState((s) => ({
            messages: { ...s.messages, [currentId]: merged },
          }));
        }
      }
    }).catch(() => {});
  }, [conversationId, fetchMessages]);

  const loadMore = useCallback(() => {
    if (!conversationId || !hasMore || loadingEntry) return;
    const oldest = messages[0];
    if (oldest) {
      fetchMessages(conversationId, oldest.created_at);
    }
  }, [conversationId, hasMore, loadingEntry, messages, fetchMessages]);

  const send = useCallback(
    async (content: string, attachments?: AttachmentPayload[], replyTo?: string) => {
      if (!conversationId) return;
      await sendMessage(conversationId, content, attachments, replyTo);
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
    loading: !!loadingEntry,
    loadMore,
    send,
    hasMore,
    optimisticMessages,
    retry,
    removeOptimistic: removeOptimisticMsg,
  };
}
