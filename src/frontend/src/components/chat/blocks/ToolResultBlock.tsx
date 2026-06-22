import React, { useMemo } from 'react';
import { CheckCircleOutlined, CloseCircleOutlined } from '@ant-design/icons';
import type { MessageBlock } from '@/types/message';
import styles from './Blocks.module.css';

interface ToolResultBlockProps {
  block: MessageBlock;
}

/**
 * tool_result block——结果气泡。is_error 时红色样式 + 错误图标。
 */
function ToolResultBlockInner({ block }: ToolResultBlockProps) {
  const isError = block.is_error === true;
  const text = useMemo(() => block.text ?? '', [block.text]);
  return (
    <div className={`${styles.toolResultBlock} ${isError ? styles.error : ''}`}>
      <div className={styles.toolResultHeader}>
        {isError ? <CloseCircleOutlined /> : <CheckCircleOutlined />}
        <span>{isError ? '工具执行失败' : '工具结果'}</span>
      </div>
      <div className={styles.toolResultBody}>{text}</div>
    </div>
  );
}

export const ToolResultBlock = React.memo(ToolResultBlockInner);
