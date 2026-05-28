import React, { useEffect } from 'react';
import { useSearchParams } from 'react-router-dom';
import { ChatWindow } from '@/components/chat/ChatWindow';
import { useConversation } from '@/hooks/useConversation';
import styles from './ChatView.module.css';

const ChatView: React.FC = () => {
  const { activeId, setActive } = useConversation();
  const [searchParams, setSearchParams] = useSearchParams();

  // Sync URL → activeId: respond to URL changes (incl. browser back/forward)
  useEffect(() => {
    const convId = searchParams.get('conv');
    if (convId && convId !== activeId) {
      setActive(convId);
    }
  }, [searchParams, activeId, setActive]);

  // When active conversation changes, sync to URL (skip if URL already matches)
  useEffect(() => {
    if (!activeId) return;
    setSearchParams((prev) => {
      if (prev.get('conv') === activeId) return prev;
      const next = new URLSearchParams(prev);
      next.set('conv', activeId);
      return next;
    }, { replace: true });
  }, [activeId, setSearchParams]);

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
