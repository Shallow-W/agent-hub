import React, { useMemo } from 'react';
import { Button } from 'antd';
import { ExclamationCircleOutlined, ReloadOutlined } from '@ant-design/icons';
import type { MessageBlock } from '@/types/message';
import styles from './Blocks.module.css';
import { registerBlock } from './BlockRegistry';

interface ErrorBlockProps {
  block: MessageBlock;
  /** streaming flag（registry 签名要求；error 不增量，忽略该 prop） */
  streaming?: boolean;
  /** 重试按钮回调（可选；registry 渲染路径不传，保留给其他调用方） */
  onRetry?: () => void;
}

/**
 * error block——红色背景 + 错误图标 + 可选重试按钮。
 * 与 optimistic failed message 渲染解耦：这里只负责 streaming 期间或 finalize 后的
 * 生成错误展示。
 *
 * streaming prop 不使用：error 是一次性事件，无 token 增量。
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

// 自注册：错误块——生成失败时展示，onRetry 由调用方可选传入（registry 路径不传）
registerBlock('error', { component: ErrorBlock });
