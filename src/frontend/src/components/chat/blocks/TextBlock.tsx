import React, { useMemo } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import type { MessageBlock } from '@/types/message';
import type { BlockRenderContext } from './BlockRegistry';
import styles from './Blocks.module.css';
import { registerBlock } from './BlockRegistry';

const REMARK_PLUGINS = [remarkGfm];

interface TextBlockProps {
  block: MessageBlock;
  /** streaming 状态时显示末尾闪烁光标 */
  streaming?: boolean;
  /** registry 签名要求；TextBlock 不依赖上下文，忽略 */
  ctx?: BlockRenderContext;
}

/**
 * 文本 block——按 markdown 流式渲染。
 *
 * 复用 ReactMarkdown 直接渲染（未引入 MessageBubble 的 MarkdownRenderer，因为
 * 这里不需要 code artifact 接通：流式期间 artifact 尚未持久化，且完整渲染依赖
 * cards 拆段逻辑由 MessageBubble 顶层负责；block 层只负责纯文本 markdown）。
 */
function TextBlockInner({ block, streaming = false }: TextBlockProps) {
  const content = useMemo(() => block.text ?? '', [block.text]);
  return (
    <div className={styles.textBlock}>
      <ReactMarkdown remarkPlugins={REMARK_PLUGINS}>{content}</ReactMarkdown>
      {streaming && <span className={styles.streamingCursor} aria-hidden />}
    </div>
  );
}

export const TextBlock = React.memo(TextBlockInner);

// ---------------------------------------------------------------------------
// 自注册：import './TextBlock'（经 blocks/index.ts）触发 registerBlock 副作用。
// 组件定义与注册信息内聚在同一文件，便于维护。
// ---------------------------------------------------------------------------
registerBlock('text', { component: TextBlock });
