import React, { useCallback } from 'react';
import { Button } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { AgentList } from '@/components/agent/AgentList';
import { AgentProfile } from '@/components/agent/AgentProfile';
import { ComputerProfile } from '@/components/agent/ComputerProfile';
import { useUIStore } from '@/store/uiStore';
import { useAgentStore } from '@/store/agentStore';
import { useConversationStore } from '@/store/conversationStore';
import { getOrCreateAgentChat } from '@/api/conversation';
import { useAuthStore } from '@/store/authStore';
import type { Agent } from '@/types/agent';
import styles from '@/layout/AppLayout.module.css';

const AgentsView: React.FC = () => {
  const navigate = useNavigate();
  const selectedAgentId = useUIStore((s) => s.selectedAgentId);
  const selectedMachineId = useUIStore((s) => s.selectedMachineId);
  const setSelectedAgent = useUIStore((s) => s.setSelectedAgent);
  const setSelectedMachine = useUIStore((s) => s.setSelectedMachine);
  const agents = useAgentStore((s) => s.agents);
  const fetchAgents = useAgentStore((s) => s.fetchAgents);
  const fetchDaemonMachines = useAgentStore((s) => s.fetchDaemonMachines);
  const conversations = useConversationStore((s) => s.conversations);
  const setActive = useConversationStore((s) => s.setActive);
  const fetchConversations = useConversationStore((s) => s.fetchConversations);
  const userID = useAuthStore((s) => s.user?.id);

  const selectedAgent = agents.find((a) => a.id === selectedAgentId) ?? null;

  const handleSelectAgent = useCallback((agent: Agent) => {
    setSelectedAgent(agent.id);
  }, [setSelectedAgent]);

  const handleStartAgentChat = useCallback(async (agent: Agent) => {
    const existing = conversations.find(
      (c) => c.type === 'agent' && c.peer_id === agent.id,
    );
    if (existing) {
      setActive(existing.id);
    } else {
      const conv = await getOrCreateAgentChat(agent.id);
      await fetchConversations();
      setActive(conv.id);
    }
    navigate('/');
  }, [conversations, setActive, fetchConversations, navigate, userID]);

  const handleRefresh = useCallback(async () => {
    await Promise.all([fetchAgents(true), fetchDaemonMachines(true)]);
  }, [fetchAgents, fetchDaemonMachines]);

  return (
    <>
      <div className={styles.convPanelHeader}>
        <span className={styles.convPanelTitle}>智能体</span>
        <div className={styles.convPanelTools}>
          <Button type="text" icon={<ReloadOutlined />} aria-label="刷新" onClick={handleRefresh} />
        </div>
      </div>
      <div className={styles.middleScroll}>
        <AgentList
          selectedAgentId={selectedAgentId}
          selectedMachineId={selectedMachineId}
          onSelect={handleSelectAgent}
          onSelectMachine={setSelectedMachine}
        />
      </div>
      {/* 右侧面板 */}
      {selectedAgent ? (
        <AgentProfile agent={selectedAgent} onMessage={handleStartAgentChat} />
      ) : (
        <ComputerProfile
          machineId={selectedMachineId}
          selectedAgentId={selectedAgentId}
          onSelectAgent={handleSelectAgent}
          onClearSelection={() => setSelectedMachine(null)}
        />
      )}
    </>
  );
};

export default AgentsView;
