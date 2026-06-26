const assert = require('node:assert/strict');
const test = require('node:test');
const { EventEmitter } = require('node:events');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { execFileSync } = require('node:child_process');
const {
  commandForTask,
  conversationSessions,
  daemonConn,
  ensureAgentHubCodexMcpConfig,
  ensureGitRepoForTask,
  executeTaskOnce,
  ensureOpenCodeMcpConfig,
  onWebSocket,
} = require('./agenthub-daemon.js');

test('onWebSocket supports ws EventEmitter clients', () => {
  const ws = new EventEmitter();
  let called = false;

  onWebSocket(ws, 'open', () => {
    called = true;
  });
  ws.emit('open');

  assert.equal(called, true);
});

test('onWebSocket adapts WHATWG message events', () => {
  const listeners = new Map();
  const ws = {
    addEventListener(name, handler) {
      listeners.set(name, handler);
    },
  };
  let message = '';

  onWebSocket(ws, 'message', (data) => {
    message = data;
  });
  listeners.get('message')({ data: '{"type":"ping"}' });

  assert.equal(message, '{"type":"ping"}');
});

test('onWebSocket adapts WHATWG close events', () => {
  const listeners = new Map();
  const ws = {
    addEventListener(name, handler) {
      listeners.set(name, handler);
    },
  };
  let closeCode = 0;
  let closeReason = '';

  onWebSocket(ws, 'close', (code, reason) => {
    closeCode = code;
    closeReason = reason;
  });
  listeners.get('close')({ code: 1006, reason: 'network' });

  assert.equal(closeCode, 1006);
  assert.equal(closeReason, 'network');
});

test('commandForTask runs opencode with conversation session when available', () => {
  conversationSessions.clear();
  conversationSessions.set('agent-1:conv-1', 'session-1');

  const spec = commandForTask({
    id: 'task-1',
    cli_tool: 'opencode',
    agent_id: 'agent-1',
    conversation_id: 'conv-1',
    prompt: 'hello',
    tools_config: '{"allowed_tools":["create_agent"]}',
    context_messages: '[系统指令]\nBe concise.',
  });

  assert.match(path.basename(spec.command).toLowerCase(), /^opencode(?:\.exe)?$/);
  assert.equal(spec.resultFormat, 'opencode-json');
  assert.equal(spec.persistSessionKey, 'agent-1:conv-1');
  assert.deepEqual(spec.env, {
    AGENTHUB_CONVERSATION_ID: 'conv-1',
    AGENTHUB_AGENT_ID: 'agent-1',
    AGENTHUB_TASK_ID: 'task-1',
  });
  assert.deepEqual(spec.args.slice(0, 7), [
    'run',
    '--format',
    'json',
    '--no-replay',
    '--dangerously-skip-permissions',
    '--session',
    'session-1',
  ]);
  assert.equal(spec.args.includes('--fork'), true);
  assert.match(spec.args.at(-1), /Be concise/);
  assert.match(spec.args.at(-1), /hello/);
});

test('commandForTask starts opencode without session on first conversation turn', () => {
  conversationSessions.clear();

  const spec = commandForTask({
    id: 'task-1',
    cli_tool: 'opencode',
    agent_id: 'agent-1',
    conversation_id: 'conv-1',
    prompt: 'hello',
  });

  assert.equal(spec.persistSessionKey, 'agent-1:conv-1');
  assert.equal(spec.args.includes('--session'), false);
});

test('commandForTask runs codex with non-interactive MCP-capable execution', () => {
  const tempCodexHome = fs.mkdtempSync(path.join(os.tmpdir(), 'agenthub-codex-home-'));
  const originalCodexHome = process.env.AGENTHUB_CODEX_HOME;
  process.env.AGENTHUB_CODEX_HOME = tempCodexHome;
  try {
    const spec = commandForTask({
      id: 'codex-task-1',
      cli_tool: 'codex',
      agent_id: 'agent-1',
      conversation_id: 'conv-1',
      user_id: 'user-1',
      prompt: 'create an agent',
    });

    assert.equal(spec.args[0], 'exec');
    assert.equal(spec.args.includes('--dangerously-bypass-approvals-and-sandbox'), true);
    assert.equal(spec.args.includes('--ephemeral'), true);
    assert.equal(spec.args.includes('--sandbox'), false);
    assert.equal(spec.args.includes('read-only'), false);
    assert.deepEqual(spec.env, {
      CODEX_HOME: tempCodexHome,
      AGENTHUB_CONVERSATION_ID: 'conv-1',
      AGENTHUB_USER_ID: 'user-1',
      AGENTHUB_AGENT_ID: 'agent-1',
      AGENTHUB_TASK_ID: 'codex-task-1',
    });
  } finally {
    if (originalCodexHome === undefined) {
      delete process.env.AGENTHUB_CODEX_HOME;
    } else {
      process.env.AGENTHUB_CODEX_HOME = originalCodexHome;
    }
    fs.rmSync(tempCodexHome, { recursive: true, force: true });
  }
});

test('ensureAgentHubCodexMcpConfig writes task context and auto-approved platform tools', () => {
  const tempCodexHome = fs.mkdtempSync(path.join(os.tmpdir(), 'agenthub-codex-home-'));
  const original = {
    serverURL: daemonConn.serverURL,
    apiKey: daemonConn.apiKey,
    daemonToken: daemonConn.daemonToken,
  };
  daemonConn.serverURL = 'http://agenthub.test';
  daemonConn.apiKey = 'api-key';
  daemonConn.daemonToken = 'daemon-token';
  try {
    const configFile = ensureAgentHubCodexMcpConfig(
      tempCodexHome,
      'conv-1',
      'user-1',
      'agent-1',
      'task-1',
    );
    const config = fs.readFileSync(configFile, 'utf8');
    assert.match(config, /\[mcp_servers\.agenthub-platform\]/);
    assert.match(config, /--conversation-id", "conv-1"/);
    assert.match(config, /--user-id", "user-1"/);
    assert.match(config, /--agent-id", "agent-1"/);
    assert.match(config, /--task-id", "task-1"/);
    assert.match(config, /default_tools_approval_mode = "approve"/);
  } finally {
    daemonConn.serverURL = original.serverURL;
    daemonConn.apiKey = original.apiKey;
    daemonConn.daemonToken = original.daemonToken;
    fs.rmSync(tempCodexHome, { recursive: true, force: true });
  }
});

test('parseOpenCodeOutput tests removed — superseded by cli/__tests__/opencode.test.js (Switch 7)', () => {
  // parseOpenCodeOutput was deleted from daemon.js; its logic lives in
  // OpenCodeCliSpec.parseResult, fully covered by cli/__tests__/opencode.test.js
  // (text extraction, sessionId extraction, part.updated handling, multi-line stream).
  assert.ok(true);
});

test('ensureOpenCodeMcpConfig preserves existing config and writes AgentHub server', () => {
  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'agenthub-opencode-config-'));
  const configPath = path.join(tempDir, 'opencode.json');
  const originalConfigPath = process.env.AGENTHUB_OPENCODE_CONFIG;
  process.env.AGENTHUB_OPENCODE_CONFIG = configPath;
  try {
    fs.writeFileSync(configPath, JSON.stringify({
      $schema: 'https://opencode.ai/config.json',
      model: 'provider/model',
      provider: { example: { options: { apiKey: 'secret' } } },
    }, null, 2));

    const writtenPath = ensureOpenCodeMcpConfig(['node', 'daemon.js', '--mcp']);
    const config = JSON.parse(fs.readFileSync(configPath, 'utf8'));

    assert.equal(writtenPath, configPath);
    assert.equal(config.model, 'provider/model');
    assert.equal(config.provider.example.options.apiKey, 'secret');
    assert.deepEqual(config.mcp['agenthub-platform'], {
      type: 'local',
      command: ['node', 'daemon.js', '--mcp'],
      enabled: true,
    });
  } finally {
    if (originalConfigPath === undefined) {
      delete process.env.AGENTHUB_OPENCODE_CONFIG;
    } else {
      process.env.AGENTHUB_OPENCODE_CONFIG = originalConfigPath;
    }
    fs.rmSync(tempDir, { recursive: true, force: true });
  }
});

test('executeTaskOnce ignores duplicate completed task ids', async () => {
  const task = {
    id: 'duplicate-task-1',
    cli_tool: 'echo',
    prompt: 'hello',
  };

  assert.equal(typeof await executeTaskOnce(task), 'string');
  assert.equal(await executeTaskOnce(task), null);
});

test('ensureGitRepoForTask auto-inits non-git workdir with baseline commit', () => {
  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'agenthub-git-init-'));
  try {
    // 写一个文件，模拟 agent 修改前的 workdir 内容
    fs.writeFileSync(path.join(tempDir, 'hello.txt'), 'hello\n');
    // 确认还不是 git 仓库
    assert.throws(() => execFileSync('git', ['-C', tempDir, 'rev-parse', '--git-dir'], { encoding: 'utf8' }));

    ensureGitRepoForTask(tempDir, { task_id: 't-init', cli_tool: 'claude' });

    // 现在应该是 git 仓库，且有 baseline commit
    const gitDir = execFileSync('git', ['-C', tempDir, 'rev-parse', '--git-dir'], { encoding: 'utf8' }).trim();
    assert.ok(gitDir, 'git rev-parse --git-dir should succeed after init');
    const log = execFileSync('git', ['-C', tempDir, 'log', '--oneline'], { encoding: 'utf8' }).trim();
    assert.match(log, /baseline \(auto\)/);
  } finally {
    fs.rmSync(tempDir, { recursive: true, force: true });
  }
});

test('ensureGitRepoForTask is no-op on existing git repo', () => {
  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'agenthub-git-skip-'));
  try {
    execFileSync('git', ['-C', tempDir, 'init'], { encoding: 'utf8' });
    execFileSync('git', ['-C', tempDir, 'config', 'user.email', 'test@example.com'], { encoding: 'utf8' });
    execFileSync('git', ['-C', tempDir, 'config', 'user.name', 'Test'], { encoding: 'utf8' });
    fs.writeFileSync(path.join(tempDir, 'a.txt'), 'a\n');
    execFileSync('git', ['-C', tempDir, 'add', '-A'], { encoding: 'utf8' });
    execFileSync('git', ['-C', tempDir, 'commit', '-m', 'manual'], { encoding: 'utf8' });

    ensureGitRepoForTask(tempDir, { task_id: 't-skip' });

    const log = execFileSync('git', ['-C', tempDir, 'log', '--oneline'], { encoding: 'utf8' }).trim();
    // 不应有 baseline (auto) 提交
    assert.doesNotMatch(log, /baseline \(auto\)/);
    assert.match(log, /manual/);
  } finally {
    fs.rmSync(tempDir, { recursive: true, force: true });
  }
});
