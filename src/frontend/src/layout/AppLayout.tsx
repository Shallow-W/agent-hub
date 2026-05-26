import React, { useState, useEffect } from 'react';
import { Outlet } from 'react-router-dom';
import { Button, Alert } from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import { ConversationList } from '@/components/sidebar/ConversationList';
import SettingsPanel from '@/components/settings/SettingsPanel';
import FriendList from '@/components/friends/FriendList';
import FriendRequest from '@/components/friends/FriendRequest';
import GroupCreateModal from '@/components/groups/GroupCreateModal';
import { createGroup } from '@/api/group';
import { useConversationStore } from '@/store/conversationStore';
import { useConversation } from '@/hooks/useConversation';
import { useWebSocket } from '@/hooks/useWebSocket';
import { useAuth } from '@/hooks/useAuth';
import { useMessageStore } from '@/store/messageStore';
import { message as antMessage } from 'antd';
import styles from './AppLayout.module.css';

const AppLayout: React.FC = () => {
  const { create } = useConversation();
  const fetchConversations = useConversationStore((s) => s.fetchConversations);
  const { status } = useWebSocket();
  const { user, logout: handleLogout } = useAuth();
  const [activeNav, setActiveNav] = useState('chat');
  const [groupModalOpen, setGroupModalOpen] = useState(false);

  // Update document.title with total unread count
  const unreadCounts = useMessageStore((s) => s.unreadCounts);
  const totalUnread = Object.values(unreadCounts).reduce((sum, c) => sum + c, 0);

  useEffect(() => {
    if (totalUnread > 0) {
      document.title = `(${totalUnread}) AgentHub`;
    } else {
      document.title = 'AgentHub';
    }
  }, [totalUnread]);

  const handleCreate = async () => {
    await create('single', `新对话`);
  };

  const handleGroupCreate = async (name: string, memberIds: string[]) => {
    try {
      await createGroup({ name, member_ids: memberIds });
      antMessage.success('群聊创建成功');
      setGroupModalOpen(false);
      await fetchConversations();
    } catch {
      antMessage.error('创建群聊失败');
    }
  };

  /** 中间面板内容：根据左侧导航切换 */
  const renderMiddlePanel = () => {
    if (activeNav === 'friends') {
      return (
        <>
          <div className={styles.convPanelHeader}>
            <span className={styles.convPanelTitle}>好友</span>
          </div>
          <div style={{ padding: 12, overflow: 'auto', flex: 1 }}>
            <FriendRequest />
            <div style={{ marginTop: 16, borderTop: '1px solid var(--color-border)', paddingTop: 16 }}>
              <FriendList onStartChat={() => setActiveNav('chat')} />
            </div>
          </div>
        </>
      );
    }

    if (activeNav === 'groups') {
      return (
        <>
          <div className={styles.convPanelHeader}>
            <span className={styles.convPanelTitle}>群聊</span>
            <Button
              type="primary"
              icon={<PlusOutlined />}
              onClick={() => setGroupModalOpen(true)}
            >
              新建群聊
            </Button>
          </div>
          <div style={{ padding: 16, color: 'var(--color-text-secondary)' }}>
            暂无群聊
          </div>
        </>
      );
    }

    // 默认：对话列表
    return (
      <>
        <div className={styles.convPanelHeader}>
          <span className={styles.convPanelTitle}>对话</span>
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={handleCreate}
          >
            新建对话
          </Button>
        </div>
        <ConversationList />
      </>
    );
  };

  return (
    <div className={styles.container}>
      {/* WebSocket disconnect alert */}
      {status === 'disconnected' && (
        <Alert
          message="连接已断开，正在重连..."
          type="warning"
          showIcon
          banner
          style={{ position: 'fixed', top: 0, left: 0, right: 0, zIndex: 1000 }}
        />
      )}

      {/* 左侧：设置面板 */}
      <div className={styles.settingsPanel}>
        <SettingsPanel
          username={user?.username ?? ''}
          onLogout={handleLogout}
          wsStatus={status}
          onNavChange={setActiveNav}
        />
      </div>

      {/* 中间：对话/好友/群聊列表 */}
      <div className={styles.convPanel}>
        {renderMiddlePanel()}
      </div>

      {/* 右侧：聊天区域 */}
      <div className={styles.chatPanel}>
        <Outlet />
      </div>

      {/* 群聊创建弹窗 */}
      <GroupCreateModal
        open={groupModalOpen}
        onCancel={() => setGroupModalOpen(false)}
        onOk={handleGroupCreate}
      />
    </div>
  );
};

export default AppLayout;
