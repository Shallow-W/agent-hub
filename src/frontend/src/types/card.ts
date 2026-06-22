/**
 * 交互式卡片类型定义。
 *
 * 命名约定（三方完全一致，单一事实源）：
 *   type key  = 接口名去 Card 后缀的小写 = 组件名去 Card 后缀
 *
 *   type key      接口            组件
 *   plan          PlanCard        PlanCard
 *   approval      ApprovalCard    ApprovalCard
 *   progress      ProgressCard    ProgressCard
 *   info          InfoCard        InfoCard
 *
 * card_type 字符串是 render_card MCP 工具的协议契约——必须与 daemon 的
 * tool inputSchema (card_type 枚举) + 后端系统提示词严格一致。
 * 改这些字符串要同步改三处：daemon 工具定义、后端 context_agent_config.go、本文件。
 *
 * 新增卡片类型：
 *   1. daemon 工具 inputSchema 加 card_type 枚举值 + run 分支
 *   2. 后端 context_agent_config.go 加一行使用说明
 *   3. 此处加 type union 成员 + 接口（按上述命名约定）
 *   4. 写一个 XxxCard.tsx 组件（文件名 = 导出名 = 接口名）
 *   5. 在 CardRegistry.tsx 末尾 registerCard('xxx', { component, reduceAction?, actionToMessage? })
 */

import type React from 'react';

/** 卡片类型——与 render_card 工具的 card_type 协议契约一致。 */
export type CardType = 'plan' | 'approval' | 'progress' | 'info' | 'diff' | 'project' | string;

export interface BaseCard {
  type: CardType;
  id: string;
  title?: string;
  /** 卡片状态（pending / resolved / completed / failed / dismissed 等），由前端用户交互更新 */
  state?: string;
}

// ---------------------------------------------------------------------------
// 具体卡片接口——字段名与 daemon render_card 工具 run() 输出严格对齐
// ---------------------------------------------------------------------------

export interface PlanOption {
  id: string;
  label: string;
  description?: string;
  tasks?: string[];
  recommended?: boolean;
}

/** plan 卡片里的单个问题。用户选择后写入 selected_option，state=resolved 后只读。 */
export interface PlanQuestion {
  id: string;
  title: string;
  options: PlanOption[];
  /** 用户已选择的选项 ID（提交后由前端写入） */
  selected_option?: string;
  /** pending | resolved */
  state?: string;
}

/** 方案选择卡片（card_type=plan）—— 支持多问题，用户翻页逐个选择后统一提交。 */
export interface PlanCard extends BaseCard {
  type: 'plan';
  questions: PlanQuestion[];
  // 卡片级 state（继承自 BaseCard）：全部问题提交后置 'resolved'
}

export type ProgressTaskStatus = 'done' | 'running' | 'pending' | 'failed';

export interface ProgressTask {
  name: string;
  status: ProgressTaskStatus;
}

/** 任务进度卡片（card_type=progress）。 */
export interface ProgressCard extends BaseCard {
  type: 'progress';
  tasks: ProgressTask[];
}

export interface ApprovalAction {
  id: string;
  label: string;
  style?: 'primary' | 'danger';
}

/** 审批确认卡片（card_type=approval）。 */
export interface ApprovalCard extends BaseCard {
  type: 'approval';
  message: string;
  actions: ApprovalAction[];
  /** 用户已执行的动作 ID（state === 'resolved' 时由前端写入） */
  selected_action?: string;
}

/** 信息展示卡片（card_type=info）—— 键值对表格，只读。 */
export interface InfoCard extends BaseCard {
  type: 'info';
  fields: Record<string, string>;
}

/** 文件变更项（diff 卡片里的单个文件）。 */
export interface DiffChange {
  /** 文件路径（相对路径，如 src/App.tsx） */
  path: string;
  /** 变更类型：added 新增 / modified 修改 / deleted 删除 */
  status: 'added' | 'modified' | 'deleted';
}

/** 文件变更卡片（card_type=diff）—— 展示本次修改的文件清单，点击进入版本对比视图，只读。
 *  agent 只上报 workDir + files（相对路径）；status 和前后内容由平台通过 git 查询。 */
export interface DiffCard extends BaseCard {
  type: 'diff';
  /** 项目根目录绝对路径（agent 上报，查 git 的基准）。 */
  workDir: string;
  /** 改动文件的相对路径数组（agent 上报，如 ["App.tsx", "src/index.css"]）。 */
  files: string[];
}

/** 项目目录卡片。agent 写完文件后上报工作目录，前端据此打开文件抽屉。
 *  解耦设计：路径生产（agent 上报）与文件浏览（抽屉）只通过 workDir 耦合。 */
export interface ProjectCard extends BaseCard {
  type: 'project';
  /** agent 机器上的绝对路径（唯一契约）。点击卡片打开抽屉浏览此目录。 */
  workDir: string;
  /** 可选项目说明，显示在卡片上。 */
  summary?: string;
}

export type InteractiveCard = PlanCard | ApprovalCard | ProgressCard | InfoCard | DiffCard | ProjectCard;

// ---------------------------------------------------------------------------
// CardSpec + CardProps —— 自描述注册单元
// ---------------------------------------------------------------------------

export interface CardProps<T extends InteractiveCard = InteractiveCard> {
  card: T;
  conversationId: string;
  messageId: string;
  /** 当前消息的 agent id（仅 agent 消息有，project 卡片调 daemon RPC 用）。 */
  agentId?: string;
  /** 用户交互回调（选方案/确认操作）。组件内部触发，由 MessageBubble 委托给 CardSpec 处理。 */
  onAction: (cardId: string, action: string, data?: Record<string, unknown>) => void;
  /** 当前消息的产物列表（可选，diff 卡片用它的 root_id 加载版本历史做对比）。 */
  artifacts?: import('../types/message').Artifact[];
}

/**
 * 卡片规格——自描述的注册单元。每个卡片类型注册时声明：
 *   - component: 渲染组件
 *   - reduceAction: 用户交互后如何把当前 card 约减成新 card（用于持久化）。无此字段 = 只读卡片。
 *   - actionToMessage: 把用户交互翻译成发给 Agent 的人类可读文本。无此字段 = 用默认兜底文案。
 *
 * 这样新增交互类型只改一个文件，MessageBubble 不再 hardcode 任何 action。
 */
export interface CardSpec<T extends InteractiveCard = InteractiveCard> {
  component: React.FC<CardProps<T>>;
  /** 用户交互后约减出新 card（如标记 resolved + selected_option）。返回新对象，不修改入参。 */
  reduceAction?: (card: T, action: string, data?: Record<string, unknown>) => T;
  /** 把用户交互翻译成发给 Agent 的文本（出现在聊天流里，Agent 可读）。 */
  actionToMessage?: (card: T, action: string, data?: Record<string, unknown>) => string;
}

export type CardRenderer = React.FC<CardProps>;
