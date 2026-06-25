import React, { useState, useMemo } from 'react';
import {
  CheckCircleOutlined,
  CloseCircleOutlined,
  DownOutlined,
  RightOutlined,
} from '@ant-design/icons';
import type { MessageBlock } from '@/types/message';
import type { BlockRenderContext } from './BlockRegistry';
import styles from './Blocks.module.css';
import { registerBlock } from './BlockRegistry';

interface ToolResultBlockProps {
  block: MessageBlock;
  /** 默认折叠；点击切换（与 ToolCallBlock 一致） */
  defaultExpanded?: boolean;
  /** streaming flag（registry 签名要求；tool_result 不增量，忽略该 prop） */
  streaming?: boolean;
  /** registry 签名要求；ToolResultBlock 不依赖上下文，忽略 */
  ctx?: BlockRenderContext;
}

/**
 * tool_result block——结果气泡。is_error 时红色样式 + 错误图标。
 *
 * streaming prop 不使用：tool_result 是一次性事件（daemon 完成工具调用后整体推送），
 * 不存在 token 级增量。保留 prop 是为了匹配 BlockSpec 统一签名 (block, streaming?)。
 *
 * 折叠行为与 ToolCallBlock 对齐：默认折叠，点击 header toggle；无文本时 fallback 到结果状态图标。
 */
function ToolResultBlockInner({
  block,
  defaultExpanded = false,
}: ToolResultBlockProps) {
  const [expanded, setExpanded] = useState(defaultExpanded);
  const isError = block.is_error === true;
  const text = useMemo(() => block.text ?? '', [block.text]);
  const hasText = text.length > 0;
  const resultIcon = isError ? <CloseCircleOutlined /> : <CheckCircleOutlined />;
  return (
    <div className={`${styles.toolResultBlock} ${isError ? styles.error : ''}`}>
      <div
        className={styles.toolResultHeader}
        role="button"
        tabIndex={0}
        onClick={() => hasText && setExpanded((v) => !v)}
        onKeyDown={(e) => {
          if ((e.key === 'Enter' || e.key === ' ') && hasText) {
            e.preventDefault();
            setExpanded((v) => !v);
          }
        }}
      >
        {hasText ? (expanded ? <DownOutlined /> : <RightOutlined />) : resultIcon}
        <span>{isError ? '工具执行失败' : '工具结果'}</span>
      </div>
      {expanded && hasText && (
        <div className={styles.toolResultBody}>{text}</div>
      )}
    </div>
  );
}

export const ToolResultBlock = React.memo(ToolResultBlockInner);

// 自注册：工具结果块——一次性推送，is_error 切红色样式
registerBlock('tool_result', { component: ToolResultBlock });
