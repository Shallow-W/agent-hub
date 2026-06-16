import React from 'react';
import { Button } from 'antd';
import { PlusOutlined, ReloadOutlined, UploadOutlined } from '@ant-design/icons';
import { ChatWindow } from '@/components/chat/ChatWindow';
import { ConversationList } from '@/components/sidebar/ConversationList';
import { useConversation } from '@/hooks/useConversation';
import { useConversationStore } from '@/store/conversationStore';


import styles from '@/layout/AppLayout.module.css';
import viewStyles from './ChatView.module.css';

interface ChatViewProps {
  onCreate?: () => void;
  
  onUpload?: () => void;
}

const ChatView: React.FC<ChatViewProps> = ({ onCreate,  onUpload }) => {
  const { activeId } = useConversation();
  const fetchConversations = useConversationStore((s) => s.fetchConversations);

  if (!activeId) {
    return (
      <>
        <div className={styles.convPanelHeader}>
          <span className={styles.convPanelTitle}>消息</span>
          <div className={styles.convPanelTools}>
            {onUpload && <Button type="text" icon={<UploadOutlined />} aria-label="上传" onClick={onUpload} />}
            {onCreate && <Button type="text" icon={<PlusOutlined />} aria-label="添加" onClick={onCreate} />}
            <Button type="text" icon={<ReloadOutlined />} aria-label="刷新" onClick={() => fetchConversations()} />
          </div>
        </div>
        <ConversationList onNavigateContacts={() => {}} />
        <div className={viewStyles.empty}>
          <span className={viewStyles.icon} role="img" aria-label="chat">&#x1F916;</span>
          <div className={viewStyles.title}>欢迎使用 AgentHub</div>
          <div className={viewStyles.subtitle}>选择一个对话或创建新对话开始聊天</div>
        </div>
      </>
    );
  }

  return (
    <>
      <div className={styles.convPanelHeader}>
        <span className={styles.convPanelTitle}>消息</span>
        <div className={styles.convPanelTools}>
          {onUpload && <Button type="text" icon={<UploadOutlined />} aria-label="上传" onClick={onUpload} />}
          {onCreate && <Button type="text" icon={<PlusOutlined />} aria-label="添加" onClick={onCreate} />}
          <Button type="text" icon={<ReloadOutlined />} aria-label="刷新" onClick={() => fetchConversations()} />
        </div>
      </div>
      <ConversationList onNavigateContacts={() => {}} />
      <ChatWindow />
    </>
  );
};

export default ChatView;
