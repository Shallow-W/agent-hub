import React, { useState, useMemo } from 'react';
import { DownOutlined, RightOutlined } from '@ant-design/icons';
import type { MessageBlock } from '@/types/message';
import styles from './Blocks.module.css';
import { registerBlock } from './BlockRegistry';

interface ThinkingBlockProps {
  block: MessageBlock;
  /** 默认折叠；点击切换 */
  defaultExpanded?: boolean;
  /** streaming 状态时显示末尾闪烁光标 */
  streaming?: boolean;
}

/**
 * thinking block——默认折叠，点击展开。
 * 同一思考过程流式累积的 text，append 到同一 block。
 */
function ThinkingBlockInner({ block, defaultExpanded = false, streaming = false }: ThinkingBlockProps) {
  const [expanded, setExpanded] = useState(defaultExpanded);
  const text = useMemo(() => block.text ?? '', [block.text]);
  return (
    <div className={styles.thinkingBlock}>
      <div
        className={styles.thinkingHeader}
        role="button"
        tabIndex={0}
        onClick={() => setExpanded((v) => !v)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            setExpanded((v) => !v);
          }
        }}
      >
        {expanded ? <DownOutlined /> : <RightOutlined />}
        <span>思考过程</span>
        {streaming && !expanded && <span className={styles.streamingCursor} aria-hidden />}
      </div>
      {expanded && (
        <div className={styles.thinkingBody}>
          {text}
          {streaming && <span className={styles.streamingCursor} aria-hidden />}
        </div>
      )}
    </div>
  );
}

export const ThinkingBlock = React.memo(ThinkingBlockInner);

// 自注册：思考块——默认折叠，streaming 时显示光标
registerBlock('thinking', { component: ThinkingBlock });
