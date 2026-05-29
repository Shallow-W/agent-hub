import React, { useEffect, useState, useCallback, useRef } from 'react';
import { Avatar, Tooltip, Button, Dropdown, message as antMessage } from 'antd';
import {
  FolderOpenOutlined,
  LogoutOutlined,
  MoreOutlined,
  SearchOutlined,
  SettingOutlined,
  StopOutlined,
  UserAddOutlined,
  InfoCircleOutlined,
  DeleteOutlined,
} from '@ant-design/icons';
import type { MenuProps } from 'antd';
import { useConversation } from '@/hooks/useConversation';
import { useAuthStore } from '@/store/authStore';
import { useConversationStore } from '@/store/conversationStore';
import { useWsStore } from '@/store/wsStore';
import { useMessageStore } from '@/store/messageStore';
import * as convApi from '@/api/conversation';
import { leaveGroup, dissolveGroup } from '@/api/group';
import type { Message } from '@/types/message';
import { MessageList } from './MessageList';
import { ChatInput } from './ChatInput';
import { ChatSearchPanel } from './ChatSearchPanel';
import { useMessages } from '@/hooks/useMessages';
import GroupMemberPanel from '@/components/groups/GroupMemberPanel';
import GroupInfoDrawer from '@/components/groups/GroupInfoDrawer';
import { searchMessages } from '@/api/search';
import { uploadFile } from '@/api/upload';
import type { AttachmentPayload } from '@/types/attachment';
import styles from './ChatWindow.module.css';

const ACCEPTED_TYPES = '.jpg,.jpeg,.png,.gif,.webp,.pdf';
const MAX_FILE_SIZE = 50 * 1024 * 1024; // 50MB
const EMPTY_TYPING: { userId: string; username?: string }[] = [];

export const ChatWindow: React.FC = () => {
  const { conversations, activeId } = useConversation();
  const user = useAuthStore((s) => s.user);
  const fetchConversations = useConversationStore((s) => s.fetchConversations);
  const activeConv = conversations.find((c) => c.id === activeId);
  const memberPanelOpen = useConversationStore((s) => s.memberPanelOpen);
  const setMemberPanelOpen = useConversationStore((s) => s.setMemberPanelOpen);
  const currentUserId = useAuthStore((s) => s.user?.id);
  const markAllRead = useMessageStore((s) => s.markAllRead);
  const typingUsersMap = useWsStore((s) => activeId ? (s.typingUsers[activeId] ?? EMPTY_TYPING) : EMPTY_TYPING);
  const [replyTo, setReplyTo] = useState<Message | null>(null);
  const [searchOpen, setSearchOpen] = useState(false);
  const [searchResults, setSearchResults] = useState<Message[]>([]);
  const [searchLoading, setSearchLoading] = useState(false);
  const [hasSearched, setHasSearched] = useState(false);
  const [groupInfoOpen, setGroupInfoOpen] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const wsClient = useWsStore((s) => s.wsClient);
  const streamingContent = useMessageStore(
    (s) => (activeId ? s.streamingContent[activeId] : undefined),
  );
  const isStreaming = (streamingContent ?? '').length > 0;

  const { send: sendMessage } = useMessages(activeId ?? null);

  const handleFileSelect = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const files = e.target.files;
    if (!files || !activeId) return;
    const fileArr = Array.from(files);
    const validFiles = fileArr.filter((f) => {
      if (f.size > MAX_FILE_SIZE) {
        antMessage.error(`${f.name} 超过 50MB 限制`);
        return false;
      }
      return true;
    });
    if (!validFiles.length) {
      if (fileInputRef.current) fileInputRef.current.value = '';
      return;
    }
    Promise.all(
      validFiles.map(async (f) => {
        try {
          return await uploadFile(f);
        } catch {
          antMessage.error(`${f.name} 上传失败`);
          return null;
        }
      }),
    ).then((results) => {
      const attachments = results.filter((r): r is AttachmentPayload => r !== null);
      if (attachments.length > 0) {
        const names = validFiles.map((f) => f.name).join(', ');
        sendMessage(names, attachments);
      }
    });
    if (fileInputRef.current) fileInputRef.current.value = '';
  }, [activeId, sendMessage]);

  const handleStopTask = useCallback(() => {
    if (!wsClient || !activeId) return;
    wsClient.send(JSON.stringify({
      type: 'user.stop_stream',
      data: { conversation_id: activeId },
    }));
    useMessageStore.setState((s) => {
      const next = { ...s.streamingContent };
      delete next[activeId];
      return { streamingContent: next };
    });
    antMessage.info('已停止生成');
  }, [wsClient, activeId]);

  useEffect(() => {
    if (!activeId) return;
    markAllRead(activeId);
    convApi.markConversationRead(activeId).catch(() => {});
    setReplyTo(null);
    setSearchOpen(false);
    setSearchResults([]);
    setHasSearched(false);
  }, [activeId, markAllRead]);

  // Join WebSocket room when switching conversations so real-time messages arrive
  useEffect(() => {
    if (!wsClient || !activeId) return;
    wsClient.send(JSON.stringify({
      type: 'join_room',
      data: { conversation_id: activeId },
    }));
  }, [wsClient, activeId]);

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

  const handleSelectSearchResult = useCallback((msg: Message) => {
    toggleSearch();
    requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        const el = document.querySelector(`[data-message-id="${msg.id}"]`);
        if (el instanceof HTMLElement) {
          const highlightClass = styles.highlightFlash ?? '';
          el.scrollIntoView({ behavior: 'smooth', block: 'center' });
          el.classList.add(highlightClass);
          el.addEventListener('animationend', () => el.classList.remove(highlightClass), { once: true });
        } else {
          antMessage.info('消息不在当前视图');
        }
      });
    });
  }, [toggleSearch]);

  if (!activeConv) return null;

  const isGroup = activeConv.type === 'group';
  const displayName = isGroup
    ? activeConv.title
    : (activeConv.peer_name || activeConv.title);
  const avatarText = displayName.charAt(0).toUpperCase();
  const otherTyping = typingUsersMap.filter((u: { userId: string }) => u.userId !== currentUserId);

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
            key: 'info' as const,
            icon: <InfoCircleOutlined />,
            label: '群聊信息',
            onClick: () => setGroupInfoOpen(true),
          },
          {
            key: 'settings' as const,
            icon: <SettingOutlined />,
            label: '群聊设置',
            onClick: () => setMemberPanelOpen(true),
          },
          { type: 'divider' as const },
          ...(activeConv.user_id === user?.id
            ? [{
                key: 'dissolve' as const,
                icon: <DeleteOutlined />,
                label: '解散群聊',
                danger: true as const,
                onClick: () => {
                  antMessage.warning('确认解散？', 0);
                  import('antd').then(({ Modal }) => {
                    Modal.confirm({
                      title: '解散群聊',
                      content: '解散后所有成员将被移除，聊天记录将清除，此操作不可撤销。',
                      okText: '确认解散',
                      okType: 'danger',
                      cancelText: '取消',
                      onOk: async () => {
                        try {
                          await dissolveGroup(activeConv.id);
                          antMessage.success('群聊已解散');
                          fetchConversations();
                          useConversationStore.getState().setActive(null);
                        } catch {
                          antMessage.error('解散失败');
                        }
                      },
                    });
                  });
                },
              }]
            : [{
                key: 'leave' as const,
                icon: <LogoutOutlined />,
                label: '退出群聊',
                danger: true as const,
                onClick: () => {
                  import('antd').then(({ Modal }) => {
                    Modal.confirm({
                      title: '退出群聊',
                      content: '退出后将不再接收此群聊消息。',
                      okText: '确认退出',
                      okType: 'danger',
                      cancelText: '取消',
                      onOk: async () => {
                        try {
                          await leaveGroup(activeConv.id);
                          antMessage.success('已退出群聊');
                          fetchConversations();
                          useConversationStore.getState().setActive(null);
                        } catch {
                          antMessage.error('退出失败');
                        }
                      },
                    });
                  });
                },
              }]),
        ]
      : []),
  ];

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <Tooltip title={isGroup ? '查看群聊信息' : undefined} mouseEnterDelay={0.8}>
          <div
            className={styles.headerLeft}
            style={isGroup ? { cursor: 'pointer', borderRadius: 6 } : undefined}
            role={isGroup ? 'button' : undefined}
            tabIndex={isGroup ? 0 : undefined}
            onClick={() => isGroup && setGroupInfoOpen(true)}
            onKeyDown={(e) => {
              if (isGroup && (e.key === 'Enter' || e.key === ' ')) {
                e.preventDefault();
                setGroupInfoOpen(true);
              }
            }}
          >
            <Avatar className={styles.conversationAvatar} size={26}>
              {avatarText}
            </Avatar>
            <h1 className={styles.title}>
              {displayName}
            </h1>
            {isGroup && activeConv.member_count && <span className={styles.memberCount}>{activeConv.member_count}</span>}
          </div>
        </Tooltip>
        <div className={styles.headerActions}>
          <Tooltip title="文件">
            <Button type="text" icon={<FolderOpenOutlined />} size="small" onClick={() => fileInputRef.current?.click()} />
          </Tooltip>
          <input
            ref={fileInputRef}
            type="file"
            accept={ACCEPTED_TYPES}
            multiple
            onChange={handleFileSelect}
            className={styles.hiddenFileInput}
          />
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
            <Button type="text" icon={<StopOutlined />} size="small" disabled={!isStreaming} onClick={handleStopTask} />
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
        <ChatSearchPanel
          searchLoading={searchLoading}
          searchResults={searchResults}
          hasSearched={hasSearched}
          onSearch={handleSearch}
          onClose={toggleSearch}
          onSelectMessage={handleSelectSearchResult}
        />
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
      {isGroup && activeId && (
        <GroupInfoDrawer
          open={groupInfoOpen}
          onClose={() => setGroupInfoOpen(false)}
          conversationId={activeId}
        />
      )}
    </div>
  );
};
