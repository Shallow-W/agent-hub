import React from 'react';
import { Typography } from 'antd';
import { InfoCircleOutlined } from '@ant-design/icons';
import type { CardProps, InfoCard as InfoCardData } from '@/types/card';
import styles from './Cards.module.css';

/** 信息展示卡片（card_type=info）—— 键值对表格，只读。
 *  无 onAction、无 reduceAction——纯展示 Agent 提供的结构化摘要。
 *  组件名 InfoCard 与接口同名（约定），内部用 InfoCardData 别名规避 TS declaration merging 冲突。 */
export const InfoCard: React.FC<CardProps<InfoCardData>> = ({ card }) => {
  const entries = Object.entries(card.fields ?? {});

  return (
    <div className={styles.card}>
      <div className={styles.cardHeader}>
        <InfoCircleOutlined className={styles.cardIcon} />
        <Typography.Text strong className={styles.cardTitle}>
          {card.title || '信息'}
        </Typography.Text>
      </div>

      {entries.length > 0 ? (
        <table className={styles.infoFieldTable}>
          <tbody>
            {entries.map(([key, value], idx) => (
              <tr key={`${key}-${idx}`} className={styles.infoFieldRow}>
                <td className={styles.infoFieldKey}>{key}</td>
                <td className={styles.infoFieldValue}>{String(value)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : (
        <Typography.Text type="secondary">（无内容）</Typography.Text>
      )}
    </div>
  );
};
