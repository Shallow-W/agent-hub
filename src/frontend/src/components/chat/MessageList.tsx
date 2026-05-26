import React, { useRef, useEffect } from 'react';
import { Empty, Spin } from 'antd';
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
  const { messages, streamingContent, loading, loadMore, hasMore } =
    useMessages(conversationId);
  const bottomRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 100;
    if (nearBottom) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
    }
  }, [messages, streamingContent]);

  const isEmpty = messages.length === 0 && !streamingContent;

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
