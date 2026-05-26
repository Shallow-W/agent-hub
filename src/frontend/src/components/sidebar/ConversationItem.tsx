import React from 'react';
import { Avatar, Dropdown, Badge } from 'antd';
import type { MenuProps } from 'antd';
import {
  PushpinOutlined,
  InboxOutlined,
  DeleteOutlined,
  UserOutlined,
  TeamOutlined,
  UserAddOutlined,
} from '@ant-design/icons';
import type { Conversation } from '@/types/conversation';
import styles from './ConversationItem.module.css';

interface ConversationItemProps {
  conversation: Conversation;
  active: boolean;
  onSelect: () => void;
  onDelete: () => void;
  onTogglePin: () => void;
  onInviteMembers?: () => void;
  lastMessage?: string;
  unreadCount?: number;
  online?: boolean;
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
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const yesterday = new Date(today.getTime() - 86400000);
  const msgDate = new Date(date.getFullYear(), date.getMonth(), date.getDate());
  const hh = String(date.getHours()).padStart(2, '0');
  const mm = String(date.getMinutes()).padStart(2, '0');

  if (msgDate.getTime() === today.getTime()) {
    return `${hh}:${mm}`;
  }
  if (msgDate.getTime() === yesterday.getTime()) {
    return '昨天';
  }
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${month}-${day}`;
}

function truncate(text: string, maxLen: number): string {
  if (text.length <= maxLen) return text;
  return text.slice(0, maxLen) + '...';
}

export const ConversationItem: React.FC<ConversationItemProps> = ({
  conversation,
  active,
  onSelect,
  onDelete,
  onTogglePin,
  onInviteMembers,
  lastMessage,
  unreadCount = 0,
  online = false,
}) => {
  const firstChar = conversation.title ? conversation.title.charAt(0).toUpperCase() : '?';
  const avatarColor = getAvatarColor(conversation.title || '?');
  const isGroup = conversation.type === 'group';

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
    ...(isGroup && onInviteMembers
      ? [
          {
            key: 'invite',
            icon: <UserAddOutlined />,
            label: '邀请成员',
            onClick: (info: { domEvent: { stopPropagation: () => void } }) => {
              info.domEvent.stopPropagation();
              onInviteMembers();
            },
          },
        ]
      : []),
    {
      key: 'archive',
      icon: <InboxOutlined />,
      label: '归档',
      onClick: (info) => {
        info.domEvent.stopPropagation();
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
      <Badge dot={conversation.pinned} color="#1677ff" offset={[-4, 30]}>
        <div className={styles.avatarWrapper}>
          <Avatar
            style={{ backgroundColor: avatarColor, flexShrink: 0 }}
            size={36}
            icon={isGroup ? <TeamOutlined /> : <UserOutlined />}
          >
            {!isGroup ? firstChar : undefined}
          </Avatar>
          {!isGroup && (
            <span
              className={`${styles.onlineDot} ${online ? styles.online : styles.offline}`}
            />
          )}
        </div>
      </Badge>

      <div className={styles.content}>
        <div className={styles.titleRow}>
          <span className={styles.title}>{conversation.title}</span>
          <span className={styles.time}>
            {formatTime(conversation.updated_at)}
          </span>
        </div>
        <div className={styles.subtitleRow}>
          <span className={styles.subtitle}>
            {lastMessage ? truncate(lastMessage, 20) : ''}
          </span>
          {unreadCount > 0 && (
            <Badge count={unreadCount} size="small" style={{ flexShrink: 0 }} />
          )}
        </div>
      </div>

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
