import React, { useEffect } from 'react';
import { Avatar, Button } from 'antd';
import { PlusOutlined, ReloadOutlined, RobotOutlined, UploadOutlined } from '@ant-design/icons';
import { ConversationList } from '@/components/sidebar/ConversationList';
import ContactsPanel from '@/components/contacts/ContactsPanel';
import { AgentList } from '@/components/agent/AgentList';
import KnowledgePanel from '@/components/knowledge/KnowledgePanel';
import type { KnowledgeFile } from '@/types/knowledge';
import { useAgentStore } from '@/store/agentStore';
import { parseSkills } from '@/components/agent/agentPresentation';
import type { Conversation } from '@/types/conversation';
import type { Agent } from '@/types/agent';
import styles from './AppLayout.module.css';
import skillStyles from './SkillsAgentList.module.css';

interface MiddlePanelProps {
  activeNav: string;
  conversations: Conversation[];
  onCreate: () => void;
  onCreateGroup: () => void;
  onRefresh: () => void;
  onUpload: () => void;
  onShowArchived: () => void;
  onStartChat: (friendId: string) => void;
  onStartAgentChat: (agent: Agent) => void;
  onSwitchChat: () => void;
  onSwitchContacts: () => void;
  onRefreshContacts: () => void;
  selectedAgentId: string | null;
  selectedMachineId: string | null;
  onSelectAgent: (agent: Agent) => void;
  onSelectMachine: (machineId: string) => void;
  onKnowledgeFileSelect?: (file: KnowledgeFile, kbId: string) => void;
  selectedKbId?: string | null;
  selectedFileId?: string | null;
}

const MiddlePanel: React.FC<MiddlePanelProps> = ({
  activeNav,
  conversations,
  onCreate,
  onCreateGroup,
  onRefresh,
  onUpload,
  onShowArchived,
  onStartChat,
  onStartAgentChat,
  onSwitchChat,
  onSwitchContacts,
  onRefreshContacts,
  selectedAgentId,
  selectedMachineId,
  onSelectAgent,
  onSelectMachine,
  onKnowledgeFileSelect,
  selectedKbId,
  selectedFileId,
}) => {
  const renderPanelTools = (onAdd: () => void) => (
    <div className={styles.convPanelTools}>
      <Button type="text" icon={<UploadOutlined />} aria-label="上传" onClick={onUpload} />
      <Button type="text" icon={<PlusOutlined />} aria-label="添加" onClick={onAdd} />
      <Button type="text" icon={<ReloadOutlined />} aria-label="刷新" onClick={onRefresh} />
    </div>
  );

  if (activeNav === 'skills') {
    return <SkillsAgentList selectedAgentId={selectedAgentId} onSelectAgent={onSelectAgent} />;
  }

  if (activeNav === 'models') {
    return (
      <>
        <div className={styles.convPanelHeader}>
          <span className={styles.convPanelTitle}>智能体</span>
        </div>
        <div className={styles.middleScroll}>
          <AgentList
            selectedAgentId={selectedAgentId}
            selectedMachineId={selectedMachineId}
            onSelect={onSelectAgent}
            onSelectMachine={onSelectMachine}
          />
        </div>
      </>
    );
  }

  if (activeNav === 'knowledge') {
    return (
      <KnowledgePanel
        onFileSelect={onKnowledgeFileSelect}
        selectedFileId={selectedFileId}
        selectedKbId={selectedKbId}
      />
    );
  }

  if (activeNav === 'contacts') {
    return (
      <>
        <div className={styles.convPanelHeader}>
          <span className={styles.convPanelTitle}>联系人</span>
          <div className={styles.convPanelTools}>
            <Button type="text" icon={<PlusOutlined />} aria-label="新建群聊" onClick={onCreateGroup} />
            <Button type="text" icon={<ReloadOutlined />} aria-label="刷新" onClick={onRefreshContacts} />
          </div>
        </div>
        <ContactsPanel
          conversations={conversations}
          onStartChat={onStartChat}
          onStartAgentChat={onStartAgentChat}
          onSwitchChat={onSwitchChat}
        />
      </>
    );
  }

  return (
    <>
      <div className={styles.convPanelHeader}>
        <span className={styles.convPanelTitle}>消息</span>
        {renderPanelTools(onCreate)}
      </div>
      <ConversationList onNavigateContacts={onSwitchContacts} />
      <button className={styles.archivedLink} onClick={onShowArchived} type="button">
        查看归档对话
      </button>
    </>
  );
};

interface SkillsAgentListProps {
  selectedAgentId: string | null;
  onSelectAgent: (agent: Agent) => void;
}

const SkillsAgentList: React.FC<SkillsAgentListProps> = ({ selectedAgentId, onSelectAgent }) => {
  const agents = useAgentStore((s) => s.agents);
  const fetchAgents = useAgentStore((s) => s.fetchAgents);

  useEffect(() => {
    fetchAgents().catch(() => {});
  }, [fetchAgents]);

  return (
    <>
      <div className={styles.convPanelHeader}>
        <span className={styles.convPanelTitle}>技能</span>
      </div>
      <div className={styles.middleScroll}>
        <div className={skillStyles.grid}>
          {agents.length === 0 && (
            <div className={skillStyles.empty}>暂无 Agent</div>
          )}
          {agents.map((agent) => {
            const skillCount = parseSkills(agent.capabilities_json).length;
            const isSelected = agent.id === selectedAgentId;
            return (
              <button
                key={agent.id}
                className={`${skillStyles.card} ${isSelected ? skillStyles.cardActive : ''}`}
                type="button"
                onClick={() => onSelectAgent(agent)}
              >
                <Avatar size={36} src={agent.avatar || undefined} icon={<RobotOutlined />} className={skillStyles.avatar} />
                <div className={skillStyles.info}>
                  <span className={skillStyles.name}>{agent.name}</span>
                  <span className={skillStyles.meta}>{skillCount} skills</span>
                </div>
              </button>
            );
          })}
        </div>
      </div>
    </>
  );
};

export default MiddlePanel;
