import React, { useState } from 'react';
import { Avatar, Dropdown, Input, Modal } from 'antd';
import type { MenuProps } from 'antd';
import {
  DeleteOutlined,
  EditOutlined,
  InboxOutlined,
  PushpinOutlined,
  RobotOutlined,
  TeamOutlined,
  UserAddOutlined,
} from '@ant-design/icons';
import { useAuthStore } from '@/store/authStore';
import type { Conversation } from '@/types/conversation';
import { resolveAgentAvatar, resolveUserAvatar } from '@/components/agent/agentPresentation';
import styles from './ConversationItem.module.css';

interface ConversationItemProps {
  conversation: Conversation;
  active: boolean;
  onSelect: () => void;
  onDelete: () => void;
  onTogglePin: () => void;
  onArchive: () => void;
  onInviteMembers?: () => void;
  onRename?: (newTitle: string) => void;
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
  return `${text.slice(0, maxLen)}...`;
}

export const ConversationItem: React.FC<ConversationItemProps> = ({
  conversation,
  active,
  onSelect,
  onDelete,
  onTogglePin,
  onArchive,
  onInviteMembers,
  onRename,
  lastMessage,
  unreadCount = 0,
  online = false,
}) => {
  const [renameOpen, setRenameOpen] = useState(false);
  const [renameValue, setRenameValue] = useState('');
  const currentUserId = useAuthStore((s) => s.user?.id);
  const isGroup = conversation.type === 'group';
  const isAgent = conversation.type === 'agent';
  const isOwner = conversation.user_id === currentUserId;
  const displayName = isGroup
    ? conversation.title
    : (conversation.peer_name || conversation.title);
  const firstChar = displayName ? displayName.charAt(0).toUpperCase() : '?';
  const avatarColor = getAvatarColor(displayName || '?');

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
    ...(isGroup ? [{
      key: 'rename',
      icon: <EditOutlined />,
      label: '重命名',
      onClick: (info: { domEvent: { stopPropagation: () => void } }) => {
        info.domEvent.stopPropagation();
        setRenameValue(displayName);
        setRenameOpen(true);
      },
    }] : []),
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
        onArchive();
      },
    },
    { type: 'divider' },
    {
      key: 'delete',
      icon: <DeleteOutlined />,
      label: isGroup && isOwner ? '解散并删除' : '删除',
      danger: true,
      onClick: (info) => {
        info.domEvent.stopPropagation();
        Modal.confirm({
          title: isGroup && isOwner ? '解散并删除群聊' : '删除对话',
          content: isGroup && isOwner
            ? `确定要解散并删除「${displayName}」吗？所有成员都会失去这个群聊和聊天记录。`
            : `确定要删除「${displayName}」吗？`,
          okText: isGroup && isOwner ? '解散并删除' : '删除',
          okType: 'danger',
          cancelText: '取消',
          onOk: onDelete,
        });
      },
    },
  ];

  const pinnedClass = conversation.pinned ? ` ${styles.pinned}` : '';

  return (
    <div
      className={`${styles.item}${pinnedClass} ${active ? styles.active : ''}`}
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
      <div className={styles.avatarWrapper}>
        {isAgent ? (
          <Avatar
            style={{ backgroundColor: '#2f9d74', flexShrink: 0 }}
            size={32}
            src={resolveAgentAvatar({ id: conversation.peer_id || '', name: conversation.peer_name || conversation.title })}
            icon={<RobotOutlined />}
          />
        ) : isGroup ? (
          <Avatar
            style={{ backgroundColor: '#722ed1', flexShrink: 0, borderRadius: 10 }}
            size={32}
            icon={<TeamOutlined />}
          />
        ) : (
          <Avatar
            style={{ backgroundColor: avatarColor, flexShrink: 0 }}
            size={32}
            src={resolveUserAvatar({ id: conversation.peer_id, username: conversation.peer_name || conversation.title })}
          >
            {firstChar}
          </Avatar>
        )}
        {!isGroup && !isAgent && (
          <span className={`${styles.onlineDot} ${online ? styles.online : styles.offline}`} />
        )}
        {unreadCount > 0 && (
          <span className={styles.unreadBadge}>
            {unreadCount > 99 ? '99+' : unreadCount}
          </span>
        )}
      </div>

      <div className={styles.content}>
        <div className={styles.titleRow}>
          <span className={styles.title}>{displayName}</span>
          <span className={styles.time}>
            {formatTime(conversation.updated_at)}
          </span>
        </div>
        <div className={styles.subtitleRow}>
          <span className={styles.subtitle}>
            {lastMessage ? truncate(lastMessage, 24) : ''}
          </span>
        </div>
      </div>

      <div className={styles.actions}>
        <Dropdown menu={{ items: menuItems }} trigger={['click']} placement="bottomRight">
          <button
            className={styles.actionBtn}
            onClick={(e) => e.stopPropagation()}
            aria-label="更多操作"
          >
            &#x22EF;
          </button>
        </Dropdown>
      </div>

      <Modal
        title="重命名"
        open={renameOpen}
        okText="确定"
        cancelText="取消"
        onOk={() => {
          const trimmed = renameValue.trim();
          if (trimmed && trimmed !== displayName && onRename) {
            onRename(trimmed);
          }
          setRenameOpen(false);
        }}
        onCancel={() => setRenameOpen(false)}
        destroyOnClose
      >
        <Input
          value={renameValue}
          onChange={(e) => setRenameValue(e.target.value)}
          onPressEnter={() => {
            const trimmed = renameValue.trim();
            if (trimmed && trimmed !== displayName && onRename) {
              onRename(trimmed);
            }
            setRenameOpen(false);
          }}
          maxLength={50}
          autoFocus
        />
      </Modal>
    </div>
  );
};
