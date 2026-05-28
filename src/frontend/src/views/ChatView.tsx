import React from 'react';
import { ChatWindow } from '@/components/chat/ChatWindow';
import { useConversation } from '@/hooks/useConversation';
import styles from './ChatView.module.css';

const ChatView: React.FC = () => {
  const { activeId } = useConversation();

  if (!activeId) {
    return (
      <div className={styles.empty}>
        <span className={styles.icon} role="img" aria-label="chat">&#x1F916;</span>
        <div className={styles.title}>欢迎使用 AgentHub</div>
        <div className={styles.subtitle}>选择一个对话或创建新对话开始聊天</div>
      </div>
    );
  }

  return <ChatWindow />;
};

export default ChatView;
