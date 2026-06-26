// streamingReducer：流式 AgentEvent → StreamingState 的纯函数 reducer。
//
// 设计目标：
// - 纯函数（不修改入参 state），返回新对象
// - 按 event.type 收窄，与 messageStore.appendDeltas 现有 switch-case 逻辑对齐
//   （这是已被验证的工作代码，PR1 仅做迁移）
// - 双兼容老 snake_case 与新 dot.case 事件类型
//
// 使用方式：
//   const state = reduceEvents(events, initialStreamingState);
//   // state.blocks / state.status
//
import type { AgentEvent } from '@/types/agentEvent';
import type { MessageBlock, MessageStatus } from '@/types/message';

/** Reducer 状态：累积的 block 列表 + 当前消息状态 + 可选 task / agent 元数据。 */
export interface StreamingState {
  blocks: MessageBlock[];
  status: MessageStatus;
  /** 关联的 daemon task_id（用于 StopButton 取消） */
  taskId?: string;
  /** 产出该消息的 agent ID（用于前端展示 agent name） */
  agentId?: string;
}

/** 初始状态：空 block 列表 + status='streaming'。 */
export const initialStreamingState: StreamingState = {
  blocks: [],
  status: 'streaming',
};

/**
 * 纯函数 reducer：把单个 AgentEvent 应用到 state，返回新 state。
 *
 * 累积规则（与 messageStore.appendDeltas 对齐）：
 * - text / text.delta：最后一个 block.kind === 'text' 时累积 text；否则新建
 * - thinking / thinking.delta：同 text，累积到 thinking block
 * - tool_use / tool.call.start：
 *   - tool 非空 → 新 tool_use block（带 tool_name / tool_use_id）
 *   - tool 为空 → input_json_delta，追加到最后一个 tool_use block 的 text
 *     （注意：当前线上 daemon 把 partial_json 放在 `input` 字段，但 appendDeltas
 *     读 `content`；reducer 为了保持运行时行为一致，也读 `content`。
 *     未来 PR 会统一为 `input` 或 `delta` 字段。）
 * - tool.call.input：追加 delta 到匹配 tool_use_id 的 tool_use block
 *   （若找不到则追加到最后一个 tool_use block）
 * - tool.call.end：no-op（block 边界，不累积）
 * - tool_result / tool.result：总是新 block
 * - error：总是新 block，status='error'
 * - cancel：status='canceled'，不产生 block
 * - turn_end / session.end / session_end：status='complete'，不产生 block
 */
export function streamingReducer(
  state: StreamingState,
  event: AgentEvent,
): StreamingState {
  // 终态保护：一旦进入 complete/error/canceled，后续事件忽略。
  // 与 messageStore.completeStreaming / StopButton 流程对齐：终态后不应再 reduce。
  if (state.status !== 'streaming') {
    return state;
  }

  switch (event.type) {
    case 'text':
    case 'text.delta': {
      const text = event.content ?? '';
      if (!text) return state;
      const blocks = [...state.blocks];
      const last = blocks[blocks.length - 1];
      if (last && last.kind === 'text') {
        blocks[blocks.length - 1] = { ...last, text: last.text + text };
      } else {
        blocks.push({ index: nextIndex(blocks), kind: 'text', text });
      }
      return { ...state, blocks };
    }

    case 'thinking':
    case 'thinking.delta': {
      const text = event.content ?? '';
      if (!text) return state;
      const blocks = [...state.blocks];
      const last = blocks[blocks.length - 1];
      if (last && last.kind === 'thinking') {
        blocks[blocks.length - 1] = { ...last, text: last.text + text };
      } else {
        blocks.push({ index: nextIndex(blocks), kind: 'thinking', text });
      }
      return { ...state, blocks };
    }

    case 'tool_use':
    case 'tool.call.start': {
      const blocks = [...state.blocks];
      const toolName = event.tool ?? '';
      if (toolName) {
        // 工具名非空 → 开启新 tool_use block
        blocks.push({
          index: nextIndex(blocks),
          kind: 'tool_use',
          text: '',
          tool_name: toolName,
          tool_use_id: event.tool_use_id,
        });
        return { ...state, blocks };
      }
      // 空工具名 → input_json_delta（老协议）
      // 当前 daemon 把 partial_json 放 `input` 字段，appendDeltas 读 `content`；
      // reducer 双兼容：优先 `content`，其次 `input`（若为 string）。
      const inputDelta =
        typeof event.content === 'string'
          ? event.content
          : typeof event.input === 'string'
            ? event.input
            : '';
      if (!inputDelta) return state;
      const last = blocks[blocks.length - 1];
      if (last && last.kind === 'tool_use') {
        blocks[blocks.length - 1] = { ...last, text: last.text + inputDelta };
        return { ...state, blocks };
      }
      // 找不到 tool_use block 容错：忽略（与 appendDeltas 行为一致）
      return state;
    }

    case 'tool.call.input': {
      // 新协议 partial JSON 增量
      const blocks = [...state.blocks];
      const delta = event.delta ?? '';
      if (!delta) return state;
      // 找到匹配 tool_use_id 的 tool_use block；找不到则回退到最后一个 tool_use block
      const idx = findToolUseBlock(blocks, event.tool_use_id);
      if (idx === -1) return state;
      const target = blocks[idx]!;
      blocks[idx] = { ...target, text: target.text + delta };
      return { ...state, blocks };
    }

    case 'tool.call.end': {
      // block 边界，不产生新 block（与 content_block_stop 同语义）
      return state;
    }

    case 'tool_result':
    case 'tool.result': {
      const blocks = [...state.blocks];
      const output =
        typeof event.output === 'string'
          ? event.output
          : typeof event.content === 'string'
            ? event.content
            : '';
      const isError = event.is_error === true || event.isError === true;
      blocks.push({
        index: nextIndex(blocks),
        kind: 'tool_result',
        text: output,
        is_error: isError,
      });
      return { ...state, blocks };
    }

    case 'error': {
      const blocks = [...state.blocks];
      blocks.push({
        index: nextIndex(blocks),
        kind: 'error',
        text: event.message ?? '生成失败',
        is_error: true,
      });
      return { ...state, blocks, status: 'error' };
    }

    case 'cancel': {
      return { ...state, status: 'canceled' };
    }

    case 'turn_end':
    case 'session.end':
    case 'session_end': {
      return { ...state, status: 'complete' };
    }

    case 'session.start': {
      // session 元数据事件：当前 reducer 不消费（agent 元信息走 store 层 meta 通道）
      return state;
    }

    default: {
      // 未知事件类型忽略，不破坏流
      return state;
    }
  }
}

/**
 * 批量 reduce：把 events 数组依次应用到 state。
 *
 * @param events AgentEvent[]
 * @param initialState 起始状态；默认 initialStreamingState
 */
export function reduceEvents(
  events: AgentEvent[],
  initialState: StreamingState = initialStreamingState,
): StreamingState {
  // 空数组返回原引用（语义不变）
  if (events.length === 0) return initialState;
  return events.reduce(streamingReducer, initialState);
}

/** 计算下一个 block 的 index：数组为空返回 0，否则 last.index + 1。 */
function nextIndex(blocks: MessageBlock[]): number {
  if (blocks.length === 0) return 0;
  const last = blocks[blocks.length - 1]!;
  return last.index + 1;
}

/**
 * 查找 tool_use block：优先匹配 tool_use_id，找不到则回退到最后一个 tool_use block。
 * 仍找不到返回 -1。
 */
function findToolUseBlock(blocks: MessageBlock[], toolUseId?: string): number {
  if (toolUseId) {
    for (let i = blocks.length - 1; i >= 0; i--) {
      const b = blocks[i]!;
      if (b.kind === 'tool_use' && b.tool_use_id === toolUseId) {
        return i;
      }
    }
  }
  // 回退：最后一个 tool_use block
  for (let i = blocks.length - 1; i >= 0; i--) {
    if (blocks[i]!.kind === 'tool_use') {
      return i;
    }
  }
  return -1;
}
