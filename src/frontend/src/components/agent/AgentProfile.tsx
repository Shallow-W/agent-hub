import React, { useEffect, useState } from 'react';
import { Avatar, Button, Input, Popconfirm, Tag, message } from 'antd';
import {
  BellOutlined,
  MessageOutlined,
  RobotOutlined,
  SafetyOutlined,
  DeleteOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  SaveOutlined,
} from '@ant-design/icons';
import type { Agent } from '@/types/agent';
import { useAgentStore } from '@/store/agentStore';
import {
  formatDateTime,
  getAgentDescription,
  getModelLabel,
  getRuntimeLabel,
  parseCapabilities,
} from './agentPresentation';
import styles from './AgentProfile.module.css';

interface AgentProfileProps {
  agent: Agent | null;
}

const tabItems = [
  { key: 'profile', label: 'PROFILE' },
  { key: 'skills', label: 'SKILLS' },
  { key: 'permissions', label: 'PERMISSIONS', icon: <SafetyOutlined /> },
  { key: 'dms', label: 'AGENT DMS', icon: <MessageOutlined /> },
  { key: 'reminders', label: 'REMINDERS', icon: <BellOutlined /> },
];

function getStatusText(agent: Agent): string {
  return agent.status === 'online' ? 'Online' : agent.status;
}

export const AgentProfile: React.FC<AgentProfileProps> = ({ agent }) => {
  const updateAgent = useAgentStore((s) => s.updateAgent);
  const deleteAgent = useAgentStore((s) => s.deleteAgent);
  const [activeTab, setActiveTab] = useState('profile');
  const [name, setName] = useState('');
  const [avatar, setAvatar] = useState('');
  const [descriptionValue, setDescriptionValue] = useState('');
  const [tagsValue, setTagsValue] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!agent) return;
    const capabilities = parseCapabilities(agent.capabilities_json);
    setActiveTab('profile');
    setName(agent.name);
    setAvatar(agent.avatar ?? '');
    setDescriptionValue(agent.system_prompt ?? getAgentDescription(agent));
    setTagsValue(capabilities.join(', '));
  }, [agent]);

  if (!agent) {
    return (
      <div className={styles.emptyState}>
        选择一个 Agent 查看运行配置和能力说明
      </div>
    );
  }

  const capabilities = parseCapabilities(agent.capabilities_json);
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
    setSaving(true);
    try {
      await updateAgent(agent.id, {
        name: nextName,
        cli_tool: agent.cli_tool,
        avatar: avatar.trim() || undefined,
        system_prompt: descriptionValue.trim() || undefined,
        capabilities_json: JSON.stringify(editableTags),
      });
      message.success('Agent Profile 已保存');
    } catch {
      message.error('保存 Agent 失败');
    } finally {
      setSaving(false);
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
            {item.icon} {item.key === 'skills' ? `SKILLS (${capabilities.length})` : item.label}
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
            <Input.TextArea
              autoSize={{ minRows: 3, maxRows: 6 }}
              value={descriptionValue}
              onChange={(event) => setDescriptionValue(event.target.value)}
              placeholder="描述这个 Agent 的角色、边界和工作风格"
            />
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
            <Button icon={<PlayCircleOutlined />} onClick={() => message.info('启动 Agent 的后端接口接入后即可执行')}>
              启动 Agent
            </Button>
            <Button icon={<ReloadOutlined />} onClick={() => message.info('重启 Agent 的后端接口接入后即可执行')}>
              重启 Agent
            </Button>
            <Popconfirm title="确定删除这个 Agent？" okText="删除" cancelText="取消" onConfirm={handleDelete}>
              <Button danger icon={<DeleteOutlined />}>
                删除 Agent
              </Button>
            </Popconfirm>
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
            <div className={styles.sectionTitle}>SKILLS ({capabilities.length})</div>
            {capabilities.length === 0 ? (
              <div className={styles.emptyState}>暂无 Skill 或能力标签</div>
            ) : (
              <div className={styles.capabilityList}>
                {capabilities.map((item) => (
                  <div className={styles.capabilityItem} key={item}>
                    <strong>{item}</strong>
                    <div className={styles.value}>
                      {item} capability detected from local CLI registration.
                    </div>
                  </div>
                ))}
              </div>
            )}
          </section>
        )}
      </div>
    </div>
  );
};
