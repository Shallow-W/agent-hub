import React, { useMemo } from 'react';
import { Button, Input } from 'antd';
import { CloseOutlined } from '@ant-design/icons';
import type { Message } from '@/types/message';
import styles from './ChatSearchPanel.module.css';
import { SimpleList as List } from '@/components/common/SimpleList';

interface ChatSearchPanelProps {
  searchLoading: boolean;
  searchResults: Message[];
  hasSearched: boolean;
  keyword: string;
  onSearch: (value: string) => void;
  onClose: () => void;
  onSelectMessage: (message: Message) => void;
}

/** 截断内容并在关键词处前后各保留一定上下文 */
function snippetWithKeyword(content: string, keyword: string, maxLen = 120): string {
  if (!keyword) return content.length > maxLen ? content.slice(0, maxLen) + '...' : content;
  const lower = content.toLowerCase();
  const kwLower = keyword.toLowerCase();
  const idx = lower.indexOf(kwLower);
  if (idx === -1) return content.length > maxLen ? content.slice(0, maxLen) + '...' : content;
  const start = Math.max(0, idx - 30);
  const end = Math.min(content.length, idx + keyword.length + 70);
  const prefix = start > 0 ? '...' : '';
  const suffix = end < content.length ? '...' : '';
  return prefix + content.slice(start, end) + suffix;
}

/** 高亮关键词片段：返回 React 节点数组 */
function highlightKeyword(text: string, keyword: string): React.ReactNode[] {
  if (!keyword) return [text];
  const parts: React.ReactNode[] = [];
  const lower = text.toLowerCase();
  const kwLower = keyword.toLowerCase();
  let lastIdx = 0;
  let matchIdx = lower.indexOf(kwLower, lastIdx);
  let key = 0;

  while (matchIdx !== -1) {
    if (matchIdx > lastIdx) {
      parts.push(<span key={key++}>{text.slice(lastIdx, matchIdx)}</span>);
    }
    parts.push(
      <mark key={key++} className={styles.highlight}>
        {text.slice(matchIdx, matchIdx + keyword.length)}
      </mark>,
    );
    lastIdx = matchIdx + keyword.length;
    matchIdx = lower.indexOf(kwLower, lastIdx);
  }

  if (lastIdx < text.length) {
    parts.push(<span key={key++}>{text.slice(lastIdx)}</span>);
  }

  return parts.length > 0 ? parts : [text];
}

export const ChatSearchPanel: React.FC<ChatSearchPanelProps> = ({
  searchLoading,
  searchResults,
  hasSearched,
  keyword,
  onSearch,
  onClose,
  onSelectMessage,
}) => {
  const displayResults = useMemo(
    () => searchResults.map((msg) => ({
      msg,
      snippet: snippetWithKeyword(msg.content, keyword),
    })),
    [searchResults, keyword],
  );

  return (
    <div className={styles.searchPanel}>
      <div className={styles.searchBar}>
        <Input.Search
          className={styles.searchInput}
          placeholder="搜索消息"
          allowClear
          enterButton
          loading={searchLoading}
          onSearch={onSearch}
        />
        <Button
          type="text"
          icon={<CloseOutlined />}
          size="small"
          onClick={onClose}
        />
      </div>
      {displayResults.length > 0 && (
        <List
          className={styles.searchResults}
          dataSource={displayResults}
          renderItem={({ msg, snippet }) => (
            <List.Item
              className={styles.searchResultItem}
              onClick={() => onSelectMessage(msg)}
            >
              <div className={styles.resultContent}>
                <span className={styles.searchResultSender}>
                  {msg.username || msg.role}
                </span>
                <span className={styles.searchResultText}>
                  {highlightKeyword(snippet, keyword)}
                </span>
              </div>
              <span className={styles.searchResultTime}>
                {new Date(msg.created_at).toLocaleString()}
              </span>
            </List.Item>
          )}
        />
      )}
      {hasSearched && searchResults.length === 0 && !searchLoading && (
        <div className={styles.emptySearch}>
          未找到相关消息
        </div>
      )}
    </div>
  );
};
