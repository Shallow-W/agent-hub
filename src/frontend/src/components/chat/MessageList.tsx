import React, { useRef, useEffect } from 'react';
import { useMessages } from '@/hooks/useMessages';
import { MessageBubble } from './MessageBubble';
import styles from './MessageList.module.css';

interface MessageListProps {
  conversationId: string;
}

export const MessageList: React.FC<MessageListProps> = ({ conversationId }) => {
  const { messages, streamingContent, loading, loadMore, hasMore } =
    useMessages(conversationId);
  const bottomRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  // 有新消息时自动滚动到底部（除非用户主动向上滚动）
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
          <button
            className={styles.loadMoreBtn}
            onClick={loadMore}
            disabled={loading}
          >
            {loading ? '加载中...' : '加载更多'}
          </button>
        </div>
      )}
      {isEmpty ? (
        <div className={styles.empty}>
          <span className={styles.emptyIcon} role="img" aria-label="chat">&#x1F4AC;</span>
          <span className={styles.emptyText}>开始新对话</span>
        </div>
      ) : (
        <>
          {messages.map((msg) => (
            <MessageBubble key={msg.id} message={msg} />
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
