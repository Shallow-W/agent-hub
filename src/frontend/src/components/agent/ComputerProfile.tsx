import React, { useMemo, useState } from 'react';
import { Avatar, Button, Popconfirm, Tag, Typography } from 'antd';
import { message } from '@/utils/message';
import {
  AppstoreOutlined,
  DeleteOutlined,
  DesktopOutlined,
  DashboardOutlined,
  LinkOutlined,
  PlusOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import { useAgentStore } from '@/store/agentStore';
import { getManagementTools, hasManagementToolsInConfig } from './toolAssignments';
import type { Agent, AgentCandidate, DaemonMachine } from '@/types/agent';
import { AgentCreateModal } from './AgentCreateModal';
import { AvatarPickerModal } from './AvatarPickerModal';
import { formatDateTime } from './agentPresentation';
import { SectionHeader } from '@/components/common/SectionHeader';
import { StatusBadge, type StatusBadgeStatus } from '@/components/common/StatusBadge';
import { ResourceChart } from './ResourceChart';
import { AgentBaseCard } from './AgentBaseCard';
import { AddedAgentCard } from './AddedAgentCard';
import { buildCommands, DAEMON_VERSION } from '@/utils/connectCommand';
import styles from './ComputerProfile.module.css';

interface ComputerProfileProps {
  machineId: string | null;
  selectedAgentId?: string | null;
  onSelectAgent?: (agent: Agent) => void;
  onClearSelection?: () => void;
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
  const startAgent = useAgentStore((s) => s.startAgent);
  const stopAgent = useAgentStore((s) => s.stopAgent);
  const restartAgent = useAgentStore((s) => s.restartAgent);
  const [createOpen, setCreateOpen] = useState(false);
  const [avatarPickerAgent, setAvatarPickerAgent] = useState<Agent | null>(null);
  const [reconnectCommands, setReconnectCommands] = useState<{ npx: string; node: string } | null>(null);
  const [reconnecting, setReconnecting] = useState(false);
  const [lifecycleLoading, setLifecycleLoading] = useState<Record<string, boolean>>({});

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
      await Promise.all([refreshMachines(true), refreshCandidates(true), refreshAgents(true)]);
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
      setReconnectCommands(buildCommands(result.command, result.daemon_npm_path, machine.name));
    } catch {
      message.error('获取连接命令失败');
    } finally {
      setReconnecting(false);
    }
  };

  const handleCreateAgent = async (candidateId: string, name: string, systemPrompt: string, toolsConfig: string, customSkills: string) => {
    const candidate = machineCandidates.find((item) => item.id === candidateId);
    if (!candidate) {
      message.error('Agent 底座不存在，请刷新后重试');
      return;
    }
    await addAgentCandidate(candidateId, {
      name,
      cli_tool: candidate.cli_tool,
      system_prompt: systemPrompt,
      tools_config: toolsConfig,
      custom_skills: customSkills,
      enable_management_tools: hasManagementToolsInConfig(toolsConfig, getManagementTools()),
    });
  };

  const handleStartAgent = async (agentId: string) => {
    setLifecycleLoading((prev) => ({ ...prev, [agentId]: true }));
    try {
      await startAgent(agentId);
      message.success('Agent 已启动');
    } catch {
      message.error('启动 Agent 失败');
    } finally {
      setLifecycleLoading((prev) => ({ ...prev, [agentId]: false }));
    }
  };

  const handleStopAgent = async (agentId: string) => {
    setLifecycleLoading((prev) => ({ ...prev, [agentId]: true }));
    try {
      await stopAgent(agentId);
      message.success('Agent 已停止');
    } catch {
      message.error('停止 Agent 失败');
    } finally {
      setLifecycleLoading((prev) => ({ ...prev, [agentId]: false }));
    }
  };

  const handleRestartAgent = async (agentId: string) => {
    setLifecycleLoading((prev) => ({ ...prev, [agentId]: true }));
    try {
      await restartAgent(agentId);
      message.success('Agent 已重启');
    } catch {
      message.error('重启 Agent 失败');
    } finally {
      setLifecycleLoading((prev) => ({ ...prev, [agentId]: false }));
    }
  };

  const handleToggleAgent = (agentId: string, action: 'start' | 'stop' | 'restart') => {
    if (action === 'start') return handleStartAgent(agentId);
    if (action === 'stop') return handleStopAgent(agentId);
    return handleRestartAgent(agentId);
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
              <StatusBadge
                status={machineStatusBadge(machine.status)}
                label={machineStatusLabel(machine.status)}
                size="md"
              />
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
      {reconnectCommands && (
        <div className={styles.reconnectBox}>
          <div className={styles.reconnectHint}>
            在目标电脑上执行以下命令重新连接：
            <Tag color="blue" className={styles.versionTag}>Daemon v{DAEMON_VERSION}</Tag>
          </div>
          <div className={styles.reconnectCommandsList}>
            <div className={styles.reconnectRow}>
              <div className={styles.reconnectLabel}>
                <Tag>NPX</Tag>
                <span className={styles.reconnectHintInline}>在线（npm 安装）</span>
              </div>
              <Typography.Text copyable code className={styles.reconnectCode}>
                {reconnectCommands.npx}
              </Typography.Text>
            </div>
            <div className={styles.reconnectRow}>
              <div className={styles.reconnectLabel}>
                <Tag>Node</Tag>
                <span className={styles.reconnectHintInline}>本地（开发用，跳过 npm）</span>
              </div>
              <Typography.Text copyable code className={styles.reconnectCode}>
                {reconnectCommands.node}
              </Typography.Text>
            </div>
          </div>
        </div>
      )}

      <section className={styles.section}>
        <SectionHeader
          icon={<DesktopOutlined />}
          title="电脑信息"
          description="运行环境与连接状态"
        />
        <div className={styles.infoGrid}>
          <div className={styles.infoItem}>
            <span className={styles.infoLabel}>电脑状态</span>
            <span className={styles.infoValue}>
              <StatusBadge
                status={machineStatusBadge(machine.status)}
                label={machineStatusLabel(machine.status)}
              />
            </span>
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
        <SectionHeader
          icon={<DashboardOutlined />}
          title="系统资源"
          description="CPU / 内存 / 磁盘 / 网络 实时占用"
        />
        <div className={styles.resourceGrid}>
          <ResourceChart metric="cpu" title="CPU" machineId={machine.id} />
          <ResourceChart metric="memory" title="内存" machineId={machine.id} />
          <ResourceChart metric="disk" title="磁盘" machineId={machine.id} />
          <ResourceChart metric="network" title="网络" machineId={machine.id} />
        </div>
      </section>

      <section className={styles.section}>
        <SectionHeader
          icon={<AppstoreOutlined />}
          title="Agent 底座"
          description={`${machineCandidates.length} 个可用 CLI 工具`}
        />
        {machineCandidates.length === 0 ? (
          <div className={styles.emptyRow}>暂无底座</div>
        ) : (
          <div className={styles.baseGrid}>
            {machineCandidates.map((candidate: AgentCandidate) => {
              const caps = (() => {
                if (!candidate.capabilities_json) return [];
                try {
                  const parsed = JSON.parse(candidate.capabilities_json);
                  if (Array.isArray(parsed)) {
                    return parsed.filter((c): c is string => typeof c === 'string').slice(0, 4);
                  }
                  if (parsed && typeof parsed === 'object') {
                    return Object.keys(parsed).slice(0, 4);
                  }
                  return [];
                } catch {
                  return [];
                }
              })();
              return (
                <AgentBaseCard
                  key={candidate.id}
                  cliTool={candidate.cli_tool}
                  name={candidate.name}
                  version={candidate.version}
                  capabilities={caps}
                />
              );
            })}
          </div>
        )}
      </section>

      <section className={styles.section}>
        <SectionHeader
          icon={<AppstoreOutlined />}
          title="已添加 Agent"
          description={`${machineAgents.length} 个 Agent 员工`}
          extra={
            <Button
              size="small"
              icon={<PlusOutlined />}
              disabled={machineCandidates.length === 0}
              onClick={() => setCreateOpen(true)}
            >
              添加 Agent 员工
            </Button>
          }
        />
        {machineAgents.length === 0 ? (
          <div className={styles.emptyRow}>暂无 Agent 员工</div>
        ) : (
          <div className={styles.agentGrid}>
            {machineAgents.map((agent) => (
              <AddedAgentCard
                key={agent.id}
                agent={agent}
                isActive={agent.id === selectedAgentId}
                lifecycleLoading={lifecycleLoading[agent.id]}
                onSelect={onSelectAgent}
                onToggle={handleToggleAgent}
              />
            ))}
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
      <AvatarPickerModal
        agent={avatarPickerAgent}
        open={avatarPickerAgent !== null}
        onClose={() => setAvatarPickerAgent(null)}
      />
    </div>
  );
};
