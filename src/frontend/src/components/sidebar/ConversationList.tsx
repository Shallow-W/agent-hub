import React, { useState, useEffect, useRef } from 'react';
import { Input, Spin, Empty } from 'antd';
import { useConversation } from '@/hooks/useConversation';
import { useMessageStore } from '@/store/messageStore';
import { ConversationItem } from './ConversationItem';
import styles from './ConversationList.module.css';

export const ConversationList: React.FC = () => {
  const { conversations, activeId, loading, setActive, remove, togglePin } =
    useConversation();

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
        <div className={styles.loading}>
          <Spin size="small" />
          <span>加载中...</span>
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
            />
          ))
        )}
      </div>
    </div>
  );
};

/** Wrapper that reads last message from message store */
const ConversationItemWrapper: React.FC<{
  conversation: Parameters<typeof ConversationItem>[0]['conversation'];
  active: boolean;
  onSelect: () => void;
  onDelete: () => void;
  onTogglePin: () => void;
}> = ({ conversation, active, onSelect, onDelete, onTogglePin }) => {
  const messages = useMessageStore(
    (s) => s.messages[conversation.id] ?? [],
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
      lastMessage={lastMessage}
    />
  );
};
