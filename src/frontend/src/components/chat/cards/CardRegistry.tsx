import type { CardProps, CardType, InteractiveCard } from '@/types/card';

/**
 * 卡片渲染器注册表。
 *
 * 新增卡片类型只需 3 步：
 * 1. types/card.ts 加 type union 成员 + 接口
 * 2. 写一个 XxxCardView.tsx 组件
 * 3. 在此文件底部加一行 registerCard('xxx', XxxCardView)
 *
 * MessageBubble 通过 renderCards(cards) 统一渲染，不直接引用任何具体组件。
 */

const registry = new Map<CardType, React.FC<CardProps>>();

export function registerCard(type: CardType, component: React.FC<CardProps>): void {
  registry.set(type, component);
}

export function getCardRenderer(type: CardType): React.FC<CardProps> | undefined {
  return registry.get(type);
}

export function hasCardRenderer(type: CardType): boolean {
  return registry.has(type);
}

export function registeredCardTypes(): CardType[] {
  return [...registry.keys()];
}

/**
 * 渲染卡片数组。对每个卡片查找对应渲染器，未注册的 type 跳过。
 */
export function renderCards(
  cards: InteractiveCard[],
  conversationId: string,
  messageId: string,
  onAction: (cardId: string, action: string, data?: Record<string, unknown>) => void,
): React.ReactNode[] {
  return cards
    .map((card, idx) => {
      const Renderer = getCardRenderer(card.type);
      if (!Renderer) return null;
      return (
        <Renderer
          key={`${card.type}-${card.id ?? idx}`}
          card={card}
          conversationId={conversationId}
          messageId={messageId}
          onAction={onAction}
        />
      );
    })
    .filter(Boolean);
}

// ---------------------------------------------------------------------------
// 注册内置卡片类型（在文件加载时自动注册）
// ---------------------------------------------------------------------------

// 延迟导入避免循环依赖——组件文件可能导入 CardRegistry 的 helper
import { PlanCardView } from './PlanCardView';
import { ProgressCardView } from './ProgressCardView';
import { ConfirmCardView } from './ConfirmCardView';

// 组件使用具体的卡片泛型（PlanCard/ProgressCard/ConfirmCard），
// 注册时 cast 为通用 CardProps——运行时类型安全由 card.type 判别保证。
registerCard('plan', PlanCardView as React.FC<CardProps>);
registerCard('progress', ProgressCardView as React.FC<CardProps>);
registerCard('confirm', ConfirmCardView as React.FC<CardProps>);
