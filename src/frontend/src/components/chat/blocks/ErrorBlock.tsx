import React, { useMemo } from 'react';
import { Button } from 'antd';
import { ExclamationCircleOutlined, ReloadOutlined } from '@ant-design/icons';
import type { MessageBlock } from '@/types/message';
import styles from './Blocks.module.css';

interface ErrorBlockProps {
  block: MessageBlock;
  /** 重试按钮回调（可选） */
  onRetry?: () => void;
}

/**
 * error block——红色背景 + 错误图标 + 可选重试按钮。
 * 与 optimistic failed message 渲染解耦：这里只负责 streaming 期间或 finalize 后的
 * 生成错误展示。
 */
function ErrorBlockInner({ block, onRetry }: ErrorBlockProps) {
  const text = useMemo(() => block.text || '生成失败，请重试', [block.text]);
  return (
    <div className={styles.errorBlock}>
      <div className={styles.errorHeader}>
        <ExclamationCircleOutlined />
        <span>生成失败</span>
      </div>
      <div className={styles.errorBody}>{text}</div>
      {onRetry && (
        <Button
          type="link"
          size="small"
          icon={<ReloadOutlined />}
          onClick={onRetry}
          className={styles.retryBtn}
        >
          重试
        </Button>
      )}
    </div>
  );
}

export const ErrorBlock = React.memo(ErrorBlockInner);
