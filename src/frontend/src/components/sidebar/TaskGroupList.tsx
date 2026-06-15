import React, { useState, useEffect, useMemo } from 'react';
import { Input } from 'antd';
import { message as antMessage } from '@/utils/message';
import { FolderOutlined, LeftOutlined, RightOutlined, SearchOutlined } from '@ant-design/icons';
import { useConversationStore } from '@/store/conversationStore';
import { useMessageStore } from '@/store/messageStore';
import * as convApi from '@/api/conversation';
import { ConversationItem } from './ConversationItem';
import type { Conversation } from '@/types/conversation';
import type { Message } from '@/types/message';
import listStyles from './ConversationList.module.css';

const EMPTY_MESSAGES: Message[] = [];

export const TaskGroupList: React.FC = () => {
  const conversations = useConversationStore((s) => s.conversations);
  const activeId = useConversationStore((s) => s.activeConversationId);
  const setActive = useConversationStore((s) => s.setActive);
  const archiveConversationLocal = useConversationStore((s) => s.archiveConversationLocal);
  const fetchConversations = useConversationStore((s) => s.fetchConversations);

  const [query, setQuery] = useState('');
  const [showArchived, setShowArchived] = useState(false);
  const [archivedConvs, setArchivedConvs] = useState<Conversation[]>([]);
  const [archivedCount, setArchivedCount] = useState(0);

  const filtered = useMemo(() => {
    const groups = conversations.filter((c) => c.type === 'group');
    const n = query.trim().toLowerCase();
    return n ? groups.filter((c) => (c.title ?? '').toLowerCase().includes(n)) : groups;
  }, [conversations, query]);

  useEffect(() => {
    convApi.getArchivedConversations()
      .then((list) => {
        const items = list ?? [];
        const groups = items.filter((c) => c.type === 'group');
        setArchivedCount(groups.length);
        setArchivedConvs(groups);
      })
      .catch(() => {});
  }, []);

  const handleOpenArchived = () => {
    setShowArchived(true);
  };

  const handleUnarchive = async (convId: string) => {
    try {
      await convApi.unarchiveConversation(convId);
      const next = archivedConvs.filter((c) => c.id !== convId);
      setArchivedConvs(next);
      setArchivedCount(next.length);
      await fetchConversations();
      antMessage.success('已取消归档');
      if (next.length === 0) setShowArchived(false);
    } catch {
      antMessage.error('取消归档失败');
    }
  };

  const hasQuery = query.trim().length > 0;
  const noResults = filtered.length === 0;
  const noResultsText = hasQuery ? '无匹配结果' : '暂无群聊';

  // archived view...
  if (showArchived) {
    return (
      <div className={listStyles.list}>
        <div className={listStyles.searchWrap}>
          <div className={listStyles.archiveHeader}>
            <button className={listStyles.archiveBack} type="button" onClick={() => setShowArchived(false)}>
              <LeftOutlined /> 返回群聊
            </button>
            <span className={listStyles.archiveHeaderTitle}>归档群聊</span>
          </div>
        </div>
        <div className={listStyles.items}>
          {archivedConvs.length === 0 ? (
            <div className={listStyles.noResults}>暂无归档群聊</div>
          ) : (
            archivedConvs.map((conv) => (
              <ArchivedItem key={conv.id} conversation={conv} onUnarchive={() => handleUnarchive(conv.id)} />
            ))
          )}
        </div>
      </div>
    );
  }

  return (
    <div className={listStyles.list}>
      <div className={listStyles.searchWrap}>
        <Input
          prefix={<SearchOutlined />}
          placeholder="搜索群聊..."
          allowClear
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          className={listStyles.searchInput}
        />
      </div>
      <div className={listStyles.items}>
        {archivedCount > 0 && (
          <button className={listStyles.archiveFolder} type="button" onClick={handleOpenArchived}>
            <div className={listStyles.archiveFolderIcon}>
              <FolderOutlined />
            </div>
            <div className={listStyles.archiveFolderInfo}>
              <span className={listStyles.archiveFolderTitle}>归档群聊</span>
              <span className={listStyles.archiveFolderCount}>{archivedCount} 个群聊</span>
            </div>
            <RightOutlined className={listStyles.archiveFolderArrow} />
          </button>
        )}
        {noResults ? (
          <div className={listStyles.noResults}>{noResultsText}</div>
        ) : (
          filtered.map((conv) => (
            <GroupItem
              key={conv.id}
              conversation={conv}
              active={conv.id === activeId}
              onSelect={() => setActive(conv.id)}
              onArchive={async () => {
                try {
                  await convApi.archiveConversation(conv.id);
                  archiveConversationLocal(conv.id);
                  setArchivedCount((prev) => prev + 1);
                } catch {
                  antMessage.error('归档失败');
                }
              }}
            />
          ))
        )}
      </div>
    </div>
  );
};

const GroupItem: React.FC<{
  conversation: Conversation;
  active: boolean;
  onSelect: () => void;
  onArchive: () => void;
}> = ({ conversation, active, onSelect, onArchive }) => {
  const messages = useMessageStore((s) => s.messages[conversation.id] ?? EMPTY_MESSAGES);
  const unreadCount = useMessageStore((s) => s.unreadCounts[conversation.id] ?? 0);
  const lastMsg = messages.length > 0 ? messages[messages.length - 1] : undefined;
  const lastMessage = lastMsg?.content || conversation.last_message;

  return (
    <ConversationItem
      conversation={conversation}
      active={active}
      onSelect={onSelect}
      onDelete={() => {}}
      onTogglePin={() => {}}
      onRename={conversation.type === 'group' ? () => {} : undefined}
      onArchive={onArchive}
      lastMessage={lastMessage}
      unreadCount={unreadCount}
    />
  );
};

const ArchivedItem: React.FC<{
  conversation: Conversation;
  onUnarchive: () => void;
}> = ({ conversation, onUnarchive }) => {
  const messages = useMessageStore((s) => s.messages[conversation.id] ?? EMPTY_MESSAGES);
  const lastMsg = messages.length > 0 ? messages[messages.length - 1] : undefined;
  const lastMessage = lastMsg?.content || conversation.last_message;

  return (
    <ConversationItem
      conversation={conversation}
      active={false}
      onSelect={() => {}}
      onDelete={() => {}}
      onTogglePin={() => {}}
      onArchive={() => {}}
      onUnarchive={onUnarchive}
      lastMessage={lastMessage}
      unreadCount={0}
    />
  );
};
