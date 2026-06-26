// AgentEvent：三端共享的流式事件契约（daemon JS 源、frontend TS 镜像、backend Go 镜像）。
//
// 设计原则：
// - discriminated union：按 type 收窄 payload 字段
// - 双兼容：同时接受老 daemon 的 snake_case 命名（text/thinking/tool_use/tool_result/turn_end/session_end）
//   与新命名（text.delta/thinking.delta/tool.call.start/tool.call.input/tool.call.end/tool.result/session.end）
// - 可选元数据：seq/adapter/ts 全部 optional，保证老事件无这些字段也能 reduce
//
// 注意：payload 字段命名沿用现有 daemon 线上协议（content/tool/output/is_error/message），
// 以减少 daemon 侧改动。未来 PR 升级 daemon 时可改用新命名，reducer 双兼容。
//

/** AgentEvent 类型枚举（含老 snake_case 与新 dot.case 双命名）。 */
export type AgentEventType =
  // 新命名（dot.case）
  | 'session.start'
  | 'session.end'
  | 'text.delta'
  | 'thinking.delta'
  | 'tool.call.start'
  | 'tool.call.input'
  | 'tool.call.end'
  | 'tool.result'
  | 'error'
  | 'cancel'
  // 老命名（snake_case）—— 兼容当前 daemon 线上协议
  | 'text'
  | 'thinking'
  | 'tool_use'
  | 'tool_result'
  | 'turn_end'
  | 'session_end';

/** 信封字段：daemon 可选注入的元数据，reducer 不强制。 */
export interface AgentEventEnvelope {
  /** daemon 侧自增序号，用于未来 WS 断点续传（reducer 当前不依赖） */
  seq?: number;
  /** 产生事件的 adapter 名（'claude' / 'codex' / ...） */
  adapter?: string;
  /** 毫秒时间戳 */
  ts?: number;
}

/** AgentEvent：按 type 收窄 payload 的 discriminated union。 */
export type AgentEvent =
  // session 开始（新命名）
  | (AgentEventEnvelope & {
      type: 'session.start';
      agent?: { name?: string; model?: string };
    })
  // 文本增量（新/老命名合并）
  | (AgentEventEnvelope & {
      type: 'text' | 'text.delta';
      content: string;
    })
  // 思考增量（新/老命名合并）
  | (AgentEventEnvelope & {
      type: 'thinking' | 'thinking.delta';
      content: string;
    })
  // 工具调用开始（新/老命名合并）
  //   老 daemon：tool 字段非空表示工具开始，空表示 input_json_delta（partial）
  //   新协议：tool.call.start 只表示开始，input 走 tool.call.input
  | (AgentEventEnvelope & {
      type: 'tool_use' | 'tool.call.start';
      /** 工具名（首次 delta 非空；老协议 input_json_delta 时为空字符串） */
      tool: string;
      /** 工具调用唯一 ID（与后续 tool_result / tool.call.end 对齐） */
      tool_use_id?: string;
      /**
       * 工具入参——老协议把 input_json_delta 的 partial_json 放到这里。
       * 当前线上 daemon 对 tool_use(input_json_delta) 事件发 `input` 字段，
       * 但 reducer 为了与现有 appendDeltas 行为对齐，读 `content` 字段。
       * 后续 PR 会统一为只读 `input`。
       */
      input?: unknown;
      /** partial_json 增量字段（老 daemon 兼容） */
      content?: string;
    })
  // 工具入参增量（新命名）
  | (AgentEventEnvelope & {
      type: 'tool.call.input';
      tool_use_id: string;
      /** partial JSON 片段 */
      delta: string;
    })
  // 工具调用结束（新命名）
  | (AgentEventEnvelope & {
      type: 'tool.call.end';
      tool_use_id: string;
    })
  // 工具结果（新/老命名合并）
  | (AgentEventEnvelope & {
      type: 'tool_result' | 'tool.result';
      tool_use_id?: string;
      /** 工具输出文本 */
      output?: string;
      /** 是否错误（老协议 isError，新协议 is_error） */
      isError?: boolean;
      is_error?: boolean;
      /** 新协议用 content 替代 output，reducer 两者皆兼容 */
      content?: string;
    })
  // 错误事件
  | (AgentEventEnvelope & {
      type: 'error';
      message: string;
      code?: string;
    })
  // 取消事件
  | (AgentEventEnvelope & {
      type: 'cancel';
      reason?: string;
    })
  // turn / session 结束（新/老命名合并）
  | (AgentEventEnvelope & {
      type: 'turn_end' | 'session.end' | 'session_end';
      /** turn_end：最终汇总文本（可选） */
      result?: string;
    });
