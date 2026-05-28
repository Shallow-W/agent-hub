import React, { useState, useEffect, useCallback, useRef } from 'react';
import { Outlet, useNavigate, useLocation } from 'react-router-dom';
import { Alert, message as antMessage } from 'antd';
import {
  LeftOutlined,
  RightOutlined,
} from '@ant-design/icons';
import SettingsPanel from '@/components/settings/SettingsPanel';
import GroupCreateModal from '@/components/groups/GroupCreateModal';
import { createGroup } from '@/api/group';
import { getArchivedConversations, unarchiveConversation } from '@/api/conversation';
import { useConversationStore } from '@/store/conversationStore';
import { useConversation } from '@/hooks/useConversation';
import type { Conversation } from '@/types/conversation';
import { useWebSocket } from '@/hooks/useWebSocket';
import { useAuth } from '@/hooks/useAuth';
import { useMessageStore } from '@/store/messageStore';
import { useFriendStore } from '@/store/friendStore';
import { getOrCreatePrivateChat } from '@/api/conversation';
import ArchivedConversationsModal from './ArchivedConversationsModal';
import MiddlePanel from './MiddlePanel';
import NewConversationModal from './NewConversationModal';
import styles from './AppLayout.module.css';

const AppLayout: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const { create, conversations } = useConversation();
  const fetchConversations = useConversationStore((s) => s.fetchConversations);
  const setActive = useConversationStore((s) => s.setActive);
  const { status } = useWebSocket();
  const wasConnectedRef = useRef(false);
  useEffect(() => {
    if (status === 'connected') wasConnectedRef.current = true;
  }, [status]);
  const showDisconnectAlert = status === 'disconnected' && wasConnectedRef.current;
  const { user, logout: handleLogout } = useAuth();
  const fetchFriends = useFriendStore((s) => s.fetchFriends);
  const fetchPending = useFriendStore((s) => s.fetchPending);
  const [activeNav, setActiveNav] = useState('chat');
  const [groupModalOpen, setGroupModalOpen] = useState(false);
  const [archivedModalOpen, setArchivedModalOpen] = useState(false);
  const [archivedConvs, setArchivedConvs] = useState<Conversation[]>([]);
  const [settingsCollapsed, setSettingsCollapsed] = useState(true);
  const [newConvModalOpen, setNewConvModalOpen] = useState(false);

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

  // 切换到联系人页时自动拉取数据
  useEffect(() => {
    if (activeNav === 'contacts') {
      fetchFriends();
      fetchPending();
    }
  }, [activeNav, fetchFriends, fetchPending]);

  // 只订阅未读总数，避免每个会话未读变化都触发布局重渲染。
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

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement).tagName;
      const isInput = tag === 'INPUT' || tag === 'TEXTAREA' || (e.target as HTMLElement).isContentEditable;

      if (e.key === 'Escape') {
        const memberPanelOpen = useConversationStore.getState().memberPanelOpen;
        if (memberPanelOpen) {
          useConversationStore.getState().setMemberPanelOpen(false);
          return;
        }
        if (newConvModalOpen) { setNewConvModalOpen(false); return; }
        if (archivedModalOpen) { setArchivedModalOpen(false); return; }
        if (groupModalOpen) { setGroupModalOpen(false); return; }
        return;
      }

      if (isInput) return;

      const mod = e.metaKey || e.ctrlKey;

      if (mod && e.key === 'k') {
        e.preventDefault();
        document.querySelector<HTMLInputElement>('[data-conv-search] input')?.focus();
        return;
      }

      if (mod && e.key === 'n') {
        e.preventDefault();
        handleCreate();
        return;
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [archivedModalOpen, groupModalOpen, newConvModalOpen]);

  const showArchived = async () => {
    try {
      const list = await getArchivedConversations();
      setArchivedConvs(list ?? []);
      setArchivedModalOpen(true);
    } catch {
      antMessage.error('获取归档对话失败');
    }
  };

  const handleUnarchive = async (convId: string) => {
    try {
      await unarchiveConversation(convId);
      setArchivedConvs((prev) => prev.filter((c) => c.id !== convId));
      await fetchConversations();
      antMessage.success('已取消归档');
    } catch {
      antMessage.error('取消归档失败');
    }
  };

  const handleCreate = () => {
    setNewConvModalOpen(true);
  };

  const handleRefreshContacts = useCallback(async () => {
    await Promise.all([
      fetchFriends(),
      fetchPending(),
      fetchConversations(),
    ]);
  }, [fetchFriends, fetchPending, fetchConversations]);

  const handleUpload = () => {
    antMessage.info('请在当前对话输入框左侧添加附件');
  };

  const handleGroupCreate = async (name: string, memberIds: string[]) => {
    try {
      const conv = await createGroup({ name, member_ids: memberIds });
      antMessage.success('群聊创建成功');
      setGroupModalOpen(false);
      await fetchConversations();
      // UX-02: 自动激活新创建的群聊
      if (conv?.id) {
        setActive(conv.id);
        // UX-09: 群聊创建后自动打开成员面板
        useConversationStore.getState().setMemberPanelOpen(true);
      }
    } catch {
      antMessage.error('创建群聊失败');
    }
  };

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

  return (
    <div className={styles.container}>
      {/* WebSocket disconnect alert — 仅在连接建立后断开时显示 */}
      {showDisconnectAlert && (
        <Alert
          message="连接已断开，正在重连..."
          type="warning"
          showIcon
          banner
          className={styles.disconnectAlert}
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
          activeKey={activeNav}
          onCreate={handleCreate}
          collapsed={settingsCollapsed}
        />
      </div>

      {/* 折叠/展开切换按钮 - 放在 settingsPanel 外部，避免 overflow 问题 */}
      <button
        className={`${styles.toggleBtn} ${
          settingsCollapsed ? styles.toggleBtnCollapsed : styles.toggleBtnExpanded
        }`}
        onClick={() => setSettingsCollapsed((c) => !c)}
        aria-label={settingsCollapsed ? '展开侧栏' : '折叠侧栏'}
      >
        {settingsCollapsed ? <RightOutlined /> : <LeftOutlined />}
      </button>

      {/* 中间：对话/好友/群聊列表 */}
      <div className={styles.convPanel}>
        <MiddlePanel
          activeNav={activeNav}
          conversations={conversations}
          onCreate={handleCreate}
          onCreateGroup={() => setGroupModalOpen(true)}
          onRefresh={() => fetchConversations()}
          onUpload={handleUpload}
          onShowArchived={showArchived}
          onStartChat={handleStartChat}
          onSwitchChat={() => setActiveNav('chat')}
          onSwitchContacts={() => setActiveNav('contacts')}
          onRefreshContacts={handleRefreshContacts}
        />
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
      <NewConversationModal
        open={newConvModalOpen}
        onCancel={() => setNewConvModalOpen(false)}
        onCreate={async (title) => {
          await create('single', title);
          setNewConvModalOpen(false);
        }}
      />
      <ArchivedConversationsModal
        open={archivedModalOpen}
        onCancel={() => setArchivedModalOpen(false)}
        conversations={archivedConvs}
        onUnarchive={handleUnarchive}
      />
    </div>
  );
};

export default AppLayout;
