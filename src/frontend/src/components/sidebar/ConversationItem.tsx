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

const AVATAR_COLORS: readonly string[] = [
  '#1677ff',
  '#52c41a',
  '#faad14',
  '#eb2f96',
  '#722ed1',
  '#13c2c2',
];

function getAvatarColor(title: string): string {
  let hash = 0;
  for (let i = 0; i < title.length; i++) {
    hash = title.charCodeAt(i) + ((hash << 5) - hash);
  }
  return AVATAR_COLORS[Math.abs(hash) % AVATAR_COLORS.length]!;
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
  const firstChar = conversation.title ? conversation.title.charAt(0).toUpperCase() : '?';
  const avatarColor = getAvatarColor(conversation.title || '?');

  return (
    <div
      className={`${styles.item} ${active ? styles.active : ''}`}
      onClick={onSelect}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onSelect();
        }
      }}
    >
      {/* Avatar */}
      <div className={styles.avatar} style={{ background: avatarColor }}>
        {firstChar}
      </div>

      {/* Middle: title + subtitle */}
      <div className={styles.content}>
        <div className={styles.titleRow}>
          {conversation.pinned && <span className={styles.pinBadge} />}
          <span className={styles.title}>{conversation.title}</span>
        </div>
        <div className={styles.subtitleRow}>
          <span className={styles.time}>
            {formatTime(conversation.updated_at)}
          </span>
        </div>
      </div>

      {/* Actions on hover */}
      <div className={styles.actions}>
        <button
          className={styles.actionBtn}
          onClick={(e) => {
            e.stopPropagation();
            onTogglePin();
          }}
          title={conversation.pinned ? '取消置顶' : '置顶'}
        >
          {conversation.pinned ? '📌' : '📍'}
        </button>
        <button
          className={styles.actionBtn}
          onClick={(e) => {
            e.stopPropagation();
            // TODO: 归档功能待实现
          }}
          title="归档"
        >
          &#128230;
        </button>
        <button
          className={`${styles.actionBtn} ${styles.actionBtnDanger}`}
          onClick={(e) => {
            e.stopPropagation();
            onDelete();
          }}
          title="删除"
        >
          &#128465;
        </button>
      </div>
    </div>
  );
};
