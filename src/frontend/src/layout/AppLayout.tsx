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
import {
  addConversationAgent,
  getOrCreateAgentChat,
  getOrCreatePrivateChat,
} from '@/api/conversation';
import { useConversationStore } from '@/store/conversationStore';
import { useConversation } from '@/hooks/useConversation';
import { useWebSocket } from '@/hooks/useWebSocket';
import { useAuth } from '@/hooks/useAuth';
import { useMessageStore } from '@/store/messageStore';
import { useFriendStore } from '@/store/friendStore';
import { useAgentStore } from '@/store/agentStore';
import { useAppBootstrap } from '@/hooks/useAppBootstrap';
import MiddlePanel from './MiddlePanel';
import NewConversationModal from './NewConversationModal';
import { AgentProfile } from '@/components/agent/AgentProfile';
import { AgentSkillsPanel } from '@/components/agent/AgentSkillsPanel';
import { ComputerProfile } from '@/components/agent/ComputerProfile';
import KnowledgeFilePreview from '@/components/knowledge/KnowledgeFilePreview';
import type { Agent } from '@/types/agent';
import type { KnowledgeFile } from '@/types/knowledge';
import styles from './AppLayout.module.css';

const AppLayout: React.FC = () => {
  useAppBootstrap();

  const navigate = useNavigate();
  const location = useLocation();
  const { create, conversations } = useConversation();
  const fetchConversations = useConversationStore((s) => s.fetchConversations);
  const setActive = useConversationStore((s) => s.setActive);
  const bindDirectAgentChat = useConversationStore((s) => s.bindDirectAgentChat);
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
  const [settingsCollapsed, setSettingsCollapsed] = useState(true);
  const [newConvModalOpen, setNewConvModalOpen] = useState(false);
  const [selectedAgentId, setSelectedAgentId] = useState<string | null>(null);
  const [selectedMachineId, setSelectedMachineId] = useState<string | null>(null);
  const [selectedKnowledgeFile, setSelectedKnowledgeFile] = useState<KnowledgeFile | null>(null);
  const [selectedKbId, setSelectedKbId] = useState<string | null>(null);
  const creatingAgentChatRef = useRef<string | null>(null);

  const handleNavChange = useCallback((key: string) => {
    setActiveNav(key);
    // 切换面板时清除知识库文件选中
    if (key !== 'knowledge') {
      setSelectedKnowledgeFile(null);
      setSelectedKbId(null);
    }
    if (key === 'settings') {
      navigate('/settings');
      return;
    }
    if (key === 'workspace') {
      navigate('/tasks');
      return;
    }
    if (location.pathname !== '/') {
      navigate('/');
    }
  }, [navigate, location.pathname]);

  useEffect(() => {
    if (location.pathname.startsWith('/tasks')) {
      setActiveNav('workspace');
      return;
    }
    if (location.pathname.startsWith('/settings')) {
      setActiveNav('settings');
      return;
    }
    if (activeNav === 'workspace' || activeNav === 'settings') {
      setActiveNav('chat');
    }
  }, [activeNav, location.pathname]);

  // 切换到联系人页时自动拉取数据
  useEffect(() => {
    if (activeNav === 'contacts') {
      fetchFriends();
      fetchPending();
    }
  }, [activeNav, fetchFriends, fetchPending]);

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
  }, [groupModalOpen, newConvModalOpen]);

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
      if (conv?.id) {
        setActive(conv.id);
        useConversationStore.getState().setMemberPanelOpen(true);
      }
    } catch {
      antMessage.error('创建群聊失败');
    }
  };

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

  const handleStartAgentChat = useCallback(async (agent: Agent) => {
    if (creatingAgentChatRef.current === agent.id) return;
    creatingAgentChatRef.current = agent.id;
    try {
      const conv = await getOrCreateAgentChat(agent.id);
      bindDirectAgentChat(conv.id, agent.id);
      await fetchConversations();
      setActive(conv.id);
      setActiveNav('chat');
    } catch {
      antMessage.error('创建智能体对话失败');
    } finally {
      creatingAgentChatRef.current = null;
    }
  }, [bindDirectAgentChat, fetchConversations, setActive]);

  const agents = useAgentStore((s) => s.agents);
  const selectedAgent = selectedAgentId ? agents.find((a) => a.id === selectedAgentId) ?? null : null;

  const handleSelectAgent = useCallback((agent: Agent) => {
    setSelectedAgentId(agent.id);
    if (agent.machine_id) {
      setSelectedMachineId(agent.machine_id);
    }
  }, []);

  const handleSelectMachine = useCallback((machineId: string) => {
    setSelectedMachineId(machineId);
    setSelectedAgentId(null);
  }, []);

  const handleKnowledgeFileSelect = useCallback((file: KnowledgeFile, kbId: string) => {
    setSelectedKnowledgeFile(file);
    setSelectedKbId(kbId);
  }, []);

  return (
    <div className={styles.container}>
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
          onNavChange={handleNavChange}
          activeKey={activeNav}
          onCreate={handleCreate}
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

      <div className={styles.convPanel}>
        <MiddlePanel
          activeNav={activeNav}
          conversations={conversations}
          onCreate={handleCreate}
          onCreateGroup={() => setGroupModalOpen(true)}
          onRefresh={() => fetchConversations()}
          onUpload={handleUpload}
          onStartChat={handleStartChat}
          onStartAgentChat={handleStartAgentChat}
          onSwitchChat={() => setActiveNav('chat')}
          onSwitchContacts={() => setActiveNav('contacts')}
          onRefreshContacts={handleRefreshContacts}
          selectedAgentId={selectedAgentId}
          selectedMachineId={selectedMachineId}
          onSelectAgent={handleSelectAgent}
          onSelectMachine={handleSelectMachine}
          onKnowledgeFileSelect={handleKnowledgeFileSelect}
          selectedFileId={selectedKnowledgeFile?.id ?? null}
          selectedKbId={selectedKbId}
        />
      </div>

      {/* 右侧：聊天区域 / 智能体详情 */}
      <div className={`${styles.chatPanel} ${activeNav === 'workspace' ? styles.taskPanel : ''}`}>
        {activeNav === 'knowledge' ? (
          selectedKnowledgeFile && selectedKbId ? (
            <KnowledgeFilePreview file={selectedKnowledgeFile} kbId={selectedKbId} />
          ) : (
            <div className={styles.emptyRightPanel}>
              <div className={styles.emptyRightIcon}>📚</div>
              <div className={styles.emptyRightTitle}>知识库管理</div>
              <div className={styles.emptyRightDesc}>在左侧面板中管理你的知识库和文件</div>
            </div>
          )
        ) : activeNav === 'skills' ? (
          selectedAgent ? (
            <AgentSkillsPanel agent={selectedAgent} />
          ) : (
            <div className={styles.skillsEmptyPanel}>
              <div className={styles.emptyRightTitle}>选择一个 Agent 管理技能</div>
              <div className={styles.emptyRightDesc}>左侧会展示每个 Agent 的已分配 Skills 和底座 Skills 数量</div>
            </div>
          )
        ) : activeNav === 'models' ? (
          selectedAgent ? (
            <AgentProfile agent={selectedAgent} />
          ) : (
            <ComputerProfile
              machineId={selectedMachineId}
              selectedAgentId={selectedAgentId}
              onSelectAgent={handleSelectAgent}
              onClearSelection={() => setSelectedMachineId(null)}
            />
          )
        ) : (
          <Outlet />
        )}
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
              setActiveNav('chat');
              if (location.pathname !== '/') {
                navigate('/');
              }
              useConversationStore.getState().setMemberPanelOpen(true);
              if (failed > 0) {
                antMessage.warning(`对话已创建，${failed} 个智能体拉入失败`);
              }
            } else {
              await create('single', title);
              setActiveNav('chat');
              if (location.pathname !== '/') {
                navigate('/');
              }
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
