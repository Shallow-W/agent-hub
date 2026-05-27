import React, { useState, useEffect, useCallback } from 'react';
import { Outlet } from 'react-router-dom';
import { Button, Alert } from 'antd';
import { PlusOutlined, LeftOutlined, RightOutlined } from '@ant-design/icons';
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
import { useFriendStore } from '@/store/friendStore';
import { getOrCreatePrivateChat } from '@/api/conversation';
import { message as antMessage } from 'antd';
import ResizeHandle from '@/components/common/ResizeHandle';
import styles from './AppLayout.module.css';

const AppLayout: React.FC = () => {
  const { create, conversations } = useConversation();
  const fetchConversations = useConversationStore((s) => s.fetchConversations);
  const setActive = useConversationStore((s) => s.setActive);
  const { status } = useWebSocket();
  const { user, logout: handleLogout } = useAuth();
  const fetchFriends = useFriendStore((s) => s.fetchFriends);
  const fetchPending = useFriendStore((s) => s.fetchPending);
  const [activeNav, setActiveNav] = useState('chat');
  const [groupModalOpen, setGroupModalOpen] = useState(false);
  const [settingsCollapsed, setSettingsCollapsed] = useState(false);
  const [convPanelWidth, setConvPanelWidth] = useState(300);

  // 切换到好友页时自动拉取数据
  useEffect(() => {
    if (activeNav === 'friends') {
      fetchFriends();
      fetchPending();
    }
  }, [activeNav, fetchFriends, fetchPending]);

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

  /** 拖拽调整中间面板宽度 */
  const handleResize = useCallback((deltaX: number) => {
    setConvPanelWidth((prev) => Math.min(500, Math.max(200, prev + deltaX)));
  }, []);

  /** 点击好友开始私聊 */
  const handleStartChat = useCallback(async (friendId: string) => {
    try {
      const conv = await getOrCreatePrivateChat(friendId);
      await fetchConversations();
      setActive(conv.id);
      setActiveNav('chat');
    } catch {
      antMessage.error('创建私聊失败');
    }
  }, [fetchConversations, setActive]);

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
              <FriendList onStartChat={handleStartChat} />
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
            <Button
              type="primary"
              icon={<PlusOutlined />}
              onClick={() => setGroupModalOpen(true)}
            >
              新建群聊
            </Button>
          </div>
          {groupConvs.length === 0 ? (
            <div style={{ padding: 16, color: 'var(--color-text-secondary)' }}>
              暂无群聊
            </div>
          ) : (
            <div style={{ flex: 1, overflow: 'auto', padding: '8px 0' }}>
              {groupConvs.map((conv) => (
                <div
                  key={conv.id}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    padding: '10px 16px',
                    cursor: 'pointer',
                    transition: 'background-color 0.2s ease',
                  }}
                  onClick={() => {
                    useConversationStore.getState().setActive(conv.id);
                    setActiveNav('chat');
                  }}
                  onMouseEnter={(e) => {
                    (e.currentTarget as HTMLElement).style.backgroundColor = 'var(--color-bg-hover)';
                  }}
                  onMouseLeave={(e) => {
                    (e.currentTarget as HTMLElement).style.backgroundColor = 'transparent';
                  }}
                >
                  <div
                    style={{
                      width: 36,
                      height: 36,
                      borderRadius: 8,
                      background: 'linear-gradient(135deg, #1677ff, #4096ff)',
                      color: '#fff',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      fontSize: 16,
                      fontWeight: 700,
                      flexShrink: 0,
                      marginRight: 12,
                    }}
                  >
                    {conv.title.charAt(0).toUpperCase()}
                  </div>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{
                      fontSize: 14,
                      fontWeight: 500,
                      color: 'var(--color-text)',
                      whiteSpace: 'nowrap',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                    }}>
                      {conv.title}
                    </div>
                    <div style={{ fontSize: 11, color: 'var(--color-text-tertiary)', marginTop: 2 }}>
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
      <div
        className={styles.settingsPanel}
        style={{ width: settingsCollapsed ? 64 : 'var(--settings-width)' }}
      >
        <SettingsPanel
          username={user?.username ?? ''}
          onLogout={handleLogout}
          wsStatus={status}
          onNavChange={setActiveNav}
          collapsed={settingsCollapsed}
        />
      </div>

      {/* 折叠/展开切换按钮 - 放在 settingsPanel 外部，避免 overflow 问题 */}
      <button
        className={styles.toggleBtn}
        onClick={() => setSettingsCollapsed((c) => !c)}
        aria-label={settingsCollapsed ? '展开侧栏' : '折叠侧栏'}
        style={{ left: settingsCollapsed ? 64 - 13 : 220 - 13 }}
      >
        {settingsCollapsed ? <RightOutlined /> : <LeftOutlined />}
      </button>

      {/* 中间：对话/好友/群聊列表 */}
      <div className={styles.convPanel} style={{ width: convPanelWidth, minWidth: 200, maxWidth: 500 }}>
        {renderMiddlePanel()}
      </div>

      {/* 拖拽分隔条 */}
      <ResizeHandle onResize={handleResize} />

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
