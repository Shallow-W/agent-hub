import React, { useState } from 'react';
import { Button, Radio, Space, Typography } from 'antd';
import { CheckCircleOutlined, RocketOutlined } from '@ant-design/icons';
import type { CardProps, PlanCard } from '@/types/card';
import styles from './Cards.module.css';

/** 方案选择卡片——用户选择方案后通过 onAction 通知后端。
 *  支持已解决状态（card.state === 'resolved'）——历史消息中显示已选择。 */
export const PlanCardView: React.FC<CardProps<PlanCard>> = ({ card, onAction }) => {
  // 从卡片持久化状态恢复（历史消息查看时）
  const isResolved = card.state === 'resolved';
  const persistedSelection = card.selected_option;

  const [selected, setSelected] = useState<string>(persistedSelection || '');
  const [submitted, setSubmitted] = useState(isResolved);

  const handleConfirm = () => {
    if (!selected) return;
    setSubmitted(true);
    onAction(card.id, 'select_plan', { option_id: selected });
  };

  return (
    <div className={styles.card}>
      <div className={styles.cardHeader}>
        <RocketOutlined className={styles.cardIcon} />
        <Typography.Text strong className={styles.cardTitle}>
          {card.title || '方案选择'}
        </Typography.Text>
        {submitted && (
          <Typography.Text type="success" className={styles.cardBadge}>
            <CheckCircleOutlined /> {isResolved ? '已选择' : '已确认'}
          </Typography.Text>
        )}
      </div>

      <Radio.Group
        value={selected}
        onChange={(e) => setSelected(e.target.value)}
        disabled={submitted}
        className={styles.radioGroup}
      >
        <Space direction="vertical" style={{ width: '100%' }}>
          {card.options.map((option) => (
            <Radio key={option.id} value={option.id} className={styles.radioItem}>
              <div className={styles.optionRow}>
                <span className={styles.optionLabel}>
                  {option.label}
                  {option.recommended && (
                    <Typography.Text type="success" className={styles.recommended}>
                      ★ 推荐
                    </Typography.Text>
                  )}
                  {/* 已解决状态下高亮选中的选项 */}
                  {submitted && selected === option.id && (
                    <Typography.Text type="success" className={styles.recommended}>
                      ✓ 已选
                    </Typography.Text>
                  )}
                </span>
                {option.description && (
                  <Typography.Text type="secondary" className={styles.optionDesc}>
                    {option.description}
                  </Typography.Text>
                )}
                {option.tasks && option.tasks.length > 0 && (
                  <div className={styles.optionTasks}>
                    {option.tasks.map((task, i) => (
                      <span key={i} className={styles.taskChip}>{task}</span>
                    ))}
                  </div>
                )}
              </div>
            </Radio>
          ))}
        </Space>
      </Radio.Group>

      {!submitted && (
        <div className={styles.cardFooter}>
          <Button type="primary" size="small" disabled={!selected} onClick={handleConfirm}>
            确认选择
          </Button>
        </div>
      )}
    </div>
  );
};
