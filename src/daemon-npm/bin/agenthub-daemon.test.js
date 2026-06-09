const assert = require('node:assert/strict');
const test = require('node:test');
const { EventEmitter } = require('node:events');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const {
  commandForTask,
  conversationSessions,
  executeTaskOnce,
  ensureOpenCodeMcpConfig,
  onWebSocket,
  parseOpenCodeOutput,
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
    CODEX_HOME: path.join(os.homedir(), '.agenthub', 'codex'),
    AGENTHUB_CONVERSATION_ID: 'conv-1',
    AGENTHUB_USER_ID: 'user-1',
    AGENTHUB_AGENT_ID: 'agent-1',
  });
});

test('parseOpenCodeOutput extracts assistant text and session id from message events', () => {
  const stdout = [
    JSON.stringify({ type: 'session.created', sessionID: 'session-1' }),
    JSON.stringify({
      type: 'message',
      message: {
        role: 'assistant',
        content: [{ type: 'text', text: 'hello' }, { type: 'text', text: ' world' }],
      },
    }),
  ].join('\n');

  assert.deepEqual(parseOpenCodeOutput(stdout), {
    text: 'hello world',
    sessionId: 'session-1',
  });
});

test('parseOpenCodeOutput prefers latest updated text parts', () => {
  const stdout = [
    JSON.stringify({ type: 'part.updated', session: { id: 'session-2' }, part: { id: 'p1', type: 'text', text: 'hel' } }),
    JSON.stringify({ type: 'part.updated', part: { id: 'p1', type: 'text', text: 'hello' } }),
  ].join('\n');

  assert.deepEqual(parseOpenCodeOutput(stdout), {
    text: 'hello',
    sessionId: 'session-2',
  });
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
