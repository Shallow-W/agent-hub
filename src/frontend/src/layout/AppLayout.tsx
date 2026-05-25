import React from 'react';
import { Outlet } from 'react-router-dom';
import { ConversationList } from '@/components/sidebar/ConversationList';
import SettingsPanel from '@/components/settings/SettingsPanel';
import { useConversation } from '@/hooks/useConversation';
import { useWebSocket } from '@/hooks/useWebSocket';
import { useAuth } from '@/hooks/useAuth';
import Button from '@/components/common/Button';
import styles from './AppLayout.module.css';

const AppLayout: React.FC = () => {
  const { create } = useConversation();
  const { status } = useWebSocket();
  const { user, logout: handleLogout } = useAuth();

  const handleCreate = async () => {
    await create('single', `新对话`);
  };

  return (
    <div className={styles.container}>
      {/* 左侧：设置面板 */}
      <div className={styles.settingsPanel}>
        <SettingsPanel
          username={user?.username ?? ''}
          onLogout={handleLogout}
          wsStatus={status}
        />
      </div>

      {/* 中间：对话列表 */}
      <div className={styles.convPanel}>
        <div className={styles.convPanelHeader}>
          <span className={styles.convPanelTitle}>对话</span>
          <Button variant="primary" onClick={handleCreate}>
            新建对话
          </Button>
        </div>
        <ConversationList />
      </div>

      {/* 右侧：聊天区域 */}
      <div className={styles.chatPanel}>
        <Outlet />
      </div>
    </div>
  );
};

export default AppLayout;
