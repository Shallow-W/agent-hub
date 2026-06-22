/**
 * Block 组件统一入口——import 本文件会触发所有 block 组件的 registerBlock 副作用。
 *
 * MessageBubble.tsx 改为 `import { renderBlock } from './blocks'`（而非各自 import 5 个组件）
 * 以保证：
 *   1. 所有 block 自注册副作用必然被执行（只要 MessageBundle 被 render 一次）
 *   2. 调用方只依赖 registry 抽象，不直接引用具体组件（解耦，便于增删 block 类型）
 *
 * 顺序：先 import 各组件文件（触发 registerBlock），再 re-export registry API。
 * 组件之间的 import 顺序不影响 registry 行为（副作用只是 Map.set）。
 */

// 触发各 block 组件文件的 registerBlock 副作用
import './TextBlock';
import './ThinkingBlock';
import './ToolCallBlock';
import './ToolResultBlock';
import './ErrorBlock';

// re-export registry 公共 API
export {
  registerBlock,
  getBlockSpec,
  hasBlockRenderer,
  registeredBlockKinds,
  renderBlock,
} from './BlockRegistry';
export type { BlockSpec } from './BlockRegistry';
