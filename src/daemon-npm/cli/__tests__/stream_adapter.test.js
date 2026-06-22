'use strict';

// StreamBuffer 单测：验证 50ms 时间窗口节流、边界事件立即 flush、close 行为。

const { test } = require('node:test');
const assert = require('node:assert');

const { StreamBuffer } = require('../stream_adapter');
const { textEvent, turnEndEvent, errorEvent, sessionEndEvent } = require('../events');

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
