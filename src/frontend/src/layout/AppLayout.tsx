import React from 'react';
import { Outlet } from 'react-router-dom';
import { ConversationList } from '@/components/sidebar/ConversationList';
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

  const wsIndicator =
    status === 'connected' ? '🟢' : status === 'connecting' ? '🟡' : '🔴';

  return (
    <div className={styles.container}>
      <div className={styles.sidebar}>
        <div className={styles.sidebarHeader}>
          <div>
            <span className={styles.sidebarTitle}>对话</span>
            <span title={`WebSocket: ${status}`}>{wsIndicator}</span>
          </div>
          <Button variant="primary" onClick={handleCreate}>
            新建对话
          </Button>
        </div>
        <ConversationList />
        <div className={styles.userInfo}>
          <span className={styles.username}>{user?.username}</span>
          <Button variant="secondary" onClick={handleLogout}>
            退出
          </Button>
        </div>
      </div>
      <div className={styles.main}>
        <Outlet />
      </div>
    </div>
  );
};

export default AppLayout;
