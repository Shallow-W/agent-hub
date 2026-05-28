import React, { useState } from 'react';
import { Outlet } from 'react-router-dom';
import { Button } from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import { ConversationList } from '@/components/sidebar/ConversationList';
import SettingsPanel from '@/components/settings/SettingsPanel';
import FriendList from '@/components/friends/FriendList';
import FriendRequest from '@/components/friends/FriendRequest';
import { RobotFriendList } from '@/components/friends/RobotFriendList';
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
  const { create, addConversationAgent } = useConversation();
  const { status } = useWebSocket();
  const { user, logout: handleLogout } = useAuth();
  const { agents } = useAgents();
  const [activeNav, setActiveNav] = useState('chat');
  const [groupModalOpen, setGroupModalOpen] = useState(false);
  const [selectedAgentId, setSelectedAgentId] = useState<string | null>(null);

  const handleCreate = async () => {
    const time = new Date().toLocaleTimeString('zh-CN', {
      hour: '2-digit',
      minute: '2-digit',
    });
    await create('single', `新对话 ${time}`);
    setActiveNav('chat');
  };

  const handleStartRobotChat = async (agent: Agent) => {
    const conv = await create('single', agent.name);
    await addConversationAgent(conv.id, agent.id);
    setActiveNav('chat');
  };

  const handleGroupCreate = (name: string, memberIds: string[]) => {
    console.log('创建群聊:', name, memberIds);
    setGroupModalOpen(false);
  };

  const selectedAgent =
    agents.find((agent) => agent.id === selectedAgentId) ?? agents[0] ?? null;

  const handleSelectAgent = (agent: Agent) => {
    setSelectedAgentId(agent.id);
  };

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
              <div className={styles.sectionTitle}>用户好友</div>
              <FriendList onStartChat={() => setActiveNav('chat')} />
            </div>
            <div className={styles.friendListSection}>
              <div className={styles.sectionTitle}>Robot 好友</div>
              <RobotFriendList agents={agents} onStartChat={handleStartRobotChat} />
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
      <div className={styles.settingsPanel}>
        <SettingsPanel
          username={user?.username ?? ''}
          onLogout={handleLogout}
          wsStatus={status}
          onNavChange={setActiveNav}
        />
      </div>

      <div className={styles.convPanel}>
        {renderMiddlePanel()}
      </div>

      <div className={styles.chatPanel}>
        {activeNav === 'agents' ? (
          <AgentProfile agent={selectedAgent} />
        ) : (
          <Outlet />
        )}
      </div>

      <GroupCreateModal
        open={groupModalOpen}
        onCancel={() => setGroupModalOpen(false)}
        onOk={handleGroupCreate}
      />
    </div>
  );
};

export default AppLayout;
