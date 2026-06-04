import React, { useEffect, useMemo, useState } from 'react';
import { Avatar, Badge, Button, Popconfirm, Spin, Tag } from 'antd';
import {
  DownOutlined,
  DeleteOutlined,
  DesktopOutlined,
  ReloadOutlined,
  RobotOutlined,
  RightOutlined,
} from '@ant-design/icons';
import { useAgents } from '@/hooks/useAgents';
import type { Agent, AgentStatus, DaemonMachine } from '@/types/agent';
import { ConnectComputerModal } from './ConnectComputerModal';
import { formatDateTime, parseSkills } from './agentPresentation';
import styles from './AgentList.module.css';

interface AgentListProps {
  selectedAgentId: string | null;
  selectedMachineId: string | null;
  onSelect: (agent: Agent) => void;
  onSelectMachine: (machineId: string) => void;
}

const statusColor: Record<AgentStatus, string> = {
  online: 'green',
  offline: 'default',
  busy: 'processing',
  error: 'red',
};

const machineStatusLabel: Record<DaemonMachine['status'], string> = {
  connected: '已连接',
  pending: '等待连接',
  offline: '离线',
};

const machineStatusColor: Record<DaemonMachine['status'], string> = {
  connected: 'green',
  pending: 'gold',
  offline: 'default',
};

export const AgentList: React.FC<AgentListProps> = ({
  selectedAgentId,
  selectedMachineId,
  onSelect,
  onSelectMachine,
}) => {
  const [connectOpen, setConnectOpen] = useState(false);
  const [expandedMachines, setExpandedMachines] = useState<Record<string, boolean>>({});
  const {
    agents,
    machines,
    candidates,
    loading,
    machineLoading,
    error,
    addAgentCandidate,
    createDaemonMachine,
    deleteDaemonMachine,
    remove,
    refresh,
    refreshMachines,
    refreshCandidates,
  } = useAgents();

  useEffect(() => {
    if (machines.length === 0 && agents.length === 0) return;
    if (Object.keys(expandedMachines).length > 0) return;
    const next: Record<string, boolean> = {};
    const connected = machines.filter((machine) => machine.status === 'connected');
    if (connected.length > 0) {
      connected.forEach((machine) => {
        next[machine.id] = true;
      });
    } else if (machines[0]) {
      next[machines[0].id] = true;
    }
    if (Object.keys(next).length > 0) {
      setExpandedMachines(next);
    }
  }, [machines, agents, expandedMachines]);

  useEffect(() => {
    if (selectedMachineId || machines.length === 0) return;
    onSelectMachine(machines[0]!.id);
  }, [machines, onSelectMachine, selectedMachineId]);

  const handleToggleMachine = (id: string) => {
    setExpandedMachines((prev) => ({
      ...prev,
      [id]: !prev[id],
    }));
  };

  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>, agent: Agent) => {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      onSelect(agent);
    }
  };

  const handleRefreshAll = async () => {
    try {
      await Promise.all([refresh(), refreshMachines(), refreshCandidates()]);
    } catch {
      return;
    }
  };

  const machineGroups = useMemo(() => {
    const machineIdSet = new Set(machines.map((machine) => machine.id));
    const agentsByMachine = new Map<string, Agent[]>();
    agents.forEach((agent) => {
      const key = agent.machine_id && machineIdSet.has(agent.machine_id) ? agent.machine_id : 'global';
      const list = agentsByMachine.get(key) ?? [];
      list.push(agent);
      agentsByMachine.set(key, list);
    });
    const groups = machines.map((machine) => ({
      id: machine.id,
      name: machine.name,
      machineID: machine.machine_id,
      status: machine.status,
      lastSeenAt: machine.last_seen_at,
      isGlobal: false,
      agents: agentsByMachine.get(machine.id) ?? [],
    }));
    const globalAgents = agentsByMachine.get('global') ?? [];
    if (globalAgents.length > 0) {
      groups.push({
        id: 'global',
        name: '未绑定电脑',
        machineID: '',
        status: 'offline' as DaemonMachine['status'],
        lastSeenAt: undefined,
        isGlobal: true,
        agents: globalAgents,
      });
    }
    return groups;
  }, [agents, machines]);

  const totalAgents = agents.length;
  const totalMachines = machines.length;

  if ((loading || machineLoading) && agents.length === 0 && machines.length === 0 && !connectOpen) {
    return (
      <div className={styles.container}>
        <Spin size="small" />
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <div className={styles.statusRow}>
        <span>{error ?? `共 ${totalMachines} 台电脑 · ${totalAgents} 个 Agent`}</span>
        <div className={styles.statusActions}>
          <Button icon={<DesktopOutlined />} size="small" onClick={() => setConnectOpen(true)}>
            连接电脑
          </Button>
          <Button type="text" size="small" icon={<ReloadOutlined />} onClick={() => void handleRefreshAll()} />
        </div>
      </div>

      {machineGroups.length === 0 ? (
        <div className={styles.empty}>
          暂未发现电脑，请先连接本地守护进程
        </div>
      ) : (
        machineGroups.map((group) => {
          const isExpanded = expandedMachines[group.id] ?? false;
          const machineAgents = group.agents;

          return (
            <div
              className={`${styles.machineCard} ${group.id === selectedMachineId ? styles.machineCardActive : ''}`}
              key={group.id}
            >
              <button
                className={styles.machineHeader}
                type="button"
                onClick={() => {
                  onSelectMachine(group.id);
                  handleToggleMachine(group.id);
                }}
                aria-expanded={isExpanded}
              >
                <div className={styles.machineLeft}>
                  <Avatar size={36} className={styles.machineAvatar} icon={<DesktopOutlined />} />
                  <div className={styles.machineMeta}>
                    <div className={styles.machineTitleRow}>
                      <span className={styles.machineTitle}>{group.name}</span>
                      {!group.isGlobal && (
                        <Tag color={machineStatusColor[group.status]}>
                          {machineStatusLabel[group.status]}
                        </Tag>
                      )}
                    </div>
                    <span className={styles.machineSub}>
                      {group.isGlobal
                        ? '未绑定到具体电脑的 Agent'
                        : `${group.machineID || '未上报主机 ID'} · ${formatDateTime(group.lastSeenAt)}`}
                    </span>
                  </div>
                </div>
                <div className={styles.machineActions}>
                  <span className={styles.machineCount}>{group.agents.length}</span>
                  <span className={styles.expandIcon}>
                    {isExpanded ? <DownOutlined /> : <RightOutlined />}
                  </span>
                </div>
              </button>
              {isExpanded && (
                <div className={styles.machineBody}>
                  <div className={styles.sectionLabel}>已添加</div>
                  <div className={styles.agentList}>
                    {machineAgents.length === 0 ? (
                      <div className={styles.machineEmpty}>暂无 Agent</div>
                    ) : (
                      machineAgents.map((agent) => {
                        const capabilities = parseSkills(agent.capabilities_json).map((skill) => skill.name);
                        return (
                          <div
                            key={agent.id}
                            className={`${styles.agentRow} ${agent.id === selectedAgentId ? styles.agentRowActive : ''}`}
                            role="button"
                            tabIndex={0}
                            onKeyDown={(event) => handleKeyDown(event, agent)}
                            onClick={() => onSelect(agent)}
                          >
                            <div className={styles.agentHeader}>
                              <Badge color={statusColor[agent.status]} dot>
                                <Avatar size={28} icon={<RobotOutlined />} />
                              </Badge>
                              <div className={styles.agentMeta}>
                                <span className={styles.agentName}>{agent.name}</span>
                                <span className={styles.agentTool}>
                                  {agent.cli_tool}
                                  {agent.version ? ` · ${agent.version}` : ''}
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
                                  <Button danger icon={<DeleteOutlined />} size="small" type="text" onClick={(event) => event.stopPropagation()} />
                                </Popconfirm>
                              )}
                            </div>
                            {capabilities.length > 0 && (
                              <div className={styles.agentTags}>
                                {capabilities.slice(0, 3).map((item) => (
                                  <Tag key={item}>{item.length > 16 ? item.slice(0, 16) + '...' : item}</Tag>
                                ))}
                                {capabilities.length > 3 && <Tag>+{capabilities.length - 3}</Tag>}
                              </div>
                            )}
                          </div>
                        );
                      })
                    )}
                  </div>
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
          await Promise.all([refresh(), refreshMachines(), refreshCandidates()]);
        }}
      />
    </div>
  );
};
