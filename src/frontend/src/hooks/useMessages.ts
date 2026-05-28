import { useEffect, useCallback, useRef } from 'react';
import { useMessageStore } from '@/store/messageStore';
import { getUnreadMessages } from '@/api/message';
import { markConversationRead } from '@/api/conversation';
import type { OptimisticMessage, ReplyToPreview } from '@/types/message';
import type { AttachmentPayload } from '@/types/attachment';

/** Cache duration: skip re-fetch if loaded within last 30 seconds */
const CACHE_TTL_MS = 30_000;
/** Max messages kept per conversation */
const MAX_MESSAGES = 200;
/** Per-conversation last fetch timestamp */
const lastFetchedAt: Record<string, number> = {};
const EMPTY_MESSAGES_ARRAY: import('@/types/message').Message[] = [];
const EMPTY_OPTIMISTIC_ARRAY: import('@/types/message').OptimisticMessage[] = [];

/** Invalidate cache on WS reconnect so missed messages are re-fetched */
export function invalidateMessageCache() {
  Object.keys(lastFetchedAt).forEach((k) => delete lastFetchedAt[k]);
}

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

  const messages = conversationMessages ?? EMPTY_MESSAGES_ARRAY;
  const streamingContent = streaming ?? '';
  const hasMore = hasMoreEntry === true;
  const optimisticMessages: OptimisticMessage[] = optimisticEntry ?? EMPTY_OPTIMISTIC_ARRAY;

  // 追踪当前活跃的 conversationId，用于 stale check
  const activeIdRef = useRef<string | null>(null);
  const fetchIdRef = useRef(0);

  useEffect(() => {
    if (!conversationId) return;
    activeIdRef.current = conversationId;
    const currentId = conversationId;
    const currentFetchId = ++fetchIdRef.current;

    // Skip re-fetch if fetched within CACHE_TTL_MS (regardless of success/failure)
    const now = Date.now();
    const lastFetch = lastFetchedAt[currentId];
    if (lastFetch && now - lastFetch < CACHE_TTL_MS) {
      // Still mark as read even when using cache
      markConversationRead(currentId).catch(() => {});
      return;
    }

    lastFetchedAt[currentId] = now;
    fetchMessages(currentId);

    // 标记已读
    markConversationRead(currentId).catch(() => {});

    // 拉取离线/未读消息并合并
    getUnreadMessages(currentId, 100).then((unread) => {
      // stale check：如果用户已切换到其他对话，丢弃结果
      if (currentFetchId !== fetchIdRef.current) return;
      if (unread && unread.length > 0) {
        const store = useMessageStore.getState();
        const existing = store.messages[currentId] ?? [];
        const existingIds = new Set(existing.map((m) => m.id));
        const newMsgs = unread.filter((m) => !existingIds.has(m.id));
        if (newMsgs.length > 0) {
          const merged = [...existing, ...newMsgs].sort(
            (a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
          );
          const trimmed = merged.length > MAX_MESSAGES ? merged.slice(merged.length - MAX_MESSAGES) : merged;
          useMessageStore.setState((s) => ({
            messages: { ...s.messages, [currentId]: trimmed },
          }));
        }
      }
    }).catch(() => {});
  }, [conversationId, fetchMessages]);

  const loadMore = useCallback((): Promise<void> => {
    if (!conversationId || !hasMore || loadingEntry) return Promise.resolve();
    const oldest = messages[0];
    if (oldest) {
      return fetchMessages(conversationId, oldest.created_at);
    }
    return Promise.resolve();
  }, [conversationId, hasMore, loadingEntry, messages, fetchMessages]);

  const send = useCallback(
    async (content: string, attachments?: AttachmentPayload[], replyTo?: string, replyPreview?: ReplyToPreview) => {
      if (!conversationId) return;
      // Invalidate cache so next switch re-fetches
      delete lastFetchedAt[conversationId];
      await sendMessage(conversationId, content, attachments, replyTo, replyPreview);
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
