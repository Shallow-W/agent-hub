import React, { useState } from 'react';
import { Skeleton, Button, Input } from 'antd';
import { MessageOutlined, TeamOutlined, SearchOutlined } from '@ant-design/icons';
import { useConversation } from '@/hooks/useConversation';
import { useConversationStore } from '@/store/conversationStore';
import { useMessageStore } from '@/store/messageStore';
import * as convApi from '@/api/conversation';
import type { Message } from '@/types/message';
import { ConversationItem } from './ConversationItem';
import styles from './ConversationList.module.css';

const EMPTY_MESSAGES: Message[] = [];

interface ConversationListProps {
  onNavigateFriends?: () => void;
}

export const ConversationList: React.FC<ConversationListProps> = ({ onNavigateFriends }) => {
  const { conversations, activeId, loading, setActive, remove, togglePin, rename, create } =
    useConversation();
  const [searchQuery, setSearchQuery] = useState('');
  const archiveConversationLocal = useConversationStore((s) => s.archiveConversationLocal);
  const setMemberPanelOpen = useConversationStore((s) => s.setMemberPanelOpen);

  if (loading && conversations.length === 0) {
    return (
      <div className={styles.list}>
        <div style={{ padding: '8px 12px' }}>
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} style={{ display: 'flex', gap: 10, padding: '10px 0', alignItems: 'center' }}>
              <Skeleton.Avatar active size={36} />
              <div style={{ flex: 1 }}>
                <Skeleton active paragraph={{ rows: 1, width: '60%' }} title={false} />
              </div>
            </div>
          ))}
        </div>
      </div>
    );
  }

  if (conversations.length === 0) {
    return (
      <div className={styles.list}>
        <div className={styles.empty}>
          <div className={styles.emptyIcon}>
            <MessageOutlined />
          </div>
          <div className={styles.emptyTitle}>欢迎使用 AgentHub</div>
          <div className={styles.emptyDesc}>开始你的第一个对话吧</div>
          <div className={styles.emptyActions}>
            <Button
              type="primary"
              icon={<MessageOutlined />}
              onClick={() => create('single', '新对话')}
            >
              新建对话
            </Button>
            <Button
              icon={<TeamOutlined />}
              onClick={onNavigateFriends}
            >
              添加好友
            </Button>
          </div>
        </div>
      </div>
    );
  }

  const filtered = searchQuery
    ? conversations.filter((c) => c.title.toLowerCase().includes(searchQuery.toLowerCase()))
    : conversations;

  return (
    <div className={styles.list}>
      <div className={styles.searchWrap} data-conv-search>
        <Input
          prefix={<SearchOutlined />}
          placeholder="搜索对话..."
          allowClear
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          className={styles.searchInput}
        />
      </div>
      <div className={styles.items}>
        {filtered.map((conv) => (
          <ConversationItemWrapper
              key={conv.id}
              conversation={conv}
              active={conv.id === activeId}
              onSelect={() => setActive(conv.id)}
              onDelete={() => remove(conv.id)}
              onTogglePin={() => togglePin(conv.id)}
              onRename={conv.type === 'group' ? (newTitle: string) => rename(conv.id, newTitle) : undefined}
              onArchive={async () => {
                await convApi.archiveConversation(conv.id);
                archiveConversationLocal(conv.id);
              }}
              onInviteMembers={
                conv.type === 'group'
                  ? () => {
                      setActive(conv.id);
                      setMemberPanelOpen(true);
                    }
                  : undefined
              }
            />
          ))
        }
      </div>
    </div>
  );
};

/** Wrapper that reads last message and unread count from message store */
const ConversationItemWrapper: React.FC<{
  conversation: Parameters<typeof ConversationItem>[0]['conversation'];
  active: boolean;
  onSelect: () => void;
  onDelete: () => void;
  onTogglePin: () => void;
  onRename?: (newTitle: string) => void;
  onArchive: () => void;
  onInviteMembers?: () => void;
}> = ({ conversation, active, onSelect, onDelete, onTogglePin, onRename, onArchive, onInviteMembers }) => {
  const messages = useMessageStore(
    (s) => s.messages[conversation.id] ?? EMPTY_MESSAGES,
  );
  const unreadCount = useMessageStore(
    (s) => s.unreadCounts[conversation.id] ?? 0,
  );

  const lastMsg = messages.length > 0 ? messages[messages.length - 1] : undefined;
  // 优先使用本地 store 实时数据，API 数据作为兜底
  const lastMessage = lastMsg?.content || conversation.last_message;

  return (
    <ConversationItem
      conversation={conversation}
      active={active}
      onSelect={onSelect}
      onDelete={onDelete}
      onTogglePin={onTogglePin}
      onRename={onRename}
      onArchive={onArchive}
      onInviteMembers={onInviteMembers}
      lastMessage={lastMessage}
      unreadCount={unreadCount}
    />
  );
};
