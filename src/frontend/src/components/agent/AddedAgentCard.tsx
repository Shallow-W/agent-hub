import React from 'react';
import { Avatar, Button, Tooltip } from 'antd';
import {
  CaretRightOutlined,
  DeleteOutlined,
  EllipsisOutlined,
  MessageOutlined,
  PoweroffOutlined,
  ReloadOutlined,
  RobotOutlined,
} from '@ant-design/icons';
import type { Agent, AgentStatus } from '@/types/agent';
import { resolveAgentAvatar } from './agentPresentation';
import { StatusBadge, type StatusBadgeStatus } from '@/components/common/StatusBadge';
import styles from './AddedAgentCard.module.css';

interface AddedAgentCardProps {
  agent: Agent;
  isActive?: boolean;
  lifecycleLoading?: boolean;
  onOpenChat?: (agentId: string) => void;
  onSelect?: (agent: Agent) => void;
  onToggle?: (agentId: string, action: 'start' | 'stop' | 'restart') => void;
  onDelete?: (agent: Agent) => void;
}

function statusToBadge(status: AgentStatus): StatusBadgeStatus {
  switch (status) {
    case 'online':
      return 'running';
    case 'offline':
      return 'inactive';
    case 'busy':
      return 'running';
    case 'error':
      return 'error';
    case 'stopped':
      return 'idle';
    default:
      return 'inactive';
  }
}

function statusLabel(status: AgentStatus): string {
  switch (status) {
    case 'online':
      return '运行中';
    case 'offline':
      return '未运行';
    case 'busy':
      return '忙碌';
    case 'error':
      return '异常';
    case 'stopped':
      return '已停止';
    default:
      return status;
  }
}

export const AddedAgentCard: React.FC<AddedAgentCardProps> = ({
  agent,
  isActive = false,
  lifecycleLoading = false,
  onOpenChat,
  onSelect,
  onToggle,
  onDelete,
}) => {
  const isRunning = agent.status === 'online' || agent.status === 'busy';
  const canStart = agent.status === 'stopped' || agent.status === 'offline' || agent.status === 'error';
  const tags = (() => {
    if (!agent.tags || agent.tags === '[]') return [];
    try {
      const arr = JSON.parse(agent.tags);
      return Array.isArray(arr) ? arr.filter((t): t is string => typeof t === 'string') : [];
    } catch {
      return [];
    }
  })();
  const isBuiltinSystem = agent.type === 'system' && !agent.user_id;

  const handleCardClick = () => {
    onSelect?.(agent);
  };
  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      onSelect?.(agent);
    }
  };

  return (
    <div
      className={`${styles.container} ${isActive ? styles.active : ''}`}
      role="button"
      tabIndex={0}
      onClick={handleCardClick}
      onKeyDown={handleKeyDown}
    >
      <div className={styles.head}>
        <Avatar
          size={40}
          src={resolveAgentAvatar(agent)}
          icon={<RobotOutlined />}
          className={styles.avatar}
        />
        <div className={styles.info}>
          <div className={styles.titleRow}>
            <span className={styles.name}>{agent.name}</span>
            <div className={styles.badge}>
              <StatusBadge
                status={statusToBadge(agent.status)}
                label={statusLabel(agent.status)}
              />
            </div>
          </div>
          <div className={styles.sub}>
            @{agent.cli_tool}
            {agent.version ? ` · v${agent.version}` : ''}
          </div>
          {tags.length > 0 ? (
            <div className={styles.tags}>
              {tags.slice(0, 3).map((item) => (
                <span className={styles.tag} key={item}>
                  {item.length > 14 ? item.slice(0, 14) + '…' : item}
                </span>
              ))}
              {tags.length > 3 && <span className={styles.tagMore}>+{tags.length - 3}</span>}
            </div>
          ) : null}
        </div>
      </div>

      <div
        className={styles.actions}
        onClick={(e) => e.stopPropagation()}
      >
        {canStart && (
          <Tooltip title="启动">
            <Button
              type="text"
              size="small"
              icon={<CaretRightOutlined />}
              loading={lifecycleLoading}
              onClick={() => onToggle?.(agent.id, 'start')}
            />
          </Tooltip>
        )}
        {isRunning && (
          <>
            <Tooltip title="重启">
              <Button
                type="text"
                size="small"
                icon={<ReloadOutlined />}
                loading={lifecycleLoading}
                onClick={() => onToggle?.(agent.id, 'restart')}
              />
            </Tooltip>
            <Tooltip title="停止">
              <Button
                type="text"
                size="small"
                danger
                icon={<PoweroffOutlined />}
                loading={lifecycleLoading}
                onClick={() => onToggle?.(agent.id, 'stop')}
              />
            </Tooltip>
          </>
        )}
        <Tooltip title="打开对话">
          <Button
            type="text"
            size="small"
            icon={<MessageOutlined />}
            onClick={() => onOpenChat?.(agent.id)}
          />
        </Tooltip>
        {!isBuiltinSystem && onDelete && (
          <Tooltip title="删除">
            <Button
              type="text"
              size="small"
              danger
              icon={<DeleteOutlined />}
              onClick={() => onDelete?.(agent)}
            />
          </Tooltip>
        )}
        <Tooltip title="更多">
          <Button type="text" size="small" icon={<EllipsisOutlined />} disabled />
        </Tooltip>
      </div>
    </div>
  );
};

export default AddedAgentCard;
