import React from 'react';
import { Avatar, Dropdown, Badge } from 'antd';
import type { MenuProps } from 'antd';
import {
  PushpinOutlined,
  InboxOutlined,
  DeleteOutlined,
} from '@ant-design/icons';
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

  const menuItems: MenuProps['items'] = [
    {
      key: 'pin',
      icon: <PushpinOutlined />,
      label: conversation.pinned ? '取消置顶' : '置顶',
      onClick: (info) => {
        info.domEvent.stopPropagation();
        onTogglePin();
      },
    },
    {
      key: 'archive',
      icon: <InboxOutlined />,
      label: '归档',
      onClick: (info) => {
        info.domEvent.stopPropagation();
        // TODO: 归档功能待实现
      },
    },
    { type: 'divider' },
    {
      key: 'delete',
      icon: <DeleteOutlined />,
      label: '删除',
      danger: true,
      onClick: (info) => {
        info.domEvent.stopPropagation();
        onDelete();
      },
    },
  ];

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
      {/* 头像 - 使用 antd Avatar */}
      <Badge dot={conversation.pinned} color="#1677ff" offset={[-4, 30]}>
        <Avatar
          style={{ backgroundColor: avatarColor, flexShrink: 0 }}
          size={36}
        >
          {firstChar}
        </Avatar>
      </Badge>

      {/* 标题 + 时间 */}
      <div className={styles.content}>
        <div className={styles.titleRow}>
          <span className={styles.title}>{conversation.title}</span>
        </div>
        <div className={styles.subtitleRow}>
          <span className={styles.time}>
            {formatTime(conversation.updated_at)}
          </span>
        </div>
      </div>

      {/* 悬停操作 - 使用 antd Dropdown */}
      <div className={styles.actions}>
        <Dropdown
          menu={{ items: menuItems }}
          trigger={['click']}
          placement="bottomRight"
        >
          <button
            className={styles.actionBtn}
            onClick={(e) => e.stopPropagation()}
            aria-label="更多操作"
          >
            &#x22EF;
          </button>
        </Dropdown>
      </div>
    </div>
  );
};
