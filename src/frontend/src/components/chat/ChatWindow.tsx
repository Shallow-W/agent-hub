import React, { useEffect, useState, useCallback } from 'react';
import { Avatar, Tooltip, Button, Dropdown, Input, List } from 'antd';
import {
  FolderOpenOutlined,
  MoreOutlined,
  SearchOutlined,
  SettingOutlined,
  StopOutlined,
  UserAddOutlined,
  CloseOutlined,
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
import { searchMessages } from '@/api/search';
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
  const [searchOpen, setSearchOpen] = useState(false);
  const [searchResults, setSearchResults] = useState<Message[]>([]);
  const [searchLoading, setSearchLoading] = useState(false);
  const [hasSearched, setHasSearched] = useState(false);

  // Mark conversation as read when switching to it
  useEffect(() => {
    if (!activeId) return;
    markAllRead(activeId);
    convApi.markConversationRead(activeId).catch(() => {});
    setReplyTo(null);
    setSearchOpen(false);
    setSearchResults([]);
    setHasSearched(false);
  }, [activeId, markAllRead]);

  const toggleSearch = useCallback(() => {
    setSearchOpen((prev) => {
      if (prev) {
        setSearchResults([]);
        setHasSearched(false);
      }
      return !prev;
    });
  }, []);

  const handleSearch = useCallback(
    async (value: string) => {
      const keyword = value.trim();
      if (!keyword || !activeConv) return;
      setSearchLoading(true);
      try {
        const results = await searchMessages(activeConv.id, keyword);
        setSearchResults(results);
        setHasSearched(true);
      } catch {
        setSearchResults([]);
        setHasSearched(true);
      } finally {
        setSearchLoading(false);
      }
    },
    [activeConv],
  );

  if (!activeConv) {
    return null;
  }

  const isGroup = activeConv.type === 'group';
  const displayName = isGroup
    ? activeConv.title
    : (activeConv.peer_name || activeConv.title);
  const avatarText = displayName.charAt(0).toUpperCase();
  const typingUsers = typingUsersMap[activeConv.id] ?? [];
  const otherTyping = typingUsers.filter((u) => u.userId !== currentUserId);

  const menuItems: MenuProps['items'] = [
    {
      key: 'search',
      icon: <SearchOutlined />,
      label: '搜索消息',
      onClick: () => toggleSearch(),
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
          {isGroup && activeConv.member_count && <span className={styles.memberCount}>{activeConv.member_count}</span>}
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
            <Button type="text" icon={<SearchOutlined />} size="small" onClick={toggleSearch} />
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
      {searchOpen && (
        <div className={styles.searchPanel}>
          <div className={styles.searchBar}>
            <Input.Search
              placeholder="搜索消息"
              allowClear
              enterButton
              loading={searchLoading}
              onSearch={handleSearch}
              style={{ flex: 1 }}
            />
            <Button
              type="text"
              icon={<CloseOutlined />}
              size="small"
              onClick={toggleSearch}
            />
          </div>
          {searchResults.length > 0 && (
            <List
              className={styles.searchResults}
              dataSource={searchResults}
              renderItem={(msg) => (
                <List.Item
                  className={styles.searchResultItem}
                  onClick={() => {
                    toggleSearch();
                    requestAnimationFrame(() => {
                      requestAnimationFrame(() => {
                        const el = document.querySelector(`[data-message-id="${msg.id}"]`);
                        if (el instanceof HTMLElement) {
                          el.scrollIntoView({ behavior: 'smooth', block: 'center' });
                          el.classList.add(styles.highlightFlash!);
                          el.addEventListener('animationend', () => el.classList.remove(styles.highlightFlash!), { once: true });
                        }
                      });
                    });
                  }}
                  style={{ cursor: 'pointer' }}
                >
                  <List.Item.Meta
                    title={
                      <span className={styles.searchResultSender}>
                        {msg.username || msg.role}
                      </span>
                    }
                    description={
                      <span className={styles.searchResultContent}>
                        {msg.content.length > 100 ? msg.content.slice(0, 100) + '...' : msg.content}
                      </span>
                    }
                  />
                  <span className={styles.searchResultTime}>
                    {new Date(msg.created_at).toLocaleString()}
                  </span>
                </List.Item>
              )}
            />
          )}
          {hasSearched && searchResults.length === 0 && !searchLoading && (
            <div style={{ padding: '16px', textAlign: 'center', color: '#999', fontSize: 13 }}>
              未找到相关消息
            </div>
          )}
        </div>
      )}
      <MessageList conversationId={activeConv.id} onReply={setReplyTo} />
      {otherTyping.length > 0 && (
        <div className={styles.typingIndicator}>
          {otherTyping.length === 1
            ? `${otherTyping[0]?.username || otherTyping[0]?.userId || '用户'} 正在输入...`
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
