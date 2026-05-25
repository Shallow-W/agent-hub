import React from 'react';
import { useConversation } from '@/hooks/useConversation';
import { ConversationItem } from './ConversationItem';
import styles from './ConversationList.module.css';

export const ConversationList: React.FC = () => {
  const { conversations, activeId, loading, setActive, remove, togglePin } =
    useConversation();

  if (loading && conversations.length === 0) {
    return (
      <div className={styles.list}>
        <div className={styles.loading}>加载中...</div>
      </div>
    );
  }

  if (conversations.length === 0) {
    return (
      <div className={styles.list}>
        <div className={styles.empty}>暂无对话，点击「新建对话」开始</div>
      </div>
    );
  }

  return (
    <div className={styles.list}>
      {conversations.map((conv) => (
        <ConversationItem
          key={conv.id}
          conversation={conv}
          active={conv.id === activeId}
          onSelect={() => setActive(conv.id)}
          onDelete={() => remove(conv.id)}
          onTogglePin={() => togglePin(conv.id, !conv.pinned)}
        />
      ))}
    </div>
  );
};
