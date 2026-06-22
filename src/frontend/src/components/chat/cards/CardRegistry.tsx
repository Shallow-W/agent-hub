import type { CardSpec, CardType, InteractiveCard, PlanCard, ApprovalCard, ProgressCard, ProjectCard } from '@/types/card';

/**
 * 卡片渲染器注册表——自描述 CardSpec 模式。
 *
 * 命名约定（三方完全一致，单一事实源，见 types/card.ts）：
 *   type key = 接口名去 Card 后缀的小写 = 组件名去 Card 后缀
 *   plan / approval / progress / info
 *
 * 注册 key 必须与 render_card MCP 工具的 card_type 协议契约一致。
 * 不一致会导致 renderCards 静默丢弃卡片。
 *
 * 新增卡片类型只需 3 步：
 * 1. types/card.ts 加 type union 成员 + 接口（按命名约定）
 * 2. 写一个 XxxCard.tsx 组件（文件名 = 导出名 = 接口名）
 * 3. 在此文件底部 registerCard('xxx', { component, reduceAction?, actionToMessage? })
 *
 * MessageBubble 通过 renderCards(cards) 统一渲染、通过 getCardSpec(type) 委托 action 处理，
 * 不直接引用任何具体组件、不 hardcode 任何 action。
 */

const registry = new Map<CardType, CardSpec>();

/**
 * 注册一个卡片类型。建议在各卡片组件文件末尾调用（自描述），
 * 而不是集中在本文件——这样卡片的所有逻辑（视图 + 交互 + 翻译）内聚在一处。
 */
export function registerCard<T extends InteractiveCard>(type: T['type'], spec: CardSpec<T>): void {
  // 泛型 CardSpec<T>（具体卡片类型）→ 存储 CardSpec<InteractiveCard>（联合）。
  // 这是 registry 模式的标准妥协：注册时类型精确，运行时按 type 分发，
  // 取出时 component/reducer 都在各自类型域内工作。
  registry.set(type, spec as unknown as CardSpec);
}

/** 取某个类型的 CardSpec（含 component + reduceAction + actionToMessage）。
 *  查不到时回退到别名表（兼容历史 DB 数据存的旧 type key）。 */
export function getCardSpec(type: CardType): CardSpec | undefined {
  return registry.get(type) ?? registry.get(CARD_TYPE_ALIASES[type] ?? '');
}

export function hasCardRenderer(type: CardType): boolean {
  return Boolean(getCardSpec(type));
}

export function registeredCardTypes(): CardType[] {
  return [...registry.keys()];
}

/**
 * 渲染卡片数组。对每个卡片查找对应 CardSpec 的 component，未注册的 type 跳过。
 */
export function renderCards(
  cards: InteractiveCard[],
  conversationId: string,
  messageId: string,
  onAction: (cardId: string, action: string, data?: Record<string, unknown>) => void,
  artifacts?: import('@/types/message').Artifact[],
  agentId?: string,
): React.ReactNode[] {
  return cards
    .map((card, idx) => {
      const spec = getCardSpec(card.type);
      if (!spec) return null;
      const Renderer = spec.component;
      return (
        <Renderer
          key={`${card.type}-${card.id ?? idx}`}
          card={card}
          conversationId={conversationId}
          messageId={messageId}
          agentId={agentId}
          onAction={onAction}
          artifacts={artifacts}
        />
      );
    })
    .filter(Boolean);
}

// ---------------------------------------------------------------------------
// 旧 type key → 新 type key 别名表。
// 历史消息的 cards_json 可能存了重命名前的 key，通过别名让旧卡片继续渲染。
// 注意：plan_selection 已移除（plan 卡数据结构从 options 迁移到 questions，
// 旧 plan 卡不再兼容）。
// ---------------------------------------------------------------------------
const CARD_TYPE_ALIASES: Record<string, CardType> = {
  task_status: 'progress',
};

// ---------------------------------------------------------------------------
// 注册内置卡片类型——key 与 render_card 工具的 card_type 协议一致。
// 各组件自描述（component + reduceAction + actionToMessage）。
// ---------------------------------------------------------------------------

import { PlanCard as PlanCardComponent } from './PlanCard';
import { ApprovalCard as ApprovalCardComponent } from './ApprovalCard';
import { ProgressCard as ProgressCardComponent } from './ProgressCard';
import { InfoCard as InfoCardComponent } from './InfoCard';
import { DiffCard as DiffCardComponent } from './DiffCard';
import { ProjectCard as ProjectCardComponent } from './ProjectCard';

// card_type=plan —— 方案选择（多问题翻页，统一提交），交互式
registerCard<PlanCard>('plan', {
  component: PlanCardComponent,
  reduceAction: (card, action, data) => {
    if (action === 'submit_plan' && data?.answers) {
      const answers = data.answers as Record<string, string>;
      return {
        ...card,
        state: 'resolved',
        questions: card.questions.map((q) => ({
          ...q,
          state: 'resolved',
          selected_option: answers[q.id],
        })),
      };
    }
    return card;
  },
  actionToMessage: (card, action, data) => {
    if (action === 'submit_plan' && data?.answers) {
      const answers = data.answers as Record<string, string>;
      const lines = card.questions.map((q) => {
        const optId = answers[q.id];
        const opt = q.options.find((o) => o.id === optId);
        return `- ${q.title}: ${opt?.label ?? optId}`;
      });
      return `[方案选择已提交]\n${lines.join('\n')}`;
    }
    return `[卡片交互: ${action}]`;
  },
});

// card_type=approval —— 审批确认，交互式
registerCard<ApprovalCard>('approval', {
  component: ApprovalCardComponent,
  reduceAction: (card, action, data) => {
    if (action === 'confirm' && data?.action_id) {
      return { ...card, state: 'resolved', selected_action: String(data.action_id) };
    }
    return card;
  },
  actionToMessage: (card, action, data) => {
    if (action === 'confirm' && data?.action_id) {
      const actionId = String(data.action_id);
      const act = card.actions?.find((a) => a.id === actionId);
      return `[确认操作: ${actionId}] ${act?.label ?? ''}`;
    }
    return `[卡片交互: ${action}]`;
  },
});

// card_type=progress —— 任务进度，只读
registerCard<ProgressCard>('progress', {
  component: ProgressCardComponent,
  // 只读卡片：无 reduceAction / actionToMessage
});

// card_type=info —— 信息展示，只读
registerCard('info', {
  component: InfoCardComponent,
  // 只读卡片：无 reduceAction / actionToMessage
});

// card_type=diff —— 文件变更，只读（点击进 DiffViewer 版本对比）
registerCard('diff', {
  component: DiffCardComponent,
  // 只读卡片：无 reduceAction / actionToMessage
});

// card_type=project —— 项目目录，只读（点击进 FilesDrawer 浏览该目录）
registerCard<ProjectCard>('project', {
  component: ProjectCardComponent,
  // 只读卡片：抽屉控制由卡片内部 useState 完成，不走 onAction
});
