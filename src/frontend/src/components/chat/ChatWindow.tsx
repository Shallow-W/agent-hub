import React from 'react';
import { useConversation } from '@/hooks/useConversation';
import { MessageList } from './MessageList';
import { ChatInput } from './ChatInput';
import styles from './ChatWindow.module.css';

export const ChatWindow: React.FC = () => {
  const { conversations, activeId } = useConversation();
  const activeConv = conversations.find((c) => c.id === activeId);

  if (!activeConv) {
    return null;
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <span className={styles.convIcon} role="img" aria-label="conversation">&#x1F4AC;</span>
          <span className={styles.headerTitle}>{activeConv.title}</span>
        </div>
        <div className={styles.headerActions}>
          <button className={styles.headerBtn} aria-label="搜索对话" title="搜索对话">
            &#x1F50D;
          </button>
          <button className={styles.headerBtn} aria-label="更多选项" title="更多选项">
            &#x22EF;
          </button>
        </div>
      </div>
      <MessageList conversationId={activeConv.id} />
      <ChatInput conversationId={activeConv.id} />
    </div>
  );
};
