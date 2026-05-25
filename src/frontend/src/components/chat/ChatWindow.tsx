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
      <div className={styles.header}>{activeConv.title}</div>
      <MessageList conversationId={activeConv.id} />
      <ChatInput conversationId={activeConv.id} />
    </div>
  );
};
