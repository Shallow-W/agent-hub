'use strict';

// StreamBuffer —— daemon 侧的 50ms 节流缓冲器，把高频 AgentEvent 批量转发给 backend。
//
// 设计目标：
// - 减少 WS 消息量：Claude stream-json 每秒可能吐几十个 content_block_delta，逐条转发
//   会把后端打爆。50ms 批处理 → 减少 5-10 倍。
// - 保持响应性：block_stop / turn_end / session_end 等"硬边界"立即 flush，UI 即时响应。
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
 * @param {number} [options.flushMs=50] - 时间窗口，默认 50ms
 * @param {number} [options.maxBufferSize=500] - 最大累积事件数，超限立即 flush
 * @param {Set<string>} [options.immediateTypes] - 触发立即 flush 的事件类型（默认 turn_end / session_end）
 */
class StreamBuffer {
  constructor({ onFlush, flushMs = 50, maxBufferSize = 500, immediateTypes } = {}) {
    if (typeof onFlush !== 'function') {
      throw new Error('StreamBuffer requires onFlush callback');
    }
    this.onFlush = onFlush;
    this.flushMs = flushMs;
    this.maxBufferSize = maxBufferSize;
    // block_stop 标记在 AgentEvent 里没有独立字段（AgentEvent 的 text 事件没有 block_stop 字段）；
    // 通过 turn_end / session_end 等"回合边界"事件触发立即 flush。
    this.immediateTypes = new Set(immediateTypes || ['turn_end', 'session_end', 'error']);
    this.buffer = [];
    this.timer = null;
    this.closed = false;
  }

  /**
   * 推入一个或多个 AgentEvent。如果事件类型在 immediateTypes 里，立即 flush。
   * @param {...Object} events
   */
  push(...events) {
    if (this.closed) return;
    for (const evt of events) {
      if (!evt) continue;
      this.buffer.push(evt);
      if (evt.type && this.immediateTypes.has(evt.type)) {
        this.flush();
        return;
      }
    }
    if (this.buffer.length >= this.maxBufferSize) {
      this.flush();
      return;
    }
    if (!this.timer) {
      this.timer = setTimeout(() => this.flush(), this.flushMs);
      if (typeof this.timer.unref === 'function') this.timer.unref();
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
