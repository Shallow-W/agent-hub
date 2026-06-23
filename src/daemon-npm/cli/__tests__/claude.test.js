'use strict';

const { test } = require('node:test');
const assert = require('node:assert');

const { createClaudeCliSpec } = require('../claude');
const { EVENT_TYPES } = require('../events');

// 构建 mock ctx —— claude.js 当前依赖的 ctx 字段：
//   defaultSkills, resolveCommand, buildPlatformMcpArgs, addRoot, pathJoin,
//   isAgentHubWorkspace, makeSessionId, logFlow, processSpec, spawn, crypto,
//   truncateStr, EXEC_TIMEOUT_MS, agentTurnStates, createAsyncQueue, fs
function buildMockCtx(overrides = {}) {
  const calls = { resolveCommand: [], buildPlatformMcpArgs: [], spawn: [], logFlow: [] };
  const ctx = {
    defaultSkills: (caps) => caps.map((id) => ({ id, name: id })),
    resolveCommand: (name) => { calls.resolveCommand.push(name); return overrides.command || `/bin/${name}`; },
    buildPlatformMcpArgs: (conv, user, agent, taskId) => {
      calls.buildPlatformMcpArgs.push({ conv, user, agent, taskId });
      return overrides.mcpArgs || [];
    },
    addRoot: (arr, root) => { if (arr.indexOf(root) === -1) arr.push(root); },
    pathJoin: (...args) => args.join('/'),
    isAgentHubWorkspace: () => false,
    makeSessionId: (conv, agent) => `sess-${conv}-${agent}`,
    logFlow: (level, event, payload) => {
      calls.logFlow.push({ level, event, payload });
    },
    processSpec: (command, args) => ({ command, args }),
    spawn: (command, args, options) => {
      calls.spawn.push({ command, args, options });
      const stdin = { write() {}, end() {} };
      const stdout = {
        setEncoding() {},
        on() {},
      };
      const stderr = {
        setEncoding() {},
        on() {},
      };
      const child = {
        pid: 12345,
        exitCode: null,
        stdin,
        stdout,
        stderr,
        on() {},
        kill() {},
      };
      return child;
    },
    crypto: { randomUUID: () => 'mock-uuid-1234' },
    truncateStr: (s) => s,
    EXEC_TIMEOUT_MS: 1000,
    agentTurnStates: new Map(),
    createAsyncQueue: undefined, // 让 spawnPersistent 走 require('./events').createAsyncQueue
    fs: { existsSync: () => false, readFileSync: () => '', rmSync: () => {} },
    ...overrides,
  };
  return { ctx, calls };
}

test('claude.resolveCommand returns literal "claude" (no delegation to avoid recursion)', () => {
  const { ctx, calls } = buildMockCtx();
  const spec = createClaudeCliSpec(ctx);
  const result = spec.resolveCommand();
  assert.strictEqual(result, 'claude');
  // 不能回调 ctx.resolveCommand('claude')——会触发 ctx.resolveCommand ↔ spec.resolveCommand 循环递归。
  assert.deepStrictEqual(calls.resolveCommand, []);
});

test('claude.parseResult combines stdout and stderr (fallback behavior)', () => {
  const { ctx } = buildMockCtx();
  const spec = createClaudeCliSpec(ctx);
  assert.strictEqual(spec.parseResult({ stdout: 'hello', stderr: 'world' }), 'hello\nworld');
  assert.strictEqual(spec.parseResult({ stdout: 'only' }), 'only');
  assert.strictEqual(spec.parseResult({}), '(Agent CLI 没有返回内容)');
  assert.strictEqual(spec.parseResult({ stdout: '', stderr: '' }), '(Agent CLI 没有返回内容)');
});

test('claude.parseStreamEvent returns null on non-JSON lines', () => {
  const { ctx } = buildMockCtx();
  const spec = createClaudeCliSpec(ctx);
  assert.strictEqual(spec.parseStreamEvent('', ctx), null);
  assert.strictEqual(spec.parseStreamEvent('not json', ctx), null);
  assert.strictEqual(spec.parseStreamEvent('   ', ctx), null);
});

test('claude.parseStreamEvent parses assistant text content', () => {
  const { ctx } = buildMockCtx();
  const spec = createClaudeCliSpec(ctx);
  const line = JSON.stringify({
    type: 'assistant',
    message: {
      content: [{ type: 'text', text: 'hello' }],
    },
  });
  const events = spec.parseStreamEvent(line, ctx);
  assert.ok(Array.isArray(events));
  assert.strictEqual(events.length, 1);
  assert.strictEqual(events[0].type, EVENT_TYPES.TEXT);
  assert.strictEqual(events[0].content, 'hello');
});

test('claude.parseStreamEvent parses assistant with multiple content blocks', () => {
  const { ctx } = buildMockCtx();
  const spec = createClaudeCliSpec(ctx);
  const line = JSON.stringify({
    type: 'assistant',
    message: {
      content: [
        { type: 'thinking', thinking: 'pondering' },
        { type: 'text', text: 'reply' },
        { type: 'tool_use', name: 'shell', input: { cmd: 'ls' } },
      ],
    },
  });
  const events = spec.parseStreamEvent(line, ctx);
  assert.ok(Array.isArray(events));
  assert.strictEqual(events.length, 3);
  assert.strictEqual(events[0].type, EVENT_TYPES.THINKING);
  assert.strictEqual(events[0].content, 'pondering');
  assert.strictEqual(events[1].type, EVENT_TYPES.TEXT);
  assert.strictEqual(events[1].content, 'reply');
  assert.strictEqual(events[2].type, EVENT_TYPES.TOOL_USE);
  assert.strictEqual(events[2].tool, 'shell');
  assert.deepStrictEqual(events[2].input, { cmd: 'ls' });
});

test('claude.parseStreamEvent parses result event as turn_end', () => {
  const { ctx } = buildMockCtx();
  const spec = createClaudeCliSpec(ctx);
  const okLine = JSON.stringify({ type: 'result', result: 'done' });
  const ev = spec.parseStreamEvent(okLine, ctx);
  assert.strictEqual(ev.type, EVENT_TYPES.TURN_END);
  assert.strictEqual(ev.result, 'done');
  assert.strictEqual(ev.error, undefined);

  // 错误路径：TURN_END 同时携带 result（用于 logFlow 的 result_len）和 error。
  // 这是 Bug 2 修复的一部分 —— spawnPersistent 需要从 ev.result 算出 result_len
  // 来镜像原 daemon 的 agent.turn_result 日志结构。
  const errLine = JSON.stringify({
    type: 'result',
    result: 'boom',
    is_error: true,
  });
  const evErr = spec.parseStreamEvent(errLine, ctx);
  assert.strictEqual(evErr.type, EVENT_TYPES.TURN_END);
  assert.strictEqual(evErr.error, 'boom');
  assert.strictEqual(evErr.result, 'boom');
});

test('claude.parseStreamEvent treats subtype=error_during_execution as error', () => {
  const { ctx } = buildMockCtx();
  const spec = createClaudeCliSpec(ctx);
  const line = JSON.stringify({
    type: 'result',
    result: 'partial',
    subtype: 'error_during_execution',
  });
  const ev = spec.parseStreamEvent(line, ctx);
  assert.strictEqual(ev.type, EVENT_TYPES.TURN_END);
  assert.strictEqual(ev.error, 'partial');
});

test('claude.parseStreamEvent returns null on unknown event types', () => {
  const { ctx } = buildMockCtx();
  const spec = createClaudeCliSpec(ctx);
  const line = JSON.stringify({ type: 'system', subtype: 'init' });
  assert.strictEqual(spec.parseStreamEvent(line, ctx), null);
});

test('claude.spawnPersistent returns { child, sessionId, sendPrompt, events }', async () => {
  const { ctx, calls } = buildMockCtx();
  const spec = createClaudeCliSpec(ctx);
  const result = spec.spawnPersistent({
    agentId: 'agent-1',
    sessionId: null,
    systemPrompt: 'be helpful',
    resume: false,
    conversationId: 'conv-1',
    userId: 'user-1',
    taskCtx: {},
  }, ctx);

  assert.ok(result.child, 'child');
  assert.ok(result.sessionId, 'sessionId');
  assert.strictEqual(typeof result.sendPrompt, 'function');
  assert.ok(result.events, 'events');
  assert.strictEqual(result.sessionId, 'mock-uuid-1234');
  assert.strictEqual(calls.spawn.length, 1);
  assert.strictEqual(calls.buildPlatformMcpArgs.length, 1);
});

test('claude.spawnPersistent uses provided sessionId when resume=true', () => {
  const { ctx } = buildMockCtx();
  const spec = createClaudeCliSpec(ctx);
  const result = spec.spawnPersistent({
    agentId: 'a',
    sessionId: 'provided-uuid',
    systemPrompt: '',
    resume: true,
    conversationId: 'c',
    userId: 'u',
    taskCtx: {},
  }, ctx);
  assert.strictEqual(result.sessionId, 'provided-uuid');
});

// === Bug fix tests (Part A) ===

test('claude.spawnPersistent passes taskId from taskCtx to buildPlatformMcpArgs (Bug 1)', () => {
  const { ctx, calls } = buildMockCtx();
  const spec = createClaudeCliSpec(ctx);
  spec.spawnPersistent({
    agentId: 'agent-1',
    sessionId: null,
    systemPrompt: '',
    resume: false,
    conversationId: 'conv-1',
    userId: 'user-1',
    taskCtx: { taskId: 'task-xyz-123' },
  }, ctx);

  assert.strictEqual(calls.buildPlatformMcpArgs.length, 1);
  assert.deepStrictEqual(calls.buildPlatformMcpArgs[0], {
    conv: 'conv-1',
    user: 'user-1',
    agent: 'agent-1',
    taskId: 'task-xyz-123',
  });
});

test('claude.spawnPersistent passes null taskId when taskCtx undefined (Bug 1 safety)', () => {
  const { ctx, calls } = buildMockCtx();
  const spec = createClaudeCliSpec(ctx);
  spec.spawnPersistent({
    agentId: 'agent-1',
    sessionId: null,
    systemPrompt: '',
    resume: false,
    conversationId: 'conv-1',
    userId: 'user-1',
    taskCtx: undefined,
  }, ctx);

  assert.strictEqual(calls.buildPlatformMcpArgs.length, 1);
  assert.strictEqual(calls.buildPlatformMcpArgs[0].taskId, null);
});

test('claude.spawnPersistent emits agent.turn_result logFlow on TURN_END (Bug 2)', async () => {
  // 此测试需要驱动 child.stdout.on('data', ...) 回调。
  // 重写 spawn mock 让 stdout.on 能同步触发 listener。
  const { ctx, calls } = buildMockCtx();
  let stdoutListener = null;
  ctx.spawn = (command, args, options) => {
    calls.spawn.push({ command, args, options });
    const stdin = { write() {}, end() {} };
    const stdout = {
      setEncoding() {},
      on(_evt, fn) { stdoutListener = fn; },
    };
    const stderr = { setEncoding() {}, on() {} };
    const child = {
      pid: 12345,
      exitCode: null,
      stdin,
      stdout,
      stderr,
      on() {},
      kill() {},
    };
    return child;
  };

  const spec = createClaudeCliSpec(ctx);
  spec.spawnPersistent({
    agentId: 'agent-turn-1',
    sessionId: 'sess-turn-1',
    systemPrompt: '',
    resume: false,
    conversationId: 'conv-turn-1',
    userId: 'user-turn-1',
    taskCtx: { taskId: 'task-turn-1' },
  }, ctx);

  // 模拟 Claude stream-json 输出一行 result 事件（成功路径）
  const okLine = JSON.stringify({
    type: 'result',
    result: 'all done',
    subtype: 'success',
  });
  assert.ok(stdoutListener, 'stdout data listener should be registered');
  stdoutListener(okLine + '\n');

  const turnResultLog = calls.logFlow.find(
    (entry) => entry.event === 'agent.turn_result',
  );
  assert.ok(turnResultLog, 'agent.turn_result logFlow should be emitted');
  assert.strictEqual(turnResultLog.level, 'info');
  assert.deepStrictEqual(turnResultLog.payload, {
    agent_id: 'agent-turn-1',
    conversation_id: 'conv-turn-1',
    session_id: 'sess-turn-1',
    is_error: false,
    subtype: 'success',
    result_len: 'all done'.length,
  });
});

test('claude.spawnPersistent emits warn-level agent.turn_result on error TURN_END (Bug 2)', async () => {
  const { ctx, calls } = buildMockCtx();
  let stdoutListener = null;
  ctx.spawn = () => {
    const stdin = { write() {}, end() {} };
    const stdout = {
      setEncoding() {},
      on(_evt, fn) { stdoutListener = fn; },
    };
    const stderr = { setEncoding() {}, on() {} };
    return {
      pid: 12345, exitCode: null, stdin, stdout, stderr,
      on() {}, kill() {},
    };
  };

  const spec = createClaudeCliSpec(ctx);
  spec.spawnPersistent({
    agentId: 'agent-err',
    sessionId: 'sess-err',
    systemPrompt: '',
    resume: false,
    conversationId: 'conv-err',
    userId: 'user-err',
    taskCtx: {},
  }, ctx);

  const errLine = JSON.stringify({
    type: 'result',
    result: 'kaboom',
    is_error: true,
    subtype: 'error_during_execution',
  });
  stdoutListener(errLine + '\n');

  const turnResultLog = calls.logFlow.find(
    (entry) => entry.event === 'agent.turn_result',
  );
  assert.ok(turnResultLog, 'agent.turn_result logFlow should be emitted on error');
  assert.strictEqual(turnResultLog.level, 'warn');
  assert.strictEqual(turnResultLog.payload.is_error, true);
  assert.strictEqual(turnResultLog.payload.subtype, 'error_during_execution');
  assert.strictEqual(turnResultLog.payload.result_len, 'kaboom'.length);
});

// === Part D — Switch 4/6 gap coverage ===
// 这些测试覆盖原 daemon.js spawnStreamJsonProcess 里被 thin wrapper 替代后的
// 100+ 行 Claude 专用逻辑。逻辑本身搬到 spec.spawnPersistent，下面针对三条
// 关键路径补单测：sessionId 策略、--system-prompt 条件、stdin stream-json frame。

test('claude.spawnPersistent: resume=false uses --session-id flag with new UUID', () => {
  const { ctx, calls } = buildMockCtx();
  const spec = createClaudeCliSpec(ctx);
  spec.spawnPersistent({
    agentId: 'a',
    sessionId: null,
    systemPrompt: '',
    resume: false,
    conversationId: 'c',
    userId: 'u',
    taskCtx: {},
  }, ctx);

  assert.strictEqual(calls.spawn.length, 1);
  const args = calls.spawn[0].args;
  // resume=false → '--session-id' flag + new random UUID
  const sessionFlagIdx = args.indexOf('--session-id');
  assert.notStrictEqual(sessionFlagIdx, -1, 'must include --session-id flag');
  assert.strictEqual(args[sessionFlagIdx + 1], 'mock-uuid-1234');
  assert.strictEqual(args.includes('--resume'), false);
});

test('claude.spawnPersistent: resume=true uses --resume flag with provided sessionId', () => {
  const { ctx, calls } = buildMockCtx();
  const spec = createClaudeCliSpec(ctx);
  spec.spawnPersistent({
    agentId: 'a',
    sessionId: 'existing-uuid-abc',
    systemPrompt: '',
    resume: true,
    conversationId: 'c',
    userId: 'u',
    taskCtx: {},
  }, ctx);

  const args = calls.spawn[0].args;
  const resumeIdx = args.indexOf('--resume');
  assert.notStrictEqual(resumeIdx, -1, 'must include --resume flag');
  assert.strictEqual(args[resumeIdx + 1], 'existing-uuid-abc');
  assert.strictEqual(args.includes('--session-id'), false);
});

test('claude.spawnPersistent: provided sessionId wins over randomUUID when resume=false', () => {
  const { ctx, calls } = buildMockCtx();
  const spec = createClaudeCliSpec(ctx);
  spec.spawnPersistent({
    agentId: 'a',
    sessionId: 'reuse-this-id',
    systemPrompt: '',
    resume: false,
    conversationId: 'c',
    userId: 'u',
    taskCtx: {},
  }, ctx);

  const args = calls.spawn[0].args;
  // 即使 resume=false，传入 sessionId 时仍复用（走 --session-id <id> 而非 randomUUID）
  const sessionFlagIdx = args.indexOf('--session-id');
  assert.notStrictEqual(sessionFlagIdx, -1);
  assert.strictEqual(args[sessionFlagIdx + 1], 'reuse-this-id');
});

test('claude.spawnPersistent: --system-prompt only added when systemPrompt is non-empty', () => {
  const { ctx, calls } = buildMockCtx();
  const spec = createClaudeCliSpec(ctx);

  // Case 1: non-empty systemPrompt
  spec.spawnPersistent({
    agentId: 'a1',
    sessionId: null,
    systemPrompt: 'be helpful',
    resume: false,
    conversationId: 'c1',
    userId: 'u1',
    taskCtx: {},
  }, ctx);

  const argsWithPrompt = calls.spawn[0].args;
  const idx1 = argsWithPrompt.indexOf('--system-prompt');
  assert.notStrictEqual(idx1, -1, '--system-prompt flag must be present');
  assert.strictEqual(argsWithPrompt[idx1 + 1], 'be helpful');

  // Case 2: empty systemPrompt
  spec.spawnPersistent({
    agentId: 'a2',
    sessionId: null,
    systemPrompt: '',
    resume: false,
    conversationId: 'c2',
    userId: 'u2',
    taskCtx: {},
  }, ctx);

  const argsWithoutPrompt = calls.spawn[1].args;
  assert.strictEqual(argsWithoutPrompt.includes('--system-prompt'), false, '--system-prompt must not be added when empty');

  // Case 3: undefined systemPrompt
  spec.spawnPersistent({
    agentId: 'a3',
    sessionId: null,
    systemPrompt: undefined,
    resume: false,
    conversationId: 'c3',
    userId: 'u3',
    taskCtx: {},
  }, ctx);

  const argsUndefined = calls.spawn[2].args;
  assert.strictEqual(argsUndefined.includes('--system-prompt'), false, '--system-prompt must not be added when undefined');
});

test('claude.spawnPersistent.sendPrompt: writes stream-json user frame to stdin', async () => {
  const { ctx } = buildMockCtx();
  const stdinWrites = [];
  let stdinListener = null;
  ctx.spawn = () => {
    const stdin = {
      write(data) { stdinWrites.push(data); },
      end() {},
    };
    const stdout = {
      setEncoding() {},
      on(_evt, fn) { stdinListener = fn; },
    };
    const stderr = { setEncoding() {}, on() {} };
    return {
      pid: 12345,
      exitCode: null,
      stdin,
      stdout,
      stderr,
      on() {},
      kill() {},
    };
  };

  const spec = createClaudeCliSpec(ctx);
  const result = spec.spawnPersistent({
    agentId: 'a',
    sessionId: 'sess-frame',
    systemPrompt: '',
    resume: false,
    conversationId: 'c',
    userId: 'u',
    taskCtx: {},
  }, ctx);

  // sendPrompt is serialized through a promise chain (queueTail) — the resolver
  // isn't set synchronously. Yield to the microtask queue first.
  const pendingPromise = result.sendPrompt('hello world');
  await new Promise((r) => setImmediate(r));
  await new Promise((r) => setImmediate(r));

  assert.ok(stdinListener, 'stdout data listener should be registered');
  // Drive the result event so sendPrompt resolves.
  stdinListener(JSON.stringify({ type: 'result', result: 'echo: hello world' }) + '\n');
  const response = await pendingPromise;
  assert.strictEqual(response.result, 'echo: hello world');

  // Verify stdin got exactly one frame, in the expected stream-json shape.
  assert.strictEqual(stdinWrites.length, 1, 'sendPrompt should write exactly one frame');
  const frame = JSON.parse(stdinWrites[0].trim());
  assert.strictEqual(frame.type, 'user');
  assert.strictEqual(frame.message.role, 'user');
  assert.ok(Array.isArray(frame.message.content), 'content must be array');
  assert.strictEqual(frame.message.content.length, 1);
  assert.strictEqual(frame.message.content[0].type, 'text');
  assert.strictEqual(frame.message.content[0].text, 'hello world');
});

test('claude.spawnPersistent.sendPrompt: rejects when process already exited', async () => {
  const { ctx } = buildMockCtx();
  let exitCode = 1; // process already dead
  ctx.spawn = () => {
    const stdin = { write() {}, end() {} };
    const stdout = { setEncoding() {}, on() {} };
    const stderr = { setEncoding() {}, on() {} };
    return {
      pid: 12345,
      get exitCode() { return exitCode; },
      stdin, stdout, stderr,
      on() {}, kill() {},
    };
  };

  const spec = createClaudeCliSpec(ctx);
  const result = spec.spawnPersistent({
    agentId: 'a',
    sessionId: 'sess-dead',
    systemPrompt: '',
    resume: false,
    conversationId: 'c',
    userId: 'u',
    taskCtx: {},
  }, ctx);

  await assert.rejects(
    () => result.sendPrompt('hello'),
    /Agent process not running/,
  );
});

test('claude.spawnPersistent: includes stream-json flags and --dangerously-skip-permissions', () => {
  const { ctx, calls } = buildMockCtx();
  const spec = createClaudeCliSpec(ctx);
  spec.spawnPersistent({
    agentId: 'a',
    sessionId: null,
    systemPrompt: '',
    resume: false,
    conversationId: 'c',
    userId: 'u',
    taskCtx: {},
  }, ctx);

  const args = calls.spawn[0].args;
  // 第 0 位永远是 --dangerously-skip-permissions。
  assert.strictEqual(args[0], '--dangerously-skip-permissions');
  // stream-json 输入输出模式 + 多 turn 复用必需的三个 partial flag。
  // 这些一起存在才能在多 turn stdin 模式下拿到 content_block_delta 增量。
  assert.ok(args.includes('-p'), 'must include -p (--include-partial-messages 校验依赖)');
  assert.ok(args.includes('--output-format'), 'must include --output-format');
  assert.ok(args.includes('stream-json'), 'must include stream-json value');
  assert.ok(args.includes('--input-format'), 'must include --input-format');
  assert.ok(args.includes('--include-partial-messages'), 'must include --include-partial-messages (token streaming)');
  assert.ok(args.includes('--replay-user-messages'), 'must include --replay-user-messages (multi-turn stdin driving)');
  assert.ok(args.includes('--verbose'));
});
