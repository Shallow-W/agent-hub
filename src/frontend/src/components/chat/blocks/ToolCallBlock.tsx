import React, { useState, useMemo } from 'react';
import { DownOutlined, RightOutlined, ToolOutlined } from '@ant-design/icons';
import type { MessageBlock } from '@/types/message';
import styles from './Blocks.module.css';

interface ToolCallBlockProps {
  block: MessageBlock;
  /** 默认折叠；点击切换 */
  defaultExpanded?: boolean;
  /** streaming 状态时显示末尾闪烁光标 */
  streaming?: boolean;
}

/**
 * tool_use block——工具名 chip + 流式累积的入参。
 * tool_name 非空时开启新 block，后续 input_json_delta 追加到 text。
 */
function ToolCallBlockInner({ block, defaultExpanded = false, streaming = false }: ToolCallBlockProps) {
  const [expanded, setExpanded] = useState(defaultExpanded);
  const toolName = useMemo(() => block.tool_name ?? 'tool', [block.tool_name]);
  const text = useMemo(() => block.text ?? '', [block.text]);
  const hasInput = text.length > 0;
  return (
    <div className={styles.toolCallBlock}>
      <div
        className={styles.toolCallHeader}
        role="button"
        tabIndex={0}
        onClick={() => hasInput && setExpanded((v) => !v)}
        onKeyDown={(e) => {
          if ((e.key === 'Enter' || e.key === ' ') && hasInput) {
            e.preventDefault();
            setExpanded((v) => !v);
          }
        }}
      >
        {hasInput ? (expanded ? <DownOutlined /> : <RightOutlined />) : <ToolOutlined />}
        <span className={styles.toolChip}>{toolName}</span>
        {streaming && !hasInput && <span className={styles.streamingCursor} aria-hidden />}
      </div>
      {expanded && hasInput && (
        <div className={styles.toolInput}>
          {text}
          {streaming && <span className={styles.streamingCursor} aria-hidden />}
        </div>
      )}
    </div>
  );
}

export const ToolCallBlock = React.memo(ToolCallBlockInner);
