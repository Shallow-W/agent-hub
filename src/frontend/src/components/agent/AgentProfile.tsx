import React, { useEffect, useMemo, useState } from 'react';
import { Avatar, Button, Checkbox, Dropdown, Input, Popconfirm, Select, Tag } from 'antd';
import type { MenuProps } from 'antd';
import { message } from '@/utils/message';
import {
  EditOutlined,
  MessageOutlined,
  MoreOutlined,
  RobotOutlined,
  SettingOutlined,
  DeleteOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  SaveOutlined,
  ArrowUpOutlined,
  HistoryOutlined,
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
import { StatusBadge, type StatusBadgeStatus } from '@/components/common/StatusBadge';
import styles from './AgentProfile.module.css';

interface AgentProfileProps {
  agent: Agent | null;
  defaultTab?: string;
  onMessage?: (agent: Agent) => void;
}

interface OverviewMetric {
  key: string;
  label: string;
  value: string;
  trend: string;
}

interface ConversationItem {
  id: string;
  content: string;
  user: string;
  time: string;
  status: 'completed' | 'running' | 'failed';
}

const MOCK_METRICS: OverviewMetric[] = [
  { key: 'chats', label: '对话次数', value: '128', trend: '↑ 12 本周' },
  { key: 'active', label: '活跃时长', value: '4.2h', trend: '↑ 0.8h 本周' },
  { key: 'tool_calls', label: '工具调用', value: '1,042', trend: '↑ 8 本周' },
  { key: 'tasks', label: '执行任务', value: '36', trend: '↑ 4 本周' },
];

const MOCK_CONVERSATIONS: ConversationItem[] = [
  {
    id: 'mock-1',
    content: '帮我梳理下这个需求的 PRD，按用户故事拆分里程碑',
    user: '王小明',
    time: '06-19 20:11',
    status: 'completed',
  },
  {
    id: 'mock-2',
    content: '对比一下 Claude Code 和 Cursor 在多文件重构上的差异',
    user: '王小明',
    time: '06-18 16:45',
    status: 'completed',
  },
  {
    id: 'mock-3',
    content: '把这个功能的回归测试用例补齐',
    user: '王小明',
    time: '06-17 10:22',
    status: 'running',
  },
];

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
      return '在线';
    case 'busy':
      return '忙碌';
    case 'error':
      return '异常';
    case 'stopped':
      return '已停止';
    case 'offline':
      return '离线';
    default:
      return status;
  }
}

function conversationStatusMeta(status: ConversationItem['status']): { label: string; className: string } {
  switch (status) {
    case 'completed':
      return { label: '已完成', className: 'completed' };
    case 'running':
      return { label: '进行中', className: 'running' };
    case 'failed':
    default:
      return { label: '失败', className: 'failed' };
  }
}

export const AgentProfile: React.FC<AgentProfileProps> = ({ agent, defaultTab = 'overview', onMessage }) => {
  const updateAgent = useAgentStore((s) => s.updateAgent);
  const updateAgentTags = useAgentStore((s) => s.updateAgentTags);
  const deleteAgent = useAgentStore((s) => s.deleteAgent);
  const [activeTab, setActiveTab] = useState(defaultTab);
  const [editing, setEditing] = useState(false);
  const [descriptionExpanded, setDescriptionExpanded] = useState(false);
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
    setEditing(false);
  }, [agent?.id, defaultTab]);

  useEffect(() => {
    fetchToolCatalog()
      .then(() => setCatalogReady(true))
      .catch(() => {});
  }, []);

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
  const isOnline = agent.status === 'online';
  const computerName = agent.machine_name || 'local-computer';
  const editableTags = tagsValue.split(',').map((item) => item.trim()).filter(Boolean);
  const isBuiltinSystemAgent = agent.type === 'system' && !agent.user_id;

  const selectedToolCount = selectedTools.length;
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

  const avatarSrc = avatar.trim() ? resolveAgentAvatar({ ...agent, avatar }) : resolveAgentAvatar(agent);
  const subtitleText = `@${agent.cli_tool}${agent.version ? ` · v${agent.version}` : ''}`;
  const roleTagText = agent.cli_tool || 'agent';
  const skillsList = parseSkills(agent.custom_skills).slice(0, 3);

  const tabItems = [
    { key: 'overview', label: '概览' },
    { key: 'system_prompt', label: '系统提示词' },
    { key: 'skills', label: `技能 ${customSkillCount}` },
    { key: 'tools_config', label: `工具 ${selectedToolCount}` },
    { key: 'activity', label: '运行记录' },
  ];

  const moreMenuItems: MenuProps['items'] = [
    {
      key: 'edit',
      icon: <EditOutlined />,
      label: '编辑资料',
      onClick: () => {
        setActiveTab('overview');
        setEditing(true);
      },
    },
    { type: 'divider' },
    ...(isBuiltinSystemAgent
      ? []
      : [
          {
            key: 'delete',
            icon: <DeleteOutlined />,
            danger: true,
            label: '删除 Agent',
          },
        ]),
  ];

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
      setEditing(false);
    } catch {
      message.error('保存 Agent 失败');
    } finally {
      setSaving(false);
    }
  };

  const handleCancelEdit = () => {
    setName(agent.name);
    setAvatar(agent.avatar ?? '');
    setTagsValue(parseTagsFromJSON(agent.tags ?? ''));
    setEditing(false);
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
            size={48}
            src={avatarSrc}
            icon={<RobotOutlined />}
            onClick={() => setAvatarPickerOpen(true)}
          />
          <div className={styles.titleBlock}>
            <div className={styles.titleRow}>
              <span className={styles.title}>{agent.name}</span>
              <StatusBadge
                status={agentStatusBadge(agent.status)}
                label={agentStatusBadgeLabel(agent.status)}
                size="sm"
              />
            </div>
            <span className={styles.subtitle}>{subtitleText}</span>
            <span className={styles.roleTag}>{roleTagText}</span>
          </div>
        </div>
        <div className={styles.actions}>
          {editing ? (
            <>
              <Button onClick={handleCancelEdit}>取消</Button>
              <Button type="primary" icon={<SaveOutlined />} loading={saving} onClick={handleSaveProfile}>
                保存
              </Button>
            </>
          ) : (
            <>
              <Button icon={<MessageOutlined />} onClick={() => onMessage?.(agent)}>Message</Button>
              <Button
                type="primary"
                icon={<EditOutlined />}
                onClick={() => {
                  setActiveTab('overview');
                  setEditing(true);
                }}
              >
                编辑
              </Button>
              <Dropdown menu={{ items: moreMenuItems, onClick: ({ key }) => { if (key === 'delete') handleDelete(); } }} trigger={['click']}>
                <Button type="text" icon={<MoreOutlined />} />
              </Dropdown>
            </>
          )}
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
            {item.label}
          </button>
        ))}
      </div>

      <div className={styles.body}>
        {activeTab === 'overview' && !editing && (
          <>
            <section className={styles.overviewCard}>
              <div className={styles.overviewCardHeader}>
                <span className={styles.overviewCardTitle}>智能体简介</span>
              </div>
              <div className={`${styles.overviewDescriptionText} ${descriptionExpanded ? styles.expanded : ''}`}>
                {description
                  ? description.split(/\n+/).map((line, idx) => <p key={idx}>{line}</p>)
                  : '暂无简介'}
              </div>
              {description && description.length > 80 && (
                <button
                  type="button"
                  className={styles.expandToggle}
                  onClick={() => setDescriptionExpanded((v) => !v)}
                >
                  {descriptionExpanded ? '收起' : '显示更多'}
                </button>
              )}
            </section>

            <section className={styles.overviewCard}>
              <div className={styles.overviewCardHeader}>
                <span className={styles.overviewCardTitle}>运行概览</span>
                <span className={styles.overviewCardHint}>近 7 天</span>
              </div>
              <div className={styles.metricGrid}>
                {MOCK_METRICS.map((metric) => (
                  <div className={styles.metricCard} key={metric.key}>
                    <span className={styles.metricCardLabel}>{metric.label}</span>
                    <span className={styles.metricCardValue}>{metric.value}</span>
                    <span className={styles.metricCardTrend}>
                      <ArrowUpOutlined /> {metric.trend.replace('↑ ', '')}
                    </span>
                  </div>
                ))}
              </div>
            </section>

            <section className={styles.overviewCard}>
              <div className={styles.overviewCardHeader}>
                <span className={styles.overviewCardTitle}>技能</span>
                <button
                  type="button"
                  className={styles.viewAllLink}
                  onClick={() => setActiveTab('skills')}
                >
                  查看全部
                </button>
              </div>
              {skillsList.length === 0 ? (
                <div className={styles.skillsEmpty}>暂无自定义技能</div>
              ) : (
                <div className={styles.skillRow}>
                  {skillsList.map((skill, idx) => (
                    <div className={styles.skillMiniCard} key={skill.name + idx}>
                      <span className={styles.skillMiniIcon}>⚡</span>
                      <div className={styles.skillMiniMeta}>
                        <span className={styles.skillMiniName}>{skill.name}</span>
                        <span className={styles.skillMiniTag}>{roleTagText}</span>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </section>

            <section className={styles.overviewCard}>
              <div className={styles.overviewCardHeader}>
                <span className={styles.overviewCardTitle}>最近对话</span>
                <span className={styles.overviewCardHint}>来自当前 Agent</span>
              </div>
              <div className={styles.conversationList}>
                {MOCK_CONVERSATIONS.map((conv) => {
                  const status = conversationStatusMeta(conv.status);
                  return (
                    <div className={styles.conversationItem} key={conv.id}>
                      <span className={styles.conversationIcon}>
                        <MessageOutlined />
                      </span>
                      <div className={styles.conversationMeta}>
                        <span className={styles.conversationContent}>{conv.content}</span>
                        <span className={styles.conversationSub}>{conv.user} · {conv.time}</span>
                      </div>
                      <span className={`${styles.conversationStatus} ${styles[`conversationStatus_${status.className}`]}`}>
                        {status.label}
                      </span>
                    </div>
                  );
                })}
              </div>
            </section>

            <section className={styles.overviewCard}>
              <div className={styles.overviewCardHeader}>
                <span className={styles.overviewCardTitle}>操作</span>
                <span className={styles.overviewCardHint}>控制当前 Agent 的运行状态</span>
              </div>
              <div className={styles.overviewActions}>
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

        {activeTab === 'overview' && editing && (
          <>
            <section className={styles.section}>
              <div className={styles.sectionTitle}>基本资料</div>
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
              <div className={styles.sectionTitle}>能力说明</div>
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
              <div className={styles.sectionTitle}>运行信息</div>
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
              <div className={styles.sectionTitle}>操作</div>
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

        {activeTab === 'activity' && (
          <section className={styles.section}>
            <div className={styles.activityPlaceholder}>
              <HistoryOutlined className={styles.activityPlaceholderIcon} />
              <div className={styles.activityPlaceholderTitle}>运行记录即将上线</div>
              <div className={styles.activityPlaceholderDesc}>
                将在此处展示 Agent 的启动记录、工具调用日志和任务执行历史。
              </div>
            </div>
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
