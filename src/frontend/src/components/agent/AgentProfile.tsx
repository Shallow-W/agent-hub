import React, { useEffect, useState } from 'react';
import { Avatar, Button, Checkbox, Input, Popconfirm, Select, Switch, Tag, message } from 'antd';
import {
  MessageOutlined,
  RobotOutlined,
  SafetyOutlined,
  SettingOutlined,
  ToolOutlined,
  DeleteOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  SaveOutlined,
  CloseOutlined,
  StarOutlined,
  PlusOutlined,
  DownOutlined,
  RightOutlined,
} from '@ant-design/icons';
import type { Agent } from '@/types/agent';
import { useAgentStore } from '@/store/agentStore';
import { AvatarPickerModal } from './AvatarPickerModal';
import {
  formatDateTime,
  getAgentDescription,
  getRuntimeLabel,
  parseSkills,
  autoGenerateSkills,
  resolveAgentAvatar,
} from './agentPresentation';
import type { Skill } from './agentPresentation';
import {
  getTemplateTools,
  parseToolsConfig,
  toolCatalog,
  toolsetTemplates,
  toolsConfigToJSON,
  toolsetOptions,
} from './toolAssignments';
import styles from './AgentProfile.module.css';

interface AgentProfileProps {
  agent: Agent | null;
  defaultTab?: string;
}

const tabItems = [
  { key: 'profile', label: 'PROFILE' },
  { key: 'skills', label: 'SKILLS' },
  { key: 'permissions', label: 'PERMISSIONS', icon: <SafetyOutlined /> },
  { key: 'system_prompt', label: '系统提示词', icon: <SettingOutlined /> },
  { key: 'tools_config', label: '工具配置', icon: <ToolOutlined /> },
];

function getStatusText(agent: Agent): string {
  return agent.status === 'online' ? 'Online' : agent.status;
}

export const AgentProfile: React.FC<AgentProfileProps> = ({ agent, defaultTab = 'profile' }) => {
  const updateAgent = useAgentStore((s) => s.updateAgent);
  const updateAgentTags = useAgentStore((s) => s.updateAgentTags);
  const updateCustomSkills = useAgentStore((s) => s.updateCustomSkills);
  const deleteAgent = useAgentStore((s) => s.deleteAgent);
  const [activeTab, setActiveTab] = useState(defaultTab);
  const [name, setName] = useState('');
  const [avatar, setAvatar] = useState('');
  const [tagsValue, setTagsValue] = useState('');
  const [baseSkills, setBaseSkills] = useState<Skill[]>([]);
  const [customSkills, setCustomSkills] = useState<Skill[]>([]);
  const [newSkillName, setNewSkillName] = useState('');
  const [editingSkillIdx, setEditingSkillIdx] = useState<number | null>(null);
  const [editingSkillName, setEditingSkillName] = useState('');
  const [savingSkills, setSavingSkills] = useState(false);
  const [systemPromptValue, setSystemPromptValue] = useState('');
  const [toolsConfigValue, setToolsConfigValue] = useState('');
  const [selectedToolset, setSelectedToolset] = useState('tasks');
  const [selectedTools, setSelectedTools] = useState<string[]>(getTemplateTools('tasks'));
  const [enableManagementTools, setEnableManagementTools] = useState(false);
  const [saving, setSaving] = useState(false);
  const [avatarPickerOpen, setAvatarPickerOpen] = useState(false);
  const [baseSkillsExpanded, setBaseSkillsExpanded] = useState(true);
  const [customSkillsExpanded, setCustomSkillsExpanded] = useState(true);

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

  const parseCustomSkills = (raw?: string): Skill[] => {
    if (!raw) return [];
    try {
      const arr = JSON.parse(raw);
      return Array.isArray(arr) ? arr.filter((s: unknown): s is Skill => typeof s === 'object' && s !== null && 'name' in s) : [];
    } catch {
      return [];
    }
  };

  const customSkillsToJSON = (skills: Skill[]): string => {
    return skills.length > 0 ? JSON.stringify(skills.map((s) => ({ name: s.name, description: s.description }))) : '';
  };

  useEffect(() => {
    if (!agent) return;
    setActiveTab(defaultTab);
    setName(agent.name);
    setAvatar(agent.avatar ?? '');
    setTagsValue(parseTagsFromJSON(agent.tags ?? ''));
    setBaseSkills(parseSkills(agent.capabilities_json));
    setCustomSkills(parseCustomSkills(agent.custom_skills));
    setNewSkillName('');
    setEditingSkillIdx(null);
    setEditingSkillName('');
    setSystemPromptValue(agent.system_prompt ?? '');
    const parsedTools = parseToolsConfig(agent.tools_config);
    setSelectedToolset(parsedTools.toolset);
    setSelectedTools(parsedTools.allowedTools);
    setToolsConfigValue(toolsConfigToJSON(parsedTools.toolset, parsedTools.allowedTools));
    setEnableManagementTools(agent.enable_management_tools ?? false);
  }, [agent?.id]);

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
          enable_management_tools: enableManagementTools,
        });
      }
      message.success('Agent Profile 已保存');
    } catch {
      message.error('保存 Agent 失败');
    } finally {
      setSaving(false);
    }
  };

  const handleAddSkill = () => {
    const trimmed = newSkillName.trim();
    if (!trimmed) return;
    if (customSkills.some((s) => s.name === trimmed)) {
      message.warning('该技能已存在');
      return;
    }
    setCustomSkills((prev) => [...prev, { name: trimmed }]);
    setNewSkillName('');
  };

  const handleDeleteSkill = (idx: number) => {
    setCustomSkills((prev) => prev.filter((_, i) => i !== idx));
  };

  const handleStartEditSkill = (idx: number) => {
    setEditingSkillIdx(idx);
    setEditingSkillName(customSkills[idx]?.name ?? '');
  };

  const handleCommitEditSkill = () => {
    if (editingSkillIdx === null) return;
    const trimmed = editingSkillName.trim();
    if (trimmed) {
      setCustomSkills((prev) =>
        prev.map((s, i) => (i === editingSkillIdx ? { ...s, name: trimmed } : s))
      );
    }
    setEditingSkillIdx(null);
    setEditingSkillName('');
  };

  const handleAutoGenerate = () => {
    const generated = autoGenerateSkills(agent);
    const existingNames = new Set(customSkills.map((s) => s.name));
    const merged = [
      ...customSkills,
      ...generated.filter((s) => !existingNames.has(s.name)),
    ];
    setCustomSkills(merged);
    message.success('已自动生成技能');
  };

  const handleSaveCustomSkills = async () => {
    setSavingSkills(true);
    try {
      await updateCustomSkills(agent.id, customSkillsToJSON(customSkills));
      message.success('平台 Skills 已保存');
    } catch {
      message.error('保存平台 Skills 失败');
    } finally {
      setSavingSkills(false);
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
        system_prompt: systemPromptValue.trim() || undefined,
        tools_config: agent.tools_config ?? '',
        capabilities_json: agent.capabilities_json ?? '',
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
    const nextTools = value in toolsetTemplates ? getTemplateTools(value) : selectedTools;
    setSelectedTools(nextTools);
    setToolsConfigValue(toolsConfigToJSON(value, nextTools));
  };

  const handleToolsChange = (values: string[]) => {
    setSelectedToolset('custom');
    setSelectedTools(values);
    setToolsConfigValue(toolsConfigToJSON('custom', values));
  };

  const handleSaveToolsConfig = async () => {
    setSaving(true);
    try {
      const nextToolsConfig = toolsConfigToJSON(selectedToolset, selectedTools);
      await updateAgent(agent.id, {
        name: agent.name,
        cli_tool: agent.cli_tool,
        system_prompt: agent.system_prompt ?? '',
        tools_config: nextToolsConfig,
        capabilities_json: agent.capabilities_json ?? '',
        enable_management_tools: enableManagementTools,
      });
      setToolsConfigValue(nextToolsConfig);
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
          <Button icon={<MessageOutlined />}>Message</Button>
          <Button icon={<SaveOutlined />} loading={saving} onClick={handleSaveProfile}>保存</Button>
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
            {item.icon} {item.key === 'skills' ? `SKILLS (${customSkills.length})` : item.label}
          </button>
        ))}
      </div>

      <div className={styles.body}>
        {activeTab === 'profile' && (
          <>
            <div className={styles.profileTop}>
              <Avatar
                className={styles.clickableAvatar}
                size={74}
                src={avatar.trim() ? resolveAgentAvatar({ ...agent, avatar }) : resolveAgentAvatar(agent)}
                icon={<RobotOutlined />}
                onClick={() => setAvatarPickerOpen(true)}
              />
              <div className={styles.profileSummary}>
                <div className={styles.profileName}>
                  {agent.name}
                  <span className={`${styles.statusDot} ${isOnline ? '' : styles.offlineDot}`} />
                  <span className={styles.value}>{getStatusText(agent)}</span>
                </div>
                <div className={styles.handle}>@{agent.cli_tool}</div>
              </div>
            </div>

        <section className={styles.section}>
          <div className={styles.field}>
            <span className={styles.label}>DISPLAY NAME</span>
            <Input value={name} onChange={(event) => setName(event.target.value)} placeholder="Agent 名称" />
          </div>
          <div className={styles.field}>
            <span className={styles.label}>DESCRIPTION</span>
            <div className={styles.descriptionText}>
              {description}
            </div>
          </div>
          <div className={styles.field}>
            <span className={styles.label}>TAGS</span>
            <Input value={tagsValue} onChange={(event) => setTagsValue(event.target.value)} placeholder="coding, review, orchestration" />
            {editableTags.length > 0 && (
              <div className={styles.tagPreview}>
                {editableTags.map((item) => <Tag key={item}>{item}</Tag>)}
              </div>
            )}
          </div>
        </section>

        <section className={styles.section}>
          <div className={styles.sectionTitle}>ACTIONS</div>
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

        <section className={styles.section}>
          <div className={styles.sectionTitle}>INFO</div>
          <div className={styles.infoGrid}>
            <div>
              <span className={styles.label}>Computer</span>
              <div className={styles.value}>
                {computerName} · {agent.source} · {formatDateTime(agent.last_seen_at)}
              </div>
            </div>
            <div>
              <span className={styles.label}>Created</span>
              <div className={styles.value}>{formatDateTime(agent.created_at)}</div>
            </div>
            <div>
              <span className={styles.label}>Type</span>
              <div className={styles.value}>{agent.type === 'custom' ? '自建 Agent' : '系统 Agent'}</div>
            </div>
            <div>
              <span className={styles.label}>Version</span>
              <div className={styles.value}>{agent.version || '未上报版本'}</div>
            </div>
          </div>
        </section>

        <section className={styles.section}>
          <div className={styles.sectionTitle}>RUNTIME CONFIGURATION</div>
          <div className={styles.runtimeRow}>
            <span className={styles.runtimeBadge}>{runtimeLabel}</span>
          </div>
        </section>
          </>
        )}

        {activeTab === 'skills' && (
          <>
            <section className={styles.section}>
              <button
                className={styles.sectionToggle}
                type="button"
                onClick={() => setBaseSkillsExpanded((v) => !v)}
              >
                {baseSkillsExpanded ? <DownOutlined /> : <RightOutlined />}
                <span className={styles.sectionTitle}>底座自带 Skills (只读)</span>
                <span className={styles.sectionCount}>{baseSkills.length}</span>
              </button>
              {baseSkillsExpanded && (
                baseSkills.length === 0 ? (
                  <div className={styles.skillsEmpty}>该 Agent 底座未上报技能信息</div>
                ) : (
                  <div className={styles.skillGrid}>
                    {baseSkills.map((skill, idx) => (
                      <div className={styles.skillCard} key={idx}>
                        {skill.auto && (
                          <span className={styles.skillBadge}>auto</span>
                        )}
                        <div className={styles.skillName}>{skill.name}</div>
                        {skill.description && (
                          <div className={styles.skillDesc}>{skill.description}</div>
                        )}
                      </div>
                    ))}
                  </div>
                )
              )}
            </section>

            <section className={styles.section}>
              <button
                className={styles.sectionToggle}
                type="button"
                onClick={() => setCustomSkillsExpanded((v) => !v)}
              >
                {customSkillsExpanded ? <DownOutlined /> : <RightOutlined />}
                <span className={styles.sectionTitle}>平台 Skills (可编辑)</span>
                <span className={styles.sectionCount}>{customSkills.length}</span>
              </button>
              {customSkillsExpanded && (
                <>
                  <div className={styles.skillsHeader}>
                    <div className={styles.skillActions}>
                      <Button
                        size="small"
                        icon={<StarOutlined />}
                        onClick={handleAutoGenerate}
                      >
                        自动生成
                      </Button>
                      <Button
                        size="small"
                        icon={<SaveOutlined />}
                        loading={savingSkills}
                        onClick={handleSaveCustomSkills}
                      >
                        保存
                      </Button>
                    </div>
                  </div>
                  {customSkills.length === 0 ? (
                    <div className={styles.skillsEmpty}>
                      暂无平台 Skills，点击「自动生成」或在下方添加
                    </div>
                  ) : (
                    <div className={styles.skillGrid}>
                      {customSkills.map((skill, idx) => (
                        <div className={styles.skillCard} key={idx}>
                          <button
                            className={styles.skillDelete}
                            type="button"
                            onClick={() => handleDeleteSkill(idx)}
                            title="删除"
                          >
                            <CloseOutlined />
                          </button>
                          {editingSkillIdx === idx ? (
                            <Input
                              autoFocus
                              size="small"
                              value={editingSkillName}
                              onChange={(e) => setEditingSkillName(e.target.value)}
                              onBlur={handleCommitEditSkill}
                              onPressEnter={handleCommitEditSkill}
                              className={styles.skillNameInput}
                            />
                          ) : (
                            <div
                              className={styles.skillName}
                              onClick={() => handleStartEditSkill(idx)}
                              title="点击编辑名称"
                            >
                              {skill.name}
                            </div>
                          )}
                          {skill.description && (
                            <div className={styles.skillDesc}>{skill.description}</div>
                          )}
                        </div>
                      ))}
                    </div>
                  )}
                  <div className={styles.skillAddRow}>
                    <Input
                      className={styles.inputFlex}
                      placeholder="输入新技能名称"
                      value={newSkillName}
                      onChange={(e) => setNewSkillName(e.target.value)}
                      onPressEnter={handleAddSkill}
                    />
                    <Button icon={<PlusOutlined />} onClick={handleAddSkill}>
                      添加
                    </Button>
                  </div>
                </>
              )}
            </section>
          </>
        )}

        {activeTab === 'system_prompt' && (
          <section className={styles.section}>
            <div className={styles.sectionTitle}>系统提示词 (System Prompt)</div>
            <Input.TextArea
              autoSize={{ minRows: 8, maxRows: 24 }}
              value={systemPromptValue}
              onChange={(e) => setSystemPromptValue(e.target.value)}
              placeholder="设定 Agent 的角色、人格、行为准则和工作风格。&#10;&#10;示例：&#10;你是一个资深的 Go 后端工程师，擅长代码审查和架构设计。&#10;- 使用中文回复&#10;- 代码注释使用英文&#10;- 遵循 SOLID 原则"
              className={styles.monospaceTextarea}
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
              <Switch
                checked={enableManagementTools}
                onChange={setEnableManagementTools}
                checkedChildren="管理工具已启用"
                unCheckedChildren="管理工具已关闭"
              />
              <span className={styles.toolHelpText}>
                启用后 Agent 可自动管理平台上的 Agent 和电脑资源
              </span>
            </div>
            <div className={styles.toolControlRow}>
              <span className={styles.label}>工具集模板</span>
              <Select
                className={styles.toolsetSelect}
                value={selectedToolset}
                options={toolsetOptions}
                onChange={handleToolsetChange}
              />
            </div>
            <Checkbox.Group value={selectedTools} onChange={(values) => handleToolsChange(values as string[])}>
              <div className={styles.toolGrid}>
                {toolCatalog.map((tool) => (
                  <label className={styles.toolItem} key={tool.name}>
                    <Checkbox value={tool.name} />
                    <span>
                      <span className={styles.toolName}>{tool.label}</span>
                      <span className={styles.toolMeta}>{tool.name} · {tool.category}</span>
                    </span>
                  </label>
                ))}
              </div>
            </Checkbox.Group>
            <Input.TextArea
              autoSize={{ minRows: 4, maxRows: 10 }}
              value={toolsConfigValue}
              readOnly
              className={styles.toolJsonPreview}
            />
            <div className={styles.actionPanel}>
              <Button icon={<SaveOutlined />} loading={saving} onClick={handleSaveToolsConfig}>
                保存工具配置
              </Button>
            </div>
          </section>
        )}
      </div>
      <AvatarPickerModal
        agent={agent}
        open={avatarPickerOpen}
        onClose={() => setAvatarPickerOpen(false)}
      />
    </div>
  );
};
