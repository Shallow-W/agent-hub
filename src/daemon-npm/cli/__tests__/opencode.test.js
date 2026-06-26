'use strict';

const { test } = require('node:test');
const assert = require('node:assert');

const { createOpenCodeCliSpec } = require('../opencode');

function buildMockCtx(overrides = {}) {
  const savedMap = new Map();
  return {
    defaultSkills: (caps) => caps.map((id) => ({ id, name: id })),
    resolveCommand: (name) => `/bin/${name}`,
    commandVersion: overrides.commandVersion || (() => '1.0.0'),
    existingFile: overrides.existingFile || ((v) => v || null),
    pathJoin: (...args) => args.join('/'),
    conversationSessions: overrides.conversationSessions || savedMap,
    saveSessionMap: overrides.saveSessionMap || function saveSessionMap() {},
    logFlow: () => {},
    processSpec: (command, args) => ({ command, args }),
    spawnSync: () => ({ status: 0, stdout: '', stderr: '' }),
    sessionKeyForTask: (task) => `${task.conversation_id}:${task.agent_id}`,
    opencodeContextChanged: () => false,
    buildAgentHubContextEnv: () => ({}),
    ensureOpenCodeMcpConfig: () => '/tmp/opencode.json',
    addRoot: (arr, root) => { if (arr.indexOf(root) === -1) arr.push(root); },
    isAgentHubWorkspace: () => false,
    openClawInstallSkillRoots: () => [],
  };
}

test('opencode.resolveCommand returns AGENTHUB_OPENCODE_COMMAND when valid', () => {
  process.env.AGENTHUB_OPENCODE_COMMAND = '/custom/opencode';
  const ctx = buildMockCtx({
    existingFile: (v) => v || null,
    commandVersion: () => '1.0.0',
  });
  const spec = createOpenCodeCliSpec(ctx);
  assert.strictEqual(spec.resolveCommand(), '/custom/opencode');
  delete process.env.AGENTHUB_OPENCODE_COMMAND;
});

test('opencode.resolveCommand returns "opencode" literal when nothing matches', () => {
  delete process.env.AGENTHUB_OPENCODE_COMMAND;
  const ctx = buildMockCtx({
    existingFile: () => null,
    commandVersion: () => null,
  });
  const spec = createOpenCodeCliSpec(ctx);
  assert.strictEqual(spec.resolveCommand(), 'opencode');
});

test('opencode.parseResult returns fallback for empty stdout', () => {
  const ctx = buildMockCtx();
  const spec = createOpenCodeCliSpec(ctx);
  const result = spec.parseResult({ stdout: '' });
  assert.strictEqual(result.text, '(OpenCode CLI 没有返回内容)');
  assert.strictEqual(result.sessionId, '');
});

test('opencode.parseResult parses assistant message content', () => {
  const ctx = buildMockCtx();
  const spec = createOpenCodeCliSpec(ctx);
  const stdout = JSON.stringify({
    type: 'message',
    message: {
      role: 'assistant',
      content: [{ type: 'text', text: 'hello world' }],
    },
  });
  const result = spec.parseResult({ stdout });
  assert.strictEqual(result.text, 'hello world');
});

test('opencode.parseResult extracts session id', () => {
  const ctx = buildMockCtx();
  const spec = createOpenCodeCliSpec(ctx);
  const stdout = JSON.stringify({
    sessionID: 'opencode-sess-123',
    message: {
      role: 'assistant',
      content: 'parsed text',
    },
  });
  const result = spec.parseResult({ stdout });
  assert.strictEqual(result.sessionId, 'opencode-sess-123');
  assert.strictEqual(result.text, 'parsed text');
});

test('opencode.parseResult persists session when meta.persistSessionKey is set', () => {
  const savedMap = new Map();
  let saved = false;
  const ctx = buildMockCtx({
    conversationSessions: savedMap,
    saveSessionMap: function saveSessionMap() { saved = true; },
  });
  const spec = createOpenCodeCliSpec(ctx);
  const stdout = JSON.stringify({
    sessionId: 'opencode-sess-456',
    message: { role: 'assistant', content: 'done' },
  });
  const result = spec.parseResult({ stdout, meta: { persistSessionKey: 'agent:conv' } });
  assert.strictEqual(result.sessionId, 'opencode-sess-456');
  assert.strictEqual(savedMap.get('agent:conv'), 'opencode-sess-456');
  assert.ok(saved, 'saveSessionMap should be called');
});

test('opencode.parseResult skips session persistence without meta.persistSessionKey', () => {
  const savedMap = new Map();
  let saved = false;
  const ctx = buildMockCtx({
    conversationSessions: savedMap,
    saveSessionMap: function saveSessionMap() { saved = true; },
  });
  const spec = createOpenCodeCliSpec(ctx);
  const stdout = JSON.stringify({ sessionId: 's1', message: { role: 'assistant', content: 'hi' } });
  spec.parseResult({ stdout });
  assert.strictEqual(savedMap.size, 0);
  assert.strictEqual(saved, false);
});

test('opencode.parseResult handles multi-line JSON stream', () => {
  const ctx = buildMockCtx();
  const spec = createOpenCodeCliSpec(ctx);
  const stdout = [
    JSON.stringify({ sessionID: 'multi-sess' }),
    JSON.stringify({ message: { role: 'assistant', content: 'line1' } }),
    JSON.stringify({ message: { role: 'assistant', content: 'line2' } }),
  ].join('\n');
  const result = spec.parseResult({ stdout });
  assert.strictEqual(result.sessionId, 'multi-sess');
  assert.strictEqual(result.text, 'line1\nline2');
});

test('opencode.parseResult returns raw text when JSON parse fails', () => {
  const ctx = buildMockCtx();
  const spec = createOpenCodeCliSpec(ctx);
  const result = spec.parseResult({ stdout: 'not json garbage {{{' });
  assert.strictEqual(result.text, 'not json garbage {{{');
  assert.strictEqual(result.sessionId, '');
});
