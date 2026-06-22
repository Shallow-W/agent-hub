'use strict';

// ClaudeCliSpec.parseStreamEvent 单测：验证 content_block_delta 系列事件的增量解析。
// 覆盖：
//   - content_block_start (text/thinking/tool_use)
//   - content_block_delta (text_delta/thinking_delta/input_json_delta)
//   - content_block_stop（应返回 null，不产生事件）
//   - user (tool_result)
//   - result (turn_end)
//   - 非可识别事件 → null
//   - 非 JSON 行 → null

const { test } = require('node:test');
const assert = require('node:assert');

const { createClaudeCliSpec } = require('../claude');
const { EVENT_TYPES } = require('../events');

// Mock ctx：parseStreamEvent 不依赖 ctx 字段，但 createClaudeCliSpec 需要一个对象。
const mockCtx = {
  defaultSkills: (caps) => caps.map((id) => ({ id, name: id })),
  resolveCommand: (name) => `/bin/${name}`,
  buildPlatformMcpArgs: () => [],
  addRoot: () => {},
  pathJoin: (...args) => args.join('/'),
  isAgentHubWorkspace: () => false,
  makeSessionId: (conv, agent) => `sess-${conv}-${agent}`,
  logFlow: () => {},
  processSpec: (command, args) => ({ command, args }),
  spawn: () => ({ stdin: { write() {}, end() {} }, stdout: { on() {}, setEncoding() {} }, stderr: { on() {}, setEncoding() {} }, on() {}, pid: 0 }),
  crypto: { randomUUID: () => 'uuid-fake' },
  truncateStr: (s) => s,
  EXEC_TIMEOUT_MS: 400000,
  agentTurnStates: new Map(),
  createAsyncQueue: require('../events').createAsyncQueue,
  fs: require('node:fs'),
};

const spec = createClaudeCliSpec(mockCtx);

function call(line) {
  const result = spec.parseStreamEvent(line, mockCtx);
  if (result === null) return [];
  return Array.isArray(result) ? result : [result];
}

test('content_block_start(text): 不产生事件（保留纯净，等 delta 到来）', () => {
  const line = JSON.stringify({
    type: 'content_block_start',
    index: 0,
    content_block: { type: 'text', text: '' },
  });
  const events = call(line);
  assert.equal(events.length, 0);
});

test('content_block_start(tool_use): 产生一个空 input 的 tool_use 事件，让前端立即显示气泡', () => {
  const line = JSON.stringify({
    type: 'content_block_start',
    index: 2,
    content_block: { type: 'tool_use', id: 'toolu_abc', name: 'Read', input: {} },
  });
  const events = call(line);
  assert.equal(events.length, 1);
  assert.equal(events[0].type, EVENT_TYPES.TOOL_USE);
  assert.equal(events[0].tool, 'Read');
  assert.deepEqual(events[0].input, {});
});

test('content_block_delta(text_delta): 产生 text 事件', () => {
  const line = JSON.stringify({
    type: 'content_block_delta',
    index: 0,
    delta: { type: 'text_delta', text: 'Hello' },
  });
  const events = call(line);
  assert.equal(events.length, 1);
  assert.equal(events[0].type, EVENT_TYPES.TEXT);
  assert.equal(events[0].content, 'Hello');
});

test('content_block_delta(thinking_delta): 产生 thinking 事件', () => {
  const line = JSON.stringify({
    type: 'content_block_delta',
    index: 1,
    delta: { type: 'thinking_delta', thinking: 'Let me analyze...' },
  });
  const events = call(line);
  assert.equal(events.length, 1);
  assert.equal(events[0].type, EVENT_TYPES.THINKING);
  assert.equal(events[0].content, 'Let me analyze...');
});

test('content_block_delta(input_json_delta): 产生 tool_use 事件，tool 空字符串（仅 partial_json）', () => {
  const line = JSON.stringify({
    type: 'content_block_delta',
    index: 2,
    delta: { type: 'input_json_delta', partial_json: '{"file_path":"/tmp/x' },
  });
  const events = call(line);
  assert.equal(events.length, 1);
  assert.equal(events[0].type, EVENT_TYPES.TOOL_USE);
  assert.equal(events[0].tool, '');
  assert.equal(events[0].input, '{"file_path":"/tmp/x');
});

test('content_block_stop: 返回 null（不产生事件）', () => {
  const line = JSON.stringify({ type: 'content_block_stop', index: 0 });
  const events = call(line);
  assert.equal(events.length, 0);
});

test('user(tool_result): 产生 tool_result 事件，content 数组形式也支持', () => {
  const line = JSON.stringify({
    type: 'user',
    message: {
      role: 'user',
      content: [
        { type: 'tool_result', tool_use_id: 'toolu_abc', content: 'file contents here' },
      ],
    },
  });
  const events = call(line);
  assert.equal(events.length, 1);
  assert.equal(events[0].type, EVENT_TYPES.TOOL_RESULT);
  assert.equal(events[0].output, 'file contents here');
});

test('user(tool_result) with array content: 把 content[].text 拼接', () => {
  const line = JSON.stringify({
    type: 'user',
    message: {
      content: [
        {
          type: 'tool_result',
          tool_use_id: 'toolu_abc',
          content: [{ type: 'text', text: 'part1-' }, { type: 'text', text: 'part2' }],
        },
      ],
    },
  });
  const events = call(line);
  assert.equal(events.length, 1);
  assert.equal(events[0].output, 'part1-part2');
});

test('result(success): 产生 turn_end 事件，携带 result 文本', () => {
  const line = JSON.stringify({
    type: 'result',
    subtype: 'success',
    result: 'Final answer',
    is_error: false,
    duration_ms: 1000,
  });
  const events = call(line);
  assert.equal(events.length, 1);
  assert.equal(events[0].type, EVENT_TYPES.TURN_END);
  assert.equal(events[0].result, 'Final answer');
  assert.equal(events[0].error, undefined);
});

test('result(error): 产生 turn_end 事件，携带 error', () => {
  const line = JSON.stringify({
    type: 'result',
    subtype: 'error_during_execution',
    result: 'something failed',
    is_error: true,
  });
  const events = call(line);
  assert.equal(events.length, 1);
  assert.equal(events[0].type, EVENT_TYPES.TURN_END);
  assert.equal(events[0].error, 'something failed');
});

test('assistant(整条): 兼容路径，content 里 text/thinking/tool_use 都提取', () => {
  const line = JSON.stringify({
    type: 'assistant',
    message: {
      role: 'assistant',
      content: [
        { type: 'thinking', thinking: 'internal reasoning' },
        { type: 'text', text: 'visible text' },
        { type: 'tool_use', id: 'toolu_x', name: 'Bash', input: { cmd: 'ls' } },
      ],
    },
  });
  const events = call(line);
  assert.equal(events.length, 3);
  assert.equal(events[0].type, EVENT_TYPES.THINKING);
  assert.equal(events[1].type, EVENT_TYPES.TEXT);
  assert.equal(events[2].type, EVENT_TYPES.TOOL_USE);
  assert.equal(events[2].tool, 'Bash');
});

test('assistant(空 content): 至少产生一个空 text 事件（保留 agentTurnStates active 触发）', () => {
  const line = JSON.stringify({
    type: 'assistant',
    message: { role: 'assistant', content: [] },
  });
  const events = call(line);
  assert.equal(events.length, 1);
  assert.equal(events[0].type, EVENT_TYPES.TEXT);
  assert.equal(events[0].content, '');
});

test('system 事件: 返回 null（当前不处理）', () => {
  const line = JSON.stringify({ type: 'system', subtype: 'init', session_id: 'x' });
  const events = call(line);
  assert.equal(events.length, 0);
});

test('非 JSON 行: 返回 null（不抛错）', () => {
  assert.equal(spec.parseStreamEvent('not-json', mockCtx), null);
});

test('空行: 返回 null', () => {
  assert.equal(spec.parseStreamEvent('', mockCtx), null);
  assert.equal(spec.parseStreamEvent('   ', mockCtx), null);
});

test('parseStreamEventAll: 把单个事件展开为数组', () => {
  const line = JSON.stringify({
    type: 'content_block_delta',
    index: 0,
    delta: { type: 'text_delta', text: 'hi' },
  });
  const arr = spec.parseStreamEventAll(line, mockCtx);
  assert.ok(Array.isArray(arr));
  assert.equal(arr.length, 1);
});
