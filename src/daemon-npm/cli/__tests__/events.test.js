'use strict';

const { test } = require('node:test');
const assert = require('node:assert');

const {
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
} = require('../events');

test('EVENT_TYPES is frozen and has expected keys', () => {
  assert.ok(Object.isFrozen(EVENT_TYPES));
  assert.strictEqual(EVENT_TYPES.TEXT, 'text');
  assert.strictEqual(EVENT_TYPES.THINKING, 'thinking');
  assert.strictEqual(EVENT_TYPES.TOOL_USE, 'tool_use');
  assert.strictEqual(EVENT_TYPES.TOOL_RESULT, 'tool_result');
  assert.strictEqual(EVENT_TYPES.CARD, 'card');
  assert.strictEqual(EVENT_TYPES.ERROR, 'error');
  assert.strictEqual(EVENT_TYPES.TURN_END, 'turn_end');
  assert.strictEqual(EVENT_TYPES.SESSION_END, 'session_end');
});

test('factory functions produce correct shape', () => {
  assert.deepStrictEqual(textEvent('hi'), { type: 'text', content: 'hi' });
  assert.deepStrictEqual(textEvent(null), { type: 'text', content: '' });

  assert.deepStrictEqual(thinkingEvent('hmm'), { type: 'thinking', content: 'hmm' });

  assert.deepStrictEqual(toolUseEvent('shell', { cmd: 'ls' }), {
    type: 'tool_use',
    tool: 'shell',
    input: { cmd: 'ls' },
  });

  assert.deepStrictEqual(toolResultEvent('shell', 'done'), {
    type: 'tool_result',
    tool: 'shell',
    output: 'done',
    isError: false,
  });
  assert.deepStrictEqual(toolResultEvent('shell', 'err', true), {
    type: 'tool_result',
    tool: 'shell',
    output: 'err',
    isError: true,
  });

  assert.deepStrictEqual(cardEvent('info', { foo: 1 }), {
    type: 'card',
    cardType: 'info',
    payload: { foo: 1 },
  });

  assert.deepStrictEqual(errorEvent('boom'), { type: 'error', message: 'boom' });

  assert.deepStrictEqual(turnEndEvent({ result: 'ok' }), { type: 'turn_end', result: 'ok' });
  assert.deepStrictEqual(turnEndEvent({ error: 'bad' }), { type: 'turn_end', error: 'bad' });
  assert.deepStrictEqual(turnEndEvent(), { type: 'turn_end' });

  assert.deepStrictEqual(sessionEndEvent({ code: 0 }), { type: 'session_end', code: 0 });
  assert.deepStrictEqual(sessionEndEvent({ signal: 'SIGTERM' }), { type: 'session_end', signal: 'SIGTERM' });
});

test('createAsyncQueue delivers pushed events in order, then ends after done()', async () => {
  const q = createAsyncQueue();
  q.push(textEvent('a'));
  q.push(textEvent('b'));
  q.push(textEvent('c'));
  q.done();

  const collected = [];
  for await (const ev of q.iter) {
    collected.push(ev);
  }

  assert.strictEqual(collected.length, 3);
  assert.strictEqual(collected[0].content, 'a');
  assert.strictEqual(collected[1].content, 'b');
  assert.strictEqual(collected[2].content, 'c');
});

test('createAsyncQueue supports interleaved push and consume', async () => {
  const q = createAsyncQueue();
  const received = [];
  const consumer = (async () => {
    for await (const ev of q.iter) {
      received.push(ev);
    }
  })();

  // Wait a microtask so consumer is awaiting
  await new Promise((r) => setImmediate(r));
  q.push(textEvent('late1'));
  q.push(textEvent('late2'));
  q.done();
  await consumer;

  assert.strictEqual(received.length, 2);
  assert.strictEqual(received[0].content, 'late1');
  assert.strictEqual(received[1].content, 'late2');
});

test('createAsyncQueue done() before any push yields empty iteration', async () => {
  const q = createAsyncQueue();
  q.done();
  const collected = [];
  for await (const ev of q.iter) collected.push(ev);
  assert.strictEqual(collected.length, 0);
});

test('createAsyncQueue push after done() is silently dropped', async () => {
  const q = createAsyncQueue();
  q.push(textEvent('before'));
  q.done();
  q.push(textEvent('after')); // should be dropped
  const collected = [];
  for await (const ev of q.iter) collected.push(ev);
  assert.strictEqual(collected.length, 1);
  assert.strictEqual(collected[0].content, 'before');
});
