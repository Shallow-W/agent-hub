import React, { useState, useEffect, useRef } from 'react';
import { Input, Spin } from 'antd';
import { useConversation } from '@/hooks/useConversation';
import { ConversationItem } from './ConversationItem';
import styles from './ConversationList.module.css';

export const ConversationList: React.FC = () => {
  const { conversations, activeId, loading, setActive, remove, togglePin } =
    useConversation();

  const [searchText, setSearchText] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState('');
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // 防抖搜索：300ms
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

  // 加载状态
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

  // 空状态
  if (conversations.length === 0) {
    return (
      <div className={styles.list}>
        <div className={styles.empty}>
          <span className={styles.emptyIcon}>&#128172;</span>
          <span>暂无对话，点击「新建对话」开始</span>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.list}>
      {/* 搜索栏 - 使用 antd Input.Search */}
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

      {/* 对话列表 */}
      <div className={styles.items}>
        {filtered.length === 0 ? (
          <div className={styles.noResults}>未找到匹配的对话</div>
        ) : (
          filtered.map((conv) => (
            <ConversationItem
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
