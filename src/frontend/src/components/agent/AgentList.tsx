import React, { useState } from 'react';
import { Avatar, Badge, Button, Popconfirm, Spin, Tag } from 'antd';
import {
  DeleteOutlined,
  DesktopOutlined,
  ReloadOutlined,
  RobotOutlined,
} from '@ant-design/icons';
import { useAgents } from '@/hooks/useAgents';
import type { Agent, AgentStatus } from '@/types/agent';
import { ConnectComputerModal } from './ConnectComputerModal';
import { parseCapabilities } from './agentPresentation';
import styles from './AgentList.module.css';

interface AgentListProps {
  selectedAgentId: string | null;
  onSelect: (agent: Agent) => void;
}

const statusColor: Record<AgentStatus, string> = {
  online: 'green',
  offline: 'default',
  busy: 'processing',
  error: 'red',
};

export const AgentList: React.FC<AgentListProps> = ({
  selectedAgentId,
  onSelect,
}) => {
  const [connectOpen, setConnectOpen] = useState(false);
  const {
    agents,
    machines,
    candidates,
    loading,
    machineLoading,
    error,
    createDaemonMachine,
    deleteDaemonMachine,
    addAgentCandidate,
    remove,
    refresh,
    refreshMachines,
    refreshCandidates,
  } = useAgents();

  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>, agent: Agent) => {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      onSelect(agent);
    }
  };

  if (loading && agents.length === 0) {
    return (
      <div className={styles.container}>
        <Spin size="small" />
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <div className={styles.statusRow}>
        <span>{error ?? `共 ${agents.length} 个 Agent`}</span>
        <div className={styles.statusActions}>
          <Button
            icon={<DesktopOutlined />}
            size="small"
            onClick={() => setConnectOpen(true)}
          >
            连接电脑
          </Button>
          <Button
            type="text"
            size="small"
            icon={<ReloadOutlined />}
            onClick={refresh}
          />
        </div>
      </div>

      {agents.length === 0 ? (
        <div className={styles.empty}>
          暂未发现 Agent，请先启动本地守护进程
        </div>
      ) : (
        agents.map((agent) => {
          const capabilities = parseCapabilities(agent.capabilities_json);
          return (
            <div
              className={`${styles.card} ${agent.id === selectedAgentId ? styles.cardActive : ''}`}
              key={agent.id}
              role="button"
              tabIndex={0}
              onKeyDown={(event) => handleKeyDown(event, agent)}
              onClick={() => onSelect(agent)}
            >
              <div className={styles.cardHeader}>
                <Badge color={statusColor[agent.status]} dot>
                  <Avatar icon={<RobotOutlined />} />
                </Badge>
                <div className={styles.agentMeta}>
                  <span className={styles.agentName}>{agent.name}</span>
                  <span className={styles.agentTool}>
                    {agent.cli_tool}
                    {agent.machine_name ? ` · ${agent.machine_name}` : ''}
                  </span>
                </div>
                <Tag>{agent.type === 'custom' ? '自建' : '系统'}</Tag>
                {agent.user_id && (
                  <Popconfirm
                    cancelText="取消"
                    okText="删除"
                    title="删除这个 Agent？"
                    onConfirm={(event) => {
                      event?.stopPropagation();
                      remove(agent.id);
                    }}
                  >
                    <Button
                      danger
                      icon={<DeleteOutlined />}
                      size="small"
                      type="text"
                      onClick={(event) => event.stopPropagation()}
                    />
                  </Popconfirm>
                )}
              </div>
              {capabilities.length > 0 && (
                <div className={styles.tags}>
                  {capabilities.map((item) => (
                    <Tag key={item}>{item}</Tag>
                  ))}
                </div>
              )}
            </div>
          );
        })
      )}
      <ConnectComputerModal
        candidates={candidates}
        loading={machineLoading}
        machines={machines}
        open={connectOpen}
        onClose={() => setConnectOpen(false)}
        onAddCandidate={addAgentCandidate}
        onCreate={createDaemonMachine}
        onDeleteMachine={deleteDaemonMachine}
        onRefresh={async () => {
          await refreshMachines();
          await refreshCandidates();
        }}
      />
    </div>
  );
};
