'use strict';

// AgentEvent 协议：daemon 内部的统一事件流。
//
// 设计目标：
// - 每个 agent CLI（claude/codex/opencode/openclaw）的输出被解析成同一组事件类型；
// - one-shot 任务用单个 TURN_END 收尾，persistent 任务每个 turn 一个 TURN_END；
// - 对外 WS 协议保持不变（dispatcher 层把 AgentEvent 翻译成现有消息格式）。
//
// 事件是纯数据对象（POJO），便于序列化到日志/WS。不要在事件上挂方法。
//
// 事件类型：
//   text        —— 可展示给用户的文本片段 { content }
//   thinking    —— 思考过程（可选展示） { content }
//   tool_use    —— 工具调用开始 { tool, input }
//   tool_result —— 工具调用结果     { tool, output, isError }
//   card        —— 平台卡片         { cardType, payload }
//   error       —— 错误（不终止会话） { message }
//   turn_end    —— 一个 turn 结束   { result?, error? }
//   session_end —— persistent 进程退出 { code?, signal? }

const EVENT_TYPES = Object.freeze({
  TEXT: 'text',
  THINKING: 'thinking',
  TOOL_USE: 'tool_use',
  TOOL_RESULT: 'tool_result',
  CARD: 'card',
  ERROR: 'error',
  TURN_END: 'turn_end',
  SESSION_END: 'session_end',
});

function textEvent(content) {
  return { type: EVENT_TYPES.TEXT, content: String(content ?? '') };
}

function thinkingEvent(content) {
  return { type: EVENT_TYPES.THINKING, content: String(content ?? '') };
}

function toolUseEvent(tool, input) {
  return { type: EVENT_TYPES.TOOL_USE, tool: String(tool ?? ''), input };
}

function toolResultEvent(tool, output, isError = false) {
  return {
    type: EVENT_TYPES.TOOL_RESULT,
    tool: String(tool ?? ''),
    output,
    isError: Boolean(isError),
  };
}

function cardEvent(cardType, payload) {
  return {
    type: EVENT_TYPES.CARD,
    cardType: String(cardType ?? ''),
    payload,
  };
}

function errorEvent(message) {
  return { type: EVENT_TYPES.ERROR, message: String(message ?? '') };
}

function turnEndEvent({ result, error, subtype } = {}) {
  const ev = { type: EVENT_TYPES.TURN_END };
  if (result !== undefined) ev.result = result;
  if (error !== undefined) ev.error = error;
  if (subtype !== undefined) ev.subtype = subtype;
  return ev;
}

function sessionEndEvent({ code, signal } = {}) {
  const ev = { type: EVENT_TYPES.SESSION_END };
  if (code !== undefined) ev.code = code;
  if (signal !== undefined) ev.signal = signal;
  return ev;
}

// createAsyncQueue —— 把 push 的事件序列化成 async iterable。
//
// 使用：
//   const queue = createAsyncQueue();
//   queue.push(textEvent('hi'));
//   queue.done();
//   for await (const ev of queue.iter) { ... }
//
// push(end) 必须按调用顺序到达 iter 消费者；内部用 pending promise 链保证顺序。
// done() 后 iter 自然结束（return undefined）。
function createAsyncQueue() {
  // pending 是一个 { resolve } 的占位，下次 push/done 会 resolve 它。
  let pending = null;
  let finished = false;
  const buffer = [];

  const settle = (value) => {
    if (pending) {
      const fn = pending;
      pending = null;
      fn(value);
    }
  };

  const push = (event) => {
    if (finished) return; // 静默丢弃，避免 done 之后再写入出错
    buffer.push(event);
    settle(true);
  };

  const done = () => {
    if (finished) return;
    finished = true;
    settle(false);
  };

  const iter = {
    [Symbol.asyncIterator]() {
      return this;
    },
    async next() {
      // 先消耗 buffer 里的事件
      if (buffer.length > 0) {
        return { value: buffer.shift(), done: false };
      }
      if (finished) {
        return { value: undefined, done: true };
      }
      // 等待下一次 push/done
      await new Promise((resolve) => { pending = resolve; });
      if (buffer.length > 0) {
        return { value: buffer.shift(), done: false };
      }
      // finished=true 分支
      return { value: undefined, done: true };
    },
    return() {
      finished = true;
      buffer.length = 0;
      settle(false);
      return Promise.resolve({ value: undefined, done: true });
    },
    throw(err) {
      finished = true;
      buffer.length = 0;
      settle(false);
      return Promise.reject(err);
    },
  };

  return { push, done, iter, get finished() { return finished; } };
}

module.exports = {
  EVENT_TYPES,
  textEvent,
  thinkingEvent,
  toolUseEvent,
  toolResultEvent,
  cardEvent,
  errorEvent,
  turnEndEvent,
  sessionEndEvent,
  createAsyncQueue,
};
