import React, { useEffect, useState } from 'react';
import { Avatar, Button, Input, message } from 'antd';
import {
  RobotOutlined,
  CloseOutlined,
  SaveOutlined,
  EditOutlined,
} from '@ant-design/icons';
import type { Agent } from '@/types/agent';
import { useAgentStore } from '@/store/agentStore';
import { parseSkills } from './agentPresentation';
import type { Skill } from './agentPresentation';
import styles from './AgentSkillsPanel.module.css';

interface AgentSkillsPanelProps {
  agent: Agent;
}

export const AgentSkillsPanel: React.FC<AgentSkillsPanelProps> = ({ agent }) => {
  const updateAgent = useAgentStore((s) => s.updateAgent);
  const [skills, setSkills] = useState<Skill[]>([]);
  const [editingSkillIdx, setEditingSkillIdx] = useState<number | null>(null);
  const [editingSkillName, setEditingSkillName] = useState('');
  const [selectedSkillIdx, setSelectedSkillIdx] = useState<number | null>(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    const nextSkills = parseSkills(agent.capabilities_json);
    setSkills(nextSkills);
    setEditingSkillIdx(null);
    setEditingSkillName('');
    setSelectedSkillIdx(nextSkills.length > 0 ? 0 : null);
  }, [agent.id]);

  const selectedSkill = selectedSkillIdx === null ? null : skills[selectedSkillIdx] ?? null;

  const handleSave = async () => {
    setSaving(true);
    try {
      await updateAgent(agent.id, {
        name: agent.name,
        cli_tool: agent.cli_tool,
        avatar: agent.avatar ?? undefined,
        system_prompt: agent.system_prompt ?? undefined,
        capabilities_json: JSON.stringify(skills),
      });
      message.success('技能已保存');
    } catch {
      message.error('保存失败');
    } finally {
      setSaving(false);
    }
  };

  const handleDeleteSkill = (idx: number) => {
    setSkills((prev) => {
      const next = prev.filter((_, i) => i !== idx);
      setSelectedSkillIdx((current) => {
        if (next.length === 0) return null;
        if (current === null) return 0;
        if (current === idx) return Math.min(idx, next.length - 1);
        if (current > idx) return current - 1;
        return current;
      });
      return next;
    });
  };

  const handleStartEditName = (idx: number) => {
    setEditingSkillIdx(idx);
    setEditingSkillName(skills[idx]?.name ?? '');
  };

  const handleCommitEditName = () => {
    if (editingSkillIdx === null) return;
    const trimmed = editingSkillName.trim();
    if (trimmed) {
      setSkills((prev) =>
        prev.map((s, i) => (i === editingSkillIdx ? { ...s, name: trimmed } : s))
      );
    }
    setEditingSkillIdx(null);
    setEditingSkillName('');
  };

  const updateSelectedSkill = (patch: Partial<Skill>) => {
    if (selectedSkillIdx === null) return;
    setSkills((prev) =>
      prev.map((skill, idx) => (idx === selectedSkillIdx ? { ...skill, ...patch } : skill))
    );
  };

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <Avatar size={40} src={agent.avatar || undefined} icon={<RobotOutlined />} className={styles.avatar} />
        <div className={styles.headerInfo}>
          <span className={styles.name}>{agent.name}</span>
          <span className={styles.cliTool}>@{agent.cli_tool}</span>
        </div>
      </div>

      <div className={styles.body}>
        <div className={styles.skillsHeader}>
          <span className={styles.sectionTitle}>SKILLS ({skills.length})</span>
        </div>

        <div className={styles.workspace}>
          <div className={styles.skillList}>
            {skills.length === 0 && (
              <div className={styles.empty}>暂无技能</div>
            )}
            {skills.map((skill, idx) => {
              const isSelected = selectedSkillIdx === idx;
              return (
                <div
                  className={`${styles.skillCard} ${isSelected ? styles.skillCardSelected : ''}`}
                  key={`${skill.name}-${idx}`}
                  role="button"
                  tabIndex={0}
                  onClick={() => setSelectedSkillIdx(idx)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') setSelectedSkillIdx(idx);
                  }}
                >
                  <div className={styles.skillCardMain}>
                    {editingSkillIdx === idx ? (
                      <Input
                        autoFocus
                        size="small"
                        value={editingSkillName}
                        onChange={(e) => setEditingSkillName(e.target.value)}
                        onBlur={handleCommitEditName}
                        onPressEnter={handleCommitEditName}
                        onClick={(e) => e.stopPropagation()}
                        className={styles.skillNameInput}
                      />
                    ) : (
                      <span className={styles.skillName}>
                        {skill.name}
                        {skill.auto && <span className={styles.autoBadge}>auto</span>}
                      </span>
                    )}
                    <span className={styles.skillSummary}>
                      {skill.description || skill.detail || '暂无详细内容'}
                    </span>
                  </div>
                  <span className={styles.skillActions}>
                    <button
                      className={styles.iconBtn}
                      type="button"
                      onClick={(e) => { e.stopPropagation(); handleStartEditName(idx); }}
                      title="编辑名称"
                    >
                      <EditOutlined />
                    </button>
                    <button
                      className={styles.iconBtn}
                      type="button"
                      onClick={(e) => { e.stopPropagation(); handleDeleteSkill(idx); }}
                      title="删除技能"
                    >
                      <CloseOutlined />
                    </button>
                  </span>
                </div>
              );
            })}
          </div>

          <div className={styles.detailPane}>
            {selectedSkill ? (
              <>
                <div className={styles.detailHeader}>
                  <span className={styles.detailTitle}>{selectedSkill.name}</span>
                  {selectedSkill.auto && <span className={styles.autoBadge}>auto</span>}
                </div>
                <label className={styles.field}>
                  <span className={styles.fieldLabel}>描述</span>
                  <Input.TextArea
                    autoSize={{ minRows: 2, maxRows: 4 }}
                    value={selectedSkill.description ?? ''}
                    onChange={(e) => updateSelectedSkill({ description: e.target.value })}
                    placeholder="写这个 skill 解决什么问题、什么时候用"
                  />
                </label>
                <label className={styles.field}>
                  <span className={styles.fieldLabel}>详细内容 / 代码</span>
                  <Input.TextArea
                    autoSize={{ minRows: 12, maxRows: 20 }}
                    value={selectedSkill.detail ?? ''}
                    onChange={(e) => updateSelectedSkill({ detail: e.target.value })}
                    placeholder="把 coding 的详细规则、提示词、脚本或代码片段写在这里"
                    className={styles.detailInput}
                  />
                </label>
              </>
            ) : (
              <div className={styles.detailEmpty}>选择左侧技能后，在这里查看和编辑详细内容</div>
            )}
          </div>
        </div>
      </div>

      <div className={styles.footer}>
        <Button type="primary" icon={<SaveOutlined />} loading={saving} onClick={handleSave}>
          保存
        </Button>
      </div>
    </div>
  );
};
