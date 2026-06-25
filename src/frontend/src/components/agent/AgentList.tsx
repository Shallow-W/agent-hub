import React, { useEffect, useMemo, useState } from 'react';
import { Avatar, Button, Dropdown, Spin } from 'antd';
import type { MenuProps } from 'antd';
import {
  DownOutlined,
  DeleteOutlined,
  DesktopOutlined,
  MoreOutlined,
  ReloadOutlined,
  RobotOutlined,
  RightOutlined,
} from '@ant-design/icons';
import { useAgents } from '@/hooks/useAgents';
import type { Agent, AgentStatus, DaemonMachine } from '@/types/agent';
import { ConnectComputerModal } from './ConnectComputerModal';
import { AvatarPickerModal } from './AvatarPickerModal';
import { resolveAgentAvatar } from './agentPresentation';
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
      return '在线';
    case 'pending':
      return '等待';
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

function agentStatusLabel(status: AgentStatus): string {
  switch (status) {
    case 'online':
      return '在线';
    case 'busy':
      return '忙碌';
    case 'error':
      return '异常';
    case 'stopped':
      return '已停止';
    case 'offline':
      return '离线';
    default:
      return status;
  }
}

// 计算有效 agent 状态：machine 不在线时，agent 也显示为离线（视觉级联）
// 注意：不修改 agent.status 数据本身，仅影响展示。
function effectiveAgentStatus(
  machineStatus: DaemonMachine['status'],
  agentStatus: AgentStatus,
): AgentStatus {
  if (machineStatus === 'offline') return 'offline';
  if (machineStatus === 'pending') return 'offline';
  return agentStatus;
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
        {error ? (
          <div className={styles.statusText}><span>{error}</span></div>
        ) : (
          <div className={styles.statusText}>
            <span className={styles.statLine}>{totalMachines} 台电脑</span>
            <span className={styles.statLine}>{totalAgents} 个 Agent</span>
          </div>
        )}
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
                        : `${group.machineID || '未上报主机 ID'}`}
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
                        const menuItems: MenuProps['items'] = [
                          ...(agent.user_id
                            ? [
                                {
                                  key: 'delete',
                                  icon: <DeleteOutlined />,
                                  danger: true,
                                  label: '删除',
                                },
                              ]
                            : []),
                        ];
                        if (menuItems.length === 0) return null;

                        const visibleStatus = effectiveAgentStatus(group.status, agent.status);

                        return (
                          <div
                            key={agent.id}
                            className={`${styles.agentRow} ${agent.id === selectedAgentId ? styles.agentRowActive : ''}`}
                            role="button"
                            tabIndex={0}
                            onKeyDown={(event) => handleKeyDown(event, agent)}
                            onClick={() => onSelect(agent)}
                          >
                            <Avatar
                              size={28}
                              src={resolveAgentAvatar(agent)}
                              icon={<RobotOutlined />}
                              className={styles.agentAvatar}
                              style={{ cursor: 'pointer' }}
                              onClick={(event) => {
                                event?.stopPropagation();
                                setAvatarPickerAgent(agent);
                              }}
                            />
                            <div className={styles.agentInfo}>
                              <div className={styles.agentTitleRow}>
                                <span className={styles.agentName}>{agent.name}</span>
                                <StatusBadge
                                  status={agentStatusBadge(visibleStatus)}
                                  label={agentStatusLabel(visibleStatus)}
                                />
                              </div>
                              {agent.cli_tool ? (
                                <span className={styles.agentTool}>
                                  @{agent.cli_tool}
                                  {agent.version ? ` · v${agent.version}` : ''}
                                </span>
                              ) : (
                                <span className={styles.agentTool} style={{ visibility: 'hidden' }}>&nbsp;</span>
                              )}
                            </div>
                            <Dropdown
                              menu={{
                                items: menuItems,
                                onClick: ({ key }) => {
                                  if (key === 'delete') remove(agent.id);
                                },
                              }}
                              trigger={['click']}
                            >
                              <Button
                                type="text"
                                size="small"
                                icon={<MoreOutlined />}
                                className={styles.agentMoreBtn}
                                onClick={(e) => e.stopPropagation()}
                              />
                            </Dropdown>
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
