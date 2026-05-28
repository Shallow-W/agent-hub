import React from 'react';
import { Button } from 'antd';
import { PlusOutlined, ReloadOutlined, UploadOutlined } from '@ant-design/icons';
import { ConversationList } from '@/components/sidebar/ConversationList';
import ContactsPanel from '@/components/contacts/ContactsPanel';
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
  onSwitchContacts: () => void;
  onRefreshContacts: () => void;
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
  onSwitchContacts,
  onRefreshContacts,
}) => {
  const renderPanelTools = (onAdd: () => void) => (
    <div className={styles.convPanelTools}>
      <Button type="text" icon={<UploadOutlined />} aria-label="上传" onClick={onUpload} />
      <Button type="text" icon={<PlusOutlined />} aria-label="添加" onClick={onAdd} />
      <Button type="text" icon={<ReloadOutlined />} aria-label="刷新" onClick={onRefresh} />
    </div>
  );

  if (activeNav === 'contacts') {
    return (
      <>
        <div className={styles.convPanelHeader}>
          <span className={styles.convPanelTitle}>联系人</span>
          <div className={styles.convPanelTools}>
            <Button type="text" icon={<PlusOutlined />} aria-label="新建群聊" onClick={onCreateGroup} />
            <Button type="text" icon={<ReloadOutlined />} aria-label="刷新" onClick={onRefreshContacts} />
          </div>
        </div>
        <div className={styles.middleScroll}>
          <ContactsPanel
            conversations={conversations}
            onStartChat={onStartChat}
            onSwitchChat={onSwitchChat}
          />
        </div>
      </>
    );
  }

  return (
    <>
      <div className={styles.convPanelHeader}>
        <span className={styles.convPanelTitle}>消息</span>
        {renderPanelTools(onCreate)}
      </div>
      <ConversationList onNavigateContacts={onSwitchContacts} />
      <button className={styles.archivedLink} onClick={onShowArchived} type="button">
        查看归档对话
      </button>
    </>
  );
};

export default MiddlePanel;
