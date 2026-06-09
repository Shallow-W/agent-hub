import React, { useEffect, useMemo, useState } from 'react';
import { Button, Checkbox, Input, Modal, Select, Tag, message } from 'antd';
import type { AgentCandidate } from '@/types/agent';
import { getDefaultAgentName, parseSkills } from './agentPresentation';
import {
  getTemplateTools,
  toolCatalog,
  toolsConfigToJSON,
  toolsetOptions,
} from './toolAssignments';
import { AgentPromptTemplateField } from './AgentPromptTemplateField';
import styles from './AgentCreateModal.module.css';

interface AgentCreateModalProps {
  open: boolean;
  machineName: string;
  candidates: AgentCandidate[];
  onClose: () => void;
  onCreate: (candidateId: string, name: string, systemPrompt: string, toolsConfig: string, customSkills: string) => Promise<void>;
}

function skillsInputToJSON(value: string): string {
  const skills = value
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      const [name, ...descriptionParts] = line.split(/\s+-\s+/);
      const description = descriptionParts.join(' - ').trim();
      return {
        name: (name ?? '').trim(),
        description: description || undefined,
        trigger: description || undefined,
      };
    })
    .filter((skill) => skill.name.length > 0);
  return skills.length > 0 ? JSON.stringify(skills) : '';
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
  const [toolset, setToolset] = useState('tasks');
  const [selectedTools, setSelectedTools] = useState<string[]>(getTemplateTools('tasks'));
  const [skillInput, setSkillInput] = useState('');
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
    setToolset('tasks');
    setSelectedTools(getTemplateTools('tasks'));
    setSkillInput('');
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
      const customSkills = skillsInputToJSON(skillInput);
      await onCreate(
        candidateId,
        name.trim(),
        systemPrompt.trim(),
        toolsConfigToJSON(toolset, selectedTools),
        customSkills,
      );
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
  const selectedCandidate = candidates.find((candidate) => candidate.id === candidateId);
  const baseSkills = parseSkills(selectedCandidate?.capabilities_json);

  const handleToolsetChange = (value: string) => {
    setToolset(value);
    if (value !== 'custom') {
      setSelectedTools(getTemplateTools(value));
    }
  };

  const handleToolsChange = (values: string[]) => {
    setToolset('custom');
    setSelectedTools(values);
  };

  return (
    <Modal
      open={open}
      onCancel={onClose}
      footer={null}
      title={title}
      centered
      width={720}
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
          <AgentPromptTemplateField
            open={open}
            value={systemPrompt}
            onChange={setSystemPrompt}
          />
          <span className={styles.helper}>支持空白，后续可在详情中继续调整。</span>
        </div>
        <div className={styles.field}>
          <span className={styles.label}>工具集</span>
          <Select
            className={styles.select}
            value={toolset}
            options={toolsetOptions}
            onChange={handleToolsetChange}
          />
          <Checkbox.Group value={selectedTools} onChange={(values) => handleToolsChange(values as string[])}>
            <div className={styles.toolGrid}>
              {toolCatalog.map((tool) => (
                <label className={styles.toolItem} key={tool.name}>
                  <Checkbox value={tool.name} />
                  <span>
                    <span className={styles.toolName}>{tool.label}</span>
                    <span className={styles.toolMeta}>{tool.name}</span>
                  </span>
                </label>
              ))}
            </div>
          </Checkbox.Group>
        </div>
        <div className={styles.field}>
          <span className={styles.label}>平台 Skills</span>
          {baseSkills.length > 0 && (
            <div className={styles.baseSkills}>
              {baseSkills.slice(0, 6).map((skill) => <Tag key={skill.name}>{skill.name}</Tag>)}
              {baseSkills.length > 6 && <Tag>+{baseSkills.length - 6}</Tag>}
            </div>
          )}
          <Input.TextArea
            className={styles.textarea}
            value={skillInput}
            placeholder="每行一个平台 Skill，例如：代码审查 - 检查 bug 和测试缺口"
            autoSize={{ minRows: 3, maxRows: 6 }}
            onChange={(event) => setSkillInput(event.target.value)}
          />
          <span className={styles.helper}>底座 Skills 只读；平台 Skills 会写入该 Agent 的可分配能力索引，可在详情页继续补充触发条件和详细内容。</span>
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
