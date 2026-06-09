import React, { useEffect, useState } from 'react';
import { Avatar, Button, Input, message } from 'antd';
import {
  RobotOutlined,
  CloseOutlined,
  SaveOutlined,
  EditOutlined,
  FolderOpenOutlined,
  PlusOutlined,
  StarOutlined,
} from '@ant-design/icons';
import type { Agent } from '@/types/agent';
import { useAgentStore } from '@/store/agentStore';
import { autoGenerateSkills, parseSkills, skillsToPlatformJSON } from './agentPresentation';
import type { Skill } from './agentPresentation';
import styles from './AgentSkillsPanel.module.css';

interface AgentSkillsPanelProps {
  agent: Agent;
}

export const AgentSkillsPanel: React.FC<AgentSkillsPanelProps> = ({ agent }) => {
  const updateCustomSkills = useAgentStore((s) => s.updateCustomSkills);
  const openSkillLocation = useAgentStore((s) => s.openSkillLocation);
  const [skills, setSkills] = useState<Skill[]>([]);
  const [baseSkills, setBaseSkills] = useState<Skill[]>([]);
  const [editingSkillIdx, setEditingSkillIdx] = useState<number | null>(null);
  const [editingSkillName, setEditingSkillName] = useState('');
  const [selectedSkillIdx, setSelectedSkillIdx] = useState<number | null>(null);
  const [newSkillName, setNewSkillName] = useState('');
  const [saving, setSaving] = useState(false);
  const [openingPath, setOpeningPath] = useState(false);

  useEffect(() => {
    const nextSkills = parseSkills(agent.custom_skills);
    setBaseSkills(parseSkills(agent.capabilities_json));
    setSkills(nextSkills);
    setEditingSkillIdx(null);
    setEditingSkillName('');
    setNewSkillName('');
    setSelectedSkillIdx(nextSkills.length > 0 ? 0 : null);
  }, [agent.id, agent.capabilities_json, agent.custom_skills]);

  const selectedSkill = selectedSkillIdx === null ? null : skills[selectedSkillIdx] ?? null;

  const handleSave = async () => {
    setSaving(true);
    try {
      await updateCustomSkills(agent.id, skillsToPlatformJSON(skills));
      message.success('平台 Skills 已保存');
    } catch {
      message.error('保存失败');
    } finally {
      setSaving(false);
    }
  };

  const addSkill = (skill: Skill) => {
    const name = skill.name.trim();
    if (!name) return;
    if (skills.some((item) => item.name === name)) {
      message.warning('该技能已分配给当前 Agent');
      return;
    }
    const nextSkill: Skill = {
      name,
      description: skill.description,
      trigger: skill.trigger || skill.description,
      detail: skill.detail,
    };
    setSkills((prev) => {
      const next = [...prev, nextSkill];
      setSelectedSkillIdx(next.length - 1);
      return next;
    });
  };

  const handleAddSkill = () => {
    addSkill({ name: newSkillName });
    setNewSkillName('');
  };

  const handleAutoGenerate = () => {
    const generated = autoGenerateSkills(agent);
    const existing = new Set(skills.map((skill) => skill.name));
    const additions = generated.filter((skill) => !existing.has(skill.name));
    if (additions.length === 0) {
      message.info('当前 Agent 已包含自动生成的平台 Skills');
      return;
    }
    setSkills((prev) => [...prev, ...additions.map((skill) => ({
      name: skill.name,
      description: skill.description,
      trigger: skill.description,
      detail: skill.detail,
    }))]);
    setSelectedSkillIdx(skills.length);
    message.success('已生成平台 Skills');
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

  const handleOpenLocation = async () => {
    if (!selectedSkill?.source_path) return;
    setOpeningPath(true);
    try {
      await openSkillLocation(agent.id, selectedSkill.source_path);
      message.success('已打开所在文件夹');
    } catch (err) {
      const errorMessage = err instanceof Error && err.message
        ? err.message
        : '打开所在文件夹失败，请确认电脑 daemon 在线';
      message.error(errorMessage);
    } finally {
      setOpeningPath(false);
    }
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
          <span className={styles.sectionTitle}>平台 Skills ({skills.length})</span>
          <div className={styles.headerActions}>
            <Button size="small" icon={<StarOutlined />} onClick={handleAutoGenerate}>
              自动生成
            </Button>
          </div>
        </div>

        <div className={styles.workspace}>
          <div className={styles.skillList}>
            {skills.length === 0 && (
              <div className={styles.empty}>暂无平台 Skills</div>
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
                      {skill.description || skill.trigger || '暂无描述'}
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
                {selectedSkill.source_path && (
                  <div className={styles.sourcePath}>
                    <div className={styles.sourcePathText}>
                      <span className={styles.fieldLabel}>真实路径</span>
                      <span title={selectedSkill.source_path}>{selectedSkill.source_path}</span>
                    </div>
                    <Button
                      size="small"
                      icon={<FolderOpenOutlined />}
                      loading={openingPath}
                      onClick={handleOpenLocation}
                    >
                      打开所在文件夹
                    </Button>
                  </div>
                )}
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
                  <span className={styles.fieldLabel}>触发条件</span>
                  <Input
                    value={selectedSkill.trigger ?? ''}
                    onChange={(e) => updateSelectedSkill({ trigger: e.target.value })}
                    placeholder="例如：代码审查、权限检查、写测试时使用"
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
              <div className={styles.detailEmpty}>选择左侧平台 Skill 后，在这里查看和编辑详细内容</div>
            )}
          </div>
        </div>
      </div>

      <div className={styles.baseSkills}>
        <div className={styles.skillsHeader}>
          <span className={styles.sectionTitle}>底座 Skills 只读 ({baseSkills.length})</span>
        </div>
        {baseSkills.length === 0 ? (
          <div className={styles.empty}>当前 Agent 底座没有上报本地 Skills</div>
        ) : (
          <div className={styles.baseSkillGrid}>
            {baseSkills.map((skill, idx) => (
              <div className={styles.baseSkillCard} key={`${skill.name}-${idx}`}>
                <div className={styles.baseSkillInfo}>
                  <span className={styles.skillName}>{skill.name}</span>
                  <span className={styles.skillSummary}>{skill.description || '暂无描述'}</span>
                </div>
                <Button size="small" onClick={() => addSkill(skill)}>
                  分配
                </Button>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className={styles.footer}>
        <div className={styles.addRow}>
          <Input
            placeholder="输入新平台 Skill 名称"
            value={newSkillName}
            onChange={(e) => setNewSkillName(e.target.value)}
            onPressEnter={handleAddSkill}
          />
          <Button icon={<PlusOutlined />} onClick={handleAddSkill}>
            添加
          </Button>
        </div>
        <Button type="primary" icon={<SaveOutlined />} loading={saving} onClick={handleSave}>
          保存
        </Button>
      </div>
    </div>
  );
};
