import React from 'react';
import {
  CodeOutlined,
  ConsoleSqlOutlined,
  ExperimentOutlined,
  RobotOutlined,
} from '@ant-design/icons';
import type { ReactNode } from 'react';
import styles from './AgentBaseCard.module.css';

interface AgentBaseCardProps {
  cliTool: string;
  name: string;
  version?: string;
  description?: string;
  capabilities?: string[];
}

interface BaseMeta {
  icon: ReactNode;
  color: string;
  defaultDesc: string;
  label: string;
}

const BASE_META: Record<string, BaseMeta> = {
  claude: {
    icon: <RobotOutlined />,
    color: '#d97706',
    defaultDesc: 'Anthropic Claude 系列模型命令行工具',
    label: 'Claude',
  },
  codex: {
    icon: <CodeOutlined />,
    color: '#10b981',
    defaultDesc: 'OpenAI Codex CLI 命令行工具',
    label: 'Codex',
  },
  opencode: {
    icon: <ConsoleSqlOutlined />,
    color: '#3b82f6',
    defaultDesc: '开源 OpenCode 命令行 Agent',
    label: 'OpenCode',
  },
  openclaw: {
    icon: <ExperimentOutlined />,
    color: '#8b5cf6',
    defaultDesc: 'OpenClaw 命令行 Agent',
    label: 'OpenClaw',
  },
};

const FALLBACK_META: BaseMeta = {
  icon: <RobotOutlined />,
  color: '#64748b',
  defaultDesc: '未知 CLI 工具',
  label: 'Unknown',
};

export const AgentBaseCard: React.FC<AgentBaseCardProps> = ({
  cliTool,
  name,
  version,
  description,
  capabilities,
}) => {
  const meta = BASE_META[cliTool] ?? FALLBACK_META;
  const descriptionText = description?.trim() ? description : meta.defaultDesc;
  const caps = Array.isArray(capabilities) ? capabilities.slice(0, 4) : [];

  return (
    <div className={styles.container}>
      <div className={styles.iconWrap} style={{ background: `${meta.color}1f`, color: meta.color }}>
        <span className={styles.icon}>{meta.icon}</span>
      </div>
      <div className={styles.body}>
        <div className={styles.titleRow}>
          <span className={styles.name}>{name}</span>
          {version && <span className={styles.version}>v{version}</span>}
        </div>
        <div className={styles.cliTool} style={{ color: meta.color }}>
          @{meta.label}
        </div>
        <div className={styles.description}>{descriptionText}</div>
        {caps.length > 0 && (
          <div className={styles.capabilities}>
            {caps.map((cap) => (
              <span className={styles.cap} key={cap}>
                {cap}
              </span>
            ))}
          </div>
        )}
      </div>
    </div>
  );
};

export default AgentBaseCard;
