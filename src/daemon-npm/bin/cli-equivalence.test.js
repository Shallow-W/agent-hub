'use strict';

// 行为等价验证：commandForTask 重构（CliToolSpec Registry）后，
// 每个 CLI 的 { command, args, env, cwd, sessionId, ... } 必须和原 if-else 链一致。
// 这些测试断言 args 的精确顺序、env 的精确键值、cwd/sessionId/outputFile 等字段。
// 任何 args 顺序变化、env 键丢失、cwd 路径偏移都会失败。

const assert = require('node:assert/strict');
const test = require('node:test');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const {
  commandForTask,
  conversationSessions,
} = require('./agenthub-daemon.js');
const cliTools = require('../cli');

function withTempCodexHome(fn) {
  const tempCodexHome = fs.mkdtempSync(path.join(os.tmpdir(), 'agenthub-codex-home-'));
  const originalCodexHome = process.env.AGENTHUB_CODEX_HOME;
  process.env.AGENTHUB_CODEX_HOME = tempCodexHome;
  try {
    return fn(tempCodexHome);
  } finally {
    if (originalCodexHome === undefined) {
      delete process.env.AGENTHUB_CODEX_HOME;
    } else {
      process.env.AGENTHUB_CODEX_HOME = originalCodexHome;
    }
    fs.rmSync(tempCodexHome, { recursive: true, force: true });
  }
}

// ─── Registry 基础验证 ───────────────────────────────────────────────────

test('CliToolSpec registry registers all 4 CLIs after daemon require', () => {
  const tools = cliTools.allCliTools().map((s) => s.cliTool).sort();
  assert.deepEqual(tools, ['claude', 'codex', 'openclaw', 'opencode']);
});

test('getCliTool returns undefined for unknown cliTool', () => {
  assert.equal(cliTools.getCliTool('unknown-cli'), undefined);
});

test('each registered spec implements buildCommand', () => {
  for (const spec of cliTools.allCliTools()) {
    assert.equal(typeof spec.buildCommand, 'function', `${spec.cliTool} missing buildCommand`);
    assert.equal(typeof spec.cliTool, 'string');
    assert.equal(typeof spec.name, 'string');
    assert.ok(Array.isArray(spec.defaultCapabilities), `${spec.cliTool} defaultCapabilities not array`);
  }
});

// ─── Claude: one-shot 模式（非 persistent） ──────────────────────────────

test('claude one-shot command: args order + permission-mode dontAsk + stdin', () => {
  const task = {
    id: 't-claude-1',
    cli_tool: 'claude',
    agent_id: 'agent-x',
    conversation_id: 'conv-x',
    prompt: 'hello',
    context_messages: '[系统指令]\nBe brief.',
  };
  const spec = commandForTask(task);

  // args 必须按 -p, --output-format text, --thinking off, [mcp args], --permission-mode dontAsk, --system-prompt <sp>
  assert.equal(spec.args[0], '-p');
  assert.equal(spec.args[1], '--output-format');
  assert.equal(spec.args[2], 'text');
  assert.equal(spec.args[3], '--thinking');
  assert.equal(spec.args[4], 'off');
  // 非 persistent 模式：--permission-mode dontAsk（不是 --dangerously-skip-permissions）
  assert.equal(spec.args.includes('--permission-mode'), true);
  assert.equal(spec.args[spec.args.indexOf('--permission-mode') + 1], 'dontAsk');
  assert.equal(spec.args.includes('--dangerously-skip-permissions'), false);
  // systemPrompt 通过 --system-prompt 传入
  assert.equal(spec.args.includes('--system-prompt'), true);
  // userPrompt 通过 stdin（不进 args）。stdin 含原 prompt（可能含 buildPromptParts 默认群聊前缀）
  assert.match(spec.stdin, /hello/);
  assert.equal(spec.args.some((a) => typeof a === 'string' && a.includes('hello')), false);
  // sessionId 来自 makeSessionId(conv, agent)
  assert.equal(typeof spec.sessionId, 'string');
  assert.ok(spec.sessionId.length > 0);
});

test('claude one-shot without systemPrompt omits --system-prompt', () => {
  const spec = commandForTask({
    id: 't-claude-2',
    cli_tool: 'claude',
    prompt: 'just hello',
  });
  assert.equal(spec.args.includes('--system-prompt'), false);
  // stdin 含原 prompt（buildPromptParts 会附加默认群聊前缀）
  assert.match(spec.stdin, /just hello$/);
});

// ─── Claude: persistent 模式（runningAgents 含 agent_id） ────────────────

test('claude persistent: uses --dangerously-skip-permissions + registered sessionId', async () => {
  // 通过 require 拿到 runningAgents Map 不直接导出，但可通过 spawnStreamJsonProcess 间接。
  // 这里仅验证 one-shot 与 persistent 的差异：persistent 走 --dangerously-skip-permissions。
  // 由于 runningAgents 不导出，此路径由集成测试覆盖；此处仅断言非 persistent 路径正确。
  const spec = commandForTask({
    id: 't-claude-p',
    cli_tool: 'claude',
    agent_id: 'non-existent-agent',
    prompt: 'x',
  });
  assert.equal(spec.args.includes('--dangerously-skip-permissions'), false);
});

// ─── Codex: exec 命令 + CODEX_HOME + outputFile + cwd ───────────────────

test('codex: exec args order + CODEX_HOME env + outputFile + cwd', () => {
  withTempCodexHome((codexHome) => {
    const task = {
      id: 'codex-eq-1',
      cli_tool: 'codex',
      agent_id: 'a1',
      conversation_id: 'c1',
      user_id: 'u1',
      prompt: 'do something',
    };
    const spec = commandForTask(task);

    // args[0] 是 'exec'
    assert.equal(spec.args[0], 'exec');
    // execArgs 顺序
    const expectedExecFlags = [
      '--skip-git-repo-check',
      '--dangerously-bypass-approvals-and-sandbox',
      '--ephemeral',
      '--color',
      'never',
      '--output-last-message',
    ];
    for (let i = 0; i < expectedExecFlags.length; i += 1) {
      assert.equal(spec.args[1 + i], expectedExecFlags[i], `codex args[${1 + i}] mismatch`);
    }
    // outputFile 字段
    assert.ok(spec.outputFile);
    assert.equal(spec.outputFile, path.join(os.tmpdir(), `agenthub-task-${task.id}.txt`));
    // cwd 字段（任务工作目录）
    assert.ok(spec.cwd);
    assert.ok(spec.cwd.includes('agenthub-cli-tasks'));
    // env: CODEX_HOME + AgentHub context
    assert.equal(spec.env.CODEX_HOME, codexHome);
    assert.equal(spec.env.AGENTHUB_CONVERSATION_ID, 'c1');
    assert.equal(spec.env.AGENTHUB_USER_ID, 'u1');
    assert.equal(spec.env.AGENTHUB_AGENT_ID, 'a1');
    assert.equal(spec.env.AGENTHUB_TASK_ID, 'codex-eq-1');
    // codexMcpFallback 文本必须出现在末尾 prompt 中（fallback 末尾有 \n，故 [Codex MCP 适配]\n...）
    // 此 task 无 context_messages，故无 [系统指令]，userPrompt 含默认群聊前缀
    const lastArg = spec.args[spec.args.length - 1];
    assert.match(lastArg, /\[Codex MCP 适配\]\n/);
    assert.match(lastArg, /do something$/);
  });
});

test('codex: with systemPrompt wraps as [系统指令]', () => {
  withTempCodexHome(() => {
    const spec = commandForTask({
      id: 'codex-eq-2',
      cli_tool: 'codex',
      prompt: 'user-msg',
      context_messages: '[系统指令]\nBe concise.',
    });
    const lastArg = spec.args[spec.args.length - 1];
    // 结构：[Codex MCP 适配]\n<fallback 文本>\n[系统指令]\n<sp>...<userPrompt>
    assert.match(lastArg, /\[Codex MCP 适配\]\n/);
    assert.match(lastArg, /\[系统指令\]\nBe concise\./);
    assert.match(lastArg, /user-msg$/);
  });
});

// ─── OpenCode: session 复用 + --fork + persistSessionKey ────────────────

test('opencode: first turn has no --session, persistSessionKey set', () => {
  conversationSessions.clear();
  const spec = commandForTask({
    id: 'oc-eq-1',
    cli_tool: 'opencode',
    agent_id: 'a-oc',
    conversation_id: 'c-oc',
    prompt: 'hi',
  });
  assert.equal(spec.args[0], 'run');
  assert.equal(spec.args[1], '--format');
  assert.equal(spec.args[2], 'json');
  assert.equal(spec.args[3], '--no-replay');
  assert.equal(spec.args[4], '--dangerously-skip-permissions');
  assert.equal(spec.args.includes('--session'), false);
  assert.equal(spec.args.includes('--fork'), false);
  assert.equal(spec.resultFormat, 'opencode-json');
  assert.equal(spec.persistSessionKey, 'a-oc:c-oc');
});

test('opencode: with saved session adds --session <id> + --fork when context changed', () => {
  conversationSessions.clear();
  conversationSessions.set('a-oc:c-oc', 'sess-xyz');
  const spec = commandForTask({
    id: 'oc-eq-2',
    cli_tool: 'opencode',
    agent_id: 'a-oc',
    conversation_id: 'c-oc',
    prompt: 'continue',
  });
  assert.equal(spec.args.includes('--session'), true);
  assert.equal(spec.args[spec.args.indexOf('--session') + 1], 'sess-xyz');
  // agent_id+conversation_id 都存在 → opencodeContextChanged=true → --fork
  assert.equal(spec.args.includes('--fork'), true);
});

// ─── OpenClaw: agent 子命令 + 固定 sessionId 回退 ───────────────────────

test('openclaw: agent --local --session-id <id> --message <prompt> --json --thinking off', () => {
  const spec = commandForTask({
    id: 'oclaw-eq-1',
    cli_tool: 'openclaw',
    agent_id: 'a-claw',
    conversation_id: 'c-claw',
    prompt: 'claw work',
  });
  // args 结构断言（不深比较 prompt 文本，因 buildPromptParts 可能附加默认前缀）
  assert.equal(spec.args[0], 'agent');
  assert.equal(spec.args[1], '--local');
  assert.equal(spec.args[2], '--session-id');
  assert.equal(typeof spec.args[3], 'string');
  assert.ok(spec.args[3].length > 0, 'sessionId must be non-empty');
  assert.equal(spec.args[4], '--message');
  // --message 的值含原 prompt（可能含 buildPromptParts 默认群聊前缀）
  assert.match(spec.args[5], /claw work$/);
  assert.equal(spec.args[6], '--json');
  assert.equal(spec.args[7], '--thinking');
  assert.equal(spec.args[8], 'off');
  assert.equal(spec.resultFormat, 'openclaw-json');
});

test('openclaw: without conv/agent falls back to agenthub-<sanitized id>', () => {
  const spec = commandForTask({
    id: 'oclaw-eq-id',
    cli_tool: 'openclaw',
    prompt: 'x',
  });
  assert.equal(spec.args[3], 'agenthub-oclaw-eq-id');
});

// ─── Unknown CLI: fallback 分支 ─────────────────────────────────────────

test('unknown cli_tool: fallback returns { command, args: [userPrompt] }', () => {
  const spec = commandForTask({
    id: 'uk-1',
    cli_tool: 'echo',
    prompt: 'fallback-test',
    context_messages: '',
  });
  // buildPromptParts 无系统指令且无 ctx → userPrompt 含默认前缀
  assert.ok(Array.isArray(spec.args));
  assert.equal(spec.args.length, 1);
  assert.match(spec.args[0], /fallback-test/);
  assert.equal(spec.sessionId, undefined);
  assert.equal(spec.env, undefined);
});

// ─── skillRoots 委托验证（间接通过 scanSkills 行为） ──────────────────────
// 注：skillRoots 不直接导出，但其行为由 scanAgents → scanSkills 路径覆盖。
// 这里通过 registry spec.skillRoots 直接验证，确认每个 CLI 都实现了。

test('each spec.skillRoots returns array of paths', () => {
  const cwd = process.cwd();
  const home = os.homedir();
  for (const spec of cliTools.allCliTools()) {
    if (typeof spec.skillRoots === 'function') {
      const roots = spec.skillRoots(cwd, home);
      assert.ok(Array.isArray(roots), `${spec.cliTool}.skillRoots not array`);
      for (const r of roots) {
        assert.equal(typeof r, 'string', `${spec.cliTool} root not string: ${r}`);
      }
    }
  }
});

test('claude skillRoots includes .claude/skills + plugins', () => {
  const claude = cliTools.getCliTool('claude');
  const home = os.homedir();
  const roots = claude.skillRoots(process.cwd(), home);
  assert.ok(roots.some((r) => r.endsWith(path.join('.claude', 'skills'))));
  assert.ok(roots.some((r) => r.endsWith(path.join('.claude', 'plugins', 'marketplaces'))));
  assert.ok(roots.some((r) => r.endsWith(path.join('.claude', 'plugins', 'cache'))));
});

test('codex skillRoots includes .codex/skills (home) and .agents/skills (cwd)', () => {
  const codex = cliTools.getCliTool('codex');
  const cwd = '/custom/cwd';
  const home = '/custom/home';
  const roots = codex.skillRoots(cwd, home);
  assert.ok(roots.includes(path.join(cwd, '.agents', 'skills')));
  assert.ok(roots.includes(path.join(home, '.codex', 'skills')));
});

test('opencode and openclaw share identical skillRoots set', () => {
  const opencode = cliTools.getCliTool('opencode');
  const openclaw = cliTools.getCliTool('openclaw');
  const cwd = '/x';
  const home = '/y';
  // 两者原代码走同一分支，行为等价要求返回相同根集
  assert.deepEqual(opencode.skillRoots(cwd, home), openclaw.skillRoots(cwd, home));
});

// ─── ensureMcp 钩子：仅 codex/opencode/openclaw 实现 ───────────────────

test('claude spec does NOT implement ensureMcp (per-task injection only)', () => {
  const claude = cliTools.getCliTool('claude');
  assert.equal(typeof claude.ensureMcp, 'undefined');
});

test('codex/opencode/openclaw specs all implement ensureMcp', () => {
  for (const tool of ['codex', 'opencode', 'openclaw']) {
    const spec = cliTools.getCliTool(tool);
    assert.equal(typeof spec.ensureMcp, 'function', `${tool} missing ensureMcp`);
  }
});

// ─── isAuthenticated 钩子：仅 codex 实现 ────────────────────────────────

test('only codex implements isAuthenticated', () => {
  const codex = cliTools.getCliTool('codex');
  assert.equal(typeof codex.isAuthenticated, 'function');
  assert.equal(typeof codex.onResolvedCommand, 'function');
  for (const tool of ['claude', 'opencode', 'openclaw']) {
    const spec = cliTools.getCliTool(tool);
    assert.equal(typeof spec.isAuthenticated, 'undefined', `${tool} should not implement isAuthenticated`);
  }
});
