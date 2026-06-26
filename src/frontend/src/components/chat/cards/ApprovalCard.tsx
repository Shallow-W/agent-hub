import React, { useState } from 'react';
import { Button, Typography } from 'antd';
import { ExclamationCircleOutlined } from '@ant-design/icons';
import type { CardProps, ApprovalCard as ApprovalCardData } from '@/types/card';
import styles from './Cards.module.css';

/** 审批确认卡片（card_type=approval）——用户点击允许/拒绝后通过 onAction 通知后端。
 *  支持已解决状态（card.state === 'resolved'）——历史消息中显示已处理。
 *  组件名 ApprovalCard 与接口同名（约定），内部用 ApprovalCardData 别名规避合并冲突。 */
export const ApprovalCard: React.FC<CardProps<ApprovalCardData>> = ({ card, onAction }) => {
  const isResolved = card.state === 'resolved';
  const persistedAction = card.selected_action;

  const [resolved, setResolved] = useState<string>(persistedAction || '');

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
            {isResolved ? '已处理' : '已确认'}
          </Typography.Text>
        </div>
        <Typography.Text type="secondary">{card.message}</Typography.Text>
        {action && (
          <div style={{ marginTop: 8 }}>
            <Typography.Text type={action.style === 'danger' ? 'danger' : 'success'}>
              {action.label}
            </Typography.Text>
          </div>
        )}
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
