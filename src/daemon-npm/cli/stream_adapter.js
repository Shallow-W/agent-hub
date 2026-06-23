'use strict';

// StreamBuffer —— daemon 侧的 16ms 节流缓冲器，把高频 AgentEvent 批量转发给 backend。
//
// 设计目标：
// - 减少 WS 消息量：Claude stream-json 每秒可能吐几十个 content_block_delta，逐条转发
//   会把后端打爆。16ms 批处理（约 60fps）→ 既保留 token-by-token 的视觉流式感，
//   又把 WS 消息数控制在 ~60 条/秒以内。
// - 保持响应性：block_stop / turn_end / session_end 等"硬边界"立即 flush，UI 即时响应。
// - kind 切换立即 flush：thinking → text、text → tool_use 等切换时，把 pending 的上一类
//   事件立刻 flush 出去，前端能清晰看到"思考完成 → 回答开始"，避免两类事件被同批
//   合并导致视觉上"思考完直接整段文本一次性出现"。
// - 复用现有 AgentEvent 协议（见 cli/events.js），不引入新的抽象层。
//
// 用法：
//   const buf = new StreamBuffer({ onFlush: (events) => bus.emit('task.progress', { events }) });
//   slot.setEventHandler((ev) => buf.push(ev));
//   // 进程结束时：
//   buf.flush(); buf.close();
//
// 注意：StreamBuffer 只做聚合和定时，不改事件结构。后端收到的事件格式与 daemon 内部一致。

/**
 * StreamBuffer 构造。
 * @param {Object} options
 * @param {(events: Array) => void} options.onFlush - flush 时调用，接收批量事件
 * @param {number} [options.flushMs=16] - 时间窗口，默认 16ms（约 60fps，匹配屏幕刷新率，
 *   同时保证短文本回复也能流式呈现——50ms 下短回复会在 1-2 个 timer tick 内一次性到达）
 * @param {number} [options.maxBufferSize=500] - 最大累积事件数，超限立即 flush
 * @param {Set<string>} [options.immediateTypes] - 触发立即 flush 的事件类型（默认 turn_end / session_end / error / cancel）
 * @param {boolean} [options.flushOnKindSwitch=true] - kind 切换时立即 flush（thinking→text 等）
 */
class StreamBuffer {
  constructor({
    onFlush,
    flushMs = 16,
    maxBufferSize = 500,
    immediateTypes,
    flushOnKindSwitch = true,
  } = {}) {
    if (typeof onFlush !== 'function') {
      throw new Error('StreamBuffer requires onFlush callback');
    }
    this.onFlush = onFlush;
    this.flushMs = flushMs;
    this.maxBufferSize = maxBufferSize;
    // block_stop 标记在 AgentEvent 里没有独立字段（AgentEvent 的 text 事件没有 block_stop 字段）；
    // 通过 turn_end / session_end 等"回合边界"事件触发立即 flush。
    // cancel 也加入：用户停止生成时需立即把已缓冲的内容 flush 出去再发 cancel。
    this.immediateTypes = new Set(immediateTypes || ['turn_end', 'session_end', 'error', 'cancel']);
    // kind 切换立即 flush：避免 thinking 与 text 被同一批合并，前端看起来 text 一次性出现。
    // 注意：tool_use 与 tool_result 通常成对出现且语义不同，同样按 kind 切换切批。
    this.flushOnKindSwitch = flushOnKindSwitch;
    this.buffer = [];
    this.timer = null;
    this.closed = false;
  }

  /**
   * 推入一个或多个 AgentEvent。
   * - immediateTypes 里的事件立即 flush；
   * - flushOnKindSwitch 开启且当前事件与上一个 buffered 事件 type 不同时，先 flush；
   * - 达到 maxBufferSize 立即 flush；
   * - 否则按 flushMs 定时 flush。
   * @param {...Object} events
   */
  push(...events) {
    if (this.closed) return;
    for (const evt of events) {
      if (!evt) continue;
      // kind 切换：上一个事件 type 与当前不同时，先把 pending 批次 flush 出去，
      // 让前端看到清晰的边界（例如 thinking 完成后再开始 text）。
      if (
        this.flushOnKindSwitch
        && this.buffer.length > 0
        && this.buffer[this.buffer.length - 1].type !== evt.type
      ) {
        this.flush();
      }
      this.buffer.push(evt);
      if (evt.type && this.immediateTypes.has(evt.type)) {
        this.flush();
        continue;
      }
      if (this.buffer.length >= this.maxBufferSize) {
        this.flush();
        continue;
      }
      if (!this.timer) {
        this.timer = setTimeout(() => this.flush(), this.flushMs);
        if (typeof this.timer.unref === 'function') this.timer.unref();
      }
    }
  }

  /**
   * 立即冲刷缓冲。调用 onFlush(events)，清空 buffer 和 timer。
   */
  flush() {
    if (this.timer) {
      clearTimeout(this.timer);
      this.timer = null;
    }
    if (this.buffer.length === 0) return;
    const events = this.buffer;
    this.buffer = [];
    try {
      this.onFlush(events);
    } catch {
      // onFlush 的错误不阻断后续流式；调用方应自己记日志。
    }
  }

  /**
   * 关闭缓冲，停止接收新事件。后续 push 无效。关闭前 flush 残留事件。
   */
  close() {
    this.closed = true;
    this.flush();
  }
}

module.exports = { StreamBuffer };
