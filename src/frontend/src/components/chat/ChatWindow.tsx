import React, { useEffect, useState } from 'react';
import { Avatar, Tooltip, Button, Dropdown } from 'antd';
import {
  FolderOpenOutlined,
  MoreOutlined,
  SearchOutlined,
  SettingOutlined,
  StopOutlined,
  UserAddOutlined,
} from '@ant-design/icons';
import type { MenuProps } from 'antd';
import { useConversation } from '@/hooks/useConversation';
import { useAuthStore } from '@/store/authStore';
import { useConversationStore } from '@/store/conversationStore';
import { useWsStore } from '@/store/wsStore';
import { useMessageStore } from '@/store/messageStore';
import * as convApi from '@/api/conversation';
import type { Message } from '@/types/message';
import { MessageList } from './MessageList';
import { ChatInput } from './ChatInput';
import GroupMemberPanel from '@/components/groups/GroupMemberPanel';
import styles from './ChatWindow.module.css';

export const ChatWindow: React.FC = () => {
  const { conversations, activeId } = useConversation();
  const user = useAuthStore((s) => s.user);
  const fetchConversations = useConversationStore((s) => s.fetchConversations);
  const activeConv = conversations.find((c) => c.id === activeId);
  const memberPanelOpen = useConversationStore((s) => s.memberPanelOpen);
  const setMemberPanelOpen = useConversationStore((s) => s.setMemberPanelOpen);
  const currentUserId = useAuthStore((s) => s.user?.id);
  const markAllRead = useMessageStore((s) => s.markAllRead);
  const typingUsersMap = useWsStore((s) => s.typingUsers);
  const [replyTo, setReplyTo] = useState<Message | null>(null);

  // Mark conversation as read when switching to it
  useEffect(() => {
    if (!activeId) return;
    markAllRead(activeId);
    convApi.markConversationRead(activeId).catch(() => {});
    setReplyTo(null);
  }, [activeId, markAllRead]);

  if (!activeConv) {
    return null;
  }

  const isGroup = activeConv.type === 'group';
  const displayName = isGroup
    ? activeConv.title
    : (activeConv.peer_name || activeConv.title);
  const avatarText = displayName.charAt(0).toUpperCase();
  const typingUsers = typingUsersMap[activeConv.id] ?? [];
  const otherTyping = typingUsers.filter((id) => id !== currentUserId);

  const menuItems: MenuProps['items'] = [
    {
      key: 'search',
      icon: <SearchOutlined />,
      label: '搜索消息',
    },
    ...(isGroup
      ? [
          {
            key: 'settings' as const,
            icon: <SettingOutlined />,
            label: '群聊设置',
            onClick: () => setMemberPanelOpen(true),
          },
        ]
      : []),
  ];

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <Avatar className={styles.conversationAvatar} size={26}>
            {avatarText}
          </Avatar>
          <h1 className={styles.title}>
            {displayName}
          </h1>
          {isGroup && <span className={styles.memberCount}>9</span>}
        </div>
        <div className={styles.headerActions}>
          <Tooltip title="文件">
            <Button type="text" icon={<FolderOpenOutlined />} size="small" />
          </Tooltip>
          {isGroup && (
            <Tooltip title="邀请成员">
              <Button
                type="text"
                icon={<UserAddOutlined />}
                size="small"
                onClick={() => setMemberPanelOpen(true)}
              />
            </Tooltip>
          )}
          <Tooltip title="搜索消息">
            <Button type="text" icon={<SearchOutlined />} size="small" />
          </Tooltip>
          <Tooltip title="停止任务">
            <Button type="text" icon={<StopOutlined />} size="small" />
          </Tooltip>
          <Tooltip title={isGroup ? '群聊设置' : '对话设置'}>
            <Button
              type="text"
              icon={<SettingOutlined />}
              size="small"
              onClick={() => isGroup && setMemberPanelOpen(true)}
            />
          </Tooltip>
          <Dropdown
            menu={{ items: menuItems }}
            trigger={['click']}
            placement="bottomRight"
          >
            <Tooltip title="更多操作">
              <Button type="text" icon={<MoreOutlined />} size="small" />
            </Tooltip>
          </Dropdown>
        </div>
      </div>
      <MessageList conversationId={activeConv.id} onReply={setReplyTo} />
      {otherTyping.length > 0 && (
        <div className={styles.typingIndicator}>
          {otherTyping.length === 1
            ? `${otherTyping[0]} 正在输入...`
            : `${otherTyping.length} 人正在输入...`}
        </div>
      )}
      <ChatInput
        conversationId={activeConv.id}
        replyTo={replyTo}
        onCancelReply={() => setReplyTo(null)}
      />
      {isGroup && activeId && (
        <GroupMemberPanel
          open={memberPanelOpen}
          onClose={() => setMemberPanelOpen(false)}
          conversationId={activeId}
          currentUserId={user?.id ?? ''}
          onGroupLeft={() => fetchConversations()}
        />
      )}
    </div>
  );
};
