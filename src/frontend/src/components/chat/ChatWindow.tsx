import React, { useState, useEffect } from 'react';
import { Typography, Tooltip, Button, Dropdown, Badge } from 'antd';
import { SearchOutlined, MoreOutlined, SettingOutlined } from '@ant-design/icons';
import type { MenuProps } from 'antd';
import { useConversation } from '@/hooks/useConversation';
import { useAuthStore } from '@/store/authStore';
import { useConversationStore } from '@/store/conversationStore';
import { useWsStore } from '@/store/wsStore';
import { useMessageStore } from '@/store/messageStore';
import * as convApi from '@/api/conversation';
import { MessageList } from './MessageList';
import { ChatInput } from './ChatInput';
import GroupMemberPanel from '@/components/groups/GroupMemberPanel';
import styles from './ChatWindow.module.css';

const { Title } = Typography;

export const ChatWindow: React.FC = () => {
  const { conversations, activeId } = useConversation();
  const user = useAuthStore((s) => s.user);
  const fetchConversations = useConversationStore((s) => s.fetchConversations);
  const activeConv = conversations.find((c) => c.id === activeId);
  const [memberPanelOpen, setMemberPanelOpen] = useState(false);
  const currentUserId = useAuthStore((s) => s.user?.id);
  const markAllRead = useMessageStore((s) => s.markAllRead);
  const typingUsersMap = useWsStore((s) => s.typingUsers);

  // Mark conversation as read when switching to it
  useEffect(() => {
    if (!activeConv) return;
    markAllRead(activeConv.id);
    convApi.markConversationRead(activeConv.id).catch(() => {});
  }, [activeConv, markAllRead]);

  if (!activeConv) {
    return null;
  }

  const isGroup = activeConv.type === 'group';
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
          <Title level={5} style={{ margin: 0 }} ellipsis>
            {activeConv.title}
          </Title>
          <Badge status="success" style={{ marginLeft: 4 }} />
        </div>
        <div className={styles.headerActions}>
          <Tooltip title="搜索消息">
            <Button type="text" icon={<SearchOutlined />} size="small" />
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
      <MessageList conversationId={activeConv.id} />
      {otherTyping.length > 0 && (
        <div className={styles.typingIndicator}>
          {otherTyping.length === 1
            ? `${otherTyping[0]} 正在输入...`
            : `${otherTyping.length} 人正在输入...`}
        </div>
      )}
      <ChatInput conversationId={activeConv.id} />
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
