import React, { useRef, useEffect } from 'react';
import { Empty, Spin, Skeleton } from 'antd';
import { useMessages } from '@/hooks/useMessages';
import { MessageBubble } from './MessageBubble';
import type { Message } from '@/types/message';
import styles from './MessageList.module.css';

interface MessageListProps {
  conversationId: string;
}

/** Check if two messages from the same role are within 5 minutes */
function isGrouped(prev: Message, curr: Message): boolean {
  if (prev.role !== curr.role) return false;
  const diff = new Date(curr.created_at).getTime() - new Date(prev.created_at).getTime();
  return diff < 5 * 60 * 1000;
}

export const MessageList: React.FC<MessageListProps> = ({ conversationId }) => {
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

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 100;
    if (nearBottom) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
    }
  }, [messages, streamingContent, optimisticMessages]);

  // Skeleton loading state
  if (loading && messages.length === 0) {
    return (
      <div className={styles.container}>
        <div style={{ padding: '16px 20px' }}>
          {Array.from({ length: 3 }).map((_, i) => (
            <div
              key={i}
              style={{
                display: 'flex',
                gap: 10,
                marginBottom: 16,
                flexDirection: i % 2 === 0 ? 'row' : 'row-reverse',
              }}
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
    <div className={styles.container} ref={containerRef}>
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

            return (
              <MessageBubble
                key={msg.id}
                message={msg}
                showAvatar={!grouped}
                isGrouped={grouped}
              />
            );
          })}
          {/* Optimistic messages */}
          {optimisticMessages.map((optMsg) => (
            <MessageBubble
              key={optMsg.id}
              message={optMsg}
              showAvatar
              optimisticStatus={optMsg.optimisticStatus}
              onRetry={optMsg.optimisticStatus === 'failed' ? () => retry(optMsg.id) : undefined}
              onRemove={optMsg.optimisticStatus === 'failed' ? () => removeOpt(optMsg.id) : undefined}
            />
          ))}
          {streamingContent && (
            <MessageBubble
              message={{
                id: 'streaming',
                conversation_id: conversationId,
                role: 'assistant',
                content: streamingContent,
                artifacts_json: null,
                created_at: new Date().toISOString(),
              }}
              streaming
            />
          )}
        </>
      )}
      <div ref={bottomRef} />
    </div>
  );
};
