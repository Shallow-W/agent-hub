import React, { useState, useEffect, useRef, useCallback } from 'react';
import { useConversation } from '@/hooks/useConversation';
import { ConversationItem } from './ConversationItem';
import styles from './ConversationList.module.css';

export const ConversationList: React.FC = () => {
  const { conversations, activeId, loading, setActive, remove, togglePin } =
    useConversation();

  const [searchText, setSearchText] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState('');
  const [archiveOpen, setArchiveOpen] = useState(false);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Debounced search: 300ms
  useEffect(() => {
    if (timerRef.current !== null) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => {
      setDebouncedSearch(searchText);
    }, 300);
    return () => {
      if (timerRef.current !== null) clearTimeout(timerRef.current);
    };
  }, [searchText]);

  const handleSearchChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      setSearchText(e.target.value);
    },
    [],
  );

  const handleClear = useCallback(() => {
    setSearchText('');
    setDebouncedSearch('');
  }, []);

  const filtered = debouncedSearch
    ? conversations.filter((c) =>
        c.title.toLowerCase().includes(debouncedSearch.toLowerCase()),
      )
    : conversations;

  const archivedCount = 0; // Placeholder until archive feature is implemented

  // Loading state
  if (loading && conversations.length === 0) {
    return (
      <div className={styles.list}>
        <div className={styles.loading}>
          <div className={styles.pulseDot} />
          <span>加载中...</span>
        </div>
      </div>
    );
  }

  // Empty state
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
      {/* Search Bar */}
      <div className={styles.searchWrapper}>
        <div className={styles.searchBox}>
          <span className={styles.searchIcon}>&#128269;</span>
          <input
            className={styles.searchInput}
            type="text"
            placeholder="搜索对话..."
            value={searchText}
            onChange={handleSearchChange}
          />
          {searchText && (
            <button
              className={styles.clearBtn}
              onClick={handleClear}
              aria-label="清除搜索"
            >
              &times;
            </button>
          )}
        </div>
      </div>

      {/* Conversation Items */}
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

      {/* Archived Section */}
      <div className={styles.archiveSeparator} />
      <button
        className={styles.archiveToggle}
        onClick={() => setArchiveOpen((v) => !v)}
      >
        <span
          className={`${styles.archiveChevron} ${
            archiveOpen ? styles.archiveChevronOpen : ''
          }`}
        >
          &#9654;
        </span>
        <span>已归档</span>
        <span className={styles.archiveCount}>{archivedCount}</span>
      </button>
    </div>
  );
};
