/**
 * 交互式卡片类型定义。
 *
 * Agent 在回复中嵌入结构化 JSON 卡片（plan/progress/confirm/result 等），
 * 前端通过 CardRegistry 按 type 派发渲染组件。
 *
 * 新增卡片类型只需：
 * 1. 在此文件加 type union 成员 + 接口
 * 2. 写一个 XxxCardView.tsx 组件
 * 3. 在 CardRegistry.ts 加一行 registerCard('xxx', XxxCardView)
 */

// ---------------------------------------------------------------------------
// 卡片类型 union（可扩展）
// ---------------------------------------------------------------------------

export type CardType = 'plan' | 'progress' | 'confirm' | 'result' | string;

// ---------------------------------------------------------------------------
// 卡片数据接口
// ---------------------------------------------------------------------------

/** 基础卡片接口——所有卡片类型必须包含的字段 */
export interface BaseCard {
  type: CardType;
  id: string;
  title?: string;
  /** 卡片状态（pending / completed / failed / dismissed 等），由前端用户交互更新 */
  state?: string;
}

/** 方案选择卡片 */
export interface PlanOption {
  id: string;
  label: string;
  description?: string;
  tasks?: string[];
  recommended?: boolean;
}

export interface PlanCard extends BaseCard {
  type: 'plan';
  options: PlanOption[];
  /** 用户已选择的选项 ID（state=resolved 时由后端持久化） */
  selected_option?: string;
}

/** 执行进度卡片 */
export interface ProgressTask {
  name: string;
  status: 'done' | 'running' | 'pending' | 'failed';
}

export interface ProgressCard extends BaseCard {
  type: 'progress';
  tasks: ProgressTask[];
}

/** 确认操作卡片 */
export interface ConfirmAction {
  id: string;
  label: string;
  style?: 'primary' | 'danger';
}

export interface ConfirmCard extends BaseCard {
  type: 'confirm';
  message: string;
  actions: ConfirmAction[];
  /** 用户已选择的操作 ID（state=resolved 时由后端持久化） */
  selected_action?: string;
}

/** 结果展示卡片 */
export interface ResultCard extends BaseCard {
  type: 'result';
  summary?: string;
  url?: string;
  language?: string;
  content?: string;
}

/** 所有卡片类型的联合 */
export type InteractiveCard = PlanCard | ProgressCard | ConfirmCard | ResultCard;

// ---------------------------------------------------------------------------
// CardRegistry 接口
// ---------------------------------------------------------------------------

export interface CardProps<T extends InteractiveCard = InteractiveCard> {
  card: T;
  conversationId: string;
  messageId: string;
  /** 用户交互回调（选方案/确认操作）→ 发送结构化消息给 Agent */
  onAction: (cardId: string, action: string, data?: Record<string, unknown>) => void;
}

export type CardRenderer = React.FC<CardProps>;
