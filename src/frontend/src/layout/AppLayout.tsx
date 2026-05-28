import React, { useState, useEffect, useCallback } from 'react';
import { Outlet, useNavigate, useLocation } from 'react-router-dom';
import { Button, Alert, Avatar } from 'antd';
import {
  PlusOutlined,
  LeftOutlined,
  RightOutlined,
  ReloadOutlined,
  UploadOutlined,
} from '@ant-design/icons';
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

const SETTINGS_COLLAPSED_WIDTH = 44;
const SETTINGS_EXPANDED_WIDTH = 184;

const AppLayout: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const { create, conversations } = useConversation();
  const fetchConversations = useConversationStore((s) => s.fetchConversations);
  const setActive = useConversationStore((s) => s.setActive);
  const { status } = useWebSocket();
  const { user, logout: handleLogout } = useAuth();
  const fetchFriends = useFriendStore((s) => s.fetchFriends);
  const fetchPending = useFriendStore((s) => s.fetchPending);
  const [activeNav, setActiveNav] = useState('chat');
  const [groupModalOpen, setGroupModalOpen] = useState(false);
  const [settingsCollapsed, setSettingsCollapsed] = useState(true);
  const [convPanelWidth, setConvPanelWidth] = useState(166);

  /** 导航切换：设置页使用路由跳转 */
  const handleNavChange = useCallback((key: string) => {
    if (key === 'settings') {
      navigate('/settings');
      return;
    }
    setActiveNav(key);
    if (location.pathname !== '/') {
      navigate('/');
    }
  }, [navigate, location.pathname]);

  // 切换到好友页时自动拉取数据
  useEffect(() => {
    if (activeNav === 'friends') {
      fetchFriends();
      fetchPending();
    }
  }, [activeNav, fetchFriends, fetchPending]);

  // Derive total unread count without subscribing to the full unreadCounts object.
  // This avoids re-rendering AppLayout on every individual count change.
  const totalUnread = useMessageStore((s) =>
    Object.values(s.unreadCounts).reduce((sum, c) => sum + c, 0),
  );

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
    setConvPanelWidth((prev) => Math.min(220, Math.max(150, prev + deltaX)));
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
    const renderPanelTools = (onAdd: () => void) => (
      <div className={styles.convPanelTools}>
        <Button type="text" icon={<UploadOutlined />} aria-label="上传" />
        <Button type="text" icon={<PlusOutlined />} aria-label="添加" onClick={onAdd} />
        <Button type="text" icon={<ReloadOutlined />} aria-label="刷新" onClick={() => fetchConversations()} />
      </div>
    );

    if (activeNav === 'friends') {
      return (
        <>
          <div className={styles.convPanelHeader}>
            <span className={styles.convPanelTitle}>消息</span>
            {renderPanelTools(handleCreate)}
          </div>
          <div className={styles.middleScroll}>
            <FriendRequest />
            <div className={styles.middleSection}>
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
            <span className={styles.convPanelTitle}>消息</span>
            {renderPanelTools(() => setGroupModalOpen(true))}
          </div>
          {groupConvs.length === 0 ? (
            <div className={styles.emptyState}>
              暂无群聊
            </div>
          ) : (
            <div className={styles.groupList}>
              {groupConvs.map((conv) => (
                <div
                  key={conv.id}
                  className={styles.groupItem}
                  onClick={() => {
                    useConversationStore.getState().setActive(conv.id);
                    setActiveNav('chat');
                  }}
                >
                  <Avatar className={styles.groupAvatar} shape="square">
                    {conv.title.charAt(0).toUpperCase()}
                  </Avatar>
                  <div className={styles.groupMeta}>
                    <div className={styles.groupTitle}>
                      {conv.title}
                    </div>
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

    // 默认：对话列表
    return (
      <>
        <div className={styles.convPanelHeader}>
          <span className={styles.convPanelTitle}>消息</span>
          {renderPanelTools(handleCreate)}
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
        className={`${styles.settingsPanel} ${settingsCollapsed ? styles.settingsPanelCollapsed : ''}`}
      >
        <SettingsPanel
          username={user?.username ?? ''}
          onLogout={handleLogout}
          wsStatus={status}
          onNavChange={handleNavChange}
          collapsed={settingsCollapsed}
        />
      </div>

      {/* 折叠/展开切换按钮 - 放在 settingsPanel 外部，避免 overflow 问题 */}
      <button
        className={styles.toggleBtn}
        onClick={() => setSettingsCollapsed((c) => !c)}
        aria-label={settingsCollapsed ? '展开侧栏' : '折叠侧栏'}
        style={{
          left: (settingsCollapsed ? SETTINGS_COLLAPSED_WIDTH : SETTINGS_EXPANDED_WIDTH) - 11,
        }}
      >
        {settingsCollapsed ? <RightOutlined /> : <LeftOutlined />}
      </button>

      {/* 中间：对话/好友/群聊列表 */}
      <div className={styles.convPanel} style={{ width: convPanelWidth }}>
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
