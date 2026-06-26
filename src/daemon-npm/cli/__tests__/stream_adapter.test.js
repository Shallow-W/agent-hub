'use strict';

// StreamBuffer 单测：验证 16ms 时间窗口节流、kind 切换立即 flush、边界事件立即 flush、close 行为。

const { test } = require('node:test');
const assert = require('node:assert');

const { StreamBuffer } = require('../stream_adapter');
const {
  textEvent,
  thinkingEvent,
  toolUseEvent,
  toolResultEvent,
  cancelEvent,
  turnEndEvent,
  errorEvent,
  sessionEndEvent,
} = require('../events');

test('StreamBuffer: 空事件不触发 flush', () => {
  let flushCount = 0;
  const buf = new StreamBuffer({ onFlush: () => { flushCount += 1; }, flushMs: 10 });
  buf.flush();
  assert.equal(flushCount, 0);
});

test('StreamBuffer: 普通事件累积到 flushMs 后批量发出', async () => {
  const flushed = [];
  const buf = new StreamBuffer({ onFlush: (evs) => flushed.push(...evs), flushMs: 20 });
  buf.push(textEvent('a'));
  buf.push(textEvent('b'));
  buf.push(textEvent('c'));
  // 还没 flush
  assert.equal(flushed.length, 0);
  // 等待 flushMs + 余量
  await new Promise((r) => setTimeout(r, 50));
  assert.equal(flushed.length, 3);
  assert.deepEqual(flushed.map((e) => e.content), ['a', 'b', 'c']);
});

test('StreamBuffer: turn_end / session_end / error 立即 flush', () => {
  const flushed = [];
  const buf = new StreamBuffer({ onFlush: (evs) => flushed.push(...evs), flushMs: 1000 });
  buf.push(textEvent('partial'));
  buf.push(turnEndEvent({ result: 'done' }));
  // 不需要等 timer——immediate type 同步触发 flush
  assert.equal(flushed.length, 2);
  assert.equal(flushed[0].type, 'text');
  assert.equal(flushed[1].type, 'turn_end');
});

test('StreamBuffer: error 事件也立即 flush', () => {
  const flushed = [];
  const buf = new StreamBuffer({ onFlush: (evs) => flushed.push(...evs), flushMs: 1000 });
  buf.push(textEvent('before-error'));
  buf.push(errorEvent('boom'));
  assert.equal(flushed.length, 2);
});

test('StreamBuffer: session_end 立即 flush（进程退出时关键）', () => {
  const flushed = [];
  const buf = new StreamBuffer({ onFlush: (evs) => flushed.push(...evs), flushMs: 1000 });
  buf.push(textEvent('last'));
  buf.push(sessionEndEvent({ code: 0 }));
  assert.equal(flushed.length, 2);
});

test('StreamBuffer: maxBufferSize 达到时立即 flush（防爆内存）', () => {
  const flushed = [];
  const buf = new StreamBuffer({
    onFlush: (evs) => flushed.push(...evs),
    flushMs: 10000,
    maxBufferSize: 3,
  });
  buf.push(textEvent('1'));
  buf.push(textEvent('2'));
  assert.equal(flushed.length, 0);
  buf.push(textEvent('3'));
  assert.equal(flushed.length, 3);
});

test('StreamBuffer: close 后 push 无效', () => {
  const flushed = [];
  const buf = new StreamBuffer({ onFlush: (evs) => flushed.push(...evs), flushMs: 10 });
  buf.push(textEvent('before'));
  buf.close();
  assert.equal(flushed.length, 1);
  buf.push(textEvent('after'));
  assert.equal(flushed.length, 1);
});

test('StreamBuffer: close 再次 flush 残留事件（保留失败前的部分输出）', async () => {
  const flushed = [];
  const buf = new StreamBuffer({ onFlush: (evs) => flushed.push(...evs), flushMs: 1000 });
  buf.push(textEvent('partial1'));
  buf.push(textEvent('partial2'));
  // 没 flushMs 到期，close 强制 flush
  buf.close();
  assert.equal(flushed.length, 2);
});

test('StreamBuffer: onFlush 抛错不阻断后续 push', async () => {
  let callCount = 0;
  const buf = new StreamBuffer({
    onFlush: () => {
      callCount += 1;
      if (callCount === 1) throw new Error('transient');
    },
    flushMs: 10,
  });
  buf.push(textEvent('a'));
  buf.flush();
  // 第一次抛错后，后续 push 仍能工作
  buf.push(textEvent('b'));
  await new Promise((r) => setTimeout(r, 30));
  assert.equal(callCount, 2);
});

test('StreamBuffer: requires onFlush callback', () => {
  assert.throws(() => new StreamBuffer({}), /onFlush callback/);
});

test('StreamBuffer: ignore null/undefined events', () => {
  const flushed = [];
  const buf = new StreamBuffer({ onFlush: (evs) => flushed.push(...evs), flushMs: 1000 });
  buf.push(null, undefined, textEvent('valid'));
  buf.close();
  assert.equal(flushed.length, 1);
  assert.equal(flushed[0].content, 'valid');
});

// ====== 新增：kind 切换立即 flush（修复"短文本回复一次性显示"的核心 bug） ======

test('StreamBuffer: kind 切换立即 flush（thinking → text 边界清晰）', () => {
  const batches = [];
  const buf = new StreamBuffer({
    onFlush: (evs) => batches.push(evs),
    flushMs: 1000,
  });
  buf.push(thinkingEvent('想1'));
  buf.push(thinkingEvent('想2'));
  // kind 切换：thinking → text，pending thinking 批次立即 flush
  buf.push(textEvent('答1'));
  buf.push(textEvent('答2'));
  // 主动 flush 收尾，拿到最后一个批次（不依赖 timer）
  buf.flush();
  // 应当有 2 个批次：[thinking, thinking], [text, text]
  assert.equal(batches.length, 2, 'kind 切换应触发立即 flush，产生 2 个独立批次');
  assert.deepEqual(
    batches[0].map((e) => e.type),
    ['thinking', 'thinking'],
  );
  assert.deepEqual(
    batches[1].map((e) => e.type),
    ['text', 'text'],
  );
  buf.close();
});

test('StreamBuffer: text → tool_use → tool_result 三类切换各成一批', () => {
  const batches = [];
  const buf = new StreamBuffer({
    onFlush: (evs) => batches.push(evs),
    flushMs: 1000,
  });
  buf.push(textEvent('准备调用工具'));
  buf.push(toolUseEvent('Read', { path: '/a' }));
  buf.push(toolResultEvent('Read', 'content'));
  buf.flush();
  assert.equal(batches.length, 3, '三种 kind 各自独立成批');
  assert.equal(batches[0][0].type, 'text');
  assert.equal(batches[1][0].type, 'tool_use');
  assert.equal(batches[2][0].type, 'tool_result');
  buf.close();
});

test('StreamBuffer: flushOnKindSwitch=false 时退回到纯时间窗批处理', () => {
  const batches = [];
  const buf = new StreamBuffer({
    onFlush: (evs) => batches.push(evs),
    flushMs: 1000,
    flushOnKindSwitch: false,
  });
  buf.push(thinkingEvent('想'));
  buf.push(textEvent('答'));
  // 关闭 kind 切换 flush 后，两类事件应合并到同一 buffer（直到 timer 或 close）
  assert.equal(batches.length, 0);
  buf.close();
  assert.equal(batches.length, 1);
  assert.deepEqual(
    batches[0].map((e) => e.type),
    ['thinking', 'text'],
  );
});

test('StreamBuffer: cancel 事件立即 flush（与 turn_end 同级）', () => {
  const batches = [];
  const buf = new StreamBuffer({
    onFlush: (evs) => batches.push(evs),
    flushMs: 1000,
  });
  buf.push(textEvent('未完'));
  buf.push(cancelEvent('用户取消'));
  assert.equal(batches.length, 2);
  assert.equal(batches[0][0].type, 'text');
  assert.equal(batches[1][0].type, 'cancel');
});

test('StreamBuffer: 默认 flushMs=16（短文本回复也能流式呈现）', () => {
  // 不传 flushMs 时应使用 16ms 默认值，而不是旧的 50ms
  const buf = new StreamBuffer({ onFlush: () => {} });
  assert.equal(buf.flushMs, 16);
});
