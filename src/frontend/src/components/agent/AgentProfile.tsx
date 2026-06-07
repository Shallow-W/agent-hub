import React, { useEffect, useState } from 'react';
import { Avatar, Button, Input, Popconfirm, Switch, Tag, Typography, message } from 'antd';
import {
  BellOutlined,
  MessageOutlined,
  RobotOutlined,
  SafetyOutlined,
  SettingOutlined,
  ToolOutlined,
  DeleteOutlined,
  LinkOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  SaveOutlined,
  CloseOutlined,
  StarOutlined,
  PlusOutlined,
} from '@ant-design/icons';
import type { Agent } from '@/types/agent';
import { useAgentStore } from '@/store/agentStore';
import {
  formatDateTime,
  getAgentDescription,
  getModelLabel,
  getRuntimeLabel,
  parseCapabilities,
  parseSkills,
  autoGenerateSkills,
} from './agentPresentation';
import type { Skill } from './agentPresentation';
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
  { key: 'dms', label: 'AGENT DMS', icon: <MessageOutlined /> },
  { key: 'reminders', label: 'REMINDERS', icon: <BellOutlined /> },
];

function getStatusText(agent: Agent): string {
  return agent.status === 'online' ? 'Online' : agent.status;
}

export const AgentProfile: React.FC<AgentProfileProps> = ({ agent, defaultTab = 'profile' }) => {
  const updateAgent = useAgentStore((s) => s.updateAgent);
  const deleteAgent = useAgentStore((s) => s.deleteAgent);
  const [activeTab, setActiveTab] = useState(defaultTab);
  const [name, setName] = useState('');
  const [avatar, setAvatar] = useState('');
  const [tagsValue, setTagsValue] = useState('');
  const [skills, setSkills] = useState<Skill[]>([]);
  const [newSkillName, setNewSkillName] = useState('');
  const [editingSkillIdx, setEditingSkillIdx] = useState<number | null>(null);
  const [editingSkillName, setEditingSkillName] = useState('');
  const [systemPromptValue, setSystemPromptValue] = useState('');
  const [toolsConfigValue, setToolsConfigValue] = useState('');
  const [enableManagementTools, setEnableManagementTools] = useState(false);
  const [saving, setSaving] = useState(false);
  const [reconnectCmd, setReconnectCmd] = useState<string | null>(null);
  const [reconnecting, setReconnecting] = useState(false);

  useEffect(() => {
    if (!agent) return;
    const capabilities = parseCapabilities(agent.capabilities_json);
    setActiveTab(defaultTab);
    setName(agent.name);
    setAvatar(agent.avatar ?? '');
    setTagsValue(capabilities.join(', '));
    setSkills(parseSkills(agent.capabilities_json));
    setNewSkillName('');
    setEditingSkillIdx(null);
    setEditingSkillName('');
    setSystemPromptValue(agent.system_prompt ?? '');
    setToolsConfigValue(agent.tools_config ?? '');
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
  const modelLabel = getModelLabel(agent);
  const isOnline = agent.status === 'online';
  const computerName = agent.machine_name || 'local-computer';
  const editableTags = tagsValue.split(',').map((item) => item.trim()).filter(Boolean);

  const handleSaveProfile = async () => {
    const nextName = name.trim();
    if (!nextName) {
      message.warning('Agent 名称不能为空');
      return;
    }
    const tagList = tagsValue.split(',').map((t) => t.trim()).filter(Boolean);
    const skillsByName = new Map(skills.map((s) => [s.name, s]));
    const merged = tagList.map((tag) => skillsByName.get(tag) || { name: tag });
    setSaving(true);
    try {
      await updateAgent(agent.id, {
        name: nextName,
        cli_tool: agent.cli_tool,
        avatar: avatar.trim() || undefined,
        system_prompt: agent.system_prompt ?? '',
        tools_config: agent.tools_config ?? '',
        capabilities_json: JSON.stringify(merged),
        enable_management_tools: enableManagementTools,
      });
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
    if (skills.some((s) => s.name === trimmed)) {
      message.warning('该技能已存在');
      return;
    }
    setSkills((prev) => [...prev, { name: trimmed }]);
    setNewSkillName('');
  };

  const handleDeleteSkill = (idx: number) => {
    setSkills((prev) => prev.filter((_, i) => i !== idx));
  };

  const handleStartEditSkill = (idx: number) => {
    setEditingSkillIdx(idx);
    setEditingSkillName(skills[idx]?.name ?? '');
  };

  const handleCommitEditSkill = () => {
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

  const handleAutoGenerate = () => {
    const generated = autoGenerateSkills(agent);
    const existingNames = new Set(skills.map((s) => s.name));
    const merged = [
      ...skills,
      ...generated.filter((s) => !existingNames.has(s.name)),
    ];
    setSkills(merged);
    message.success('已自动生成技能');
  };

  const handleDelete = async () => {
    try {
      await deleteAgent(agent.id);
      message.success('Agent 已删除');
    } catch {
      message.error('删除 Agent 失败');
    }
  };

  const handleReconnect = async () => {
    if (!agent.machine_id) {
      message.warning('该 Agent 未绑定电脑，无法重连');
      return;
    }
    setReconnecting(true);
    try {
      const { getMachineConnectCommand } = await import('@/api/agent');
      const result = await getMachineConnectCommand(agent.machine_id);
      // 后端返回的 command 包含正确的 --server-url，前端不自行拼接，
      // 仅在 daemon_npm_path 存在时将 npm 包替换为本地 file: 路径
      if (result.daemon_npm_path) {
        setReconnectCmd(
          result.command.replace(
            /npx\s+@agenthub\/daemon(\S+)?/,
            `npx "@agenthub/daemon@file:${result.daemon_npm_path}"`,
          ),
        );
      } else {
        setReconnectCmd(result.command);
      }
    } catch {
      message.error('获取连接命令失败');
    } finally {
      setReconnecting(false);
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

  const handleSaveToolsConfig = async () => {
    setSaving(true);
    try {
      await updateAgent(agent.id, {
        name: agent.name,
        cli_tool: agent.cli_tool,
        system_prompt: agent.system_prompt ?? '',
        tools_config: toolsConfigValue.trim() || undefined,
        capabilities_json: agent.capabilities_json ?? '',
        enable_management_tools: enableManagementTools,
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
          <Avatar size={40} src={avatar || undefined} icon={<RobotOutlined />} />
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
            {item.icon} {item.key === 'skills' ? `SKILLS (${skills.length})` : item.label}
          </button>
        ))}
      </div>

      <div className={styles.body}>
        {activeTab === 'profile' && (
          <>
            <div className={styles.profileTop}>
              <Avatar size={74} src={avatar || undefined} icon={<RobotOutlined />} />
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
          <div className={styles.formGrid}>
            <div className={styles.field}>
              <span className={styles.label}>AVATAR</span>
              <Input value={avatar} onChange={(event) => setAvatar(event.target.value)} placeholder="头像 URL，可留空使用默认头像" />
            </div>
            <div className={styles.field}>
              <span className={styles.label}>DISPLAY NAME</span>
              <Input value={name} onChange={(event) => setName(event.target.value)} placeholder="Agent 名称" />
            </div>
          </div>
          <div className={styles.field}>
            <span className={styles.label}>DESCRIPTION</span>
            <div style={{ color: 'var(--text-secondary)', fontSize: 13, lineHeight: 1.6, whiteSpace: 'pre-wrap' }}>
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
            {!isOnline && agent.machine_id && (
              <Button icon={<LinkOutlined />} loading={reconnecting} onClick={handleReconnect}>
                重新连接
              </Button>
            )}
            {isOnline && (
              <Button icon={<PlayCircleOutlined />} onClick={() => message.info('启动 Agent 的后端接口接入后即可执行')}>
                启动 Agent
              </Button>
            )}
            <Button icon={<ReloadOutlined />} onClick={() => message.info('重启 Agent 的后端接口接入后即可执行')}>
              重启 Agent
            </Button>
            <Popconfirm title="确定删除这个 Agent？" okText="删除" cancelText="取消" onConfirm={handleDelete}>
              <Button danger icon={<DeleteOutlined />}>
                删除 Agent
              </Button>
            </Popconfirm>
          </div>
          {reconnectCmd && (
            <div className={styles.reconnectBox} style={{ marginTop: 8, padding: 12, background: 'var(--color-bg-secondary)', borderRadius: 8 }}>
              <div style={{ fontSize: 12, color: 'var(--text-secondary)', marginBottom: 6 }}>
                在目标电脑上执行以下命令重新连接：
              </div>
              <Typography.Text copyable code style={{ fontSize: 12, wordBreak: 'break-all' }}>
                {reconnectCmd}
              </Typography.Text>
            </div>
          )}
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
            <span className={`${styles.runtimeBadge} ${styles.modelBadge}`}>{modelLabel}</span>
            <span className={`${styles.runtimeBadge} ${styles.reasoningBadge}`}>Medium</span>
          </div>
        </section>

        <section className={styles.section}>
          <div className={styles.sectionTitle}>ENVIRONMENT VARIABLES</div>
          <span className={styles.envValue}>
            AGENTHUB_MACHINE_KEY=********
          </span>
        </section>
          </>
        )}

        {activeTab === 'skills' && (
          <section className={styles.section}>
            <div className={styles.skillsHeader}>
              <div className={styles.sectionTitle}>SKILLS ({skills.length})</div>
              <Button
                size="small"
                icon={<StarOutlined />}
                onClick={handleAutoGenerate}
              >
                自动生成
              </Button>
            </div>
            {skills.length === 0 ? (
              <div className={styles.skillsEmpty}>
                暂无技能，点击「自动生成」或在下方添加
              </div>
            ) : (
              <div className={styles.skillGrid}>
                {skills.map((skill, idx) => (
                  <div className={styles.skillCard} key={idx}>
                    <button
                      className={styles.skillDelete}
                      type="button"
                      onClick={() => handleDeleteSkill(idx)}
                      title="删除"
                    >
                      <CloseOutlined />
                    </button>
                    {skill.auto && (
                      <span className={styles.skillBadge}>auto</span>
                    )}
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
                placeholder="输入新技能名称"
                value={newSkillName}
                onChange={(e) => setNewSkillName(e.target.value)}
                onPressEnter={handleAddSkill}
                style={{ flex: 1 }}
              />
              <Button icon={<PlusOutlined />} onClick={handleAddSkill}>
                添加
              </Button>
            </div>
          </section>
        )}

        {activeTab === 'system_prompt' && (
          <section className={styles.section}>
            <div className={styles.sectionTitle}>系统提示词 (System Prompt)</div>
            <Input.TextArea
              autoSize={{ minRows: 8, maxRows: 24 }}
              value={systemPromptValue}
              onChange={(e) => setSystemPromptValue(e.target.value)}
              placeholder="设定 Agent 的角色、人格、行为准则和工作风格。&#10;&#10;示例：&#10;你是一个资深的 Go 后端工程师，擅长代码审查和架构设计。&#10;- 使用中文回复&#10;- 代码注释使用英文&#10;- 遵循 SOLID 原则"
              style={{ fontFamily: 'monospace' }}
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
            <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 16 }}>
              <Switch
                checked={enableManagementTools}
                onChange={setEnableManagementTools}
                checkedChildren="管理工具已启用"
                unCheckedChildren="管理工具已关闭"
              />
              <span style={{ color: 'var(--text-secondary)', fontSize: 12 }}>
                启用后 Agent 可自动管理平台上的 Agent 和电脑资源
              </span>
            </div>
            <Input.TextArea
              autoSize={{ minRows: 8, maxRows: 24 }}
              value={toolsConfigValue}
              onChange={(e) => setToolsConfigValue(e.target.value)}
              placeholder="以 Markdown 格式描述 Agent 可用的工具和调用方式。&#10;&#10;示例：&#10;## web_search&#10;- 描述：搜索互联网获取最新信息&#10;- 参数：query (string) - 搜索关键词&#10;&#10;## code_run&#10;- 描述：在沙箱中执行代码片段&#10;- 参数：language, code"
              style={{ fontFamily: 'monospace' }}
            />
            <div className={styles.actionPanel}>
              <Button icon={<SaveOutlined />} loading={saving} onClick={handleSaveToolsConfig}>
                保存工具配置
              </Button>
            </div>
          </section>
        )}
      </div>
    </div>
  );
};
