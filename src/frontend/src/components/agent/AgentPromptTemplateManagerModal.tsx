import React, { useEffect, useMemo, useState } from 'react';
import { Button, Input, Modal, Popconfirm, message } from 'antd';
import {
  DeleteOutlined,
  ImportOutlined,
  PlusOutlined,
  SaveOutlined,
} from '@ant-design/icons';
import type { AgentPromptTemplate, AgentPromptTemplateRequest } from '@/types/agent';
import {
  createAgentPromptTemplate,
  deleteAgentPromptTemplate,
  getAgentPromptTemplates,
  importDefaultAgentPromptTemplates,
  updateAgentPromptTemplate,
} from '@/api/agentPromptTemplate';
import styles from './AgentPromptTemplates.module.css';

interface AgentPromptTemplateManagerModalProps {
  open: boolean;
  templates: AgentPromptTemplate[];
  onClose: () => void;
  onTemplatesChange: (templates: AgentPromptTemplate[]) => void;
}

const EMPTY_DRAFT: AgentPromptTemplateRequest = {
  name: '',
  category: '通用',
  description: '',
  system_prompt: '',
};

function draftFromTemplate(template: AgentPromptTemplate): AgentPromptTemplateRequest {
  return {
    name: template.name,
    category: template.category ?? '通用',
    description: template.description ?? '',
    system_prompt: template.system_prompt ?? '',
  };
}

function groupTemplates(templates: AgentPromptTemplate[]) {
  const groups = new Map<string, AgentPromptTemplate[]>();
  templates.forEach((template) => {
    const category = template.category?.trim() || '通用';
    const items = groups.get(category) ?? [];
    items.push(template);
    groups.set(category, items);
  });
  return Array.from(groups.entries()).map(([category, items]) => ({ category, items }));
}

export const AgentPromptTemplateManagerModal: React.FC<AgentPromptTemplateManagerModalProps> = ({
  open,
  templates,
  onClose,
  onTemplatesChange,
}) => {
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const [draft, setDraft] = useState<AgentPromptTemplateRequest>(EMPTY_DRAFT);
  const [saving, setSaving] = useState(false);
  const [loading, setLoading] = useState(false);
  const groups = useMemo(() => groupTemplates(templates), [templates]);
  const selectedTemplate = selectedID
    ? templates.find((template) => template.id === selectedID) ?? null
    : null;

  useEffect(() => {
    if (!open) return;
    const first = templates[0];
    if (first) {
      setSelectedID(first.id);
      setDraft(draftFromTemplate(first));
    } else {
      setSelectedID(null);
      setDraft(EMPTY_DRAFT);
    }
  }, [open]);

  const refreshTemplates = async () => {
    const items = await getAgentPromptTemplates();
    onTemplatesChange(items);
    return items;
  };

  const handleSelect = (template: AgentPromptTemplate) => {
    setSelectedID(template.id);
    setDraft(draftFromTemplate(template));
  };

  const handleNew = () => {
    setSelectedID(null);
    setDraft(EMPTY_DRAFT);
  };

  const handleImportDefaults = async () => {
    setLoading(true);
    try {
      const imported = await importDefaultAgentPromptTemplates();
      await refreshTemplates();
      if (imported.length === 0) {
        message.info('默认 Prompt 模板已存在');
      } else {
        message.success(`已导入 ${imported.length} 个默认 Prompt 模板`);
      }
    } catch (err) {
      const errorMessage = err instanceof Error && err.message ? err.message : '导入默认 Prompt 模板失败';
      message.error(errorMessage);
    } finally {
      setLoading(false);
    }
  };

  const handleSave = async () => {
    const name = draft.name.trim();
    if (!name) {
      message.warning('模板名称不能为空');
      return;
    }
    setSaving(true);
    try {
      const body: AgentPromptTemplateRequest = {
        name,
        category: draft.category,
        description: draft.description,
        system_prompt: draft.system_prompt,
      };
      const saved = selectedTemplate
        ? await updateAgentPromptTemplate(selectedTemplate.id, body)
        : await createAgentPromptTemplate(body);
      const next = await refreshTemplates();
      setSelectedID(saved.id);
      setDraft(draftFromTemplate(next.find((item) => item.id === saved.id) ?? saved));
      message.success('Prompt 模板已保存');
    } catch (err) {
      const errorMessage = err instanceof Error && err.message ? err.message : '保存 Prompt 模板失败';
      message.error(errorMessage);
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    if (!selectedTemplate) return;
    setLoading(true);
    try {
      await deleteAgentPromptTemplate(selectedTemplate.id);
      const next = templates.filter((template) => template.id !== selectedTemplate.id);
      onTemplatesChange(next);
      const first = next[0];
      if (first) {
        setSelectedID(first.id);
        setDraft(draftFromTemplate(first));
      } else {
        setSelectedID(null);
        setDraft(EMPTY_DRAFT);
      }
      message.success('Prompt 模板已删除');
    } catch {
      message.error('删除 Prompt 模板失败');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Modal
      centered
      footer={null}
      onCancel={onClose}
      open={open}
      title="管理 Agent Prompt 模板"
      width={880}
    >
      <div className={styles.manager}>
        <div className={styles.sidebar}>
          <div className={styles.sidebarTools}>
            <Button size="small" icon={<PlusOutlined />} onClick={handleNew}>
              新建
            </Button>
            <Button size="small" icon={<ImportOutlined />} loading={loading} onClick={handleImportDefaults}>
              导入默认
            </Button>
          </div>
          <div className={styles.templateList}>
            {templates.length === 0 && (
              <div className={styles.emptyList}>暂无模板，可新建或导入默认模板</div>
            )}
            {groups.map((group) => (
              <div key={group.category}>
                <div className={styles.categoryTitle}>{group.category}</div>
                {group.items.map((template) => (
                  <button
                    className={`${styles.templateCard} ${template.id === selectedID ? styles.templateCardActive : ''}`}
                    key={template.id}
                    type="button"
                    onClick={() => handleSelect(template)}
                  >
                    <span className={styles.templateName}>{template.name}</span>
                    <span className={styles.templateDesc}>{template.description || '暂无描述'}</span>
                  </button>
                ))}
              </div>
            ))}
          </div>
        </div>

        <div className={styles.editor}>
          <div className={styles.editorHeader}>
            <span className={styles.editorTitle}>{selectedTemplate ? '编辑模板' : '新建模板'}</span>
          </div>
          <label className={styles.field}>
            <span className={styles.fieldLabel}>名称</span>
            <Input
              className={styles.input}
              maxLength={100}
              placeholder="例如：代码实现 Agent"
              value={draft.name}
              onChange={(event) => setDraft((prev) => ({ ...prev, name: event.target.value }))}
            />
          </label>
          <label className={styles.field}>
            <span className={styles.fieldLabel}>分类</span>
            <Input
              className={styles.input}
              maxLength={80}
              placeholder="例如：开发、产品、研究"
              value={draft.category}
              onChange={(event) => setDraft((prev) => ({ ...prev, category: event.target.value }))}
            />
          </label>
          <label className={styles.field}>
            <span className={styles.fieldLabel}>描述</span>
            <Input.TextArea
              autoSize={{ minRows: 2, maxRows: 4 }}
              className={styles.textarea}
              placeholder="说明这个模板适合什么 Agent"
              value={draft.description}
              onChange={(event) => setDraft((prev) => ({ ...prev, description: event.target.value }))}
            />
          </label>
          <label className={styles.field}>
            <span className={styles.fieldLabel}>System Prompt</span>
            <Input.TextArea
              autoSize={{ minRows: 12, maxRows: 18 }}
              className={`${styles.textarea} ${styles.promptInput}`}
              placeholder="写入 Agent 的系统提示词"
              value={draft.system_prompt}
              onChange={(event) => setDraft((prev) => ({ ...prev, system_prompt: event.target.value }))}
            />
          </label>
          <div className={styles.editorActions}>
            <div>
              {selectedTemplate && (
                <Popconfirm
                  cancelText="取消"
                  okText="删除"
                  title="删除这个 Prompt 模板？"
                  onConfirm={handleDelete}
                >
                  <Button danger icon={<DeleteOutlined />} loading={loading}>
                    删除
                  </Button>
                </Popconfirm>
              )}
            </div>
            <div className={styles.rightActions}>
              <Button onClick={onClose}>关闭</Button>
              <Button type="primary" icon={<SaveOutlined />} loading={saving} onClick={handleSave}>
                保存
              </Button>
            </div>
          </div>
        </div>
      </div>
    </Modal>
  );
};
