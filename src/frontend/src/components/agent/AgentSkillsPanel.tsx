import React, { useEffect, useMemo, useState } from 'react';
import { Avatar, Button, Drawer, Input, Popconfirm } from 'antd';
import { message } from '@/utils/message';
import {
  RobotOutlined,
  SaveOutlined,
  FolderOpenOutlined,
  PlusOutlined,
  DownOutlined,
  RightOutlined,
  SearchOutlined,
  CloseOutlined,
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
import { parseSkills, resolveAgentAvatar, skillsToPlatformJSON } from './agentPresentation';
import type { Skill } from './agentPresentation';
import styles from './AgentSkillsPanel.module.css';

interface AgentSkillsPanelProps {
  agent: Agent;
}

type SkillTab = 'assigned' | 'library';

export const AgentSkillsPanel: React.FC<AgentSkillsPanelProps> = ({ agent }) => {
  const updateCustomSkills = useAgentStore((s) => s.updateCustomSkills);
  const openSkillLocation = useAgentStore((s) => s.openSkillLocation);
  const [skills, setSkills] = useState<Skill[]>([]);
  const [baseSkills, setBaseSkills] = useState<Skill[]>([]);
  const [selectedSkillIdx, setSelectedSkillIdx] = useState<number | null>(null);
  const [newSkillName, setNewSkillName] = useState('');
  const [librarySkills, setLibrarySkills] = useState<PlatformSkill[]>([]);
  const [selectedLibrarySkillID, setSelectedLibrarySkillID] = useState<string | null>(null);
  const [libraryLoading, setLibraryLoading] = useState(false);
  const [importingDefaults, setImportingDefaults] = useState(false);
  const [baseExpanded, setBaseExpanded] = useState(false);
  const [saving, setSaving] = useState(false);
  const [openingPath, setOpeningPath] = useState(false);
  const [activeTab, setActiveTab] = useState<SkillTab>('assigned');
  const [searchQuery, setSearchQuery] = useState('');
  const [categoryFilter, setCategoryFilter] = useState<string>('all');
  const [detailOpen, setDetailOpen] = useState(false);

  useEffect(() => {
    const nextSkills = parseSkills(agent.custom_skills);
    setBaseSkills(parseSkills(agent.capabilities_json));
    setSkills(nextSkills);
    setSelectedSkillIdx(null);
    setNewSkillName('');
    setSelectedLibrarySkillID(null);
    setDetailOpen(false);
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
    return () => { cancelled = true; };
  }, []);

  const selectedSkill = selectedSkillIdx !== null ? skills[selectedSkillIdx] ?? null : null;
  const selectedLibrarySkill = selectedLibrarySkillID
    ? librarySkills.find((s) => s.id === selectedLibrarySkillID) ?? null
    : null;

  const assignedCategories = useMemo(() => {
    const cats = new Set<string>();
    skills.forEach((s) => cats.add(s.category?.trim() || '未分类'));
    return Array.from(cats);
  }, [skills]);

  const filteredAssignedSkills = useMemo(() => {
    let list = skills;
    if (categoryFilter !== 'all') {
      list = list.filter((s) => (s.category?.trim() || '未分类') === categoryFilter);
    }
    if (searchQuery.trim()) {
      const q = searchQuery.trim().toLowerCase();
      list = list.filter(
        (s) =>
          s.name.toLowerCase().includes(q) ||
          (s.description ?? '').toLowerCase().includes(q) ||
          (s.category ?? '').toLowerCase().includes(q),
      );
    }
    return list;
  }, [skills, categoryFilter, searchQuery]);

  const categories = useMemo(() => {
    const cats = new Set<string>();
    librarySkills.forEach((s) => cats.add(s.category?.trim() || '未分类'));
    return Array.from(cats);
  }, [librarySkills]);

  const filteredLibrarySkills = useMemo(() => {
    let list = librarySkills;
    if (categoryFilter !== 'all') {
      list = list.filter((s) => (s.category?.trim() || '未分类') === categoryFilter);
    }
    if (searchQuery.trim()) {
      const q = searchQuery.trim().toLowerCase();
      list = list.filter(
        (s) =>
          s.name.toLowerCase().includes(q) ||
          (s.description ?? '').toLowerCase().includes(q) ||
          (s.category ?? '').toLowerCase().includes(q),
      );
    }
    return list;
  }, [librarySkills, categoryFilter, searchQuery]);

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
    message.success(`已分配「${name}」`);
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
      if (selectedLibrarySkillID === skillID) setDetailOpen(false);
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
      if (selectedSkillIdx === idx) setDetailOpen(false);
      return next;
    });
  };

  const updateSelectedSkill = (patch: Partial<Skill>) => {
    if (selectedSkillIdx === null) return;
    setSkills((prev) =>
      prev.map((skill, idx) => (idx === selectedSkillIdx ? { ...skill, ...patch } : skill)),
    );
  };

  const updateSelectedLibrarySkill = (patch: Partial<PlatformSkill>) => {
    if (!selectedLibrarySkillID) return;
    setLibrarySkills((prev) =>
      prev.map((skill) => (
        skill.id === selectedLibrarySkillID ? { ...skill, ...patch } : skill
      )),
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

  const openAssignedDetail = (idx: number) => {
    setSelectedSkillIdx(idx);
    setSelectedLibrarySkillID(null);
    setDetailOpen(true);
  };

  const openLibraryDetail = (id: string) => {
    setSelectedLibrarySkillID(id);
    setSelectedSkillIdx(null);
    setDetailOpen(true);
  };

  const closeDetail = () => {
    setDetailOpen(false);
    setSelectedSkillIdx(null);
    setSelectedLibrarySkillID(null);
  };

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <Avatar size={40} src={resolveAgentAvatar(agent)} icon={<RobotOutlined />} className={styles.avatar} />
        <div className={styles.headerInfo}>
          <span className={styles.name}>{agent.name}</span>
          <span className={styles.cliTool}>@{agent.cli_tool}</span>
        </div>
        <div className={styles.headerActions}>
          <Button size="small" icon={<PlusOutlined />} onClick={handleAddSkill} disabled={!newSkillName.trim()}>
            创建并分配
          </Button>
          <Button size="small" onClick={handleImportDefaults} loading={importingDefaults}>
            导入默认
          </Button>
          <Button size="small" type="primary" icon={<SaveOutlined />} loading={saving} onClick={handleSave}>
            保存分配
          </Button>
        </div>
      </div>

      <div className={styles.quickCreateRow}>
        <Input
          placeholder="输入新 Skill 名称，然后点击「创建并分配」"
          value={newSkillName}
          onChange={(e) => setNewSkillName(e.target.value)}
          onPressEnter={handleAddSkill}
        />
      </div>

      <div className={styles.overviewStrip}>
        <div className={styles.overviewItem}>
          <span className={styles.overviewLabel}>已分配</span>
          <strong className={styles.overviewValue}>{skills.length}</strong>
        </div>
        <div className={styles.overviewItem}>
          <span className={styles.overviewLabel}>平台库</span>
          <strong className={styles.overviewValue}>{librarySkills.length}</strong>
        </div>
        <div className={styles.overviewItem}>
          <span className={styles.overviewLabel}>底座只读</span>
          <strong className={styles.overviewValue}>{baseSkills.length}</strong>
        </div>
      </div>

      <div className={styles.subTabs}>
        <button
          className={`${styles.subTab} ${activeTab === 'assigned' ? styles.subTabActive : ''}`}
          type="button"
          onClick={() => setActiveTab('assigned')}
        >
          已分配 Skills <span className={styles.subTabCount}>{skills.length}</span>
        </button>
        <button
          className={`${styles.subTab} ${activeTab === 'library' ? styles.subTabActive : ''}`}
          type="button"
          onClick={() => setActiveTab('library')}
        >
          平台库 <span className={styles.subTabCount}>{librarySkills.length}</span>
        </button>
      </div>

      {activeTab === 'assigned' && (
        <>
          <div className={styles.libraryToolbar}>
            <Input
              prefix={<SearchOutlined />}
              placeholder="搜索已分配 Skill 名称、描述..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className={styles.searchInput}
              allowClear
            />
            <div className={styles.categoryPills}>
              <button
                className={`${styles.filterPill} ${categoryFilter === 'all' ? styles.filterPillActive : ''}`}
                type="button"
                onClick={() => setCategoryFilter('all')}
              >
                全部
              </button>
              {assignedCategories.map((cat) => (
                <button
                  className={`${styles.filterPill} ${categoryFilter === cat ? styles.filterPillActive : ''}`}
                  key={cat}
                  type="button"
                  onClick={() => setCategoryFilter(cat)}
                >
                  {cat}
                </button>
              ))}
            </div>
          </div>
          <div className={styles.cardGrid}>
            {skills.length === 0 && (
              <div className={styles.emptyPanel}>
                <span className={styles.emptyTitle}>还没有已分配 Skill</span>
                <span className={styles.emptyText}>先导入默认 Skills，或切换到平台库挑选后分配给当前 Agent。</span>
                <div className={styles.emptyActions}>
                  <Button size="small" onClick={handleImportDefaults} loading={importingDefaults}>
                    导入默认 Skills
                  </Button>
                  <Button size="small" onClick={() => setActiveTab('library')}>
                    查看平台库
                  </Button>
                </div>
              </div>
            )}
            {skills.length > 0 && filteredAssignedSkills.length === 0 && (
              <div className={styles.emptyPanel}>
                <span className={styles.emptyTitle}>没有匹配的 Skill</span>
                <span className={styles.emptyText}>尝试调整搜索条件或分类筛选</span>
              </div>
            )}
            {filteredAssignedSkills.map((skill) => {
              const idx = skills.indexOf(skill);
              return (
                <div
                  className={`${styles.skillCard} ${selectedSkillIdx === idx ? styles.skillCardSelected : ''}`}
                  key={`${skill.name}-${idx}`}
                  role="button"
                  tabIndex={0}
                  onClick={() => openAssignedDetail(idx)}
                  onKeyDown={(e) => { if (e.key === 'Enter') openAssignedDetail(idx); }}
                >
                  <div className={styles.skillCardHeader}>
                    <span className={styles.skillCardName}>{skill.name}</span>
                    <span className={styles.skillCardActions}>
                      <button
                        className={styles.iconBtn}
                        type="button"
                        onClick={(e) => { e.stopPropagation(); handleDeleteSkill(idx); }}
                        title="移除"
                      >
                        <CloseOutlined />
                      </button>
                    </span>
                  </div>
                  <span className={styles.skillCardDesc}>{skill.description || skill.trigger || '暂无描述'}</span>
                  <div className={styles.skillCardFooter}>
                    {skill.category && <span className={styles.categoryBadge}>{skill.category}</span>}
                    {skill.auto && <span className={styles.autoBadge}>auto</span>}
                  </div>
                </div>
              );
            })}
          </div>
        </>
      )}

      {activeTab === 'library' && (
        <>
          <div className={styles.libraryToolbar}>
            <Input
              prefix={<SearchOutlined />}
              placeholder="搜索 Skill 名称、描述..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className={styles.searchInput}
              allowClear
            />
            <div className={styles.categoryPills}>
              <button
                className={`${styles.filterPill} ${categoryFilter === 'all' ? styles.filterPillActive : ''}`}
                type="button"
                onClick={() => setCategoryFilter('all')}
              >
                全部
              </button>
              {categories.map((cat) => (
                <button
                  className={`${styles.filterPill} ${categoryFilter === cat ? styles.filterPillActive : ''}`}
                  key={cat}
                  type="button"
                  onClick={() => setCategoryFilter(cat)}
                >
                  {cat}
                </button>
              ))}
            </div>
          </div>
          <div className={styles.cardGrid}>
            {filteredLibrarySkills.length === 0 && (
              <div className={styles.emptyPanel}>
                <span className={styles.emptyTitle}>
                  {librarySkills.length === 0 ? '平台库暂无 Skill' : '没有匹配的 Skill'}
                </span>
                <span className={styles.emptyText}>
                  {librarySkills.length === 0
                    ? '点击「导入默认」创建基础 Skill 模板'
                    : '尝试调整搜索条件或分类筛选'}
                </span>
              </div>
            )}
            {filteredLibrarySkills.map((skill) => (
              <div
                className={`${styles.skillCard} ${selectedLibrarySkillID === skill.id ? styles.skillCardSelected : ''}`}
                key={skill.id}
                role="button"
                tabIndex={0}
                onClick={() => openLibraryDetail(skill.id)}
                onKeyDown={(e) => { if (e.key === 'Enter') openLibraryDetail(skill.id); }}
              >
                <div className={styles.skillCardHeader}>
                  <span className={styles.skillCardName}>{skill.name}</span>
                  <span className={styles.skillCardActions}>
                    <Button
                      size="small"
                      onClick={(e) => { e.stopPropagation(); addSkill(skill); }}
                    >
                      分配
                    </Button>
                  </span>
                </div>
                <span className={styles.skillCardDesc}>{skill.description || skill.trigger || '暂无描述'}</span>
                <div className={styles.skillCardFooter}>
                  {skill.category && <span className={styles.categoryBadge}>{skill.category}</span>}
                </div>
              </div>
            ))}
          </div>
        </>
      )}

      <div className={styles.baseSkills}>
        <button className={styles.sectionToggle} type="button" onClick={() => setBaseExpanded((v) => !v)}>
          {baseExpanded ? <DownOutlined /> : <RightOutlined />}
          <span className={styles.sectionTitleBlock}>
            <span className={styles.sectionTitle}>底座只读</span>
            <span className={styles.sectionDescription}>本地 Agent 上报的原始 Skills，可入库后再编辑</span>
          </span>
          <span className={styles.sectionCount}>{baseSkills.length}</span>
        </button>
        {baseExpanded && baseSkills.length === 0 && (
          <div className={styles.empty}>当前 Agent 底座没有上报本地 Skills</div>
        )}
        {baseExpanded && baseSkills.length > 0 && (
          <div className={styles.baseSkillGrid}>
            {baseSkills.map((skill, idx) => (
              <div className={styles.baseSkillCard} key={`${skill.name}-${idx}`}>
                <div className={styles.baseSkillInfo}>
                  <span className={styles.skillCardName}>{skill.name}</span>
                  <span className={styles.skillCardDesc}>{skill.description || '暂无描述'}</span>
                </div>
                <Button size="small" onClick={() => handleSaveLibrarySkill(skill)}>
                  入库
                </Button>
              </div>
            ))}
          </div>
        )}
      </div>

      <Drawer
        title={selectedLibrarySkill ? '平台库 Skill 详情' : selectedSkill ? '已分配 Skill 详情' : 'Skill 详情'}
        placement="right"
        width={460}
        open={detailOpen}
        onClose={closeDetail}
        destroyOnClose
      >
        {selectedLibrarySkill && (
          <>
            <div className={styles.drawerCategory}>
              {selectedLibrarySkill.category || '未分类'}
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
              <Popconfirm title="删除这个平台 Skill？" okText="删除" cancelText="取消" onConfirm={() => handleDeleteLibrarySkill(selectedLibrarySkill.id)}>
                <Button danger>删除</Button>
              </Popconfirm>
              <Button type="primary" onClick={() => handleSaveLibrarySkill(selectedLibrarySkill)} loading={libraryLoading}>
                保存
              </Button>
            </div>
          </>
        )}

        {selectedSkill && (
          <>
            <div className={styles.drawerCategory}>
              {selectedSkill.category || '未分类'}
              {selectedSkill.auto && <span className={styles.autoBadge} style={{ marginLeft: 8 }}>auto</span>}
            </div>
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
            <div className={styles.detailActions}>
              <Button onClick={() => handleSaveLibrarySkill(selectedSkill)} loading={libraryLoading}>
                保存到平台库
              </Button>
            </div>
          </>
        )}
      </Drawer>
    </div>
  );
};
