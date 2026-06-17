import React, { useState } from 'react';
import { Button, Typography } from 'antd';
import { ExclamationCircleOutlined } from '@ant-design/icons';
import type { CardProps, ConfirmCard } from '@/types/card';
import styles from './Cards.module.css';

/** 确认操作卡片——用户点击允许/拒绝后通过 onAction 通知后端 */
export const ConfirmCardView: React.FC<CardProps<ConfirmCard>> = ({ card, onAction }) => {
  const [resolved, setResolved] = useState<string>('');

  const handleAction = (actionId: string) => {
    setResolved(actionId);
    onAction(card.id, 'confirm', { action_id: actionId });
  };

  if (resolved) {
    const action = card.actions.find((a) => a.id === resolved);
    return (
      <div className={styles.card}>
        <div className={styles.cardHeader}>
          <ExclamationCircleOutlined className={styles.cardIcon} />
          <Typography.Text strong className={styles.cardTitle}>
            {card.title || '操作确认'}
          </Typography.Text>
          <Typography.Text type={action?.style === 'danger' ? 'danger' : 'success'}>
            {action?.label || '已处理'}
          </Typography.Text>
        </div>
        <Typography.Text type="secondary">{card.message}</Typography.Text>
      </div>
    );
  }

  return (
    <div className={styles.card}>
      <div className={styles.cardHeader}>
        <ExclamationCircleOutlined className={styles.cardIcon} />
        <Typography.Text strong className={styles.cardTitle}>
          {card.title || '需要确认'}
        </Typography.Text>
      </div>

      <Typography.Text className={styles.confirmMessage}>{card.message}</Typography.Text>

      <div className={styles.cardFooter}>
        {card.actions.map((action) => (
          <Button
            key={action.id}
            size="small"
            danger={action.style === 'danger'}
            type={action.style === 'primary' ? 'primary' : 'default'}
            onClick={() => handleAction(action.id)}
          >
            {action.label}
          </Button>
        ))}
      </div>
    </div>
  );
};
