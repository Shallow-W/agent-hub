import React, { useEffect, useMemo, useState } from 'react';
import { Avatar, Button, Popconfirm, Spin, Tag } from 'antd';
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
import { AvatarPickerModal } from './AvatarPickerModal';
import { formatDateTime, resolveAgentAvatar } from './agentPresentation';
import { StatusBadge, type StatusBadgeStatus } from '@/components/common/StatusBadge';
import styles from './AgentList.module.css';

interface AgentListProps {
  selectedAgentId: string | null;
  selectedMachineId: string | null;
  onSelect: (agent: Agent) => void;
  onSelectMachine: (machineId: string) => void;
}

function machineStatusBadge(status: DaemonMachine['status']): StatusBadgeStatus {
  switch (status) {
    case 'connected':
      return 'connected';
    case 'pending':
      return 'warning';
    case 'offline':
      return 'disconnected';
    default:
      return 'inactive';
  }
}

function machineStatusLabel(status: DaemonMachine['status']): string {
  switch (status) {
    case 'connected':
      return '已连接';
    case 'pending':
      return '等待连接';
    case 'offline':
      return '离线';
    default:
      return status;
  }
}

function agentStatusBadge(status: AgentStatus): StatusBadgeStatus {
  switch (status) {
    case 'online':
      return 'running';
    case 'busy':
      return 'running';
    case 'error':
      return 'error';
    case 'stopped':
      return 'idle';
    case 'offline':
    default:
      return 'inactive';
  }
}

function formatRelativeMin(iso?: string): string {
  if (!iso) return '';
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return '';
  const diffMs = Date.now() - then;
  if (diffMs < 0) return '刚刚';
  const min = Math.floor(diffMs / 60000);
  if (min < 1) return '刚刚';
  if (min < 60) return `${min} 分钟前`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr} 小时前`;
  const day = Math.floor(hr / 24);
  return `${day} 天前`;
}

export const AgentList: React.FC<AgentListProps> = ({
  selectedAgentId,
  selectedMachineId,
  onSelect,
  onSelectMachine,
}) => {
  const [connectOpen, setConnectOpen] = useState(false);
  const [avatarPickerAgent, setAvatarPickerAgent] = useState<Agent | null>(null);
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
      await Promise.all([refresh(true), refreshMachines(true), refreshCandidates(true)]);
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
                  // 点击中间区域：选中 machine（显示机器界面）
                  onSelectMachine(group.id);
                }}
                aria-expanded={isExpanded}
              >
                <div className={styles.machineLeft}>
                  <Avatar size={44} className={styles.machineAvatar} icon={<DesktopOutlined />} />
                  <div className={styles.machineMeta}>
                    <div className={styles.machineTitleRow}>
                      <span className={styles.machineTitle}>{group.name}</span>
                      {!group.isGlobal && (
                        <StatusBadge
                          status={machineStatusBadge(group.status)}
                          label={machineStatusLabel(group.status)}
                        />
                      )}
                    </div>
                    <span className={styles.machineSub}>
                      {group.isGlobal
                        ? '未绑定到具体电脑的 Agent'
                        : `${group.machineID || '未上报主机 ID'} · ${formatDateTime(group.lastSeenAt)}`}
                    </span>
                    <span className={styles.machineHint}>
                      {group.agents.length} 个 Agent
                      {!group.isGlobal && group.lastSeenAt
                        ? ` · 最近心跳 ${formatRelativeMin(group.lastSeenAt)}`
                        : ''}
                    </span>
                  </div>
                </div>
                <div
                  className={styles.machineActions}
                  onClick={(e) => {
                    // 点击展开图标区域：只切换 agent 列表展开/折叠
                    e.stopPropagation();
                    handleToggleMachine(group.id);
                  }}
                >
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
                              <Avatar
                                size={36}
                                src={resolveAgentAvatar(agent)}
                                icon={<RobotOutlined />}
                                className={styles.agentAvatar}
                                style={{ cursor: 'pointer' }}
                                onClick={(event) => {
                                  event?.stopPropagation();
                                  setAvatarPickerAgent(agent);
                                }}
                              />
                              <div className={styles.agentMeta}>
                                <span className={styles.agentName}>{agent.name}</span>
                                <span className={styles.agentTool}>
                                  @{agent.cli_tool}
                                  {agent.version ? ` · v${agent.version}` : ''}
                                </span>
                                {agent.system_prompt ? (
                                  <span className={styles.agentDesc}>
                                    {agent.system_prompt.slice(0, 80)}
                                    {agent.system_prompt.length > 80 ? '…' : ''}
                                  </span>
                                ) : null}
                              </div>
                              <div className={styles.agentStatus}>
                                <StatusBadge
                                  status={agentStatusBadge(agent.status)}
                                  withDot
                                />
                              </div>
                              <div className={styles.agentRightActions}>
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
                            </div>
                              {(() => {
                            const isBuiltinSystem = agent.type === 'system' && !agent.user_id;
                            if (isBuiltinSystem) return null;
                            const tags = (() => {
                              if (!agent.tags || agent.tags === '[]') return [];
                              try {
                                const arr = JSON.parse(agent.tags);
                                return Array.isArray(arr) ? arr.filter((t): t is string => typeof t === 'string') : [];
                              } catch {
                                return [];
                              }
                            })();
                            if (tags.length === 0) return null;
                            return (
                              <div className={styles.agentTags}>
                                {tags.slice(0, 3).map((item) => (
                                  <Tag key={item}>{item.length > 16 ? item.slice(0, 16) + '...' : item}</Tag>
                                ))}
                                {tags.length > 3 && <Tag>+{tags.length - 3}</Tag>}
                              </div>
                            );
                          })()}
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
          await Promise.all([refresh(true), refreshMachines(true), refreshCandidates(true)]);
        }}
      />
      <AvatarPickerModal
        agent={avatarPickerAgent}
        open={avatarPickerAgent !== null}
        onClose={() => setAvatarPickerAgent(null)}
      />
    </div>
  );
};
