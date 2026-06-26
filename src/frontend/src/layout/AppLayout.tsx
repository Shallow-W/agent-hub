import React, { useState, useEffect, useRef } from 'react';
import { Outlet, useNavigate } from 'react-router-dom';
import { Alert } from 'antd';
import { message as antMessage } from '@/utils/message';
import { LeftOutlined, RightOutlined } from '@ant-design/icons';
import SettingsPanel from '@/components/settings/SettingsPanel';
import GroupCreateModal from '@/components/groups/GroupCreateModal';
import { createGroup } from '@/api/group';
import { addConversationAgent } from '@/api/conversation';
import { useConversationStore } from '@/store/conversationStore';
import { useConversation } from '@/hooks/useConversation';
import { useWebSocket } from '@/hooks/useWebSocket';
import { useAuth } from '@/hooks/useAuth';
import { useMessageStore } from '@/store/messageStore';
import { useAppBootstrap } from '@/hooks/useAppBootstrap';
import NewConversationModal from './NewConversationModal';
import TitleBar from '@/components/common/TitleBar';
import styles from './AppLayout.module.css';

/**
 * AppLayout — 轻量 layout shell。
 *
 * 职责：侧边栏导航 + 全局 modals + WS 连接 + bootstrap。
 * 不再管理 activeNav / display:none 切换——每个视图通过 React Router
 * 独立路由渲染，自行管理左面板和右面板。
 */
const AppLayout: React.FC = () => {
  useAppBootstrap();

  const navigate = useNavigate();
  const { create } = useConversation();
  const fetchConversations = useConversationStore((s) => s.fetchConversations);
  const setActive = useConversationStore((s) => s.setActive);
  const { status } = useWebSocket();
  const wasConnectedRef = useRef(false);
  useEffect(() => {
    if (status === 'connected') wasConnectedRef.current = true;
  }, [status]);
  const showDisconnectAlert = status === 'disconnected' && wasConnectedRef.current;
  const { user, logout: handleLogout } = useAuth();

  const [groupModalOpen, setGroupModalOpen] = useState(false);
  const [settingsCollapsed, setSettingsCollapsed] = useState(true);
  const [newConvModalOpen, setNewConvModalOpen] = useState(false);

  const totalUnread = useMessageStore((s) =>
    Object.values(s.unreadCounts).reduce((sum, c) => sum + c, 0),
  );

  useEffect(() => {
    document.title = totalUnread > 0 ? `(${totalUnread}) AgentHub` : 'AgentHub';
  }, [totalUnread]);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement).tagName;
      const isInput = tag === 'INPUT' || tag === 'TEXTAREA' || (e.target as HTMLElement).isContentEditable;
      if (e.key === 'Escape') {
        if (newConvModalOpen) { setNewConvModalOpen(false); return; }
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
        setNewConvModalOpen(true);
        return;
      }
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [groupModalOpen, newConvModalOpen]);

  const handleGroupCreate = async (name: string, memberIds: string[]) => {
    try {
      const conv = await createGroup({ name, member_ids: memberIds });
      antMessage.success('群聊创建成功');
      setGroupModalOpen(false);
      await fetchConversations();
      if (conv?.id) {
        setActive(conv.id);
        useConversationStore.getState().setMemberPanelOpen(true);
        navigate('/');
      }
    } catch {
      antMessage.error('创建群聊失败');
    }
  };

  return (
    <div className={styles.container}>
      <TitleBar />

      <div className={styles.contentRow}>
        {showDisconnectAlert && (
          <Alert
            message="连接已断开，正在重连..."
            type="warning"
            showIcon
            banner
            className={styles.disconnectAlert}
          />
        )}

        <div
          className={`${styles.settingsPanel} ${settingsCollapsed ? styles.settingsPanelCollapsed : ''}`}
        >
          <SettingsPanel
            username={user?.username ?? ''}
            onLogout={handleLogout}
            wsStatus={status}
            onCreate={() => setNewConvModalOpen(true)}
            collapsed={settingsCollapsed}
          />
        </div>

        <button
          className={`${styles.toggleBtn} ${
            settingsCollapsed ? styles.toggleBtnCollapsed : styles.toggleBtnExpanded
          }`}
          onClick={() => setSettingsCollapsed((c) => !c)}
          aria-label={settingsCollapsed ? '展开侧栏' : '折叠侧栏'}
        >
          {settingsCollapsed ? <RightOutlined /> : <LeftOutlined />}
        </button>

        {/* 右侧：React Router Outlet 渲染当前路由对应的视图。
            各视图自带 chatPanel 容器，这里只需 flex:1 占满剩余空间。 */}
        <div style={{ flex: 1, display: 'flex', overflow: 'hidden', height: '100%' }}>
          <Outlet />
        </div>
      </div>

      <GroupCreateModal
        open={groupModalOpen}
        onCancel={() => setGroupModalOpen(false)}
        onOk={handleGroupCreate}
      />
      <NewConversationModal
        open={newConvModalOpen}
        onCancel={() => setNewConvModalOpen(false)}
        onCreate={async (title, memberIds, agentIds) => {
          try {
            if (memberIds.length > 0 || agentIds.length > 0) {
              const conv = await createGroup({ name: title, member_ids: memberIds });
              const results = await Promise.allSettled(
                agentIds.map((agentId) => addConversationAgent(conv.id, agentId)),
              );
              const failed = results.filter((result) => result.status === 'rejected').length;
              await fetchConversations();
              setActive(conv.id);
              navigate('/');
              useConversationStore.getState().setMemberPanelOpen(true);
              if (failed > 0) {
                antMessage.warning(`对话已创建，${failed} 个智能体拉入失败`);
              }
            } else {
              await create('single', title);
              navigate('/');
            }
            setNewConvModalOpen(false);
          } catch {
            antMessage.error('创建对话失败');
            throw new Error('创建对话失败');
          }
        }}
      />
    </div>
  );
};

export default AppLayout;
