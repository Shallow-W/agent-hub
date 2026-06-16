import React, { useRef, useEffect, useState, useCallback, useMemo } from 'react';
import { Empty, Spin, Skeleton, Badge } from 'antd';
import { ArrowDownOutlined } from '@ant-design/icons';
import { useMessages } from '@/hooks/useMessages';
import { useAuthStore } from '@/store/authStore';
import { useMessageStore } from '@/store/messageStore';
import { MessageBubble } from './MessageBubble';
import type { Message } from '@/types/message';
import type { ConversationAgent } from '@/types/conversation';
import styles from './MessageList.module.css';

interface MessageListProps {
  conversationId: string;
  onReply?: (message: Message) => void;
  onForward?: (message: Message) => void;
  onPinChanged?: () => void;
  onOpenThread?: (message: Message) => void;
  conversationAgents?: ConversationAgent[];
}

/** Extract agent_name from artifacts_json, or null */
function getAgentName(msg: Message): string | null {
  if (!msg.artifacts_json) return null;
  try { return (JSON.parse(msg.artifacts_json) as { agent_name?: string }).agent_name ?? null; } catch { return null; }
}

/** Check if two messages from the same sender are within 5 minutes */
function isGrouped(prev: Message, curr: Message): boolean {
  // Agent messages with different agent_name should NOT be grouped
  if (prev.role === 'assistant' && curr.role === 'assistant') {
    const prevAgent = getAgentName(prev);
    const currAgent = getAgentName(curr);
    if (prevAgent !== currAgent) return false;
  }
  // 两者都有 sender_id 时严格比较
  if (prev.sender_id && curr.sender_id) {
    if (prev.sender_id !== curr.sender_id) return false;
  } else if (prev.role !== curr.role) {
    // 回退：无 sender_id 时按 role 分组，不同 role 不合并
    return false;
  }
  const diff = new Date(curr.created_at).getTime() - new Date(prev.created_at).getTime();
  return diff < 5 * 60 * 1000;
}

/** Check if two messages are more than 30 minutes apart */
function needsTimeDivider(prev: Message, curr: Message): boolean {
  const diff = new Date(curr.created_at).getTime() - new Date(prev.created_at).getTime();
  return diff >= 30 * 60 * 1000;
}

function formatDividerTime(dateStr: string): string {
  const d = new Date(dateStr);
  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const yesterday = new Date(today.getTime() - 86400000);
  const msgDate = new Date(d.getFullYear(), d.getMonth(), d.getDate());
  const hh = String(d.getHours()).padStart(2, '0');
  const mm = String(d.getMinutes()).padStart(2, '0');

  if (msgDate.getTime() === today.getTime()) {
    return `今天 ${hh}:${mm}`;
  }
  if (msgDate.getTime() === yesterday.getTime()) {
    return `昨天 ${hh}:${mm}`;
  }
  const month = d.getMonth() + 1;
  const day = d.getDate();
  if (d.getFullYear() === now.getFullYear()) {
    return `${month}月${day}日 ${hh}:${mm}`;
  }
  return `${d.getFullYear()}年${month}月${day}日 ${hh}:${mm}`;
}

export const MessageList: React.FC<MessageListProps> = ({
  conversationId,
  onReply,
  onForward,
  onPinChanged,
  onOpenThread,
  conversationAgents = [],
}) => {
  const {
    messages,
    streamingContent,
    loading,
    loadMore,
    hasMore,
    optimisticMessages,
    retry,
    removeOptimistic: removeOpt,
  } = useMessages(conversationId);
  const bottomRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const currentUserId = useAuthStore((s) => s.user?.id);
  const recall = useMessageStore((s) => s.recall);
  const toggleMessagePin = useMessageStore((s) => s.toggleMessagePin);

  // Stable callbacks to preserve React.memo on MessageBubble.
  // Inline arrow functions bust memo on every parent re-render.
  const handleTogglePin = useCallback(
    (message: Message) => {
      void toggleMessagePin(conversationId, message.id, !!message.pinned)
        .finally(() => onPinChanged?.());
    },
    [conversationId, toggleMessagePin, onPinChanged],
  );
  const handleRecall = useCallback(
    (messageId: string) => recall(conversationId, messageId),
    [conversationId, recall],
  );

  const replyCounts = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const msg of messages) {
      if (msg.reply_to) {
        counts[msg.reply_to] = (counts[msg.reply_to] || 0) + 1;
      }
    }
    return counts;
  }, [messages]);
  const [showNewMsgBtn, setShowNewMsgBtn] = useState(false);
  const [unreadSinceScroll, setUnreadSinceScroll] = useState(0);
  const nearBottomRef = useRef(true);

  const handleScroll = useCallback(() => {
    const el = containerRef.current;
    if (!el) return;
    const isNearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 100;
    nearBottomRef.current = isNearBottom;
    if (isNearBottom) {
      setShowNewMsgBtn(false);
      setUnreadSinceScroll(0);
    }
    // Auto-load older messages when scrolled to top
    if (el.scrollTop < 50 && hasMore && !loading) {
      const prevHeight = el.scrollHeight;
      loadMore().then(() => {
        // Preserve scroll position after prepending older messages
        requestAnimationFrame(() => {
          const newHeight = containerRef.current?.scrollHeight ?? 0;
          if (containerRef.current) {
            containerRef.current.scrollTop = newHeight - prevHeight;
          }
        });
      });
    }
  }, [hasMore, loading, loadMore]);

  const scrollToBottom = useCallback(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
    setShowNewMsgBtn(false);
    setUnreadSinceScroll(0);
  }, []);

  const prevMessageSnapshotRef = useRef({
    count: messages.length,
    lastId: messages[messages.length - 1]?.id ?? null,
  });

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const prev = prevMessageSnapshotRef.current;
    const lastId = messages[messages.length - 1]?.id ?? null;
    const msgAppended = messages.length > prev.count && lastId !== prev.lastId;
    prevMessageSnapshotRef.current = { count: messages.length, lastId };
    if (!msgAppended && !nearBottomRef.current) return;
    // Always scroll to bottom when a new message is added (user sent or received)
    if (msgAppended || nearBottomRef.current) {
      bottomRef.current?.scrollIntoView({ behavior: 'instant' });
    } else {
      setShowNewMsgBtn(true);
      setUnreadSinceScroll((n) => n + 1);
    }
  }, [messages, streamingContent, optimisticMessages]);

  // Skeleton loading state
  if (loading && messages.length === 0) {
    return (
      <div className={styles.container}>
        <div className={styles.skeletonList}>
          {Array.from({ length: 3 }).map((_, i) => (
            <div
              key={i}
              className={`${styles.skeletonRow} ${i % 2 === 0 ? '' : styles.skeletonRowReverse}`}
            >
              <Skeleton.Avatar active size={32} />
              <Skeleton
                active
                paragraph={{ rows: 1, width: i % 2 === 0 ? '60%' : '45%' }}
                title={false}
              />
            </div>
          ))}
        </div>
      </div>
    );
  }

  const isEmpty = messages.length === 0 && !streamingContent && optimisticMessages.length === 0;

  return (
    <div className={styles.container} ref={containerRef} onScroll={handleScroll}>
      {hasMore && (
        <div className={styles.loadMore}>
          {loading ? (
            <Spin size="small" />
          ) : (
            <button
              className={styles.loadMoreBtn}
              onClick={loadMore}
            >
              加载更多
            </button>
          )}
        </div>
      )}
      {isEmpty ? (
        <div className={styles.empty}>
          <Empty
            description="开始新对话"
            image={Empty.PRESENTED_IMAGE_SIMPLE}
          />
        </div>
      ) : (
        <>
          {messages.map((msg, idx) => {
            const prev = idx > 0 ? messages[idx - 1] : undefined;
            const grouped = prev ? isGrouped(prev, msg) : false;
            const showDivider = prev ? needsTimeDivider(prev, msg) : false;
            const isOwn = msg.sender_id === currentUserId || (!msg.sender_id && msg.role === 'user');

            return (
              <React.Fragment key={msg.id}>
                {showDivider && (
                  <div className={styles.timeDivider}>
                    {formatDividerTime(msg.created_at)}
                  </div>
                )}
                <MessageBubble
                  message={msg}
                  showAvatar={!grouped}
                  isGrouped={grouped}
                  isOwn={isOwn}
                  onReply={onReply}
                  onForward={onForward}
                  onTogglePin={handleTogglePin}
                  onRecall={isOwn ? handleRecall : undefined}
                  conversationAgents={conversationAgents}
                  replyCount={replyCounts[msg.id]}
                  onOpenThread={onOpenThread}
                />
              </React.Fragment>
            );
          })}
          {/* Optimistic messages */}
          {optimisticMessages.map((optMsg) => (
            <MessageBubble
              key={optMsg.id}
              message={optMsg}
              showAvatar
              isOwn={true}
              optimisticStatus={optMsg.optimisticStatus}
              onRetry={optMsg.optimisticStatus === 'failed' ? () => retry(optMsg.id) : undefined}
              onRemove={optMsg.optimisticStatus === 'failed' ? () => removeOpt(optMsg.id) : undefined}
            />
          ))}
          {streamingContent && (
            <MessageBubble
              message={{
                id: `__streaming_${conversationId}`,
                conversation_id: conversationId,
                role: 'assistant',
                content: streamingContent,
                artifacts_json: null,
                created_at: new Date().toISOString(),
              }}
              streaming
              isOwn={false}
            />
          )}
        </>
      )}
      {showNewMsgBtn && unreadSinceScroll > 0 && (
        <button className={styles.newMsgBtn} onClick={scrollToBottom}>
          <Badge count={unreadSinceScroll > 1 ? unreadSinceScroll : 0} size="small">
            <ArrowDownOutlined />
          </Badge>
          <span className={styles.newMsgText}>新消息</span>
        </button>
      )}
      <div ref={bottomRef} />
    </div>
  );
};
