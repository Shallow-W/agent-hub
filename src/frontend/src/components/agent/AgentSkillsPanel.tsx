import React, { useEffect, useMemo, useState } from 'react';
import { Avatar, Button, Input, Popconfirm, message } from 'antd';
import {
  RobotOutlined,
  CloseOutlined,
  SaveOutlined,
  EditOutlined,
  FolderOpenOutlined,
  PlusOutlined,
  DownOutlined,
  RightOutlined,
} from '@ant-design/icons';
import type { Agent, PlatformSkill } from '@/types/agent';
import { useAgentStore } from '@/store/agentStore';
import {
  createPlatformSkill,
  deletePlatformSkill,
  getPlatformSkills,
  importDefaultPlatformSkills,
  updatePlatformSkill,
} from '@/api/platformSkill';
import { parseSkills, skillsToPlatformJSON } from './agentPresentation';
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
  const [librarySkills, setLibrarySkills] = useState<PlatformSkill[]>([]);
  const [selectedLibrarySkillID, setSelectedLibrarySkillID] = useState<string | null>(null);
  const [libraryLoading, setLibraryLoading] = useState(false);
  const [importingDefaults, setImportingDefaults] = useState(false);
  const [libraryExpanded, setLibraryExpanded] = useState(true);
  const [baseExpanded, setBaseExpanded] = useState(false);
  const [assignedExpanded, setAssignedExpanded] = useState(true);
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
    setSelectedLibrarySkillID(null);
  }, [agent.id, agent.capabilities_json, agent.custom_skills]);

  useEffect(() => {
    let cancelled = false;
    setLibraryLoading(true);
    getPlatformSkills()
      .then((items) => {
        if (!cancelled) setLibrarySkills(items);
      })
      .catch(() => {
        if (!cancelled) message.error('查询平台 Skill 库失败');
      })
      .finally(() => {
        if (!cancelled) setLibraryLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const selectedSkill = selectedSkillIdx === null ? null : skills[selectedSkillIdx] ?? null;
  const selectedLibrarySkill = selectedLibrarySkillID
    ? librarySkills.find((skill) => skill.id === selectedLibrarySkillID) ?? null
    : null;
  const libraryGroups = useMemo(() => {
    const groups = new Map<string, PlatformSkill[]>();
    librarySkills.forEach((skill) => {
      const category = skill.category?.trim() || '未分类';
      const items = groups.get(category) ?? [];
      items.push(skill);
      groups.set(category, items);
    });
    return Array.from(groups.entries()).map(([category, items]) => ({ category, items }));
  }, [librarySkills]);

  const refreshLibrarySkills = async () => {
    const items = await getPlatformSkills();
    setLibrarySkills(items);
    return items;
  };

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

  const addSkill = (skill: Skill | PlatformSkill) => {
    const name = skill.name.trim();
    if (!name) return;
    if (skills.some((item) => item.name === name)) {
      message.warning('该技能已分配给当前 Agent');
      return;
    }
    const nextSkill: Skill = {
      name,
      category: skill.category,
      description: skill.description,
      trigger: skill.trigger || skill.description,
      detail: skill.detail,
    };
    setSkills((prev) => {
      const next = [...prev, nextSkill];
      setSelectedSkillIdx(next.length - 1);
      setSelectedLibrarySkillID(null);
      return next;
    });
  };

  const toAssignedSkill = (skill: Skill | PlatformSkill): Skill => ({
    name: skill.name.trim(),
    category: skill.category,
    description: skill.description,
    trigger: skill.trigger || skill.description,
    detail: skill.detail,
  });

  const handleAddSkill = () => {
    const name = newSkillName.trim();
    if (!name) return;
    setLibraryLoading(true);
    createPlatformSkill({ name })
      .then((skill) => {
        setLibrarySkills((prev) => [skill, ...prev.filter((item) => item.id !== skill.id)]);
        addSkill(skill);
        setNewSkillName('');
        message.success('平台 Skill 已创建并分配');
      })
      .catch((err) => {
        const errorMessage = err instanceof Error && err.message ? err.message : '创建平台 Skill 失败';
        message.error(errorMessage);
      })
      .finally(() => setLibraryLoading(false));
  };

  const handleImportDefaults = async () => {
    setImportingDefaults(true);
    try {
      const imported = await importDefaultPlatformSkills();
      await refreshLibrarySkills();
      const existingNames = new Set(skills.map((skill) => skill.name.trim()).filter(Boolean));
      const additions = imported
        .filter((skill) => !existingNames.has(skill.name.trim()))
        .map(toAssignedSkill);
      if (additions.length > 0) {
        const nextSkills = [...skills, ...additions];
        setSkills(nextSkills);
        setSelectedSkillIdx(skills.length);
        setSelectedLibrarySkillID(null);
        setAssignedExpanded(true);
        message.success(`已导入并分配 ${additions.length} 个默认平台 Skills，点击保存后生效`);
      } else {
        message.info('默认平台 Skills 已在当前 Agent 的已分配列表中');
      }
    } catch (err) {
      const errorMessage = err instanceof Error && err.message ? err.message : '导入默认平台 Skills 失败';
      message.error(errorMessage);
    } finally {
      setImportingDefaults(false);
    }
  };

  const handleSaveLibrarySkill = async (skill: Skill | PlatformSkill) => {
    const name = skill.name.trim();
    if (!name) {
      message.warning('平台 Skill 名称不能为空');
      return;
    }
    setLibraryLoading(true);
    try {
      const existing = 'id' in skill
        ? librarySkills.find((item) => item.id === skill.id)
        : librarySkills.find((item) => item.name === name);
      const saved = existing
        ? await updatePlatformSkill(existing.id, {
            name,
            description: skill.description,
            trigger: skill.trigger,
            detail: skill.detail,
            category: skill.category,
          })
        : await createPlatformSkill({
            name,
            category: skill.category,
            description: skill.description,
            trigger: skill.trigger,
            detail: skill.detail,
          });
      setLibrarySkills((prev) => [saved, ...prev.filter((item) => item.id !== saved.id)]);
      setSelectedLibrarySkillID(saved.id);
      message.success('平台 Skill 库已更新');
    } catch (err) {
      const errorMessage = err instanceof Error && err.message ? err.message : '保存平台 Skill 库失败';
      message.error(errorMessage);
    } finally {
      setLibraryLoading(false);
    }
  };

  const handleDeleteLibrarySkill = async (skillID: string) => {
    setLibraryLoading(true);
    try {
      await deletePlatformSkill(skillID);
      setLibrarySkills((prev) => prev.filter((item) => item.id !== skillID));
      setSelectedLibrarySkillID((current) => (current === skillID ? null : current));
      message.success('平台 Skill 已删除');
    } catch {
      message.error('删除平台 Skill 失败');
    } finally {
      setLibraryLoading(false);
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

  const updateSelectedLibrarySkill = (patch: Partial<PlatformSkill>) => {
    if (!selectedLibrarySkillID) return;
    setLibrarySkills((prev) =>
      prev.map((skill) => (
        skill.id === selectedLibrarySkillID ? { ...skill, ...patch } : skill
      ))
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
        <div className={styles.workspace}>
          <div className={styles.leftPane}>
            <button className={styles.sectionToggle} type="button" onClick={() => setAssignedExpanded((v) => !v)}>
              {assignedExpanded ? <DownOutlined /> : <RightOutlined />}
              <span className={styles.sectionTitle}>已分配平台 Skills ({skills.length})</span>
            </button>
            {assignedExpanded && (
              <div className={styles.skillList}>
                <div className={styles.sectionToolbar}>
                  <Button size="small" onClick={handleImportDefaults} loading={importingDefaults}>
                    导入默认 Skills
                  </Button>
                </div>
                {skills.length === 0 && (
                  <div className={styles.empty}>暂无已分配平台 Skills，可导入默认 Skills 或从平台库分配</div>
                )}
                {skills.map((skill, idx) => {
                  const isSelected = selectedSkillIdx === idx;
                  return (
                    <div
                      className={`${styles.skillCard} ${isSelected ? styles.skillCardSelected : ''}`}
                      key={`${skill.name}-${idx}`}
                      role="button"
                      tabIndex={0}
                      onClick={() => {
                        setSelectedSkillIdx(idx);
                        setSelectedLibrarySkillID(null);
                      }}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                          setSelectedSkillIdx(idx);
                          setSelectedLibrarySkillID(null);
                        }
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
                            {skill.category && <span className={styles.categoryBadge}>{skill.category}</span>}
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
                          title="编辑分配名称"
                        >
                          <EditOutlined />
                        </button>
                        <button
                          className={styles.iconBtn}
                          type="button"
                          onClick={(e) => { e.stopPropagation(); handleDeleteSkill(idx); }}
                          title="从当前 Agent 移除"
                        >
                          <CloseOutlined />
                        </button>
                      </span>
                    </div>
                  );
                })}
              </div>
            )}

            <button className={styles.sectionToggle} type="button" onClick={() => setLibraryExpanded((v) => !v)}>
              {libraryExpanded ? <DownOutlined /> : <RightOutlined />}
              <span className={styles.sectionTitle}>平台 Skill 库 ({librarySkills.length})</span>
            </button>
            {libraryExpanded && (
              <div className={styles.skillList}>
                {librarySkills.length === 0 && (
                  <div className={styles.empty}>{libraryLoading ? '加载中...' : '暂无平台 Skill，可从上方已分配区导入默认 Skills 或在底部创建'}</div>
                )}
                {libraryGroups.map((group) => (
                  <div className={styles.categoryGroup} key={group.category}>
                    <div className={styles.categoryTitle}>{group.category}</div>
                    {group.items.map((skill) => (
                      <div
                        className={`${styles.skillCard} ${selectedLibrarySkillID === skill.id ? styles.skillCardSelected : ''}`}
                        key={skill.id}
                        role="button"
                        tabIndex={0}
                        onClick={() => {
                          setSelectedLibrarySkillID(skill.id);
                          setSelectedSkillIdx(null);
                        }}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter') {
                            setSelectedLibrarySkillID(skill.id);
                            setSelectedSkillIdx(null);
                          }
                        }}
                      >
                        <div className={styles.skillCardMain}>
                          <span className={styles.skillName}>
                            {skill.name}
                            {skill.category && <span className={styles.categoryBadge}>{skill.category}</span>}
                          </span>
                          <span className={styles.skillSummary}>{skill.description || skill.trigger || '暂无描述'}</span>
                        </div>
                        <span className={styles.skillActionsAlways}>
                          <Button size="small" aria-label="分配平台 Skill" onClick={(e) => { e.stopPropagation(); addSkill(skill); }}>
                            分配
                          </Button>
                          <Popconfirm title="删除这个平台 Skill？" okText="删除" cancelText="取消" onConfirm={() => handleDeleteLibrarySkill(skill.id)}>
                            <button className={styles.iconBtn} type="button" title="删除平台 Skill" onClick={(e) => e.stopPropagation()}>
                              <CloseOutlined />
                            </button>
                          </Popconfirm>
                        </span>
                      </div>
                    ))}
                  </div>
                ))}
              </div>
            )}
          </div>

          <div className={styles.detailPane}>
            {selectedLibrarySkill ? (
              <>
                <div className={styles.detailHeader}>
                  <span className={styles.detailTitle}>平台 Skill 库条目</span>
                </div>
                <label className={styles.field}>
                  <span className={styles.fieldLabel}>名称</span>
                  <Input
                    value={selectedLibrarySkill.name}
                    onChange={(e) => updateSelectedLibrarySkill({ name: e.target.value })}
                    placeholder="平台 Skill 名称"
                  />
                </label>
                <label className={styles.field}>
                  <span className={styles.fieldLabel}>分类</span>
                  <Input
                    value={selectedLibrarySkill.category ?? ''}
                    onChange={(e) => updateSelectedLibrarySkill({ category: e.target.value })}
                    placeholder="例如：产品经理、开发人员"
                  />
                </label>
                <label className={styles.field}>
                  <span className={styles.fieldLabel}>描述</span>
                  <Input.TextArea
                    autoSize={{ minRows: 2, maxRows: 4 }}
                    value={selectedLibrarySkill.description ?? ''}
                    onChange={(e) => updateSelectedLibrarySkill({ description: e.target.value })}
                    placeholder="写这个 skill 解决什么问题、什么时候用"
                  />
                </label>
                <label className={styles.field}>
                  <span className={styles.fieldLabel}>触发条件</span>
                  <Input
                    value={selectedLibrarySkill.trigger ?? ''}
                    onChange={(e) => updateSelectedLibrarySkill({ trigger: e.target.value })}
                    placeholder="例如：代码审查、权限检查、写测试时使用"
                  />
                </label>
                <label className={styles.field}>
                  <span className={styles.fieldLabel}>详细内容 / 代码</span>
                  <Input.TextArea
                    autoSize={{ minRows: 12, maxRows: 20 }}
                    value={selectedLibrarySkill.detail ?? ''}
                    onChange={(e) => updateSelectedLibrarySkill({ detail: e.target.value })}
                    placeholder="把 coding 的详细规则、提示词、脚本或代码片段写在这里"
                    className={styles.detailInput}
                  />
                </label>
                <div className={styles.detailActions}>
                  <Button onClick={() => addSkill(selectedLibrarySkill)}>
                    分配给当前 Agent
                  </Button>
                  <Button type="primary" onClick={() => handleSaveLibrarySkill(selectedLibrarySkill)} loading={libraryLoading}>
                    保存平台 Skill 库
                  </Button>
                </div>
              </>
            ) : selectedSkill ? (
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
                  <span className={styles.fieldLabel}>分类</span>
                  <Input
                    value={selectedSkill.category ?? ''}
                    onChange={(e) => updateSelectedSkill({ category: e.target.value })}
                    placeholder="例如：产品经理、开发人员"
                  />
                </label>
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
                <div className={styles.detailActions}>
                  <Button onClick={() => handleSaveLibrarySkill(selectedSkill)} loading={libraryLoading}>
                    保存到平台 Skill 库
                  </Button>
                </div>
              </>
            ) : (
              <div className={styles.detailEmpty}>选择左侧已分配 Skill 或平台 Skill 库条目后，在这里查看和编辑详细内容</div>
            )}
          </div>
        </div>
      </div>

      <div className={styles.baseSkills}>
        <button className={styles.sectionToggle} type="button" onClick={() => setBaseExpanded((v) => !v)}>
          {baseExpanded ? <DownOutlined /> : <RightOutlined />}
          <span className={styles.sectionTitle}>底座 Skills 只读 ({baseSkills.length})</span>
        </button>
        {baseExpanded && baseSkills.length === 0 ? (
          <div className={styles.empty}>当前 Agent 底座没有上报本地 Skills</div>
        ) : null}
        {baseExpanded && baseSkills.length > 0 ? (
          <div className={styles.baseSkillGrid}>
            {baseSkills.map((skill, idx) => (
              <div className={styles.baseSkillCard} key={`${skill.name}-${idx}`}>
                <div className={styles.baseSkillInfo}>
                  <span className={styles.skillName}>{skill.name}</span>
                  <span className={styles.skillSummary}>{skill.description || '暂无描述'}</span>
                </div>
                <Button size="small" onClick={() => handleSaveLibrarySkill(skill)}>
                  入库
                </Button>
              </div>
            ))}
          </div>
        ) : null}
      </div>

      <div className={styles.footer}>
        <div className={styles.addRow}>
          <Input
            placeholder="输入新平台 Skill 名称，创建到平台库并分配给当前 Agent"
            value={newSkillName}
            onChange={(e) => setNewSkillName(e.target.value)}
            onPressEnter={handleAddSkill}
          />
          <Button icon={<PlusOutlined />} onClick={handleAddSkill}>
            创建并分配
          </Button>
        </div>
        <Button type="primary" icon={<SaveOutlined />} loading={saving} onClick={handleSave}>
          保存
        </Button>
      </div>
    </div>
  );
};
