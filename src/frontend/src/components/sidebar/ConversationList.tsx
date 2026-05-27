import React from 'react';
import { Skeleton, Empty } from 'antd';
import { useConversation } from '@/hooks/useConversation';
import { useConversationStore } from '@/store/conversationStore';
import { useMessageStore } from '@/store/messageStore';
import * as convApi from '@/api/conversation';
import type { Message } from '@/types/message';
import { ConversationItem } from './ConversationItem';
import styles from './ConversationList.module.css';

// 稳定空数组引用，避免 Zustand selector 每次返回新 [] 导致无限重渲染
const EMPTY_MESSAGES: Message[] = [];

export const ConversationList: React.FC = () => {
  const { conversations, activeId, loading, setActive, remove, togglePin } =
    useConversation();
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
      <div className={styles.items}>
        {conversations.length === 0 ? (
          <div className={styles.noResults}>未找到匹配的对话</div>
        ) : (
          conversations.map((conv) => (
            <ConversationItemWrapper
              key={conv.id}
              conversation={conv}
              active={conv.id === activeId}
              onSelect={() => setActive(conv.id)}
              onDelete={() => remove(conv.id)}
              onTogglePin={() => togglePin(conv.id, !conv.pinned)}
              onArchive={async () => {
                await convApi.archiveConversation(conv.id);
                remove(conv.id);
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
  onArchive: () => void;
  onInviteMembers?: () => void;
}> = ({ conversation, active, onSelect, onDelete, onTogglePin, onArchive, onInviteMembers }) => {
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
      onArchive={onArchive}
      onInviteMembers={onInviteMembers}
      lastMessage={lastMessage}
      unreadCount={unreadCount}
    />
  );
};
