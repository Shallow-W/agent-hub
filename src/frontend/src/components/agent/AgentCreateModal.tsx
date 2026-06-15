import React, { useEffect, useMemo, useState } from 'react';
import { Button, Checkbox, Input, Modal, Select } from 'antd';
import { SettingOutlined } from '@ant-design/icons';
import { message } from '@/utils/message';
import type { AgentCandidate } from '@/types/agent';
import { getDefaultAgentName } from './agentPresentation';
import { itemToSkill } from '@/api/platformSkill';
import { quickTemplates } from '@/config/catalogConfig';
import { useCatalogDomain } from '@/hooks/useCatalogDomain';
import {
  getCategoryMeta,
  getCategoryOrder,
  getTemplateTools,
  getToolCatalogSync,
  getToolsetOptions,
  toolsConfigToJSON,
  fetchToolCatalog,
} from './toolAssignments';
import { AgentPromptTemplateField } from './AgentPromptTemplateField';
import { CreateTemplateManagerModal } from './CreateTemplateManagerModal';
import styles from './AgentCreateModal.module.css';

interface AgentCreateModalProps {
  open: boolean;
  machineName: string;
  candidates: AgentCandidate[];
  onClose: () => void;
  onCreate: (candidateId: string, name: string, systemPrompt: string, toolsConfig: string, customSkills: string) => Promise<void>;
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
  const [toolFilter, setToolFilter] = useState<string>('all');
  const [submitting, setSubmitting] = useState(false);
  const [selectedSkillIds, setSelectedSkillIds] = useState<Set<string>>(new Set());
  const [skillFilter, setSkillFilter] = useState<string>('all');
  const [skillTemplate, setSkillTemplate] = useState('none');
  const [toolManageOpen, setToolManageOpen] = useState(false);
  const [skillManageOpen, setSkillManageOpen] = useState(false);
  const [catalogReady, setCatalogReady] = useState(false);

  const { items: rawSkills } = useCatalogDomain('platform_skill');
  const librarySkills = useMemo(() => rawSkills.map(itemToSkill), [rawSkills]);

  const categoryMeta = useMemo(() => getCategoryMeta(), [catalogReady]);
  const categoryOrder = useMemo(() => getCategoryOrder(), [catalogReady]);

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
    setToolFilter('all');
    setSelectedSkillIds(new Set());
    setSkillFilter('all');
    setSkillTemplate('none');
    setToolManageOpen(false);
    setSkillManageOpen(false);
    fetchToolCatalog()
      .then(() => {
        setCatalogReady(true);
        setToolset('tasks');
        setSelectedTools(getTemplateTools('tasks'));
      })
      .catch(() => {});
  }, [open, candidates]);

  const filteredTools = useMemo(() => {
    const catalog = getToolCatalogSync();
    if (toolFilter === 'all') return catalog;
    return catalog.filter((t) => t.category === toolFilter);
  }, [toolFilter, catalogReady]);

  const skillCategories = useMemo(() => {
    const cats = new Set<string>();
    librarySkills.forEach((s) => cats.add(s.category?.trim() || '未分类'));
    return Array.from(cats);
  }, [librarySkills]);

  const filteredLibrarySkills = useMemo(() => {
    let list = librarySkills;
    if (skillFilter !== 'all') {
      list = list.filter((s) => (s.category?.trim() || '未分类') === skillFilter);
    }
    return list;
  }, [librarySkills, skillFilter]);

  const skillTemplateOptions = useMemo(() => [
    { value: 'none', label: '无' },
    ...skillCategories.map((cat) => ({ value: `cat:${cat}`, label: cat })),
    { value: 'custom', label: '自定义' },
  ], [skillCategories]);

  const handleCandidateChange = (value: string) => {
    setCandidateId(value);
    const selected = candidates.find((candidate) => candidate.id === value);
    if (selected) {
      setName(getDefaultAgentName(selected.name, selected.cli_tool));
    }
  };

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

  const handleSkillTemplateChange = (value: string) => {
    setSkillTemplate(value);
    if (value === 'none') {
      setSelectedSkillIds(new Set());
    } else if (value.startsWith('cat:')) {
      const cat = value.slice(4);
      const matched = librarySkills
        .filter((s) => (s.category?.trim() || '未分类') === cat)
        .map((s) => s.id);
      setSelectedSkillIds(new Set(matched));
    }
  };

  const toggleSkill = (id: string) => {
    setSelectedSkillIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
    setSkillTemplate('custom');
  };

  const handleApplyQuickTemplate = (key: string) => {
    const tpl = quickTemplates.find((t) => t.key === key);
    if (!tpl) return;
    setToolset(tpl.toolset);
    if (tpl.toolset !== 'custom') {
      setSelectedTools(getTemplateTools(tpl.toolset));
    }
    if (tpl.skillCategories.length > 0) {
      const matched = librarySkills
        .filter((s) => tpl.skillCategories.includes(s.category?.trim() || '未分类'))
        .map((s) => s.id);
      setSelectedSkillIds(new Set(matched));
      setSkillTemplate(`cat:${tpl.skillCategories[0]}`);
    } else {
      setSelectedSkillIds(new Set());
      setSkillTemplate('none');
    }
  };

  const handleApplyFromManager = (mode: 'tools' | 'skills', tools: string[], skillIds: string[]) => {
    setSelectedTools(tools);
    setToolset(tools.length > 0 ? 'custom' : 'none');
    setSelectedSkillIds(new Set(skillIds));
    setSkillTemplate(skillIds.length > 0 ? 'custom' : 'none');
    if (mode === 'tools') setToolManageOpen(false);
    else setSkillManageOpen(false);
  };

  const handleToolManageApply = (tools: string[], skillIds: string[]) => handleApplyFromManager('tools', tools, skillIds);
  const handleSkillManageApply = (tools: string[], skillIds: string[]) => handleApplyFromManager('skills', tools, skillIds);

  const handleSubmit = async () => {
    if (!candidateId || !name.trim()) return;
    setSubmitting(true);
    try {
      const selectedLibSkills = librarySkills.filter((s) => selectedSkillIds.has(s.id));
      const customSkillsArr = selectedLibSkills.map((s) => ({
        name: s.name.trim(),
        category: s.category,
        description: s.description,
        trigger: s.trigger || s.description,
        detail: s.detail,
      }));
      const customSkills = customSkillsArr.length > 0 ? JSON.stringify(customSkillsArr) : '';
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
  const selectedToolCount = selectedTools.length;
  const selectedSkillCount = selectedSkillIds.size;

  return (
    <Modal
      open={open}
      onCancel={onClose}
      footer={null}
      title={title}
      centered
      width={780}
    >
      <div className={styles.content}>
        <div className={styles.field}>
          <span className={styles.label}>快速模板</span>
          <div className={styles.templateRow}>
            {quickTemplates.map((tpl) => (
              <button
                key={tpl.key}
                className={styles.templatePill}
                type="button"
                onClick={() => handleApplyQuickTemplate(tpl.key)}
              >
                {tpl.label}
              </button>
            ))}
          </div>
        </div>

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
          <div className={styles.fieldHeader}>
            <span className={styles.label}>工具集</span>
            <span className={styles.countLabel}>已选 {selectedToolCount}/{getToolCatalogSync().length}</span>
          </div>
          <div className={styles.toolbar}>
            <Select
              className={styles.toolbarSelect}
              value={toolset}
              options={getToolsetOptions()}
              onChange={handleToolsetChange}
            />
            <Button
              icon={<SettingOutlined />}
              onClick={() => setToolManageOpen(true)}
            >
              管理
            </Button>
          </div>
          <div className={styles.filterBar}>
            <button
              className={`${styles.filterPill} ${toolFilter === 'all' ? styles.filterPillActive : ''}`}
              type="button"
              onClick={() => setToolFilter('all')}
            >
              全部 {getToolCatalogSync().length}
            </button>
            {categoryOrder.map((cat) => {
              const meta = categoryMeta[cat];
              if (!meta) return null;
              const count = getToolCatalogSync().filter((t) => t.category === cat).length;
              const selected = selectedTools.filter((n) => getToolCatalogSync().find((t) => t.name === n && t.category === cat)).length;
              return (
                <button
                  className={`${styles.filterPill} ${toolFilter === cat ? styles.filterPillActive : ''}`}
                  key={cat}
                  type="button"
                  onClick={() => setToolFilter(cat)}
                >
                  {meta.label} {selected}/{count}
                </button>
              );
            })}
          </div>
          <Checkbox.Group value={selectedTools} onChange={(values) => handleToolsChange(values as string[])}>
            <div className={styles.toolGrid}>
              {filteredTools.map((tool) => {
                const isSelected = selectedTools.includes(tool.name);
                const meta = categoryMeta[tool.category];
                return (
                  <label className={`${styles.toolCard} ${isSelected ? styles.toolCardSelected : ''}`} key={tool.name}>
                    <Checkbox value={tool.name} />
                    <div className={styles.toolCardContent}>
                      <span className={styles.toolCardName}>{tool.label}</span>
                      <span className={styles.toolCardDesc}>{tool.description}</span>
                      <div className={styles.toolCardFooter}>
                        <span className={styles.toolCardApi}>{tool.name}</span>
                        {meta && (
                          <span className={styles.toolCardBadge} style={{ background: `${meta.color}18`, color: meta.color }}>
                            {meta.label}
                          </span>
                        )}
                      </div>
                    </div>
                  </label>
                );
              })}
            </div>
          </Checkbox.Group>
        </div>

        <div className={styles.field}>
          <div className={styles.fieldHeader}>
            <span className={styles.label}>平台 Skills</span>
            <span className={styles.countLabel}>已选 {selectedSkillCount}</span>
          </div>
          {librarySkills.length > 0 && (
            <>
              <div className={styles.toolbar}>
                <Select
                  className={styles.toolbarSelect}
                  value={skillTemplate}
                  options={skillTemplateOptions}
                  onChange={handleSkillTemplateChange}
                />
                <Button
                  icon={<SettingOutlined />}
                  onClick={() => setSkillManageOpen(true)}
                >
                  管理
                </Button>
              </div>
              <div className={styles.filterBar}>
                <button
                  className={`${styles.filterPill} ${skillFilter === 'all' ? styles.filterPillActive : ''}`}
                  type="button"
                  onClick={() => setSkillFilter('all')}
                >
                  全部 {librarySkills.length}
                </button>
                {skillCategories.map((cat) => (
                  <button
                    className={`${styles.filterPill} ${skillFilter === cat ? styles.filterPillActive : ''}`}
                    key={cat}
                    type="button"
                    onClick={() => setSkillFilter(cat)}
                  >
                    {cat}
                  </button>
                ))}
              </div>
              <div className={styles.skillGrid}>
                {filteredLibrarySkills.map((skill) => {
                  const isSelected = selectedSkillIds.has(skill.id);
                  return (
                    <div
                      className={`${styles.skillCard} ${isSelected ? styles.skillCardSelected : ''}`}
                      key={skill.id}
                      role="button"
                      tabIndex={0}
                      onClick={() => toggleSkill(skill.id)}
                      onKeyDown={(e) => { if (e.key === 'Enter') toggleSkill(skill.id); }}
                    >
                      <div className={styles.skillCardName}>{skill.name}</div>
                      <div className={styles.skillCardDesc}>{skill.description || '暂无描述'}</div>
                      <div className={styles.skillCardFooter}>
                        {skill.category && <span className={styles.skillCardBadge}>{skill.category}</span>}
                        <span className={styles.skillCardAction}>{isSelected ? '已选' : '选择'}</span>
                      </div>
                    </div>
                  );
                })}
                {filteredLibrarySkills.length === 0 && (
                  <div className={styles.emptyHint}>暂无可选 Skill</div>
                )}
              </div>
            </>
          )}
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

      <CreateTemplateManagerModal
        open={toolManageOpen}
        mode="tools"
        currentTools={selectedTools}
        currentSkillIds={selectedSkillIds}
        librarySkills={librarySkills}
        onApply={handleToolManageApply}
        onClose={() => setToolManageOpen(false)}
      />

      <CreateTemplateManagerModal
        open={skillManageOpen}
        mode="skills"
        currentTools={selectedTools}
        currentSkillIds={selectedSkillIds}
        librarySkills={librarySkills}
        onApply={handleSkillManageApply}
        onClose={() => setSkillManageOpen(false)}
      />
    </Modal>
  );
};
