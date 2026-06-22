import React, { useEffect, useMemo, useState } from 'react';
import { Avatar, Button, Checkbox, Input, Popconfirm, Select, Tag } from 'antd';
import { message } from '@/utils/message';
import {
  DesktopOutlined,
  MessageOutlined,
  RobotOutlined,
  SettingOutlined,
  ToolOutlined,
  DeleteOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  SaveOutlined,
  SolutionOutlined,
} from '@ant-design/icons';
import type { Agent } from '@/types/agent';
import { useAgentStore } from '@/store/agentStore';
import { itemToSkill } from '@/api/platformSkill';
import { useCatalogDomain } from '@/hooks/useCatalogDomain';
import { listUserTemplates, type UserTemplate } from '@/api/userTemplate';
import { AgentSkillsPanel } from './AgentSkillsPanel';
import { AvatarPickerModal } from './AvatarPickerModal';
import {
  formatDateTime,
  getAgentDescription,
  getRuntimeLabel,
  parseSkills,
  resolveAgentAvatar,
} from './agentPresentation';
import { AgentPromptTemplateField } from './AgentPromptTemplateField';
import { CreateTemplateManagerModal } from './CreateTemplateManagerModal';
import {
  getCategoryMeta,
  getCategoryOrder,
  getManagementTools,
  getTemplateTools,
  parseToolsConfig,
  getToolCatalogSync,
  getToolsetTemplatesSync,
  toolsConfigToJSON,
  getToolsetOptions,
  fetchToolCatalog,
} from './toolAssignments';
import { SectionHeader } from '@/components/common/SectionHeader';
import { StatusBadge, type StatusBadgeStatus } from '@/components/common/StatusBadge';
import styles from './AgentProfile.module.css';

interface AgentProfileProps {
  agent: Agent | null;
  defaultTab?: string;
  onMessage?: (agent: Agent) => void;
}

const tabItems = [
  { key: 'profile', label: '资料' },
  { key: 'system_prompt', label: '系统提示词', icon: <SettingOutlined /> },
  { key: 'skills', label: '技能' },
  { key: 'tools_config', label: '工具', icon: <ToolOutlined /> },
];

function getStatusText(agent: Agent): string {
  return agent.status === 'online' ? 'Online' : agent.status;
}

function agentStatusBadge(status: Agent['status']): StatusBadgeStatus {
  switch (status) {
    case 'online':
      return 'running';
    case 'busy':
      return 'running';
    case 'error':
      return 'error';
    case 'stopped':
      return 'idle';
    case 'offline':
    default:
      return 'inactive';
  }
}

function agentStatusBadgeLabel(status: Agent['status']): string {
  switch (status) {
    case 'online':
      return '运行中';
    case 'busy':
      return '忙碌';
    case 'error':
      return '异常';
    case 'stopped':
      return '已停止';
    case 'offline':
      return '未运行';
    default:
      return status;
  }
}

export const AgentProfile: React.FC<AgentProfileProps> = ({ agent, defaultTab = 'profile', onMessage }) => {
  const updateAgent = useAgentStore((s) => s.updateAgent);
  const updateAgentTags = useAgentStore((s) => s.updateAgentTags);
  const deleteAgent = useAgentStore((s) => s.deleteAgent);
  const [activeTab, setActiveTab] = useState(defaultTab);
  const [name, setName] = useState('');
  const [avatar, setAvatar] = useState('');
  const [tagsValue, setTagsValue] = useState('');
  const [customSkillCount, setCustomSkillCount] = useState(0);
  const [systemPromptValue, setSystemPromptValue] = useState('');
  const [selectedToolset, setSelectedToolset] = useState('tasks');
  const [selectedTools, setSelectedTools] = useState<string[]>(getTemplateTools('tasks'));
  const [saving, setSaving] = useState(false);
  const [avatarPickerOpen, setAvatarPickerOpen] = useState(false);
  const [toolFilter, setToolFilter] = useState<string>('all');
  const [toolManageOpen, setToolManageOpen] = useState(false);
  const [dbToolTemplates, setDbToolTemplates] = useState<UserTemplate[]>([]);
  const [catalogReady, setCatalogReady] = useState(false);

  const { items: rawSkills } = useCatalogDomain('platform_skill');
  const librarySkills = useMemo(() => rawSkills.map(itemToSkill), [rawSkills]);

  const filteredTools = useMemo(() => {
    if (toolFilter === 'all') return getToolCatalogSync();
    return getToolCatalogSync().filter((t) => t.category === toolFilter);
  }, [toolFilter, catalogReady]);

  const categoryMeta = useMemo(() => getCategoryMeta(), [catalogReady]);
  const categoryOrder = useMemo(() => getCategoryOrder(), [catalogReady]);

  const parseTagsFromJSON = (raw: string): string => {
    if (!raw || raw === '[]') return '';
    try {
      const arr = JSON.parse(raw);
      return Array.isArray(arr) ? arr.filter((t: unknown): t is string => typeof t === 'string').join(', ') : '';
    } catch {
      return raw;
    }
  };

  const tagsToJSON = (csv: string): string => {
    const items = csv.split(',').map((s) => s.trim()).filter(Boolean);
    return items.length > 0 ? JSON.stringify(items) : '';
  };

  useEffect(() => {
    if (!agent) return;
    setActiveTab(defaultTab);
  }, [agent?.id, defaultTab]);

  // Load tool catalog eagerly; once loaded, re-apply tools_config from the agent
  // so that tool names are properly recognised.
  useEffect(() => {
    fetchToolCatalog()
      .then(() => setCatalogReady(true))
      .catch(() => {});
  }, []);

  // Restore component state from the current agent data.
  useEffect(() => {
    if (!agent) return;
    setName(agent.name);
    setAvatar(agent.avatar ?? '');
    setTagsValue(parseTagsFromJSON(agent.tags ?? ''));
    setCustomSkillCount(parseSkills(agent.custom_skills).length);
    setSystemPromptValue(agent.system_prompt ?? '');
  }, [
    agent?.id,
    agent?.name,
    agent?.avatar,
    agent?.tags,
    agent?.custom_skills,
    agent?.system_prompt,
  ]);

  // Parse tools_config — runs both when the agent changes and when the catalog
  // finishes loading, so tool names are never lost due to a race.
  useEffect(() => {
    if (!agent) return;
    const parsedTools = parseToolsConfig(agent.tools_config);
    setSelectedToolset(parsedTools.toolset);
    setSelectedTools(parsedTools.allowedTools);
  }, [agent?.id, agent?.tools_config, catalogReady]);

  useEffect(() => {
    listUserTemplates('tools').then(setDbToolTemplates).catch(() => {});
  }, []);

  const loadDbToolTemplates = async () => {
    try { setDbToolTemplates(await listUserTemplates('tools')); } catch { /* keep current */ }
  };

  interface ToolsetOption {
    value: string;
    label: string;
    tools?: string[];
  }

  const allToolsetOptions = useMemo<ToolsetOption[]>(() => [
    ...getToolsetOptions().filter((o) => o.value !== 'custom'),
    ...dbToolTemplates.map((t) => {
      const tools = 'tools' in t.content ? t.content.tools : [];
      return { value: `db-${t.id}`, label: `★ ${t.name}`, tools };
    }),
  ], [dbToolTemplates]);

  if (!agent) {
    return (
      <div className={styles.emptyState}>
        选择一个 Agent 查看运行配置和能力说明
      </div>
    );
  }

  const description = getAgentDescription(agent);
  const runtimeLabel = getRuntimeLabel(agent);
  const isOnline = agent.status === 'online';
  const computerName = agent.machine_name || 'local-computer';
  const editableTags = tagsValue.split(',').map((item) => item.trim()).filter(Boolean);
  const isBuiltinSystemAgent = agent.type === 'system' && !agent.user_id;

  const selectedToolCount = selectedTools.length;
  const statusLabel = getStatusText(agent);
  const sourceLabel = agent.source === 'daemon' ? 'Daemon' : agent.source;
  const descriptionBlocks = description
    .split(/\n+/)
    .map((line) => line.trim())
    .filter(Boolean);
  const descriptionSummary = descriptionBlocks[0] || description;
  const operationLines = descriptionBlocks
    .filter((line) => line.startsWith('-'))
    .map((line) => line.replace(/^-+\s*/, ''));
  const descriptionNotes = descriptionBlocks
    .filter((line) => !line.startsWith('-'))
    .slice(1);

  const handleSaveProfile = async () => {
    const nextName = name.trim();
    if (!nextName) {
      message.warning('Agent 名称不能为空');
      return;
    }
    setSaving(true);
    try {
      await updateAgentTags(agent.id, tagsToJSON(tagsValue));
      if (agent.type === 'custom') {
        await updateAgent(agent.id, {
          name: nextName,
          cli_tool: agent.cli_tool,
          avatar: avatar.trim() || undefined,
          system_prompt: agent.system_prompt ?? '',
          tools_config: agent.tools_config ?? '',
          capabilities_json: agent.capabilities_json ?? '',
          custom_skills: agent.custom_skills ?? '',
          enable_management_tools: agent.enable_management_tools ?? false,
        });
      }
      message.success('Agent Profile 已保存');
    } catch {
      message.error('保存 Agent 失败');
    } finally {
      setSaving(false);
    }
  };

  const handleStartAgent = async () => {
    try {
      const { startAgent } = await import('@/api/agent');
      await startAgent(agent.id);
      message.success('启动指令已发送');
    } catch {
      message.error('启动 Agent 失败');
    }
  };

  const handleRestartAgent = async () => {
    try {
      const { restartAgent } = await import('@/api/agent');
      await restartAgent(agent.id);
      message.success('重启指令已发送');
    } catch {
      message.error('重启 Agent 失败');
    }
  };

  const handleStopAgent = async () => {
    try {
      const { stopAgent } = await import('@/api/agent');
      await stopAgent(agent.id);
      message.success('停止指令已发送');
    } catch {
      message.error('停止 Agent 失败');
    }
  };

  const handleDelete = async () => {
    try {
      await deleteAgent(agent.id);
      message.success('Agent 已删除');
    } catch {
      message.error('删除 Agent 失败');
    }
  };

  const handleSaveSystemPrompt = async () => {
    setSaving(true);
    try {
      await updateAgent(agent.id, {
        name: agent.name,
        cli_tool: agent.cli_tool,
        avatar: agent.avatar || undefined,
        system_prompt: systemPromptValue.trim() || undefined,
        tools_config: agent.tools_config ?? '',
        capabilities_json: agent.capabilities_json ?? '',
        custom_skills: agent.custom_skills ?? '',
        enable_management_tools: agent.enable_management_tools ?? false,
      });
      message.success('系统提示词已保存');
    } catch {
      message.error('保存系统提示词失败');
    } finally {
      setSaving(false);
    }
  };

  const handleToolsetChange = (value: string) => {
    setSelectedToolset(value);
    if (value in getToolsetTemplatesSync()) {
      setSelectedTools(getTemplateTools(value));
    } else if (value.startsWith('db-')) {
      const opt = allToolsetOptions.find((o) => o.value === value);
      setSelectedTools(opt?.tools ?? selectedTools);
    } else {
      setSelectedTools(selectedTools);
    }
  };

  const handleToolsChange = (values: string[]) => {
    setSelectedTools(values);
  };

  const handleToolManageApply = (tools: string[], _skillIds: string[]) => {
    setSelectedTools(tools);
    setToolManageOpen(false);
  };

  const handleSaveToolsConfig = async () => {
    setSaving(true);
    try {
      const nextToolsConfig = toolsConfigToJSON(selectedToolset, selectedTools);
      const mgmt = getManagementTools();
      const hasMgmt = selectedTools.some((t) => mgmt.has(t));
      await updateAgent(agent.id, {
        name: agent.name,
        cli_tool: agent.cli_tool,
        avatar: agent.avatar || undefined,
        system_prompt: agent.system_prompt ?? '',
        tools_config: nextToolsConfig,
        capabilities_json: agent.capabilities_json ?? '',
        custom_skills: agent.custom_skills ?? '',
        enable_management_tools: (agent.enable_management_tools ?? false) || hasMgmt,
      });
      message.success('工具配置已保存');
    } catch {
      message.error('保存工具配置失败');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.identity}>
          <Avatar
            className={styles.clickableAvatar}
            size={40}
            src={avatar.trim() ? resolveAgentAvatar({ ...agent, avatar }) : resolveAgentAvatar(agent)}
            icon={<RobotOutlined />}
            onClick={() => setAvatarPickerOpen(true)}
          />
          <div className={styles.titleBlock}>
            <span className={styles.title}>{agent.name}</span>
            <span className={styles.subtitle}>{description}</span>
          </div>
        </div>
        <div className={styles.actions}>
          <Button icon={<MessageOutlined />} onClick={() => onMessage?.(agent)}>Message</Button>
          <Button type="primary" icon={<SaveOutlined />} loading={saving} onClick={handleSaveProfile}>保存</Button>
        </div>
      </div>

      <div className={styles.tabs}>
        {tabItems.map((item) => (
          <button
            className={`${styles.tab} ${item.key === activeTab ? styles.tabActive : ''}`}
            key={item.key}
            type="button"
            onClick={() => setActiveTab(item.key)}
          >
            {item.icon} {item.key === 'skills' ? `${item.label} ${customSkillCount}` : item.label}
          </button>
        ))}
      </div>

      <div className={styles.body}>
        {activeTab === 'profile' && (
          <>
            <div className={styles.profileHero}>
              <Avatar
                className={`${styles.clickableAvatar} ${styles.profileAvatar}`}
                size={74}
                src={avatar.trim() ? resolveAgentAvatar({ ...agent, avatar }) : resolveAgentAvatar(agent)}
                icon={<RobotOutlined />}
                onClick={() => setAvatarPickerOpen(true)}
              />
              <div className={styles.profileHeroMain}>
                <div className={styles.profileTitleRow}>
                  <span className={styles.profileNameText}>{agent.name}</span>
                  <StatusBadge
                    status={agentStatusBadge(agent.status)}
                    label={agentStatusBadgeLabel(agent.status)}
                    size="md"
                  />
                </div>
                <div className={styles.profileMetaLine}>@{agent.cli_tool} · {computerName}</div>
                <div className={styles.profileDescriptionPreview}>{descriptionSummary}</div>
                <div className={styles.heroBadges}>
                  <span className={styles.runtimeBadge}>{runtimeLabel}</span>
                  <span className={styles.runtimeBadge}>{agent.type === 'custom' ? '自建 Agent' : '系统 Agent'}</span>
                  <span className={styles.runtimeBadge}>{sourceLabel}</span>
                </div>
              </div>
            </div>

            <div className={styles.metricStrip}>
              <div className={styles.metricItem}>
                <span className={styles.metricLabel}>状态</span>
                <span className={styles.metricValue}>{statusLabel}</span>
              </div>
              <div className={styles.metricItem}>
                <span className={styles.metricLabel}>平台</span>
                <span className={styles.metricValue}>{runtimeLabel}</span>
              </div>
              <div className={styles.metricItem}>
                <span className={styles.metricLabel}>平台 Skills</span>
                <span className={styles.metricValue}>{customSkillCount}</span>
              </div>
              <div className={styles.metricItem}>
                <span className={styles.metricLabel}>工具</span>
                <span className={styles.metricValue}>{selectedToolCount}</span>
              </div>
            </div>

            <section className={styles.section}>
              <SectionHeader
                icon={<RobotOutlined />}
                title="基本资料"
                description="名称、标签和对外展示信息"
              />
              <div className={styles.formGrid}>
                <div className={styles.field}>
                  <span className={styles.label}>显示名称</span>
                  <Input value={name} onChange={(event) => setName(event.target.value)} placeholder="Agent 名称" />
                </div>
                <div className={styles.field}>
                  <span className={styles.label}>标签</span>
                  <Input value={tagsValue} onChange={(event) => setTagsValue(event.target.value)} placeholder="coding, review, orchestration" />
                  {editableTags.length > 0 && (
                    <div className={styles.tagPreview}>
                      {editableTags.map((item) => <Tag key={item}>{item}</Tag>)}
                    </div>
                  )}
                </div>
              </div>
            </section>

            <section className={styles.section}>
              <SectionHeader
                icon={<SolutionOutlined />}
                title="能力说明"
                description="从系统提示词中提炼出的角色和可执行操作"
              />
              <div className={styles.descriptionCard}>
                <div className={styles.descriptionHeadline}>{descriptionSummary}</div>
                {operationLines.length > 0 && (
                  <div className={styles.descriptionBlock}>
                    <span className={styles.descriptionBlockTitle}>可执行操作</span>
                    <ul className={styles.operationList}>
                      {operationLines.map((line) => <li key={line}>{line}</li>)}
                    </ul>
                  </div>
                )}
                {descriptionNotes.length > 0 && (
                  <div className={styles.descriptionText}>
                    {descriptionNotes.map((line) => <p key={line}>{line}</p>)}
                  </div>
                )}
              </div>
            </section>

            <section className={styles.section}>
              <SectionHeader
                icon={<DesktopOutlined />}
                title="运行信息"
                description="设备、来源、版本和创建时间"
              />
              <div className={styles.infoGrid}>
                <div>
                  <span className={styles.label}>电脑</span>
                  <div className={styles.value}>
                    {computerName} · {sourceLabel} · {formatDateTime(agent.last_seen_at)}
                  </div>
                </div>
                <div>
                  <span className={styles.label}>创建时间</span>
                  <div className={styles.value}>{formatDateTime(agent.created_at)}</div>
                </div>
                <div>
                  <span className={styles.label}>类型</span>
                  <div className={styles.value}>{agent.type === 'custom' ? '自建 Agent' : '系统 Agent'}</div>
                </div>
                <div>
                  <span className={styles.label}>版本</span>
                  <div className={styles.value}>{agent.version || '未上报版本'}</div>
                </div>
              </div>
            </section>

            <section className={styles.section}>
              <SectionHeader
                icon={<PlayCircleOutlined />}
                title="操作"
                description="控制当前 Agent 的运行状态"
              />
              <div className={styles.actionPanel}>
                {isOnline ? (
                  <Button icon={<PlayCircleOutlined />} danger onClick={handleStopAgent}>
                    停止 Agent
                  </Button>
                ) : (
                  <Button icon={<PlayCircleOutlined />} onClick={handleStartAgent}>
                    启动 Agent
                  </Button>
                )}
                <Button icon={<ReloadOutlined />} onClick={handleRestartAgent}>
                  重启 Agent
                </Button>
                {!isBuiltinSystemAgent && (
                  <Popconfirm title="确定删除这个 Agent？" okText="删除" cancelText="取消" onConfirm={handleDelete}>
                    <Button danger icon={<DeleteOutlined />}>
                      删除 Agent
                    </Button>
                  </Popconfirm>
                )}
              </div>
            </section>
          </>
        )}

        {activeTab === 'skills' && <AgentSkillsPanel agent={agent} />}

        {activeTab === 'system_prompt' && (
          <section className={styles.section}>
            <div className={styles.sectionTitle}>系统提示词 (System Prompt)</div>
            <AgentPromptTemplateField
              open={activeTab === 'system_prompt'}
              value={systemPromptValue}
              onChange={setSystemPromptValue}
            />
            <div className={styles.actionPanel}>
              <Button icon={<SaveOutlined />} loading={saving} onClick={handleSaveSystemPrompt}>
                保存提示词
              </Button>
            </div>
          </section>
        )}

        {activeTab === 'tools_config' && (
          <section className={styles.section}>
            <div className={styles.sectionTitle}>工具配置 (Tools Config)</div>
            <div className={styles.toolControlRow}>
              <span className={styles.label}>工具集模板</span>
              <Select
                className={styles.toolsetSelect}
                value={selectedToolset}
                options={allToolsetOptions}
                onChange={handleToolsetChange}
                getPopupContainer={(trigger) => trigger.parentElement || document.body}
              />
              <Button icon={<SettingOutlined />} onClick={() => setToolManageOpen(true)}>管理</Button>
              <Button icon={<SaveOutlined />} loading={saving} onClick={handleSaveToolsConfig}>保存</Button>
              <span className={styles.toolCountLabel}>
                已选 {selectedToolCount}/{getToolCatalogSync().length}
              </span>
            </div>
            <div className={styles.toolFilterBar}>
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
                        <span className={styles.toolName}>{tool.label}</span>
                        <span className={styles.toolDesc}>{tool.description}</span>
                        <div className={styles.toolFooter}>
                          <span className={styles.toolFooterName}>{tool.name}</span>
                          {meta && (
                            <span
                              className={styles.toolFooterBadge}
                              style={{ background: `${meta.color}18`, color: meta.color }}
                            >
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
          </section>
        )}
      </div>
      <AvatarPickerModal
        agent={agent}
        open={avatarPickerOpen}
        onClose={() => setAvatarPickerOpen(false)}
      />
      <CreateTemplateManagerModal
        open={toolManageOpen}
        mode="tools"
        currentTools={selectedTools}
        currentSkillIds={new Set()}
        librarySkills={librarySkills}
        onApply={handleToolManageApply}
        onClose={() => { setToolManageOpen(false); loadDbToolTemplates(); }}
      />
    </div>
  );
};
