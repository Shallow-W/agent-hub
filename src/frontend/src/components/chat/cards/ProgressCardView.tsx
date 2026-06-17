import React from 'react';
import { Typography, Progress } from 'antd';
import {
  CheckCircleOutlined,
  ClockCircleOutlined,
  CloseCircleOutlined,
  LoadingOutlined,
} from '@ant-design/icons';
import type { CardProps, ProgressCard, ProgressTask } from '@/types/card';
import styles from './Cards.module.css';

const statusIcon: Record<ProgressTask['status'], React.ReactNode> = {
  done: <CheckCircleOutlined style={{ color: '#52c41a' }} />,
  running: <LoadingOutlined style={{ color: '#1677ff' }} />,
  pending: <ClockCircleOutlined style={{ color: '#d9d9d9' }} />,
  failed: <CloseCircleOutlined style={{ color: '#ff4d4f' }} />,
};

/** 执行进度卡片——显示 task list + 状态图标 + 完成百分比 */
export const ProgressCardView: React.FC<CardProps<ProgressCard>> = ({ card }) => {
  const total = card.tasks.length;
  const done = card.tasks.filter((t) => t.status === 'done').length;
  const percent = total > 0 ? Math.round((done / total) * 100) : 0;
  const allDone = done === total;
  const hasFailed = card.tasks.some((t) => t.status === 'failed');

  return (
    <div className={styles.card}>
      <div className={styles.cardHeader}>
        <Typography.Text strong className={styles.cardTitle}>
          {card.title || '执行进度'}
        </Typography.Text>
        <Typography.Text type={hasFailed ? 'danger' : allDone ? 'success' : 'secondary'}>
          {done}/{total} 完成
        </Typography.Text>
      </div>

      <Progress percent={percent} size="small" status={hasFailed ? 'exception' : undefined} />

      <div className={styles.taskList}>
        {card.tasks.map((task, idx) => (
          <div key={idx} className={styles.progressTask}>
            {statusIcon[task.status]}
            <Typography.Text
              delete={task.status === 'failed'}
              type={task.status === 'done' ? 'secondary' : undefined}
              className={styles.progressTaskName}
            >
              {task.name}
            </Typography.Text>
          </div>
        ))}
      </div>
    </div>
  );
};
