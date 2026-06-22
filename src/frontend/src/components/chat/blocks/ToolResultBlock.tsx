import React, { useMemo } from 'react';
import { CheckCircleOutlined, CloseCircleOutlined } from '@ant-design/icons';
import type { MessageBlock } from '@/types/message';
import styles from './Blocks.module.css';
import { registerBlock } from './BlockRegistry';

interface ToolResultBlockProps {
  block: MessageBlock;
  /** streaming flag（registry 签名要求；tool_result 不增量，忽略该 prop） */
  streaming?: boolean;
}

/**
 * tool_result block——结果气泡。is_error 时红色样式 + 错误图标。
 *
 * streaming prop 不使用：tool_result 是一次性事件（daemon 完成工具调用后整体推送），
 * 不存在 token 级增量。保留 prop 是为了匹配 BlockSpec 统一签名 (block, streaming?)。
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

// 自注册：工具结果块——一次性推送，is_error 切红色样式
registerBlock('tool_result', { component: ToolResultBlock });
