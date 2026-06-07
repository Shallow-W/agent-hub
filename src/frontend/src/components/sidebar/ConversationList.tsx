import React, { useState, useEffect } from 'react';
import { Skeleton, Button, Input, message as antMessage } from 'antd';
import { MessageOutlined, TeamOutlined, SearchOutlined, FolderOutlined, RightOutlined, LeftOutlined } from '@ant-design/icons';
import { useConversation } from '@/hooks/useConversation';
import { useConversationStore } from '@/store/conversationStore';
import { useMessageStore } from '@/store/messageStore';
import * as convApi from '@/api/conversation';
import type { Message } from '@/types/message';
import type { Conversation } from '@/types/conversation';
import { ConversationItem } from './ConversationItem';
import styles from './ConversationList.module.css';

const EMPTY_MESSAGES: Message[] = [];

interface ConversationListProps {
  onNavigateContacts?: () => void;
}

export const ConversationList: React.FC<ConversationListProps> = ({ onNavigateContacts }) => {
  const { conversations, activeId, loading, setActive, remove, togglePin, rename, create } =
    useConversation();
  const [searchQuery, setSearchQuery] = useState('');
  const archiveConversationLocal = useConversationStore((s) => s.archiveConversationLocal);
  const setMemberPanelOpen = useConversationStore((s) => s.setMemberPanelOpen);
  const fetchConversations = useConversationStore((s) => s.fetchConversations);

  // Archived view state
  const [showArchived, setShowArchived] = useState(false);
  const [archivedConvs, setArchivedConvs] = useState<Conversation[]>([]);
  const [archivedCount, setArchivedCount] = useState(0);

  // Fetch archived count on mount
  useEffect(() => {
    convApi.getArchivedConversations()
      .then((list) => setArchivedCount(list?.length ?? 0))
      .catch(() => {});
  }, []);

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

  if (conversations.length === 0 && !showArchived) {
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
              onClick={onNavigateContacts}
            >
              添加好友
            </Button>
          </div>
        </div>
      </div>
    );
  }

  const handleOpenArchived = async () => {
    try {
      const list = await convApi.getArchivedConversations();
      setArchivedConvs(list ?? []);
      setShowArchived(true);
    } catch {
      antMessage.error('获取归档对话失败');
    }
  };

  const handleUnarchive = async (convId: string) => {
    try {
      await convApi.unarchiveConversation(convId);
      const next = archivedConvs.filter((c) => c.id !== convId);
      setArchivedConvs(next);
      setArchivedCount(next.length);
      await fetchConversations();
      antMessage.success('已取消归档');
      if (next.length === 0) {
        setShowArchived(false);
      }
    } catch {
      antMessage.error('取消归档失败');
    }
  };

  const filtered = searchQuery
    ? conversations.filter((c) => c.title.toLowerCase().includes(searchQuery.toLowerCase()))
    : conversations;

  const pinnedConvs = filtered.filter((c) => c.pinned);
  const unpinnedConvs = filtered.filter((c) => !c.pinned);
  const agentConvs = unpinnedConvs.filter((c) => c.type === 'agent');
  const groupConvs = unpinnedConvs.filter((c) => c.type === 'group');
  const singleConvs = unpinnedConvs.filter((c) => c.type === 'single');

  const renderGroup = (convs: typeof filtered, header: string) =>
    convs.length > 0 && (
      <>
        <div className={styles.sectionHeader}>{header}</div>
        {convs.map((conv) => (
          <ConversationItemWrapper
            key={conv.id}
            conversation={conv}
            active={conv.id === activeId}
            onSelect={() => setActive(conv.id)}
            onDelete={() => remove(conv.id)}
            onTogglePin={() => togglePin(conv.id)}
            onRename={conv.type === 'group' ? (newTitle: string) => rename(conv.id, newTitle) : undefined}
            onArchive={async () => {
              try {
                await convApi.archiveConversation(conv.id);
                archiveConversationLocal(conv.id);
                setArchivedCount((prev) => prev + 1);
              } catch {
                antMessage.error('归档失败');
              }
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
        ))}
      </>
    );

  // Archived view
  if (showArchived) {
    return (
      <div className={styles.list}>
        <div className={styles.searchWrap} data-conv-search>
          <div className={styles.archiveHeader}>
            <button className={styles.archiveBack} type="button" onClick={() => setShowArchived(false)}>
              <LeftOutlined /> 返回对话
            </button>
            <span className={styles.archiveHeaderTitle}>归档对话</span>
          </div>
        </div>
        <div className={styles.items}>
          {archivedConvs.length === 0 ? (
            <div className={styles.noResults}>暂无归档对话</div>
          ) : (
            archivedConvs.map((conv) => (
              <ArchivedConversationItemWrapper
                key={conv.id}
                conversation={conv}
                onUnarchive={() => handleUnarchive(conv.id)}
              />
            ))
          )}
        </div>
      </div>
    );
  }

  // Normal view
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
        {archivedCount > 0 && (
          <button className={styles.archiveFolder} type="button" onClick={handleOpenArchived}>
            <div className={styles.archiveFolderIcon}>
              <FolderOutlined />
            </div>
            <div className={styles.archiveFolderInfo}>
              <span className={styles.archiveFolderTitle}>归档对话</span>
              <span className={styles.archiveFolderCount}>{archivedCount} 个对话</span>
            </div>
            <RightOutlined className={styles.archiveFolderArrow} />
          </button>
        )}
        {renderGroup(pinnedConvs, '置顶')}
        {renderGroup(agentConvs, '智能体')}
        {renderGroup(groupConvs, '群聊')}
        {renderGroup(singleConvs, '单聊')}
      </div>
    </div>
  );
};

/** Wrapper for normal conversations */
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

/** Wrapper for archived conversations */
const ArchivedConversationItemWrapper: React.FC<{
  conversation: Conversation;
  onUnarchive: () => void;
}> = ({ conversation, onUnarchive }) => {
  const messages = useMessageStore(
    (s) => s.messages[conversation.id] ?? EMPTY_MESSAGES,
  );
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
