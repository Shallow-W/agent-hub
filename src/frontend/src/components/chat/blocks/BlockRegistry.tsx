import type { ComponentType, ReactNode } from 'react';
import type { MessageBlock, BlockKind } from '@/types/message';

/**
 * Block 渲染器注册表——自描述 BlockSpec 模式。
 *
 * 仿 `cards/CardRegistry.tsx` 的设计：
 *   - 注册 key = BlockKind（与 daemon AgentEvent kind 对齐，见 types/message.ts）
 *   - 各 block 组件在自身文件末尾 registerBlock，逻辑内聚
 *   - MessageBubble 通过 renderBlock(blocks) 统一渲染，不再硬编码 switch
 *   - aliases 表兼容历史 blocks_json 中的旧 kind 字符串
 *
 * 与 CardRegistry 的区别：
 *   - Block 只有视图、无交互 reducer（block 是被动展示）
 *   - Block 组件签名统一为 (props: { block, streaming? })
 *   - streaming prop 仅传给"最后一个 block"（由 renderBlock 调用方决定）
 *
 * 新增 block 类型只需 3 步：
 *   1. types/message.ts 的 BlockKind union 加成员
 *   2. 写一个 XxxBlock.tsx 组件（签名 `(props: { block, streaming? }) => JSX.Element`）
 *   3. 组件文件末尾 `registerBlock('xxx', { component: XxxBlock })`
 *
 * import './index'（或 MessageBubble import './blocks'）触发所有 block 自注册副作用。
 */

export interface BlockSpec<T extends MessageBlock = MessageBlock> {
  /** 组件签名统一：读 block 数据，可选 streaming flag 控制光标 */
  component: ComponentType<{ block: T; streaming?: boolean }>;
}

const registry = new Map<BlockKind, BlockSpec>();

/**
 * 历史 blocks_json 可能存的旧 kind 字符串 → 当前 BlockKind。
 * 当前无历史包袱（BlockKind 自出生即 'text' / 'thinking' / 'tool_use' / 'tool_result' / 'error'），
 * 留空对象作为扩展点（未来如出现重命名，例如 'tool' → 'tool_use'，在此添加）。
 */
const BLOCK_KIND_ALIASES: Record<string, BlockKind> = {};

/**
 * 注册一个 block 类型。建议在各 block 组件文件末尾调用（自描述），
 * 而不是集中在本文件——这样 block 的类型 + 视图逻辑内聚在一处。
 *
 * 泛型 BlockSpec<T>（具体 block 类型）→ 存储 BlockSpec<MessageBlock>（联合）。
 * 与 CardRegistry 同样的 registry 模式标准妥协：注册时类型精确，运行时按 kind 分发。
 */
export function registerBlock<T extends MessageBlock>(
  kind: T['kind'],
  spec: BlockSpec<T>,
): void {
  registry.set(kind, spec as unknown as BlockSpec);
}

/** 取某个 kind 的 BlockSpec。查不到时回退到别名表（兼容历史 blocks_json）。 */
export function getBlockSpec(kind: BlockKind): BlockSpec | undefined {
  // 别名回退：历史 blocks_json 可能存了旧 kind 字符串，映射到当前 BlockKind。
  // BLOCK_KIND_ALIASES[key] 为 undefined 时跳过别名查询（避免空字符串误查）。
  const aliased = BLOCK_KIND_ALIASES[kind];
  return registry.get(kind) ?? (aliased ? registry.get(aliased) : undefined);
}

export function hasBlockRenderer(kind: BlockKind): boolean {
  return Boolean(getBlockSpec(kind));
}

export function registeredBlockKinds(): BlockKind[] {
  return [...registry.keys()];
}

/**
 * 渲染单个 block。查不到 spec 时返回 null（静默跳过，与 CardRegistry 行为一致）。
 *
 * @param block MessageBlock 实例
 * @param streaming 是否处于流式状态（调用方决定：通常仅最后一个 block 且消息未 complete 时为 true）
 */
export function renderBlock(block: MessageBlock, streaming: boolean): ReactNode {
  const spec = getBlockSpec(block.kind);
  if (!spec) return null;
  const Renderer = spec.component;
  return <Renderer block={block} streaming={streaming} />;
}
