import React from 'react';
import type { Conversation } from '@/types/conversation';
import styles from './ConversationItem.module.css';

interface ConversationItemProps {
  conversation: Conversation;
  active: boolean;
  onSelect: () => void;
  onDelete: () => void;
  onTogglePin: () => void;
}

function formatTime(dateStr: string): string {
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMin = Math.floor(diffMs / 60000);

  if (diffMin < 1) return '刚刚';
  if (diffMin < 60) return `${diffMin}分钟前`;
  if (diffMin < 1440) return `${Math.floor(diffMin / 60)}小时前`;
  return `${Math.floor(diffMin / 1440)}天前`;
}

export const ConversationItem: React.FC<ConversationItemProps> = ({
  conversation,
  active,
  onSelect,
  onDelete,
  onTogglePin,
}) => {
  return (
    <div
      className={`${styles.item} ${active ? styles.active : ''}`}
      onClick={onSelect}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === 'Enter') onSelect();
      }}
    >
      <div className={styles.content}>
        <div className={styles.titleRow}>
          <span className={styles.title}>
            {conversation.pinned && (
              <span className={styles.pinIcon}>📌</span>
            )}
            {conversation.title}
          </span>
          <span className={styles.time}>
            {formatTime(conversation.updated_at)}
          </span>
        </div>
      </div>
      <div className={styles.actions}>
        <button
          className={styles.actionBtn}
          onClick={(e) => {
            e.stopPropagation();
            onTogglePin();
          }}
        >
          {conversation.pinned ? '取消置顶' : '置顶'}
        </button>
        <button
          className={`${styles.actionBtn} ${styles.actionBtnDanger}`}
          onClick={(e) => {
            e.stopPropagation();
            onDelete();
          }}
        >
          删除
        </button>
      </div>
    </div>
  );
};
