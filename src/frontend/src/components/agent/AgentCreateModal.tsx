import React, { useEffect, useMemo, useState } from 'react';
import { Button, Input, Modal, Select, message } from 'antd';
import type { AgentCandidate } from '@/types/agent';
import { getDefaultAgentName } from './agentPresentation';
import styles from './AgentCreateModal.module.css';

interface AgentCreateModalProps {
  open: boolean;
  machineName: string;
  candidates: AgentCandidate[];
  onClose: () => void;
  onCreate: (candidateId: string, name: string, systemPrompt: string) => Promise<void>;
}

export const AgentCreateModal: React.FC<AgentCreateModalProps> = ({
  open,
  machineName,
  candidates,
  onClose,
  onCreate,
}) => {
  const [candidateId, setCandidateId] = useState('');
  const [name, setName] = useState('');
  const [systemPrompt, setSystemPrompt] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const options = useMemo(
    () => candidates.map((candidate) => ({
      value: candidate.id,
      label: `${candidate.name} · ${candidate.cli_tool}`,
    })),
    [candidates],
  );

  useEffect(() => {
    if (!open) return;
    const first = candidates[0];
    setCandidateId(first?.id ?? '');
    setName(first ? getDefaultAgentName(first.name, first.cli_tool) : '');
    setSystemPrompt('');
  }, [open, candidates]);

  const handleCandidateChange = (value: string) => {
    setCandidateId(value);
    const selected = candidates.find((candidate) => candidate.id === value);
    if (selected) {
      setName(getDefaultAgentName(selected.name, selected.cli_tool));
    }
  };

  const handleSubmit = async () => {
    if (!candidateId || !name.trim()) return;
    setSubmitting(true);
    try {
      await onCreate(candidateId, name.trim(), systemPrompt.trim());
      message.success('Agent 已创建');
      onClose();
    } catch {
      message.error('创建 Agent 失败');
    } finally {
      setSubmitting(false);
    }
  };

  const title = machineName ? `创建 Agent · ${machineName}` : '创建 Agent';
  const canSubmit = Boolean(candidateId && name.trim());
  const hasCandidates = options.length > 0;

  return (
    <Modal
      open={open}
      onCancel={onClose}
      footer={null}
      title={title}
      centered
      width={520}
    >
      <div className={styles.content}>
        <div className={styles.field}>
          <span className={styles.label}>底座</span>
          <Select
            className={styles.select}
            placeholder={hasCandidates ? '选择底座' : '当前电脑暂无可用底座'}
            options={options}
            value={candidateId || undefined}
            onChange={handleCandidateChange}
            disabled={!hasCandidates}
          />
        </div>
        <div className={styles.field}>
          <span className={styles.label}>Agent 名称</span>
          <Input
            className={styles.input}
            value={name}
            maxLength={100}
            placeholder="输入一个好记的名字"
            onChange={(event) => setName(event.target.value)}
          />
        </div>
        <div className={styles.field}>
          <span className={styles.label}>人格设定</span>
          <Input.TextArea
            className={styles.textarea}
            value={systemPrompt}
            placeholder="描述你希望这个 Agent 的风格、角色与边界"
            autoSize={{ minRows: 3, maxRows: 6 }}
            onChange={(event) => setSystemPrompt(event.target.value)}
          />
          <span className={styles.helper}>支持空白，后续可在详情中继续调整。</span>
        </div>
        <div className={styles.footer}>
          <Button onClick={onClose}>取消</Button>
          <Button
            type="primary"
            loading={submitting}
            onClick={handleSubmit}
            disabled={!canSubmit || submitting}
          >
            创建
          </Button>
        </div>
      </div>
    </Modal>
  );
};
