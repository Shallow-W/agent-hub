import React from 'react';
import { ChatWindow } from '@/components/chat/ChatWindow';
import { useConversation } from '@/hooks/useConversation';
import styles from './ChatView.module.css';

const ChatView: React.FC = () => {
  const { activeId } = useConversation();

  if (!activeId) {
    return (
      <div className={styles.empty}>选择或创建对话</div>
    );
  }

  return <ChatWindow />;
};

export default ChatView;
