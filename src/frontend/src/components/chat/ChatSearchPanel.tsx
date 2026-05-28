import React from 'react';
import { Button, Input, List } from 'antd';
import { CloseOutlined } from '@ant-design/icons';
import type { Message } from '@/types/message';
import styles from './ChatSearchPanel.module.css';

interface ChatSearchPanelProps {
  searchLoading: boolean;
  searchResults: Message[];
  hasSearched: boolean;
  onSearch: (value: string) => void;
  onClose: () => void;
  onSelectMessage: (message: Message) => void;
}

export const ChatSearchPanel: React.FC<ChatSearchPanelProps> = ({
  searchLoading,
  searchResults,
  hasSearched,
  onSearch,
  onClose,
  onSelectMessage,
}) => (
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
    {searchResults.length > 0 && (
      <List
        className={styles.searchResults}
        dataSource={searchResults}
        renderItem={(msg) => (
          <List.Item
            className={styles.searchResultItem}
            onClick={() => onSelectMessage(msg)}
          >
            <List.Item.Meta
              title={
                <span className={styles.searchResultSender}>
                  {msg.username || msg.role}
                </span>
              }
              description={
                <span className={styles.searchResultContent}>
                  {msg.content.length > 100 ? `${msg.content.slice(0, 100)}...` : msg.content}
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
      <div className={styles.emptySearch}>
        未找到相关消息
      </div>
    )}
  </div>
);
