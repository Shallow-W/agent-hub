import React, { useMemo, useState } from 'react';
import { Avatar, Button, Popconfirm, Tag, message, Typography } from 'antd';
import {
  DeleteOutlined,
  DesktopOutlined,
  LinkOutlined,
  PlusOutlined,
  ReloadOutlined,
  RobotOutlined,
} from '@ant-design/icons';
import { useAgentStore } from '@/store/agentStore';
import type { Agent, AgentCandidate, DaemonMachine } from '@/types/agent';
import { AgentCreateModal } from './AgentCreateModal';
import { formatDateTime, parseCapabilities } from './agentPresentation';
import styles from './ComputerProfile.module.css';

interface ComputerProfileProps {
  machineId: string | null;
  selectedAgentId?: string | null;
  onSelectAgent?: (agent: Agent) => void;
  onClearSelection?: () => void;
}

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

function inferOS(machine: DaemonMachine): string {
  const text = `${machine.name} ${machine.machine_id}`.toLowerCase();
  if (text.includes('darwin') || text.includes('mac')) return 'macOS';
  if (text.includes('win')) return 'Windows';
  if (text.includes('linux') || text.includes('ubuntu')) return 'Linux';
  return '未上报';
}

export const ComputerProfile: React.FC<ComputerProfileProps> = ({
  machineId,
  selectedAgentId,
  onSelectAgent,
  onClearSelection,
}) => {
  const machines = useAgentStore((s) => s.machines);
  const agents = useAgentStore((s) => s.agents);
  const candidates = useAgentStore((s) => s.candidates);
  const machineLoading = useAgentStore((s) => s.machineLoading);
  const deleteDaemonMachine = useAgentStore((s) => s.deleteDaemonMachine);
  const refreshMachines = useAgentStore((s) => s.fetchDaemonMachines);
  const refreshCandidates = useAgentStore((s) => s.fetchAgentCandidates);
  const refreshAgents = useAgentStore((s) => s.fetchAgents);
  const addAgentCandidate = useAgentStore((s) => s.addAgentCandidate);
  const [createOpen, setCreateOpen] = useState(false);
  const [reconnectCmd, setReconnectCmd] = useState<string | null>(null);
  const [reconnecting, setReconnecting] = useState(false);

  const machine = machines.find((item) => item.id === machineId) ?? null;
  const machineAgents = useMemo(
    () => agents.filter((agent) => agent.machine_id === machineId),
    [agents, machineId],
  );
  const machineCandidates = useMemo(
    () => candidates.filter((candidate) => candidate.machine_id === machineId),
    [candidates, machineId],
  );

  const handleRefresh = async () => {
    try {
      await Promise.all([refreshMachines(), refreshCandidates(), refreshAgents()]);
    } catch {
      message.error('刷新电脑信息失败');
    }
  };

  const handleDelete = async () => {
    if (!machine) return;
    try {
      await deleteDaemonMachine(machine.id);
      message.success('电脑已删除');
      onClearSelection?.();
    } catch {
      message.error('删除电脑失败');
    }
  };

  const handleReconnect = async () => {
    if (!machine) return;
    setReconnecting(true);
    try {
      const { getMachineConnectCommand } = await import('@/api/agent');
      const result = await getMachineConnectCommand(machine.id);
      // 后端返回的 command 包含正确的 --server-url，前端不自行拼接
      if (result.daemon_npm_path) {
        setReconnectCmd(
          result.command.replace(
            /npx\s+@agenthub\/daemon(\S+)?/,
            `npx "@agenthub/daemon@file:${result.daemon_npm_path}"`,
          ),
        );
      } else {
        setReconnectCmd(result.command);
      }
    } catch {
      message.error('获取连接命令失败');
    } finally {
      setReconnecting(false);
    }
  };

  const handleCreateAgent = async (candidateId: string, name: string, systemPrompt: string) => {
    await addAgentCandidate(candidateId, name, systemPrompt);
  };

  if (!machine) {
    return (
      <div className={styles.empty}>
        请选择一台电脑查看详情
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.titleBlock}>
          <Avatar size={44} className={styles.machineAvatar} icon={<DesktopOutlined />} />
          <div>
            <div className={styles.titleRow}>
              <span className={styles.title}>{machine.name}</span>
              <Tag color={machineStatusColor[machine.status]}>
                {machineStatusLabel[machine.status]}
              </Tag>
            </div>
            <div className={styles.subtitle}>
              {inferOS(machine)} · {machine.machine_id || '未上报主机名'}
            </div>
          </div>
        </div>
        <div className={styles.headerActions}>
          {machine.status !== 'connected' && (
            <Button
              size="small"
              icon={<LinkOutlined />}
              loading={reconnecting}
              onClick={handleReconnect}
            >
              重新连接
            </Button>
          )}
          <Button
            size="small"
            icon={<ReloadOutlined />}
            loading={machineLoading}
            onClick={handleRefresh}
          >
            刷新
          </Button>
          <Popconfirm
            title="确定删除这台电脑吗？"
            okText="删除"
            cancelText="取消"
            onConfirm={handleDelete}
          >
            <Button danger size="small" icon={<DeleteOutlined />}>
              删除
            </Button>
          </Popconfirm>
        </div>
      </div>
      {reconnectCmd && (
        <div className={styles.reconnectBox} style={{ margin: '0 0 8px', padding: 12, background: 'var(--color-bg-secondary)', borderRadius: 8 }}>
          <div style={{ fontSize: 12, color: 'var(--text-secondary)', marginBottom: 6 }}>
            在目标电脑上执行以下命令重新连接：
          </div>
          <Typography.Text
            copyable
            code
            style={{ fontSize: 12, wordBreak: 'break-all' }}
          >
            {reconnectCmd}
          </Typography.Text>
        </div>
      )}

      <section className={styles.section}>
        <div className={styles.sectionHeader}>
          <span className={styles.sectionTitle}>电脑信息</span>
          <span className={styles.sectionHint}>运行环境与连接状态</span>
        </div>
        <div className={styles.infoGrid}>
          <div className={styles.infoItem}>
            <span className={styles.infoLabel}>电脑状态</span>
            <span className={styles.infoValue}>{machineStatusLabel[machine.status]}</span>
          </div>
          <div className={styles.infoItem}>
            <span className={styles.infoLabel}>主机名</span>
            <span className={styles.infoValue}>{machine.machine_id || '未上报'}</span>
          </div>
          <div className={styles.infoItem}>
            <span className={styles.infoLabel}>操作系统</span>
            <span className={styles.infoValue}>{inferOS(machine)}</span>
          </div>
          <div className={styles.infoItem}>
            <span className={styles.infoLabel}>Agent 底座</span>
            <span className={styles.infoValue}>{machineCandidates.length} 个</span>
          </div>
          <div className={styles.infoItem}>
            <span className={styles.infoLabel}>Agent 员工</span>
            <span className={styles.infoValue}>{machineAgents.length} 个</span>
          </div>
          <div className={styles.infoItem}>
            <span className={styles.infoLabel}>最近心跳</span>
            <span className={styles.infoValue}>{formatDateTime(machine.last_seen_at)}</span>
          </div>
          <div className={styles.infoItem}>
            <span className={styles.infoLabel}>创建时间</span>
            <span className={styles.infoValue}>{formatDateTime(machine.created_at)}</span>
          </div>
        </div>
      </section>

      <section className={styles.section}>
        <div className={styles.sectionHeader}>
          <span className={styles.sectionTitle}>Agent 底座</span>
          <span className={styles.sectionHint}>{machineCandidates.length} 个</span>
        </div>
        {machineCandidates.length === 0 ? (
          <div className={styles.emptyRow}>暂无底座</div>
        ) : (
          <div className={styles.baseList}>
            {machineCandidates.map((candidate: AgentCandidate) => {
                const capabilityList = parseCapabilities(candidate.capabilities_json);
                return (
                <div className={styles.baseCard} key={candidate.id}>
                  <div className={styles.baseName}>{candidate.name}</div>
                  <div className={styles.baseMeta}>
                    {candidate.cli_tool}
                    {candidate.version ? ` · ${candidate.version}` : ''}
                  </div>
                  {capabilityList.length > 0 && (
                    <div className={styles.baseTags}>
                      {capabilityList.slice(0, 3).map((item) => (
                        <Tag key={item}>{item.length > 16 ? item.slice(0, 16) + '...' : item}</Tag>
                      ))}
                      {capabilityList.length > 3 && <Tag>+{capabilityList.length - 3}</Tag>}
                    </div>
                  )}
                </div>
            );
            })}
          </div>
        )}
      </section>

      <section className={styles.section}>
        <div className={styles.sectionHeader}>
          <span className={styles.sectionTitle}>已添加 Agent</span>
          <div className={styles.sectionActions}>
            <Button
              size="small"
              icon={<PlusOutlined />}
              disabled={machineCandidates.length === 0}
              onClick={() => setCreateOpen(true)}
            >
              添加 Agent 员工
            </Button>
          </div>
        </div>
        {machineAgents.length === 0 ? (
          <div className={styles.emptyRow}>暂无 Agent 员工</div>
        ) : (
          <div className={styles.agentList}>
            {machineAgents.map((agent) => {
              const capabilityList = parseCapabilities(agent.capabilities_json);
              const isActive = agent.id === selectedAgentId;
              return (
                <div
                  key={agent.id}
                  className={`${styles.agentRow} ${isActive ? styles.agentRowActive : ''}`}
                  role="button"
                  tabIndex={0}
                  onClick={() => onSelectAgent?.(agent)}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter' || event.key === ' ') {
                      event.preventDefault();
                      onSelectAgent?.(agent);
                    }
                  }}
                >
                  <div className={styles.agentMain}>
                    <Avatar size={32} icon={<RobotOutlined />} />
                    <div className={styles.agentInfo}>
                      <div className={styles.agentName}>{agent.name}</div>
                      <div className={styles.agentMeta}>
                        {agent.cli_tool}
                        {agent.version ? ` · ${agent.version}` : ''}
                      </div>
                      {agent.system_prompt && (
                        <div className={styles.agentPrompt}>{agent.system_prompt}</div>
                      )}
                    </div>
                  </div>
                  {capabilityList.length > 0 && (
                    <div className={styles.agentTags}>
                      {capabilityList.slice(0, 3).map((item) => (
                        <Tag key={item}>{item.length > 16 ? item.slice(0, 16) + '...' : item}</Tag>
                      ))}
                      {capabilityList.length > 3 && <Tag>+{capabilityList.length - 3}</Tag>}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </section>

      <AgentCreateModal
        open={createOpen}
        machineName={machine.name}
        candidates={machineCandidates}
        onClose={() => setCreateOpen(false)}
        onCreate={handleCreateAgent}
      />
    </div>
  );
};
