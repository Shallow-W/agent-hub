import React from 'react';
import styles from './StatusBadge.module.css';

export type StatusBadgeStatus =
  | 'running'
  | 'idle'
  | 'connected'
  | 'disconnected'
  | 'error'
  | 'warning'
  | 'inactive';

interface StatusBadgeProps {
  status: StatusBadgeStatus;
  label?: string;
  size?: 'sm' | 'md';
  withDot?: boolean;
}

const DEFAULT_LABELS: Record<StatusBadgeStatus, string> = {
  running: '运行中',
  connected: '已连接',
  disconnected: '已断开',
  idle: '空闲',
  inactive: '未激活',
  error: '异常',
  warning: '警告',
};

const COLOR_TONE: Record<StatusBadgeStatus, string> = {
  running: 'success',
  connected: 'success',
  idle: 'neutral',
  inactive: 'neutral',
  disconnected: 'neutral',
  error: 'danger',
  warning: 'warning',
};

export const StatusBadge: React.FC<StatusBadgeProps> = ({
  status,
  label,
  size = 'sm',
  withDot = true,
}) => {
  const tone = COLOR_TONE[status];
  const text = label ?? DEFAULT_LABELS[status];
  const sizeClass = size === 'md' ? styles.md : styles.sm;
  const toneClass = styles[tone] ?? styles.neutral;

  return (
    <span className={`${styles.badge} ${sizeClass} ${toneClass}`}>
      {withDot && <span className={styles.dot} />}
      <span className={styles.label}>{text}</span>
    </span>
  );
};

export default StatusBadge;
