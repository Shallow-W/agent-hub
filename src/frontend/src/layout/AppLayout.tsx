import React, { useState } from 'react';
import { Outlet } from 'react-router-dom';
import { Button } from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import { ConversationList } from '@/components/sidebar/ConversationList';
import SettingsPanel from '@/components/settings/SettingsPanel';
import FriendList from '@/components/friends/FriendList';
import FriendRequest from '@/components/friends/FriendRequest';
import GroupCreateModal from '@/components/groups/GroupCreateModal';
import { AgentList } from '@/components/agent/AgentList';
import { AgentProfile } from '@/components/agent/AgentProfile';
import { useConversation } from '@/hooks/useConversation';
import { useWebSocket } from '@/hooks/useWebSocket';
import { useAuth } from '@/hooks/useAuth';
import { useAgents } from '@/hooks/useAgents';
import type { Agent } from '@/types/agent';
import styles from './AppLayout.module.css';

const AppLayout: React.FC = () => {
  const { create } = useConversation();
  const { status } = useWebSocket();
  const { user, logout: handleLogout } = useAuth();
  const { agents } = useAgents();
  const [activeNav, setActiveNav] = useState('chat');
  const [groupModalOpen, setGroupModalOpen] = useState(false);
  const [selectedAgentId, setSelectedAgentId] = useState<string | null>(null);

  const handleCreate = async () => {
    await create('single', `新对话`);
  };

  const handleGroupCreate = (name: string, memberIds: string[]) => {
    // TODO: 调用后端创建群聊 API
    console.log('创建群聊:', name, memberIds);
    setGroupModalOpen(false);
  };

  const selectedAgent =
    agents.find((agent) => agent.id === selectedAgentId) ?? agents[0] ?? null;

  const handleSelectAgent = (agent: Agent) => {
    setSelectedAgentId(agent.id);
  };

  /** 中间面板内容：根据左侧导航切换 */
  const renderMiddlePanel = () => {
    if (activeNav === 'friends') {
      return (
        <>
          <div className={styles.convPanelHeader}>
            <span className={styles.convPanelTitle}>好友</span>
          </div>
          <div className={styles.middleContent}>
            <FriendRequest />
            <div className={styles.friendListSection}>
              <FriendList onStartChat={() => setActiveNav('chat')} />
            </div>
          </div>
        </>
      );
    }

    if (activeNav === 'agents') {
      return (
        <>
          <div className={styles.convPanelHeader}>
            <span className={styles.convPanelTitle}>Agent</span>
          </div>
          <AgentList
            selectedAgentId={selectedAgent?.id ?? null}
            onSelect={handleSelectAgent}
          />
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
          <div className={styles.emptyPanel}>
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
        {activeNav === 'agents' ? (
          <AgentProfile agent={selectedAgent} />
        ) : (
          <Outlet />
        )}
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
