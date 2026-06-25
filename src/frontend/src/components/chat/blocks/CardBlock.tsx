import React from 'react';
import type { MessageBlock } from '@/types/message';
import type { BlockRenderContext } from './BlockRegistry';
import { renderCards } from '../cards/CardRegistry';
import styles from './Blocks.module.css';
import { registerBlock } from './BlockRegistry';

interface CardBlockProps {
  block: MessageBlock;
  /** registry 签名要求；card 不增量，忽略该 prop */
  streaming?: boolean;
  /** 渲染上下文：CardBlock 用 conversationId / messageId / agentId / artifacts / onAction
   *  调 renderCards。MessageBubble 顶层统一构造并透传给所有 block。 */
  ctx?: BlockRenderContext;
}

/**
 * card block——把单张 InteractiveCard 渲染在 block 流的精确位置。
 *
 * 与 MessageBubble 顶层 `splitByCardPlaceholder` + 末尾 unmatchedCards 兜底的差异：
 *   - 老路径（content placeholder）：依赖 agent 在正文里写 `[CARD:id]` 占位符，
 *     cards_json 单独存结构化数据；前端按占位符拆段。双表示，容易 desync。
 *   - 新路径（card block）：fenced ```agenthub{"cards":[...]}``` block 在 backend finalize
 *     时被 SplitTextBlocksByCardFences 切分成独立 card block，与 text/thinking 平级。
 *     单一真源 = blocks_json，无 desync。
 *
 * streaming prop 不使用：card 是 finalize 时一次性切分，流式期间不会出现 card block
 * （streaming 期间 text block 里临时显示 fenced JSON 原文，task.complete 后被切分替换）。
 */
function CardBlockInner({ block, ctx }: CardBlockProps) {
  if (block.kind !== 'card' || !block.card) return null;
  if (!ctx) return null;
  const card = block.card;
  return (
    <div className={styles.blocksContainer}>
      {renderCards([card], ctx.conversationId, ctx.messageId, ctx.onAction, ctx.artifacts, ctx.agentId)}
    </div>
  );
}

export const CardBlock = React.memo(CardBlockInner);

// 自注册：交互式卡片作为 first-class block kind——与 text/thinking/tool_use 平级。
// block.card 字段持有 InteractiveCard 实例，ctx 提供渲染上下文。
registerBlock('card', { component: CardBlock });
