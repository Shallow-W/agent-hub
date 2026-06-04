import React, { useEffect, useMemo, useState } from 'react';
import { Button, Input, Modal, Popconfirm, Tag, message } from 'antd';
import {
  CheckCircleOutlined,
  CopyOutlined,
  DeleteOutlined,
  DesktopOutlined,
  PlusOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import type {
  Agent,
  AgentCandidate,
  CreateDaemonMachineResponse,
  DaemonMachine,
} from '@/types/agent';
import { parseCapabilities } from './agentPresentation';
import styles from './ConnectComputerModal.module.css';

interface ConnectComputerModalProps {
  open: boolean;
  machines: DaemonMachine[];
  candidates: AgentCandidate[];
  loading: boolean;
  onClose: () => void;
  onCreate: (name: string) => Promise<CreateDaemonMachineResponse>;
  onAddCandidate: (id: string, name: string, systemPrompt?: string) => Promise<Agent>;
  onDeleteMachine: (id: string) => Promise<void>;
  onRefresh: () => Promise<void>;
}

function defaultComputerName(): string {
  const timestamp = new Date().toLocaleTimeString('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
  });
  return `computer-${timestamp.replace(':', '')}`;
}

function getServerURL(): string {
  const port = window.location.port === '5173' ? '8080' : window.location.port;
  const host = port ? `${window.location.hostname}:${port}` : window.location.host;
  return `${window.location.protocol}//${host}`;
}

function getStatusTag(machine: DaemonMachine): React.ReactNode {
  if (machine.status === 'connected') {
    return <Tag color="success">Connected</Tag>;
  }
  return <Tag color="warning">Waiting</Tag>;
}

function quoteArg(value: string): string {
  return `"${value.replace(/"/g, '\\"')}"`;
}

function buildCommand(
  serverURL: string,
  apiKey: string,
  daemonNPMPath: string,
  machineName: string,
): string {
  const packageSpec = daemonNPMPath
    ? `@agenthub/daemon@file:${daemonNPMPath}`
    : '@agenthub/daemon@latest';
  return `npx ${quoteArg(packageSpec)} --server-url ${quoteArg(serverURL)} --api-key ${quoteArg(apiKey)} # ${machineName}`;
}

function getErrorMessage(err: unknown, fallback: string): string {
  return err instanceof Error && err.message ? err.message : fallback;
}

export const ConnectComputerModal: React.FC<ConnectComputerModalProps> = ({
  open,
  machines,
  candidates,
  loading,
  onClose,
  onCreate,
  onAddCandidate,
  onDeleteMachine,
  onRefresh,
}) => {
  const [name, setName] = useState(defaultComputerName());
  const [creating, setCreating] = useState(false);
  const [addingID, setAddingID] = useState<string | null>(null);
  const [deletingMachineID, setDeletingMachineID] = useState<string | null>(null);
  const [candidateNames, setCandidateNames] = useState<Record<string, string>>({});
  const [created, setCreated] = useState<CreateDaemonMachineResponse | null>(null);
  const [refreshError, setRefreshError] = useState<string | null>(null);
  const safeMachines = machines ?? [];
  const safeCandidates = candidates ?? [];
  const hasConnectedMachine = safeMachines.some((machine) => machine.status === 'connected');
  const machinePanelTitle = hasConnectedMachine
    ? 'Connected computers'
    : 'Waiting for computer to connect...';
  const connectCommand = useMemo(() => {
    if (!created) return null;
    return buildCommand(
      getServerURL(),
      created.api_key,
      created.daemon_npm_path,
      created.machine.name,
    );
  }, [created]);

  const refreshQuietly = async () => {
    try {
      await onRefresh();
      setRefreshError(null);
    } catch (err) {
      setRefreshError(getErrorMessage(err, '刷新电脑连接失败'));
    }
  };

  const handleRefresh = async () => {
    try {
      await onRefresh();
      setRefreshError(null);
    } catch (err) {
      const errorMessage = getErrorMessage(err, '刷新电脑连接失败');
      setRefreshError(errorMessage);
      message.error(errorMessage);
    }
  };

  useEffect(() => {
    if (!open) return undefined;
    void refreshQuietly();
    const timer = window.setInterval(() => {
      void refreshQuietly();
    }, 3000);
    return () => window.clearInterval(timer);
  }, [open]);

  const handleCreate = async () => {
    if (!name.trim()) return;
    setCreating(true);
    try {
      const result = await onCreate(name.trim());
      setCreated(result);
      await handleRefresh();
      message.success('连接命令已生成');
    } catch (err) {
      message.error(getErrorMessage(err, '创建连接失败'));
    } finally {
      setCreating(false);
    }
  };

  const handleCopy = async () => {
    if (!connectCommand) return;
    await navigator.clipboard.writeText(connectCommand);
    message.success('启动命令已复制');
  };

  const handleAddCandidate = async (candidate: AgentCandidate) => {
    const displayName = (candidateNames[candidate.id] || candidate.name).trim();
    if (!displayName) return;
    setAddingID(candidate.id);
    try {
      await onAddCandidate(candidate.id, displayName);
      await handleRefresh();
      message.success(`${displayName} 已添加`);
    } catch (err) {
      message.error(getErrorMessage(err, '添加 Agent 失败'));
    } finally {
      setAddingID(null);
    }
  };

  const handleDeleteMachine = async (machine: DaemonMachine) => {
    setDeletingMachineID(machine.id);
    try {
      await onDeleteMachine(machine.id);
      await handleRefresh();
      message.success(`${machine.name} 已删除`);
    } catch (err) {
      message.error(getErrorMessage(err, '删除电脑连接失败'));
    } finally {
      setDeletingMachineID(null);
    }
  };

  return (
    <Modal
      centered
      footer={null}
      onCancel={onClose}
      open={open}
      title="CONNECT COMPUTER"
      width={640}
    >
      <div className={styles.content}>
        <div className={styles.nameRow}>
          <Input
            maxLength={100}
            prefix={<DesktopOutlined />}
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
          <Button
            icon={<PlusOutlined />}
            loading={creating}
            type="primary"
            onClick={handleCreate}
          >
            创建连接
          </Button>
        </div>

        <div className={styles.commandPanel}>
          <div className={styles.commandHeader}>
            <span>CONNECT COMMAND</span>
            {created && <Button icon={<CopyOutlined />} onClick={handleCopy} />}
          </div>
          {created ? (
            <pre className={styles.command}>{connectCommand}</pre>
          ) : (
            <div className={styles.commandEmpty}>
              点击“创建连接”生成一条新的 npx 启动命令。出于安全考虑，已创建连接的 machine key 不会再次显示。
            </div>
          )}
        </div>

        <div className={styles.waitingPanel}>
          <div className={styles.waitingTitle}>
            <span className={hasConnectedMachine ? styles.connectedDot : styles.waitingDot} />
            {machinePanelTitle}
            <Button
              icon={<ReloadOutlined />}
              loading={loading}
              size="small"
              type="text"
              onClick={handleRefresh}
            />
          </div>
          <div className={styles.machineList}>
            {refreshError ? (
              <div className={styles.errorMessage}>{refreshError}</div>
            ) : null}
            {safeMachines.length === 0 ? (
              <span className={styles.empty}>暂无电脑连接</span>
            ) : (
              safeMachines.map((machine) => (
                <div className={styles.machineItem} key={machine.id}>
                  <div>
                    <strong>{machine.name}</strong>
                    <span className={styles.machineMeta}>
                      {machine.machine_id || '等待 daemon 上报主机名'}
                    </span>
                  </div>
                  {machine.status === 'connected' ? (
                    <CheckCircleOutlined className={styles.connectedIcon} />
                  ) : null}
                  {getStatusTag(machine)}
                  <Popconfirm
                    cancelText="取消"
                    okText="删除"
                    title="删除这台电脑连接？"
                    onConfirm={() => handleDeleteMachine(machine)}
                  >
                    <Button
                      danger
                      icon={<DeleteOutlined />}
                      loading={deletingMachineID === machine.id}
                      size="small"
                      type="text"
                    />
                  </Popconfirm>
                </div>
              ))
            )}
          </div>
        </div>

        <div className={styles.candidatePanel}>
          <div className={styles.candidateTitle}>
            <span>DETECTED CLI TOOLS</span>
            <span>{safeCandidates.length} Agents</span>
          </div>
          <div className={styles.candidateList}>
            {safeCandidates.length === 0 ? (
              <div className={styles.candidateEmpty}>
                {hasConnectedMachine
                  ? '电脑已连接，暂无可用 CLI。请重新运行上方 npx 命令扫描 Claude、Codex、OpenClaw。'
                  : '电脑连接成功后，会在这里显示可用 CLI。你可以基于同一个 CLI 添加多个 Agent。'}
              </div>
            ) : (
              safeCandidates.map((candidate) => {
                const capabilities = parseCapabilities(candidate.capabilities_json);
                return (
                  <div className={styles.candidateItem} key={candidate.id}>
                    <div className={styles.candidateMeta}>
                      <strong>{candidate.name}</strong>
                      <span>
                        {candidate.cli_tool} · {candidate.machine_name}
                        {candidate.version ? ` · ${candidate.version}` : ''}
                      </span>
                      {capabilities.length > 0 && (
                        <div className={styles.candidateTags}>
                          {capabilities.slice(0, 3).map((item) => (
                            <Tag key={item}>{item.length > 16 ? item.slice(0, 16) + '...' : item}</Tag>
                          ))}
                          {capabilities.length > 3 && <Tag>+{capabilities.length - 3}</Tag>}
                        </div>
                      )}
                    </div>
                    <Input
                      maxLength={100}
                      value={candidateNames[candidate.id] ?? candidate.name}
                      onChange={(event) => setCandidateNames((state) => ({
                        ...state,
                        [candidate.id]: event.target.value,
                      }))}
                    />
                    <Button
                      icon={<PlusOutlined />}
                      loading={addingID === candidate.id}
                      type="primary"
                      onClick={() => handleAddCandidate(candidate)}
                    >
                      添加 Agent
                    </Button>
                  </div>
                );
              })
            )}
          </div>
        </div>
      </div>
    </Modal>
  );
};
