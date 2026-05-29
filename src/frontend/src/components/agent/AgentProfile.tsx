import React from 'react';
import { Avatar, Button } from 'antd';
import {
  AppstoreOutlined,
  BellOutlined,
  FolderOpenOutlined,
  LinkOutlined,
  MessageOutlined,
  RobotOutlined,
  SafetyOutlined,
} from '@ant-design/icons';
import type { Agent } from '@/types/agent';
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
  { key: 'permissions', label: 'PERMISSIONS', icon: <SafetyOutlined /> },
  { key: 'dms', label: 'AGENT DMS', icon: <MessageOutlined /> },
  { key: 'reminders', label: 'REMINDERS', icon: <BellOutlined /> },
  { key: 'workspace', label: 'WORKSPACE', icon: <FolderOpenOutlined /> },
  { key: 'apps', label: 'APPS', icon: <AppstoreOutlined /> },
  { key: 'activity', label: 'ACTIVITY', icon: <LinkOutlined /> },
];

function getStatusText(agent: Agent): string {
  return agent.status === 'online' ? 'Online' : agent.status;
}

export const AgentProfile: React.FC<AgentProfileProps> = ({ agent }) => {
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

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.identity}>
          <Avatar size={40} icon={<RobotOutlined />} />
          <div className={styles.titleBlock}>
            <span className={styles.title}>{agent.name}</span>
            <span className={styles.subtitle}>{description}</span>
          </div>
        </div>
        <div className={styles.actions}>
          <Button icon={<MessageOutlined />}>Message</Button>
          <Button>配置</Button>
          <Button>刷新</Button>
        </div>
      </div>

      <div className={styles.tabs}>
        {tabItems.map((item) => (
          <button
            className={`${styles.tab} ${item.key === 'profile' ? styles.tabActive : ''}`}
            key={item.key}
            type="button"
          >
            {item.icon} {item.label}
          </button>
        ))}
      </div>

      <div className={styles.body}>
        <div className={styles.profileTop}>
          <Avatar size={74} icon={<RobotOutlined />} />
          <div>
            <div className={styles.profileName}>
              {agent.name}
              <span
                className={`${styles.statusDot} ${isOnline ? '' : styles.offlineDot}`}
              />
              <span className={styles.value}>{getStatusText(agent)}</span>
            </div>
            <div className={styles.handle}>@{agent.cli_tool}</div>
          </div>
        </div>

        <section className={styles.section}>
          <div className={styles.field}>
            <span className={styles.label}>DISPLAY NAME</span>
            <div className={styles.value}>{agent.name}</div>
          </div>
          <div className={styles.field}>
            <span className={styles.label}>DESCRIPTION</span>
            <div className={styles.value}>{description}</div>
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

        <section className={styles.section}>
          <div className={styles.sectionTitle}>SKILLS ({capabilities.length})</div>
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
        </section>
      </div>
    </div>
  );
};
