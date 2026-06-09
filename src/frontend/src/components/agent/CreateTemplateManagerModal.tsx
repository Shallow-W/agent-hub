import React, { useEffect, useMemo, useState } from 'react';
import { Button, Input, Modal, Popconfirm } from 'antd';
import { DeleteOutlined, PlusOutlined, SaveOutlined } from '@ant-design/icons';
import { message } from '@/utils/message';
import type { PlatformSkill } from '@/types/agent';
import {
  categoryMeta,
  categoryOrder,
  toolCatalog,
} from './toolAssignments';
import styles from './CreateTemplateManagerModal.module.css';

interface SavedCreateTemplate {
  id: string;
  name: string;
  tools: string[];
  skillIds: string[];
  createdAt: number;
}

interface BuiltInTemplate {
  key: string;
  name: string;
  category: string;
  description: string;
  tools: string[];
  skillCategories: string[];
}

interface TemplateItem {
  id: string;
  name: string;
  category: string;
  description: string;
  tools: string[];
  skillIds: string[];
  builtin: boolean;
}

const STORAGE_KEY_TOOLS = 'agenthub-create-templates-tools';
const STORAGE_KEY_SKILLS = 'agenthub-create-templates-skills';

function loadSaved(mode: 'tools' | 'skills'): SavedCreateTemplate[] {
  try {
    return JSON.parse(localStorage.getItem(mode === 'tools' ? STORAGE_KEY_TOOLS : STORAGE_KEY_SKILLS) || '[]');
  } catch { return []; }
}

function persistSaved(mode: 'tools' | 'skills', list: SavedCreateTemplate[]) {
  localStorage.setItem(mode === 'tools' ? STORAGE_KEY_TOOLS : STORAGE_KEY_SKILLS, JSON.stringify(list));
}

interface CreateTemplateManagerModalProps {
  open: boolean;
  mode: 'tools' | 'skills';
  currentTools: string[];
  currentSkillIds: Set<string>;
  librarySkills: PlatformSkill[];
  onApply: (tools: string[], skillIds: string[]) => void;
  onClose: () => void;
}

export const CreateTemplateManagerModal: React.FC<CreateTemplateManagerModalProps> = ({
  open,
  mode,
  currentTools,
  currentSkillIds,
  librarySkills,
  onApply,
  onClose,
}) => {
  const [saved, setSaved] = useState<SavedCreateTemplate[]>(() => loadSaved(mode));
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [draftName, setDraftName] = useState('');
  const [draftTools, setDraftTools] = useState<string[]>([]);
  const [draftSkillIds, setDraftSkillIds] = useState<Set<string>>(new Set());
  const [toolSearch, setToolSearch] = useState('');
  const [skillSearch, setSkillSearch] = useState('');
  const [toolFilter, setToolFilter] = useState('all');
  const [skillFilter, setSkillFilter] = useState('all');

  const builtInTemplates: BuiltInTemplate[] = useMemo(() => {
    if (mode === 'tools') {
      return [
        { key: 'tpl-tools-full', name: '全量工具', category: '快速', description: `全部 ${toolCatalog.length} 个工具`, tools: toolCatalog.map((t) => t.name), skillCategories: [] },
        { key: 'tpl-tools-tasks', name: '任务协作', category: '快速', description: '任务相关工具集', tools: ['list_group_agents', 'get_messages', 'get_agent_skill', 'list_tasks', 'create_task', 'update_task', 'move_task_status'], skillCategories: [] },
        { key: 'tpl-tools-none', name: '空工具集', category: '快速', description: '不分配任何工具', tools: [], skillCategories: [] },
      ];
    }
    return [
      { key: 'tpl-skills-pm', name: '产品经理', category: '快速', description: '产品经理分类 Skills', tools: [], skillCategories: ['产品经理'] },
      { key: 'tpl-skills-dev', name: '开发人员', category: '快速', description: '开发人员分类 Skills', tools: [], skillCategories: ['开发人员'] },
      { key: 'tpl-skills-none', name: '无 Skills', category: '快速', description: '不分配任何 Skill', tools: [], skillCategories: [] },
    ];
  }, [mode]);

  const templates: TemplateItem[] = useMemo(() => [
    ...builtInTemplates.map((t) => {
      const skillIds = t.skillCategories.length > 0
        ? librarySkills.filter((s) => t.skillCategories.includes(s.category?.trim() || '未分类')).map((s) => s.id)
        : [];
      return { id: t.key, name: t.name, category: t.category, description: t.description, tools: t.tools, skillIds, builtin: true };
    }),
    ...saved.map((t) => ({ id: t.id, name: t.name, category: '自定义', description: mode === 'tools' ? `${t.tools.length} 个工具` : `${t.skillIds.length} 个 Skill`, tools: t.tools, skillIds: t.skillIds, builtin: false })),
  ], [builtInTemplates, saved, librarySkills, mode]);

  const groups = useMemo(() => {
    const map = new Map<string, TemplateItem[]>();
    for (const tpl of templates) {
      const cat = tpl.category || '通用';
      if (!map.has(cat)) map.set(cat, []);
      map.get(cat)!.push(tpl);
    }
    return Array.from(map.entries()).map(([category, items]) => ({ category, items }));
  }, [templates]);

  const selected = selectedId ? templates.find((t) => t.id === selectedId) ?? null : null;

  useEffect(() => {
    if (!open) return;
    setSaved(loadSaved(mode));
    setToolSearch('');
    setSkillSearch('');
    setToolFilter('all');
    setSkillFilter('all');
    const first = templates[0];
    if (first) {
      applyTemplateToDraft(first);
    } else {
      handleNew();
    }
  }, [open, mode]);

  const applyTemplateToDraft = (tpl: TemplateItem) => {
    setSelectedId(tpl.id);
    setDraftName(tpl.name);
    setDraftTools([...tpl.tools]);
    setDraftSkillIds(new Set(tpl.skillIds));
  };

  const handleNew = () => {
    setSelectedId(null);
    setDraftName('');
    setDraftTools([...currentTools]);
    setDraftSkillIds(new Set(currentSkillIds));
  };

  const handleSave = () => {
    const name = draftName.trim();
    if (!name) { message.warning('请输入模板名称'); return; }

    if (selected && !selected.builtin) {
      const next = saved.map((t) => t.id === selected.id ? { ...t, name, tools: [...draftTools], skillIds: Array.from(draftSkillIds) } : t);
      setSaved(next);
      persistSaved(mode, next);
      message.success('模板已更新');
    } else {
      const tpl: SavedCreateTemplate = {
        id: `tpl_${Date.now()}`,
        name,
        tools: [...draftTools],
        skillIds: Array.from(draftSkillIds),
        createdAt: Date.now(),
      };
      const next = [...saved, tpl];
      setSaved(next);
      persistSaved(mode, next);
      setSelectedId(tpl.id);
      message.success('模板已保存');
    }
  };

  const handleDelete = () => {
    if (!selected || selected.builtin) return;
    const next = saved.filter((t) => t.id !== selected.id);
    setSaved(next);
    persistSaved(mode, next);
    const first = next[0] ? templates.find((t) => t.id === next[0]!.id) ?? null : templates[0];
    if (first) applyTemplateToDraft(first);
    else handleNew();
    message.success('模板已删除');
  };

  const handleApply = () => {
    onApply(draftTools, Array.from(draftSkillIds));
  };

  const toggleTool = (name: string, checked: boolean) => {
    setDraftTools((prev) => checked ? [...prev, name] : prev.filter((n) => n !== name));
  };

  const toggleSkill = (id: string) => {
    setDraftSkillIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  };

  const filteredEditorTools = useMemo(() => {
    let list = toolCatalog;
    if (toolFilter !== 'all') list = list.filter((t) => t.category === toolFilter);
    if (toolSearch) list = list.filter((t) => t.label.includes(toolSearch) || t.name.includes(toolSearch) || t.description.includes(toolSearch));
    return list;
  }, [toolFilter, toolSearch]);

  const filteredEditorSkills = useMemo(() => {
    let list = librarySkills;
    if (skillFilter !== 'all') list = list.filter((s) => (s.category?.trim() || '未分类') === skillFilter);
    if (skillSearch) list = list.filter((s) => s.name.includes(skillSearch) || (s.description || '').includes(skillSearch));
    return list;
  }, [librarySkills, skillFilter, skillSearch]);

  const skillCats = useMemo(() => {
    const cats = new Set<string>();
    librarySkills.forEach((s) => cats.add(s.category?.trim() || '未分类'));
    return Array.from(cats);
  }, [librarySkills]);

  const title = mode === 'tools' ? '管理工具集模板' : '管理 Skills 模板';

  return (
    <Modal centered footer={null} onCancel={onClose} open={open} title={title} width={880}>
      <div className={styles.manager}>
        <div className={styles.sidebar}>
          <div className={styles.sidebarTools}>
            <Button size="small" icon={<PlusOutlined />} onClick={handleNew}>新建</Button>
          </div>
          <div className={styles.templateList}>
            {groups.map(({ category, items }) => (
              <div key={category}>
                <div className={styles.categoryTitle}>{category}</div>
                {items.map((tpl) => (
                  <button
                    key={tpl.id}
                    className={`${styles.templateCard} ${selectedId === tpl.id ? styles.templateCardActive : ''}`}
                    type="button"
                    onClick={() => applyTemplateToDraft(tpl)}
                  >
                    <div className={styles.templateCardName}>{tpl.name}</div>
                    <div className={styles.templateCardDesc}>{tpl.description}</div>
                  </button>
                ))}
              </div>
            ))}
            {templates.length === 0 && <div className={styles.emptySidebar}>暂无模板</div>}
          </div>
        </div>

        <div className={styles.editor}>
          <div className={styles.editorHeader}>
            <span className={styles.editorTitle}>{selectedId ? '编辑模板' : '新建模板'}</span>
          </div>
          <label className={styles.field}>
            <span className={styles.fieldLabel}>名称</span>
            <Input
              className={styles.fieldInput}
              maxLength={100}
              placeholder="模板名称"
              value={draftName}
              onChange={(e) => setDraftName(e.target.value)}
            />
          </label>

          {mode === 'tools' && (
            <div className={styles.selectionSection}>
              <div className={styles.sectionHeader}>
                工具集
                <span className={styles.sectionCount}>{draftTools.length}/{toolCatalog.length}</span>
              </div>
              <div className={styles.searchRow}>
                <Input
                  placeholder="搜索工具..."
                  size="small"
                  allowClear
                  value={toolSearch}
                  onChange={(e) => setToolSearch(e.target.value)}
                  className={styles.searchInput}
                />
              </div>
              <div className={styles.filterBar}>
                <button className={`${styles.filterPill} ${toolFilter === 'all' ? styles.filterPillActive : ''}`} type="button" onClick={() => setToolFilter('all')}>全部</button>
                {categoryOrder.map((cat) => {
                  const meta = categoryMeta[cat];
                  if (!meta) return null;
                  return (
                    <button className={`${styles.filterPill} ${toolFilter === cat ? styles.filterPillActive : ''}`} key={cat} type="button" onClick={() => setToolFilter(cat)}>
                      {meta.label}
                    </button>
                  );
                })}
              </div>
              <div className={styles.selectionGrid}>
                {filteredEditorTools.map((tool) => {
                  const checked = draftTools.includes(tool.name);
                  const meta = categoryMeta[tool.category];
                  return (
                    <label className={`${styles.toolTag} ${checked ? styles.toolTagSelected : ''}`} key={tool.name}>
                      <input type="checkbox" checked={checked} onChange={(e) => toggleTool(tool.name, e.target.checked)} />
                      <span className={styles.toolTagName}>{tool.label}</span>
                      {meta && <span className={styles.toolTagBadge} style={{ background: `${meta.color}18`, color: meta.color }}>{meta.label}</span>}
                    </label>
                  );
                })}
              </div>
            </div>
          )}

          {mode === 'skills' && (
            <div className={styles.selectionSection}>
              <div className={styles.sectionHeader}>
                平台 Skills
                <span className={styles.sectionCount}>{draftSkillIds.size}</span>
              </div>
              <div className={styles.searchRow}>
                <Input
                  placeholder="搜索 Skill..."
                  size="small"
                  allowClear
                  value={skillSearch}
                  onChange={(e) => setSkillSearch(e.target.value)}
                  className={styles.searchInput}
                />
              </div>
              {skillCats.length > 1 && (
                <div className={styles.filterBar}>
                  <button className={`${styles.filterPill} ${skillFilter === 'all' ? styles.filterPillActive : ''}`} type="button" onClick={() => setSkillFilter('all')}>全部</button>
                  {skillCats.map((cat) => (
                    <button className={`${styles.filterPill} ${skillFilter === cat ? styles.filterPillActive : ''}`} key={cat} type="button" onClick={() => setSkillFilter(cat)}>
                      {cat}
                    </button>
                  ))}
                </div>
              )}
              <div className={styles.selectionGrid}>
                {filteredEditorSkills.map((skill) => {
                  const checked = draftSkillIds.has(skill.id);
                  return (
                    <div
                      className={`${styles.skillTag} ${checked ? styles.skillTagSelected : ''}`}
                      key={skill.id}
                      role="button"
                      tabIndex={0}
                      onClick={() => toggleSkill(skill.id)}
                      onKeyDown={(e) => { if (e.key === 'Enter') toggleSkill(skill.id); }}
                    >
                      <span className={styles.skillTagName}>{skill.name}</span>
                      {skill.category && <span className={styles.skillTagBadge}>{skill.category}</span>}
                    </div>
                  );
                })}
              </div>
            </div>
          )}

          <div className={styles.editorActions}>
            <div>
              {selected && !selected.builtin && (
                <Popconfirm title="确定删除该模板？" onConfirm={handleDelete} okText="删除" cancelText="取消">
                  <Button danger icon={<DeleteOutlined />} size="small">删除</Button>
                </Popconfirm>
              )}
            </div>
            <div className={styles.rightActions}>
              <Button size="small" onClick={onClose}>关闭</Button>
              <Button size="small" icon={<SaveOutlined />} onClick={handleSave}>保存</Button>
              <Button size="small" type="primary" onClick={handleApply}>应用</Button>
            </div>
          </div>
        </div>
      </div>
    </Modal>
  );
};
