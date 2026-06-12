import React, { useMemo, useState } from 'react';
import { Button, Input, Select, message } from 'antd';
import { ImportOutlined, SettingOutlined } from '@ant-design/icons';
import {
  importDefaultAgentPromptTemplates,
  itemToTemplate,
} from '@/api/agentPromptTemplate';
import { useCatalogDomain } from '@/hooks/useCatalogDomain';
import { AgentPromptTemplateManagerModal } from './AgentPromptTemplateManagerModal';
import styles from './AgentPromptTemplates.module.css';

interface AgentPromptTemplateFieldProps {
  open: boolean;
  value: string;
  onChange: (value: string) => void;
}

export const AgentPromptTemplateField: React.FC<AgentPromptTemplateFieldProps> = ({
  open: _open,
  value,
  onChange,
}) => {
  const { items: rawTemplates, refetch: refetchTemplates, loading } = useCatalogDomain('agent_prompt_template');
  const templates = useMemo(() => rawTemplates.map(itemToTemplate), [rawTemplates]);
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const [managerOpen, setManagerOpen] = useState(false);

  const options = useMemo(
    () => templates.map((template) => ({
      value: template.id,
      label: `${template.name} · ${template.category?.trim() || '通用'}`,
    })),
    [templates],
  );

  const handleTemplateSelect = (id?: string) => {
    if (!id) {
      setSelectedID(null);
      return;
    }
    setSelectedID(id);
    const template = templates.find((item) => item.id === id);
    if (template) {
      onChange(template.system_prompt ?? '');
    }
  };

  const handleImportDefaults = async () => {
    try {
      const imported = await importDefaultAgentPromptTemplates();
      const rawItems = await refetchTemplates();
      const refreshed = rawItems.map(itemToTemplate);
      if (imported.length === 0) {
        message.info('默认 Prompt 模板已存在');
      } else {
        message.success(`已导入 ${imported.length} 个默认 Prompt 模板`);
      }
      return refreshed;
    } catch (err) {
      const errorMessage = err instanceof Error && err.message ? err.message : '导入默认 Prompt 模板失败';
      message.error(errorMessage);
      return [];
    }
  };

  return (
    <div className={styles.templateField}>
      <div className={styles.templateToolbar}>
        <Select
          allowClear
          className={styles.select}
          loading={loading}
          options={options}
          placeholder={templates.length > 0 ? '选择 System Prompt 模板' : '暂无模板，可导入默认模板'}
          value={selectedID ?? undefined}
          onChange={handleTemplateSelect}
          onClear={() => setSelectedID(null)}
        />
        <Button icon={<ImportOutlined />} loading={loading} onClick={handleImportDefaults}>
          导入默认
        </Button>
        <Button icon={<SettingOutlined />} onClick={() => setManagerOpen(true)}>
          管理
        </Button>
      </div>
      {templates.length === 0 && (
        <span className={styles.emptyHint}>先导入默认模板，或打开管理面板新建自己的 Agent Prompt 模板。</span>
      )}
      <Input.TextArea
        autoSize={{ minRows: 3, maxRows: 6 }}
        className={styles.textarea}
        placeholder="描述你希望这个 Agent 的风格、角色与边界"
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
      <AgentPromptTemplateManagerModal
        open={managerOpen}
        templates={templates}
        onClose={() => setManagerOpen(false)}
        onTemplatesChange={() => {
          refetchTemplates();
        }}
      />
    </div>
  );
};
