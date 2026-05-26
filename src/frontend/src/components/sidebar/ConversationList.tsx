import React, { useState, useEffect, useRef } from 'react';
import { Input, Skeleton, Empty } from 'antd';
import { useConversation } from '@/hooks/useConversation';
import { useConversationStore } from '@/store/conversationStore';
import { useMessageStore } from '@/store/messageStore';
import type { Message } from '@/types/message';
import { ConversationItem } from './ConversationItem';
import styles from './ConversationList.module.css';

// 稳定空数组引用，避免 Zustand selector 每次返回新 [] 导致无限重渲染
const EMPTY_MESSAGES: Message[] = [];

export const ConversationList: React.FC = () => {
  const { conversations, activeId, loading, setActive, remove, togglePin } =
    useConversation();
  const setMemberPanelOpen = useConversationStore((s) => s.setMemberPanelOpen);

  const [searchText, setSearchText] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState('');
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (timerRef.current !== null) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => {
      setDebouncedSearch(searchText);
    }, 300);
    return () => {
      if (timerRef.current !== null) clearTimeout(timerRef.current);
    };
  }, [searchText]);

  const filtered = debouncedSearch
    ? conversations.filter((c) =>
        c.title.toLowerCase().includes(debouncedSearch.toLowerCase()),
      )
    : conversations;

  if (loading && conversations.length === 0) {
    return (
      <div className={styles.list}>
        <div className={styles.searchWrapper}>
          <Skeleton.Input active block style={{ height: 34 }} />
        </div>
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
          <Empty
            description="暂无对话，点击「新建对话」开始"
            image={Empty.PRESENTED_IMAGE_SIMPLE}
          />
        </div>
      </div>
    );
  }

  return (
    <div className={styles.list}>
      <div className={styles.searchWrapper}>
        <Input.Search
          placeholder="搜索对话..."
          allowClear
          value={searchText}
          onChange={(e) => setSearchText(e.target.value)}
          onClear={() => setSearchText('')}
          style={{ height: 34 }}
        />
      </div>

      <div className={styles.items}>
        {filtered.length === 0 ? (
          <div className={styles.noResults}>未找到匹配的对话</div>
        ) : (
          filtered.map((conv) => (
            <ConversationItemWrapper
              key={conv.id}
              conversation={conv}
              active={conv.id === activeId}
              onSelect={() => setActive(conv.id)}
              onDelete={() => remove(conv.id)}
              onTogglePin={() => togglePin(conv.id, !conv.pinned)}
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
        )}
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
  onInviteMembers?: () => void;
}> = ({ conversation, active, onSelect, onDelete, onTogglePin, onInviteMembers }) => {
  const messages = useMessageStore(
    (s) => s.messages[conversation.id] ?? EMPTY_MESSAGES,
  );
  const unreadCount = useMessageStore(
    (s) => s.unreadCounts[conversation.id] ?? 0,
  );

  const lastMsg = messages.length > 0 ? messages[messages.length - 1] : undefined;
  const lastMessage = lastMsg?.content;

  return (
    <ConversationItem
      conversation={conversation}
      active={active}
      onSelect={onSelect}
      onDelete={onDelete}
      onTogglePin={onTogglePin}
      onInviteMembers={onInviteMembers}
      lastMessage={lastMessage}
      unreadCount={unreadCount}
    />
  );
};
