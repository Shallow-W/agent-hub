import React from 'react';
import { Avatar, Button } from 'antd';
import { PlusOutlined, ReloadOutlined, UploadOutlined } from '@ant-design/icons';
import { ConversationList } from '@/components/sidebar/ConversationList';
import FriendList from '@/components/friends/FriendList';
import FriendRequest from '@/components/friends/FriendRequest';
import { useConversationStore } from '@/store/conversationStore';
import type { Conversation } from '@/types/conversation';
import styles from './AppLayout.module.css';

interface MiddlePanelProps {
  activeNav: string;
  conversations: Conversation[];
  onCreate: () => void;
  onCreateGroup: () => void;
  onRefresh: () => void;
  onUpload: () => void;
  onShowArchived: () => void;
  onStartChat: (friendId: string) => void;
  onSwitchChat: () => void;
  onSwitchFriends: () => void;
}

const MiddlePanel: React.FC<MiddlePanelProps> = ({
  activeNav,
  conversations,
  onCreate,
  onCreateGroup,
  onRefresh,
  onUpload,
  onShowArchived,
  onStartChat,
  onSwitchChat,
  onSwitchFriends,
}) => {
  const renderPanelTools = (onAdd: () => void) => (
    <div className={styles.convPanelTools}>
      <Button type="text" icon={<UploadOutlined />} aria-label="上传" onClick={onUpload} />
      <Button type="text" icon={<PlusOutlined />} aria-label="添加" onClick={onAdd} />
      <Button type="text" icon={<ReloadOutlined />} aria-label="刷新" onClick={onRefresh} />
    </div>
  );

  if (activeNav === 'friends') {
    return (
      <>
        <div className={styles.convPanelHeader}>
          <span className={styles.convPanelTitle}>好友</span>
          {renderPanelTools(onCreate)}
        </div>
        <div className={styles.middleScroll}>
          <FriendRequest />
          <div className={styles.middleSection}>
            <FriendList onStartChat={onStartChat} />
          </div>
        </div>
      </>
    );
  }

  if (activeNav === 'groups') {
    const groupConvs = conversations.filter((c) => c.type === 'group');
    return (
      <>
        <div className={styles.convPanelHeader}>
          <span className={styles.convPanelTitle}>群聊</span>
          {renderPanelTools(onCreateGroup)}
        </div>
        {groupConvs.length === 0 ? (
          <div className={styles.emptyState}>暂无群聊</div>
        ) : (
          <div className={styles.groupList}>
            {groupConvs.map((conv) => (
              <div
                key={conv.id}
                className={styles.groupItem}
                onClick={() => {
                  useConversationStore.getState().setActive(conv.id);
                  onSwitchChat();
                }}
              >
                <Avatar className={styles.groupAvatar} shape="square">
                  {conv.title.charAt(0).toUpperCase()}
                </Avatar>
                <div className={styles.groupMeta}>
                  <div className={styles.groupTitle}>{conv.title}</div>
                  <div className={styles.groupTime}>
                    创建于 {new Date(conv.created_at).toLocaleDateString('zh-CN')}
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </>
    );
  }

  return (
    <>
      <div className={styles.convPanelHeader}>
        <span className={styles.convPanelTitle}>消息</span>
        {renderPanelTools(onCreate)}
      </div>
      <ConversationList onNavigateFriends={onSwitchFriends} />
      <button className={styles.archivedLink} onClick={onShowArchived} type="button">
        查看归档对话
      </button>
    </>
  );
};

export default MiddlePanel;
