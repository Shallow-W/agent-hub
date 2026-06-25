'use strict';

const { test } = require('node:test');
const assert = require('node:assert');

const { createCodexCliSpec } = require('../codex');

// ctx.codexLocalInstallPaths / codexExtensionPath / existingFile / commandVersion /
// fs / pathJoin / tmpdir / defaultSkills / resolveCommand（fallback）/ ensureAgentHubCodexHome /
// ensureAgentHubCodexMcpConfig / ensureTaskWorkdir / buildAgentHubContextEnv /
// logFlow / processSpec / spawnSync / firstLine
function buildMockCtx(overrides = {}) {
  return {
    defaultSkills: (caps) => caps.map((id) => ({ id, name: id })),
    resolveCommand: (name) => `/bin/${name}`,
    commandVersion: overrides.commandVersion || (() => '1.0.0'),
    existingFile: overrides.existingFile || ((v) => v || null),
    codexLocalInstallPaths: overrides.codexLocalInstallPaths || (() => []),
    codexExtensionPath: overrides.codexExtensionPath || (() => null),
    fs: overrides.fs || {
      existsSync: () => false,
      readFileSync: () => '',
      rmSync: () => {},
    },
    pathJoin: (...args) => args.join('/'),
    tmpdir: () => '/tmp',
    ensureAgentHubCodexHome: overrides.ensureAgentHubCodexHome || (() => '/tmp/codex-home'),
    ensureAgentHubCodexMcpConfig: overrides.ensureAgentHubCodexMcpConfig || (() => {}),
    ensureTaskWorkdir: () => '/tmp/work',
    buildAgentHubContextEnv: overrides.buildAgentHubContextEnv || (() => ({})),
    logFlow: () => {},
    processSpec: (command, args) => ({ command, args }),
    spawnSync: () => ({ status: 0, stdout: '', stderr: '' }),
    firstLine: (s) => String(s || '').split('\n')[0],
  };
}

test('codex.resolveCommand returns AGENTHUB_CODEX_COMMAND env var when valid', () => {
  process.env.AGENTHUB_CODEX_COMMAND = '/custom/codex';
  const ctx = buildMockCtx({
    existingFile: (v) => v || null,
    commandVersion: () => '1.2.3',
  });
  const spec = createCodexCliSpec(ctx);
  assert.strictEqual(spec.resolveCommand(), '/custom/codex');
  delete process.env.AGENTHUB_CODEX_COMMAND;
});

test('codex.resolveCommand falls back through candidates', () => {
  delete process.env.AGENTHUB_CODEX_COMMAND;
  const ctx = buildMockCtx({
    codexLocalInstallPaths: () => ['/home/.local/bin/codex'],
    codexExtensionPath: () => null,
    commandVersion: (cmd) => (cmd === '/home/.local/bin/codex' ? '1.0.0' : null),
  });
  const spec = createCodexCliSpec(ctx);
  assert.strictEqual(spec.resolveCommand(), '/home/.local/bin/codex');
});

test('codex.resolveCommand returns "codex" literal when nothing matches', () => {
  delete process.env.AGENTHUB_CODEX_COMMAND;
  const ctx = buildMockCtx({
    codexLocalInstallPaths: () => [],
    codexExtensionPath: () => null,
    commandVersion: () => null,
  });
  const spec = createCodexCliSpec(ctx);
  assert.strictEqual(spec.resolveCommand(), 'codex');
});

test('codex.buildCommand passes task id to MCP config and context env', () => {
  const calls = [];
  const ctx = buildMockCtx({
    ensureAgentHubCodexMcpConfig: (...args) => { calls.push(args); },
    buildAgentHubContextEnv: (conv, user, agent, taskId) => ({
      AGENTHUB_CONVERSATION_ID: conv,
      AGENTHUB_USER_ID: user,
      AGENTHUB_AGENT_ID: agent,
      AGENTHUB_TASK_ID: taskId,
    }),
  });
  const spec = createCodexCliSpec(ctx);
  const command = spec.buildCommand({
    id: 'task-from-payload',
    conversation_id: 'conv-1',
    user_id: 'user-1',
    agent_id: 'agent-1',
  }, {
    command: 'codex',
    systemPrompt: '',
    userPrompt: 'hello',
    taskId: 'task-from-context',
  });

  assert.deepStrictEqual(calls[0], [
    '/tmp/codex-home',
    'conv-1',
    'user-1',
    'agent-1',
    'task-from-context',
  ]);
  assert.strictEqual(command.env.AGENTHUB_TASK_ID, 'task-from-context');
});

test('codex.parseResult prefers outputFile when present and non-empty', () => {
  const ctx = buildMockCtx({
    fs: {
      existsSync: (p) => p === '/tmp/out.txt',
      readFileSync: () => 'file content',
      rmSync: () => {},
    },
  });
  const spec = createCodexCliSpec(ctx);
  const result = spec.parseResult({
    stdout: 'stdio content',
    outputFile: '/tmp/out.txt',
  });
  assert.strictEqual(result, 'file content');
});

test('codex.parseResult falls back to stdio when outputFile missing', () => {
  const ctx = buildMockCtx({
    fs: {
      existsSync: () => false,
      readFileSync: () => '',
      rmSync: () => {},
    },
  });
  const spec = createCodexCliSpec(ctx);
  const result = spec.parseResult({ stdout: 'stdio only', stderr: '' });
  assert.strictEqual(result, 'stdio only');
});

test('codex.parseResult combines stdout and stderr with newline', () => {
  const ctx = buildMockCtx({
    fs: { existsSync: () => false, readFileSync: () => '', rmSync: () => {} },
  });
  const spec = createCodexCliSpec(ctx);
  assert.strictEqual(spec.parseResult({ stdout: 'out', stderr: 'err' }), 'out\nerr');
});

test('codex.parseResult returns fallback message when empty', () => {
  const ctx = buildMockCtx({
    fs: { existsSync: () => false, readFileSync: () => '', rmSync: () => {} },
  });
  const spec = createCodexCliSpec(ctx);
  assert.strictEqual(spec.parseResult({}), '(Agent CLI 没有返回内容)');
});
