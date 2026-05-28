import React, { useRef, useEffect, useState, useCallback } from 'react';
import { Empty, Spin, Skeleton, Divider, Badge } from 'antd';
import { ArrowDownOutlined } from '@ant-design/icons';
import { useMessages } from '@/hooks/useMessages';
import { useAuthStore } from '@/store/authStore';
import { useMessageStore } from '@/store/messageStore';
import { MessageBubble } from './MessageBubble';
import type { Message } from '@/types/message';
import styles from './MessageList.module.css';

interface MessageListProps {
  conversationId: string;
  onReply?: (message: Message) => void;
}

/** Check if two messages from the same sender are within 5 minutes */
function isGrouped(prev: Message, curr: Message): boolean {
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
  const month = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${month}-${day} ${hh}:${mm}`;
}

export const MessageList: React.FC<MessageListProps> = ({ conversationId, onReply }) => {
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
  }, []);

  const scrollToBottom = useCallback(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
    setShowNewMsgBtn(false);
    setUnreadSinceScroll(0);
  }, []);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    if (nearBottomRef.current) {
      // Use instant scroll during streaming to avoid jitter; smooth for new messages
      bottomRef.current?.scrollIntoView({ behavior: streamingContent ? 'instant' : 'smooth' });
    } else {
      // User scrolled up — show indicator for new messages
      setShowNewMsgBtn(true);
      setUnreadSinceScroll((n) => n + 1);
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
                  <Divider plain style={{ margin: '8px 0', fontSize: 12, color: '#999' }}>
                    {formatDividerTime(msg.created_at)}
                  </Divider>
                )}
                <MessageBubble
                  message={msg}
                  showAvatar={!grouped}
                  isGrouped={grouped}
                  isOwn={isOwn}
                  onReply={onReply}
                  onRecall={isOwn ? (messageId) => recall(conversationId, messageId) : undefined}
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
                id: 'streaming',
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
          <span style={{ marginLeft: 4 }}>新消息</span>
        </button>
      )}
      <div ref={bottomRef} />
    </div>
  );
};
