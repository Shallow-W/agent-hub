#!/usr/bin/env node

const { execFileSync, spawn, spawnSync } = require('node:child_process');
const { EventEmitter } = require('node:events');
const crypto = require('node:crypto');
const fs = require('node:fs');
const http = require('node:http');
const https = require('node:https');
const os = require('node:os');
const path = require('node:path');
// CliToolSpec 注册表：把原本散落在 commandForTask / ensureGlobalMcpConfigs /
// skillRoots / scanAgents 四处分支点的 CLI 行为（claude/codex/opencode/openclaw）
// 收敛为 spec 对象。新增 CLI 只需在 cli/index.js 加一个工厂，零修改分发函数。
const cliTools = require('../cli');
const EXEC_TIMEOUT_MS = 400000;
const HEARTBEAT_INTERVAL_MS = 30000;
const WS_RECONNECT_DELAY_MS = 3000;
const WS_PING_INTERVAL_MS = 30000;
const INBOUND_WATCHDOG_MS = 70000;
const POLL_INTERVAL_MS = 3000;
const DAEMON_LOG_EVENT = 'daemon_flow';

function logValue(value) {
  if (value === undefined || value === null) return '';
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  if (Array.isArray(value) || typeof value === 'object') return JSON.stringify(value);
  const text = String(value);
  return /^[A-Za-z0-9._:/@=-]+$/.test(text) ? text : JSON.stringify(text);
}

function logFlow(level, stage, fields = {}) {
  const pairs = Object.entries(fields)
    .filter(([, value]) => value !== undefined && value !== null && value !== '')
    .map(([key, value]) => `${key}=${logValue(value)}`);
  const line = [
    new Date().toISOString(),
    DAEMON_LOG_EVENT,
    `level=${level}`,
    `stage=${stage}`,
    ...pairs,
  ].join(' ');
  if (level === 'error' || level === 'warn' || process.argv.includes('--mcp')) {
    console.error(line);
    return;
  }
  console.log(line);
}

function errorMessage(error) {
  return error instanceof Error ? error.message : String(error);
}

function firstLine(value) {
  return String(value || '').split(/\r?\n/)[0].trim();
}

let WebSocket;
try {
  WebSocket = require('ws');
} catch {
  // Node 22+ has global WebSocket
  if (typeof globalThis.WebSocket !== 'undefined') {
    WebSocket = globalThis.WebSocket;
  }
}

function safeSend(ws, data) {
  try {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(data);
      return true;
    }
  } catch { /* connection already closed */ }
  return false;
}

function sendTaskComplete(data) {
  const taskId = data && data.task_id;
  if (!taskId) return;
  const envelope = JSON.stringify({ type: 'task.complete', data });
  if (safeSend(currentDaemonWs, envelope)) {
    pendingTaskCompletions.delete(taskId);
    logFlow('info', 'task.complete_sent', {
      task_id: taskId,
      has_error: Boolean(data.error),
      result_len: typeof data.result === 'string' ? data.result.length : 0,
      artifact_count: Array.isArray(data.artifacts) ? data.artifacts.length : 0,
      pending_count: pendingTaskCompletions.size,
    });
    return;
  }
  pendingTaskCompletions.set(taskId, data);
  logFlow('warn', 'task.complete_buffered', {
    task_id: taskId,
    has_error: Boolean(data.error),
    pending_count: pendingTaskCompletions.size,
  });
}

function flushPendingTaskCompletions() {
  if (pendingTaskCompletions.size > 0) {
    logFlow('info', 'task.complete_flush_start', { pending_count: pendingTaskCompletions.size });
  }
  for (const data of pendingTaskCompletions.values()) {
    sendTaskComplete(data);
  }
}

// ---------------------------------------------------------------------------
// WS 消息处理器注册表
// 替代旧的 if-ladder 分发。新增消息类型只需 registerWsHandler('type', fn)。
// ---------------------------------------------------------------------------

const wsHandlers = new Map();

function registerWsHandler(type, handler) {
  wsHandlers.set(type, handler);
}

function dispatchWsMessage(ws, envelope) {
  const handler = wsHandlers.get(envelope.type);
  if (handler) {
    return handler(ws, envelope.data || {});
  }
  return false;
}

// ---------------------------------------------------------------------------
// 事件总线
// 解耦 WS 消息接收 → 业务执行 → 结果回传的链条。
// 每一步通过 bus.emit / bus.on 通信，新增横切逻辑（metrics/审计/重试）
// 只需加监听器，不改核心流程。
//
// 事件清单：
//   ws.connected         — WS 连接成功
//   ws.disconnected      — WS 断开
//   ws.message           — 收到任意 WS 消息（raw envelope）
//   agent.start_request  — 后端要求启动 agent
//   agent.stop_request   — 后端要求停止 agent
//   agent.restart_request— 后端要求重启 agent
//   agent.started        — agent 进程已启动
//   agent.stopped        — agent 进程已退出
//   task.dispatch        — 收到任务派发
//   task.completed       — 任务执行成功
//   task.failed          — 任务执行失败
//   daemon.ready         — daemon 完成初始化
// ---------------------------------------------------------------------------

const bus = new EventEmitter();
bus.setMaxListeners(50); // 横切监听器可能较多

// 结构化日志：所有 daemon 事件自动写入 logFlow
bus.on('agent.started', (info) =>
  logFlow('info', 'agent.started', info),
);
bus.on('agent.stopped', (info) =>
  logFlow('info', 'agent.stopped', info),
);
bus.on('task.completed', (info) =>
  logFlow('info', 'task.completed', info),
);
bus.on('task.failed', (info) =>
  logFlow('error', 'task.failed', info),
);

function onWebSocket(ws, eventName, handler) {
  if (typeof ws.on === 'function') {
    ws.on(eventName, handler);
    return;
  }
  if (typeof ws.addEventListener !== 'function') {
    throw new Error('unsupported WebSocket implementation');
  }
  ws.addEventListener(eventName, (event) => {
    if (eventName === 'message') {
      handler(event.data);
      return;
    }
    if (eventName === 'close') {
      handler(event.code, event.reason);
      return;
    }
    if (eventName === 'error') {
      handler(event.error || event);
      return;
    }
    handler(event);
  });
}

const activeSessions = new Map();
const runningAgents = new Map(); // agentID → { process, sessionId, cliTool, sendPrompt, _queue }
const idleAgentConfigs = new Map(); // agentID → { cliTool, sessionId, systemPrompt }
const agentTurnStates = new Map(); // agentID → 'idle' | 'active'
let currentDaemonWs = null;
const activeTaskIDs = new Set();
const completedTaskIDs = new Set();
const pendingTaskCompletions = new Map(); // taskID → task.complete data, flushed after WS reconnect

// Per-conversation session mapping: `${agent_id}:${conversation_id}` → sessionId
const conversationSessions = new Map();
const SESSIONS_FILE = path.join(os.homedir(), '.agenthub', 'sessions.json');

function loadSessionMap() {
  try {
    const data = fs.readFileSync(SESSIONS_FILE, 'utf8');
    for (const [key, value] of Object.entries(JSON.parse(data))) {
      conversationSessions.set(key, value);
    }
  } catch { /* file not found or invalid — start fresh */ }
}

function saveSessionMap() {
  try {
    const obj = Object.fromEntries(conversationSessions);
    fs.mkdirSync(path.dirname(SESSIONS_FILE), { recursive: true });
    fs.writeFileSync(SESSIONS_FILE, JSON.stringify(obj, null, 2));
  } catch (err) {
    logFlow('warn', 'session_map.save_failed', { file: SESSIONS_FILE, error: errorMessage(err) });
  }
}

const START_QUEUE_INTERVAL_MS = 3000;
let lastAgentStartAt = 0;
const agentStartQueue = [];

// 轮询模式下的后端连接信息，供派发任务时给 Claude Code 注入平台 MCP server。
const daemonConn = { serverURL: '', apiKey: '', daemonToken: '' };

// buildPlatformMcpServerArgs builds the daemon --mcp invocation for the current
// AgentHub task. Passing conversation/user/agent IDs here gives MCP tools a default
// group context, matching Claude Code's per-task injection behavior.
function buildPlatformMcpServerArgs(conversationId, userId, agentId) {
  if (!daemonConn.serverURL || !daemonConn.apiKey) return [];
  const mcpServerArgs = [__filename, '--server-url', daemonConn.serverURL, '--api-key', daemonConn.apiKey, '--mcp'];
  if (daemonConn.daemonToken) mcpServerArgs.push('--daemon-token', daemonConn.daemonToken);
  if (conversationId) mcpServerArgs.push('--conversation-id', conversationId);
  if (userId) mcpServerArgs.push('--user-id', userId);
  if (agentId) mcpServerArgs.push('--agent-id', agentId);
  return mcpServerArgs;
}

// buildPlatformMcpArgs generates Claude Code MCP injection args for this task.
function buildPlatformMcpArgs(conversationId, userId, agentId) {
  const mcpServerArgs = buildPlatformMcpServerArgs(conversationId, userId, agentId);
  if (mcpServerArgs.length === 0) return [];
  const mcpConfig = JSON.stringify({
    mcpServers: {
      'agenthub-platform': {
        command: 'node',
        args: mcpServerArgs,
      },
    },
  });
  return ['--mcp-config', mcpConfig, '--allowedTools', 'mcp__agenthub-platform'];
}

function buildAgentHubContextEnv(conversationId, userId, agentId) {
  const env = {};
  if (conversationId) env.AGENTHUB_CONVERSATION_ID = conversationId;
  if (userId) env.AGENTHUB_USER_ID = userId;
  if (agentId) env.AGENTHUB_AGENT_ID = agentId;
  return Object.keys(env).length > 0 ? env : undefined;
}

function opencodeContextChanged(task, savedSessionId) {
  if (!savedSessionId) return false;
  return Boolean(task && task.agent_id && task.conversation_id);
}

// ensureGlobalMcpConfigs 为不支持按次注入的 CLI（openclaw/codex/opencode）在启动时
// 幂等写入全局 MCP 配置，把本 daemon 以 --mcp 模式注册为 agenthub-platform server。
// 仅对本机实际安装的 CLI 生效，失败仅告警、不影响 daemon 连接。
// 改造后：遍历所有已注册 CliToolSpec，调用其可选 ensureMcp(mcpArgs) 钩子。
function ensureGlobalMcpConfigs(serverURL, apiKey) {
  const mcpArgs = [__filename, '--server-url', serverURL, '--api-key', apiKey, '--mcp'];
  if (daemonConn.daemonToken) mcpArgs.push('--daemon-token', daemonConn.daemonToken);
  for (const spec of cliTools.allCliTools()) {
    if (typeof spec.ensureMcp === 'function') spec.ensureMcp(mcpArgs);
  }
}

function openCodeConfigPath() {
  if (process.env.AGENTHUB_OPENCODE_CONFIG) return process.env.AGENTHUB_OPENCODE_CONFIG;
  return path.join(os.homedir(), '.config', 'opencode', 'opencode.json');
}

function ensureOpenCodeMcpConfig(command) {
  const configPath = openCodeConfigPath();
  let config = {};
  if (fs.existsSync(configPath)) {
    const raw = fs.readFileSync(configPath, 'utf8');
    config = raw.trim() ? JSON.parse(raw) : {};
    if (!config || typeof config !== 'object' || Array.isArray(config)) {
      throw new Error('OpenCode config root must be a JSON object');
    }
  }
  if (!config.$schema) config.$schema = 'https://opencode.ai/config.json';
  if (!config.mcp || typeof config.mcp !== 'object' || Array.isArray(config.mcp)) config.mcp = {};
  config.mcp['agenthub-platform'] = {
    type: 'local',
    command,
    enabled: true,
  };
  fs.mkdirSync(path.dirname(configPath), { recursive: true });
  fs.writeFileSync(configPath, `${JSON.stringify(config, null, 2)}\n`);
  return configPath;
}

function killSessionProcess(sessionId) {
  const child = activeSessions.get(sessionId);
  if (!child) return;
  try {
    if (process.platform === 'win32') {
      spawn('taskkill', ['/pid', String(child.pid), '/T', '/F'], { windowsHide: true });
    } else {
      process.kill(-child.pid, 'SIGKILL');
    }
    logFlow('info', 'process.kill_session', { session_id: sessionId, pid: child.pid });
  } catch { /* already dead */ }
  activeSessions.delete(sessionId);
}

const OPEN_PATH_TIMEOUT_MS = 5000;
const MIN_DESCRIPTION_CHARS = 6;
const OPEN_PATH_TOOL = '__agenthub_open_path__';

// CANDIDATES 现在由 CliToolSpec Registry 派生（见 initCliTools 调用点）。
// 每次调用都重新映射，确保注册表变化（测试替换 spec）即时反映。
function getCandidates() {
  return cliTools.allCliTools().map((spec) => ({
    name: spec.name,
    cli_tool: spec.cliTool,
    capabilities: spec.defaultCapabilities,
  }));
}

function defaultSkills(names) {
  return names.map((name) => ({
    name,
    description: defaultSkillDescription(name),
    auto: true,
  }));
}

function defaultSkillDescription(name) {
  const descriptions = {
    coding: 'Handle local coding tasks such as implementation, refactoring, and project edits.',
    review: 'Review code changes, identify risks, and suggest focused improvements.',
    orchestration: 'Coordinate multi-step work and route tasks across local Agent workflows.',
  };
  return descriptions[name] || `Provides the ${name} skill for local Agent workflows.`;
}

function npmWrapperScript(command) {
  if (process.platform !== 'win32') return null;
  const root = process.env.APPDATA;
  if (!root) return null;
  const scripts = {
    claude: path.join(root, 'npm', 'node_modules', '@anthropic-ai', 'claude-code', 'cli.js'),
    openclaw: path.join(root, 'npm', 'node_modules', 'openclaw', 'openclaw.mjs'),
  };
  const script = scripts[command];
  return script && fs.existsSync(script) ? script : null;
}

function processSpec(command, args) {
  const wrapperScript = npmWrapperScript(command);
  if (wrapperScript) {
    return { command: 'node', args: [wrapperScript, ...args] };
  }
  if (process.platform === 'win32' && !command.toLowerCase().endsWith('.exe')) {
    return { command: 'cmd.exe', args: ['/d', '/s', '/c', command, ...args] };
  }
  return { command, args };
}

function readArg(name) {
  const prefix = `${name}=`;
  for (let i = 2; i < process.argv.length; i += 1) {
    const current = process.argv[i];
    if (current === name) {
      return process.argv[i + 1] || '';
    }
    if (current.startsWith(prefix)) {
      return current.slice(prefix.length);
    }
  }
  return '';
}

function sleep(ms) {
  return new Promise((resolve) => {
    setTimeout(resolve, ms);
  });
}

function makeSessionId(conversationID, agentID) {
  const namespace = 'a1b2c3d4-e5f6-7890-abcd-ef1234567890';
  const name = `conv:${conversationID}:agent:${agentID}`;
  const hash = crypto.createHash('sha1');
  hash.update(Buffer.from(namespace.replace(/-/g, ''), 'hex'));
  hash.update(name);
  const digest = hash.digest();
  digest[6] = (digest[6] & 0x0f) | 0x50;
  digest[8] = (digest[8] & 0x3f) | 0x80;
  const hex = digest.toString('hex', 0, 16);
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20, 32)}`;
}

function sessionKeyForTask(task) {
  return task && task.agent_id && task.conversation_id ? `${task.agent_id}:${task.conversation_id}` : '';
}

function commandOutput(command, args, timeout = 5000) {
  try {
    const spec = processSpec(command, args);
    return execFileSync(spec.command, spec.args, {
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'ignore'],
      timeout,
      windowsHide: true,
    }).trim();
  } catch {
    return null;
  }
}

function commandVersion(command) {
  return commandOutput(command, ['--version']);
}

function codexLoginStatus(command) {
  const spec = processSpec(command, ['login', 'status']);
  const result = spawnSync(spec.command, spec.args, {
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe'],
    timeout: 5000,
    windowsHide: true,
  });
  if (result.status !== 0) return null;
  return `${result.stdout || ''}${result.stderr || ''}`.trim();
}

function existingFile(value) {
  return value && fs.existsSync(value) ? value : null;
}

function codexLocalInstallPaths() {
  if (process.platform !== 'win32') return [];
  const root = process.env.LOCALAPPDATA;
  if (!root) return [];
  const binRoot = path.join(root, 'OpenAI', 'Codex', 'bin');
  try {
    return fs.readdirSync(binRoot, { withFileTypes: true })
      .filter((entry) => entry.isDirectory())
      .map((entry) => path.join(binRoot, entry.name, 'codex.exe'))
      .filter((candidate) => fs.existsSync(candidate))
      .sort((a, b) => {
        try {
          return fs.statSync(b).mtimeMs - fs.statSync(a).mtimeMs;
        } catch {
          return 0;
        }
      });
  } catch {
    return [];
  }
}

function codexExtensionPath() {
  if (process.platform !== 'win32') return null;
  const root = process.env.USERPROFILE;
  if (!root) return null;
  const extensionRoot = path.join(root, '.vscode', 'extensions');
  try {
    const matches = fs.readdirSync(extensionRoot)
      .filter((name) => name.startsWith('openai.chatgpt-'))
      .sort()
      .reverse();
    for (const match of matches) {
      const candidate = path.join(extensionRoot, match, 'bin', 'windows-x86_64', 'codex.exe');
      if (fs.existsSync(candidate)) return candidate;
    }
  } catch {
    return null;
  }
  return null;
}

function resolveCodexCommand() {
  const candidates = [
    existingFile(process.env.AGENTHUB_CODEX_COMMAND),
    ...codexLocalInstallPaths(),
    codexExtensionPath(),
    'codex',
  ].filter(Boolean);
  for (const candidate of candidates) {
    if (commandVersion(candidate) !== null) return candidate;
  }
  return 'codex';
}

function resolveOpenCodeCommand() {
  const candidates = [
    existingFile(process.env.AGENTHUB_OPENCODE_COMMAND),
    existingFile(process.platform === 'win32' && process.env.APPDATA
      ? path.join(process.env.APPDATA, 'npm', 'node_modules', 'opencode-ai', 'bin', 'opencode.exe')
      : null),
    'opencode',
  ].filter(Boolean);
  for (const candidate of candidates) {
    if (commandVersion(candidate) !== null) return candidate;
  }
  return 'opencode';
}

function resolveCommand(cliTool) {
  if (cliTool === 'codex') {
    return resolveCodexCommand();
  }
  if (cliTool === 'opencode') {
    return resolveOpenCodeCommand();
  }
  return cliTool;
}

function scanAgents() {
  return getCandidates()
    .map((candidate) => {
      const spec = cliTools.getCliTool(candidate.cli_tool);
      const command = resolveCommand(candidate.cli_tool);
      // 保留原 codex 调试日志（onResolvedCommand 钩子，仅 codex 实现）。
      if (spec && typeof spec.onResolvedCommand === 'function') {
        spec.onResolvedCommand(command);
      }
      const version = commandVersion(command);
      if (version === null) return null;
      // 登录态检测委托给 spec.isAuthenticated（仅 codex 实现 isCodexAuthenticated）。
      if (spec && typeof spec.isAuthenticated === 'function' && !spec.isAuthenticated(command)) return null;
      const skills = scanSkills(candidate.cli_tool);
      return {
        name: candidate.name,
        cli_tool: candidate.cli_tool,
        version,
        capabilities: skills.length > 0 ? skills : candidate.capabilities,
      };
    })
    .filter(Boolean);
}

function scanSkills(cliTool) {
  const skills = [];
  const seen = new Set();
  for (const root of skillRoots(cliTool)) {
    for (const skillPath of findSkillFiles(root)) {
      let content = '';
      try {
        content = fs.readFileSync(skillPath, 'utf8');
      } catch {
        continue;
      }
      const skill = parseSkillFile(path.basename(path.dirname(skillPath)), skillPath, content);
      const key = skill.name.toLowerCase();
      if (!key || seen.has(key)) continue;
      seen.add(key);
      skills.push(skill);
    }
  }
  return skills;
}

function findSkillFiles(root) {
  const results = [];
  function walk(current) {
    let entries = [];
    try {
      entries = fs.readdirSync(current, { withFileTypes: true });
    } catch {
      return;
    }
    for (const entry of entries) {
      const entryPath = path.join(current, entry.name);
      if (entry.isDirectory()) {
        if (entry.name === '.git') continue;
        walk(entryPath);
        continue;
      }
      if (entry.isFile() && entry.name === 'SKILL.md') {
        results.push(entryPath);
      }
    }
  }
  walk(root);
  return results;
}

function addRoot(roots, root) {
  if (root && !roots.includes(root)) roots.push(root);
}

function isAgentHubWorkspace(root) {
  const daemonPackage = path.join(root, 'src', 'daemon-npm', 'package.json');
  const frontendPackage = path.join(root, 'src', 'frontend', 'package.json');
  if (!fs.existsSync(daemonPackage) || !fs.existsSync(frontendPackage)) return false;
  try {
    const pkg = JSON.parse(fs.readFileSync(daemonPackage, 'utf8'));
    return pkg.name === '@hust-agenthub/daemon';
  } catch {
    return false;
  }
}

function agentHubWorkspaceForPath(targetPath) {
  let current = path.dirname(path.resolve(targetPath));
  while (current && current !== path.dirname(current)) {
    if (isAgentHubWorkspace(current)) return current;
    current = path.dirname(current);
  }
  return null;
}

function openClawInstallSkillRoots(home) {
  const installsPath = path.join(home, '.openclaw', 'plugins', 'installs.json');
  let installs = null;
  try {
    installs = JSON.parse(fs.readFileSync(installsPath, 'utf8'));
  } catch {
    return [];
  }

  const roots = [];
  const records = installs && typeof installs.installRecords === 'object'
    ? Object.values(installs.installRecords)
    : [];
  for (const record of records) {
    if (!record || typeof record !== 'object') continue;
    if (record.installPath) addRoot(roots, path.join(String(record.installPath), 'skills'));
    if (record.sourcePath) addRoot(roots, path.join(String(record.sourcePath), 'skills'));
  }
  return roots;
}

function skillRoots(cliTool) {
  // 委托给 CliToolSpec.skillRoots（claude/codex/opencode/openclaw 各自实现）。
  // 未知 CLI 返回空数组，与原 if-else 链未匹配分支行为一致。
  const spec = cliTools.getCliTool(cliTool);
  if (!spec || typeof spec.skillRoots !== 'function') return [];
  const cwd = process.cwd();
  const home = os.homedir();
  return spec.skillRoots(cwd, home);
}

function parseSkillFile(fallbackName, sourcePath, content) {
  const skill = {
    name: fallbackName,
    detail: content.length > 500 ? content.slice(0, 500) + "..." : content,
    source_path: sourcePath,
    auto: true,
  };
  const lines = content.split(/\r?\n/);
  if (lines[0]?.trim() !== '---') {
    skill.description = normalizeSkillDescription(skill.name, skill.description, content);
    return skill;
  }
  for (let i = 1; i < lines.length; i += 1) {
    const line = lines[i].trim();
    if (line === '---') break;
    const separator = line.indexOf(':');
    if (separator === -1) continue;
    const key = line.slice(0, separator).trim();
    const parsed = readFrontmatterValue(lines, i, line.slice(separator + 1).trim());
    const value = parsed.value;
    i = parsed.nextIndex - 1;
    if (key === 'name' && value) skill.name = value;
    if (key === 'description') skill.description = value;
  }
  skill.description = normalizeSkillDescription(skill.name, skill.description, content);
  return skill;
}

function normalizeSkillDescription(name, description, content) {
  const current = String(description || '').trim();
  if (isUsefulDescription(current)) return current;
  return inferSkillDescription(name, content);
}

function isUsefulDescription(description) {
  const text = String(description || '').trim();
  if (!text) return false;
  if (/^(ok|todo|tbd|none|n\/a|na|null|undefined|test|demo|sample|example)$/i.test(text)) {
    return false;
  }
  const compact = text.replace(/[\s\p{P}\p{S}]/gu, '');
  return descriptionLength(compact) >= MIN_DESCRIPTION_CHARS;
}

function inferSkillDescription(name, content) {
  const body = stripFrontmatter(content);
  const chunks = [];
  let inFence = false;
  for (const rawLine of body.split(/\r?\n/)) {
    let line = rawLine.trim();
    if (line.startsWith('```') || line.startsWith('~~~')) {
      inFence = !inFence;
      continue;
    }
    if (inFence || !line) continue;
    line = cleanMarkdownLine(line);
    if (!line || line.toLowerCase() === String(name || '').toLowerCase()) continue;
    chunks.push(line);
    if (descriptionLength(chunks.join(' ')) >= 120) break;
  }
  const summary = truncateDescription(chunks.join(' ').replace(/\s+/g, ' ').trim());
  if (summary) return summary;
  return `Provides the ${name || 'selected'} skill for local Agent workflows.`;
}

function stripFrontmatter(content) {
  const lines = String(content || '').split(/\r?\n/);
  if (lines[0]?.trim() !== '---') return String(content || '');
  for (let i = 1; i < lines.length; i += 1) {
    if (lines[i].trim() === '---') {
      return lines.slice(i + 1).join('\n');
    }
  }
  return String(content || '');
}

function cleanMarkdownLine(line) {
  return line
    .replace(/^#{1,6}\s+/, '')
    .replace(/^[-*+]\s+/, '')
    .replace(/^\d+[.)]\s+/, '')
    .replace(/^>\s?/, '')
    .replace(/!\[[^\]]*]\([^)]*\)/g, '')
    .replace(/\[([^\]]+)]\([^)]*\)/g, '$1')
    .replace(/`([^`]+)`/g, '$1')
    .replace(/\*\*([^*]+)\*\*/g, '$1')
    .replace(/\*([^*]+)\*/g, '$1')
    .replace(/__([^_]+)__/g, '$1')
    .replace(/_([^_]+)_/g, '$1')
    .trim();
}

function truncateDescription(text) {
  if (!text) return '';
  const chars = Array.from(text);
  if (chars.length <= 180) return text;
  return `${chars.slice(0, 177).join('').trimEnd()}...`;
}

function descriptionLength(text) {
  return Array.from(String(text || '')).length;
}

function readFrontmatterValue(lines, index, rawValue) {
  let value = rawValue.replace(/^['"]|['"]$/g, '');
  let nextIndex = index + 1;
  if (rawValue !== '>' && rawValue !== '|') {
    return { value, nextIndex };
  }
  const folded = rawValue === '>';
  const parts = [];
  for (let i = index + 1; i < lines.length; i += 1) {
    const current = lines[i];
    const trimmed = current.trim();
    if (trimmed === '---' || isFrontmatterKeyLine(current)) {
      nextIndex = i;
      break;
    }
    parts.push(trimmed);
    nextIndex = i + 1;
  }
  value = folded ? parts.join(' ') : parts.join('\n');
  return { value: value.trim(), nextIndex };
}

function isFrontmatterKeyLine(line) {
  if (/^\s/.test(line)) return false;
  return /^[A-Za-z0-9_-]+\s*:/.test(line.trim());
}

function apiURL(serverURL, apiKey, pathname) {
  const url = new URL(serverURL);
  url.pathname = `${url.pathname.replace(/\/$/, '')}${pathname}`;
  url.searchParams.set('token', apiKey);
  url.hash = '';
  return url;
}

function requestJSON(method, url, body, bearerToken) {
  const data = body === undefined ? null : Buffer.from(JSON.stringify(body));
  const transport = url.protocol === 'https:' ? https : http;

  const headers = {};
  if (data) {
    headers['Content-Type'] = 'application/json';
    headers['Content-Length'] = data.length;
  }
  if (bearerToken) {
    headers['Authorization'] = `Bearer ${bearerToken}`;
  }

  return new Promise((resolve, reject) => {
    const req = transport.request(url, {
      method,
      headers: Object.keys(headers).length ? headers : undefined,
    }, (res) => {
      let response = '';
      res.setEncoding('utf8');
      res.on('data', (chunk) => {
        response += chunk;
      });
      res.on('end', () => {
        if (!res.statusCode || res.statusCode < 200 || res.statusCode >= 300) {
          reject(new Error(`HTTP ${res.statusCode}: ${response}`));
          return;
        }
        try {
          resolve(response ? JSON.parse(response) : null);
        } catch (error) {
          reject(error);
        }
      });
    });
    req.on('error', reject);
    if (data) req.write(data);
    req.end();
  });
}

function truncateStr(s, max) {
  if (!s || s.length <= max) return s || '';
  return s.slice(0, max) + '...';
}

function buildPromptParts(task) {
  let ctx = task.context_messages;
  ctx = typeof ctx === 'string' ? ctx : '';

  // 长度保护：超过 8000 字符时截断中间部分，保留头部和尾部
  const maxCtxLen = 8000;
  if (ctx.length > maxCtxLen) {
    const headLen = Math.floor(maxCtxLen * 0.4);
    const tailLen = Math.floor(maxCtxLen * 0.4);
    ctx = ctx.slice(0, headLen) + '\n...[上下文已截断]...\n' + ctx.slice(ctx.length - tailLen);
  }

  // 从 context_messages 中提取系统指令和工具配置作为 system prompt
  let systemPrompt = '';
  let remainingCtx = ctx;
  const contextSectionBoundary = '(?=\\n\\n\\[可用工具\\]|\\n\\n\\[平台 Skills\\]|\\n\\n\\[群聊背景\\]|\\n\\n\\[调度指令\\]|\\n\\n\\[依赖输出\\]|\\n\\n\\{|$)';

  const sysSection = remainingCtx.match(new RegExp(`^(\\[系统指令\\]\\n[\\s\\S]*?)${contextSectionBoundary}`));
  if (sysSection) {
    systemPrompt += sysSection[1].replace('[系统指令]\n', '').trim();
    remainingCtx = remainingCtx.slice(sysSection[0].length).replace(/^\s+/, '');
  }

  const toolsSection = remainingCtx.match(new RegExp(`^(\\[可用工具\\]\\n[\\s\\S]*?)${contextSectionBoundary}`));
  if (toolsSection) {
    systemPrompt += (systemPrompt ? '\n\n' : '') + '# 可用工具\n' + toolsSection[1].replace('[可用工具]\n', '').trim();
    remainingCtx = remainingCtx.slice(toolsSection[0].length).replace(/^\s+/, '');
  }

  const skillsSection = remainingCtx.match(new RegExp(`^(\\[平台 Skills\\]\\n[\\s\\S]*?)${contextSectionBoundary}`));
  if (skillsSection) {
    systemPrompt += (systemPrompt ? '\n\n' : '') + '# 平台 Skills\n' + skillsSection[1].replace('[平台 Skills]\n', '').trim();
    remainingCtx = remainingCtx.slice(skillsSection[0].length).replace(/^\s+/, '');
  }

  remainingCtx = remainingCtx.trim();

  // 构建用户 prompt
  const parts = [];

  if (remainingCtx) {
    let handoffs = null;
    try {
      handoffs = JSON.parse(remainingCtx);
    } catch {
      // not JSON — treat as plain text (orchestrator dispatch context)
    }

    if (handoffs !== null && Array.isArray(handoffs)) {
      parts.push('[历史 Agent 交接]');
      for (const h of handoffs) {
        const req = truncateStr(h.user_request, 100);
        const res = truncateStr(h.result, 200);
        parts.push(`- ${h.agent_name}: 用户问 "${req}" → 回复：${res}`);
      }
      parts.push('');
      parts.push('你是 AgentHub 群聊中被 @提及的机器人，请参考上述交接上下文回答用户消息。');
    } else {
      parts.push(remainingCtx);
    }
  } else if (!systemPrompt) {
    parts.push('你是 AgentHub 群聊中被 @提及的机器人，请直接回答用户当前消息。');
  }

  parts.push('');
  parts.push(task.prompt);

  return { systemPrompt, userPrompt: parts.join('\n') };
}

function ensureTaskWorkdir(task) {
  const safeID = String(task.id || crypto.randomUUID()).replace(/[^a-zA-Z0-9_-]/g, '-');
  const dir = path.join(os.tmpdir(), 'agenthub-cli-tasks', safeID);
  fs.mkdirSync(dir, { recursive: true });
  return dir;
}

function ensureAgentHubCodexHome() {
  const configured = process.env.AGENTHUB_CODEX_HOME;
  const dir = configured || path.join(os.homedir(), '.agenthub', 'codex');
  fs.mkdirSync(dir, { recursive: true });
  const configFile = path.join(dir, 'config.toml');
  if (!fs.existsSync(configFile)) {
    fs.writeFileSync(configFile, [
      'model_provider = "OpenAI"',
      'model = "gpt-5.5"',
      'review_model = "gpt-5.5"',
      'model_reasoning_effort = "xhigh"',
      'disable_response_storage = true',
      'network_access = "enabled"',
      'windows_wsl_setup_acknowledged = true',
      '',
      '[model_providers.OpenAI]',
      'name = "OpenAI"',
      'base_url = "https://www.aitoken-api.com"',
      'wire_api = "responses"',
      'requires_openai_auth = true',
      '',
      '[features]',
      'goals = true',
      '',
    ].join('\n'), 'utf8');
  }
  return dir;
}

function tomlString(value) {
  return JSON.stringify(String(value));
}

function tomlArray(values) {
  return `[${values.map(tomlString).join(', ')}]`;
}

function ensureAgentHubCodexMcpConfig(codexHome, conversationId, userId, agentId) {
  const configFile = path.join(codexHome, 'config.toml');
  let content = fs.existsSync(configFile) ? fs.readFileSync(configFile, 'utf8') : '';
  const sectionPattern = /\n?\[mcp_servers\.agenthub-platform\]\n[\s\S]*?(?=\n\[[^\]]+\]|\s*$)/;
  content = content.replace(sectionPattern, '').trimEnd();
  const args = buildPlatformMcpServerArgs(conversationId, userId, agentId);
  if (args.length === 0) return configFile;
  const section = [
    '',
    '[mcp_servers.agenthub-platform]',
    'command = "node"',
    `args = ${tomlArray(args)}`,
    '',
  ].join('\n');
  fs.writeFileSync(configFile, `${content}${section}`, 'utf8');
  return configFile;
}

// initCliToolsCtx 是传给 CliToolSpec 工厂的依赖注入对象。
// 包含 spec 实现需要的全部 daemon 辅助函数（避免 spec 直接 require 主文件造成循环依赖）。
// 新增 spec 时若需要新辅助函数，在此对象补一个键即可。
const initCliToolsCtx = {
  // 通用工具
  pathJoin: path.join,
  tmpdir: os.tmpdir,
  addRoot,
  errorMessage,
  firstLine,
  logFlow,
  // 命令解析 / 版本检测
  resolveCommand,
  commandVersion,
  processSpec,
  spawnSync,
  // prompt / context 辅助
  buildPlatformMcpArgs,
  buildAgentHubContextEnv,
  buildPlatformMcpServerArgs,
  makeSessionId,
  sessionKeyForTask,
  opencodeContextChanged,
  // codex 专用
  ensureAgentHubCodexHome,
  ensureAgentHubCodexMcpConfig,
  ensureTaskWorkdir,
  codexLoginStatus,
  // opencode 专用
  ensureOpenCodeMcpConfig,
  // skill 扫描辅助
  isAgentHubWorkspace,
  openClawInstallSkillRoots,
  // 默认能力构造（CANDIDATES 派生用）
  defaultSkills,
  // conversationSessions（运行时可变的会话映射）
  conversationSessions,
};

// 启动时注册 4 个 CLI spec。函数声明会被提升，此处所有辅助函数已可用。
cliTools.initCliTools(initCliToolsCtx);

function commandForTask(task) {
  const { systemPrompt, userPrompt } = buildPromptParts(task);
  const command = resolveCommand(task.cli_tool);
  // 委托给 CliToolSpec.buildCommand。未知 cli_tool 走 fallback（保留原 default 分支行为）。
  const spec = cliTools.getCliTool(task.cli_tool);
  if (spec && typeof spec.buildCommand === 'function') {
    return spec.buildCommand(task, {
      command,
      systemPrompt,
      userPrompt,
      runningAgents,
    });
  }
  return { command, args: [userPrompt] };
}

async function executeTask(task) {
  if (task.cli_tool === OPEN_PATH_TOOL) {
    logFlow('info', 'task.open_path_start', { task_id: task.id });
    return openSkillLocation(task.prompt);
  }
  const spec = commandForTask(task);
  const taskMeta = {
    task_id: task.id,
    cli_tool: task.cli_tool || 'unknown',
    agent_id: task.agent_id,
    conversation_id: task.conversation_id,
  };
  logFlow('info', 'task.command_prepared', {
    ...taskMeta,
    command: spec.command,
    args_count: Array.isArray(spec.args) ? spec.args.length : 0,
    has_stdin: typeof spec.stdin === 'string' && spec.stdin.length > 0,
    stdin_len: typeof spec.stdin === 'string' ? spec.stdin.length : 0,
    session_id: spec.sessionId,
    output_file: Boolean(spec.outputFile),
    result_format: spec.resultFormat,
  });

  let stdout;
  let stderr;
  if (spec.sessionId) {
    logFlow('info', 'task.session_resume_start', { ...taskMeta, session_id: spec.sessionId });
    killSessionProcess(spec.sessionId);
    await sleep(1000);

    try {
      ({ stdout, stderr } = await runProcess(
        spec.command,
        ['--resume', spec.sessionId, ...spec.args],
        spec.stdin,
        spec.sessionId,
        spec.cwd,
        spec.env,
        { ...taskMeta, mode: 'resume' },
      ));
    } catch (_err) {
      logFlow('warn', 'task.session_resume_failed', {
        ...taskMeta,
        session_id: spec.sessionId,
        error: errorMessage(_err),
      });
      killSessionProcess(spec.sessionId);
      await sleep(500);
      try {
        ({ stdout, stderr } = await runProcess(
          spec.command,
          ['--session-id', spec.sessionId, ...spec.args],
          spec.stdin,
          spec.sessionId,
          spec.cwd,
          spec.env,
          { ...taskMeta, mode: 'session_id_retry' },
        ));
      } catch (_err2) {
        const freshId = `agenthub-${String(task.id || crypto.randomUUID()).replace(/[^a-zA-Z0-9_-]/g, '-')}`;
        logFlow('warn', 'task.session_fresh_start', {
          ...taskMeta,
          previous_session_id: spec.sessionId,
          fresh_session_id: freshId,
          error: errorMessage(_err2),
        });
        ({ stdout, stderr } = await runProcess(
          spec.command,
          ['--session-id', freshId, ...spec.args],
          spec.stdin,
          freshId,
          spec.cwd,
          spec.env,
          { ...taskMeta, mode: 'fresh_session' },
        ));
      }
    }
  } else {
    ({ stdout, stderr } = await runProcess(spec.command, spec.args, spec.stdin, undefined, spec.cwd, spec.env, { ...taskMeta, mode: 'one_shot' }));
  }

  if (spec.outputFile && fs.existsSync(spec.outputFile)) {
    const text = fs.readFileSync(spec.outputFile, 'utf8').trim();
    fs.rmSync(spec.outputFile, { force: true });
    if (text) {
      logFlow('info', 'task.result_ready', { ...taskMeta, source: 'output_file', result_len: text.length });
      return text;
    }
  }
  if (spec.resultFormat === 'openclaw-json') {
    const parsed = parseOpenClawOutput(stdout);
    logFlow('info', 'task.result_ready', { ...taskMeta, source: 'openclaw_json', result_len: parsed.length });
    return parsed;
  }
  if (spec.resultFormat === 'opencode-json') {
    const parsed = parseOpenCodeOutput(stdout);
    if (spec.persistSessionKey && parsed.sessionId) {
      conversationSessions.set(spec.persistSessionKey, parsed.sessionId);
      saveSessionMap();
    }
    logFlow('info', 'task.result_ready', {
      ...taskMeta,
      source: 'opencode_json',
      result_len: parsed.text.length,
      session_id: parsed.sessionId,
      session_persisted: Boolean(spec.persistSessionKey && parsed.sessionId),
    });
    return parsed.text;
  }
  const text = `${stdout || ''}${stderr ? `\n${stderr}` : ''}`.trim();
  logFlow('info', 'task.result_ready', { ...taskMeta, source: 'stdio', result_len: text.length });
  return text || '(Agent CLI 没有返回内容)';
}

async function executeTaskOnce(task) {
  const taskID = task && task.id;
  if (!taskID) return executeTask(task);
  if (activeTaskIDs.has(taskID) || completedTaskIDs.has(taskID)) {
    logFlow('warn', 'task.dispatch_duplicate_ignored', {
      task_id: taskID,
      cli_tool: task.cli_tool,
      agent_id: task.agent_id,
      conversation_id: task.conversation_id,
    });
    return null;
  }
  activeTaskIDs.add(taskID);
  try {
    const result = await executeTask(task);
    completedTaskIDs.add(taskID);
    if (completedTaskIDs.size > 1000) {
      completedTaskIDs.delete(completedTaskIDs.values().next().value);
    }
    return result;
  } finally {
    activeTaskIDs.delete(taskID);
  }
}

function openSkillLocation(prompt) {
  let payload = null;
  try {
    payload = JSON.parse(prompt);
  } catch {
    throw new Error('Invalid open path payload');
  }
  const sourcePath = String(payload.source_path || '').trim();
  if (!sourcePath || path.basename(sourcePath) !== 'SKILL.md') {
    throw new Error('Invalid skill file path');
  }
  if (agentHubWorkspaceForPath(sourcePath)) {
    throw new Error('Refuse to open stale AgentHub workspace skill source. Reconnect this computer to refresh skills.');
  }
  if (!fs.existsSync(sourcePath) || !fs.statSync(sourcePath).isFile()) {
    throw new Error(`Skill file not found: ${sourcePath}`);
  }
  const folder = path.dirname(sourcePath);
  let command = 'xdg-open';
  let args = [folder];
  let hideWindow = false;
  if (process.platform === 'win32') {
    command = 'explorer.exe';
    args = [`/select,${sourcePath}`];
  } else if (process.platform === 'darwin') {
    command = 'open';
    args = ['-R', sourcePath];
  }
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      detached: true,
      stdio: 'ignore',
      windowsHide: hideWindow,
    });
    let settled = false;
    const finish = (callback) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      callback();
    };
    const timer = setTimeout(() => {
      child.unref();
      finish(() => resolve(`Opened folder ${folder}`));
    }, OPEN_PATH_TIMEOUT_MS);
    child.on('error', (error) => {
      finish(() => reject(new Error(`Open folder failed: ${error.message}`)));
    });
    child.on('close', (code) => {
      if (process.platform === 'win32' || code === 0) {
        finish(() => resolve(`Opened folder ${folder}`));
        return;
      }
      finish(() => reject(new Error(`Open folder exited with code ${code}`)));
    });
  });
}

function parseOpenClawOutput(stdout) {
  const text = stdout.trim();
  if (!text) return '(OpenClaw CLI 没有返回内容)';
  try {
    const parsed = JSON.parse(text);

    // 常见字段：finalAssistantVisibleText / finalAssistantRawText
    if (typeof parsed.finalAssistantVisibleText === 'string' && parsed.finalAssistantVisibleText.trim()) {
      return parsed.finalAssistantVisibleText.trim();
    }
    if (typeof parsed.finalAssistantRawText === 'string' && parsed.finalAssistantRawText.trim()) {
      return parsed.finalAssistantRawText.trim();
    }

    // payloads 数组（OpenClaw 标准格式）
    if (Array.isArray(parsed.payloads)) {
      const payloadText = parsed.payloads
        .map((payload) => (typeof payload?.text === 'string' ? payload.text : ''))
        .filter(Boolean)
        .join('\n')
        .trim();
      if (payloadText) return payloadText;
    }

    // messages 数组格式（兼容更多 CLI 输出）
    if (Array.isArray(parsed.messages)) {
      const msgText = parsed.messages
        .filter((m) => typeof m?.content === 'string' && m.role === 'assistant')
        .map((m) => m.content)
        .join('\n')
        .trim();
      if (msgText) return msgText;
    }

    // content 字段（简化 JSON 格式）
    if (typeof parsed.content === 'string' && parsed.content.trim()) {
      return parsed.content.trim();
    }

    // response / result / output 字段
    for (const key of ['response', 'result', 'output', 'text', 'message']) {
      if (typeof parsed[key] === 'string' && parsed[key].trim()) {
        return parsed[key].trim();
      }
    }

    // 嵌套 data 字段
    if (parsed.data && typeof parsed.data === 'object') {
      for (const key of ['text', 'content', 'message', 'response']) {
        if (typeof parsed.data[key] === 'string' && parsed.data[key].trim()) {
          return parsed.data[key].trim();
        }
      }
    }
  } catch {
    return text;
  }
  return text;
}

// parseArtifacts 从 Agent 回复的 Markdown 文本中提取结构化产物（MVP：code + webpage）。
// 字段名必须与 backend ws.ArtifactResult / model.Artifact 的 json tag 对齐：
//   { type, language, filename, title, url, content }
function extractOpenCodeSessionId(value) {
  if (!value || typeof value !== 'object') return '';
  for (const key of ['sessionID', 'sessionId', 'session_id']) {
    if (typeof value[key] === 'string' && value[key].trim()) return value[key].trim();
  }
  if (value.session && typeof value.session === 'object') {
    for (const key of ['id', 'sessionID', 'sessionId', 'session_id']) {
      if (typeof value.session[key] === 'string' && value.session[key].trim()) return value.session[key].trim();
    }
  }
  if (value.message && typeof value.message === 'object') {
    return extractOpenCodeSessionId(value.message);
  }
  return '';
}

function textFromOpenCodeContent(content) {
  if (typeof content === 'string') return content;
  if (Array.isArray(content)) {
    return content
      .map((part) => textFromOpenCodeContent(part))
      .filter(Boolean)
      .join('');
  }
  if (!content || typeof content !== 'object') return '';
  if (typeof content.text === 'string') return content.text;
  if (typeof content.content === 'string') return content.content;
  if (Array.isArray(content.parts)) return textFromOpenCodeContent(content.parts);
  return '';
}

function collectOpenCodeText(value, directMessages, partChunks, partUpdates) {
  if (!value || typeof value !== 'object') return;

  const message = value.message && typeof value.message === 'object' ? value.message : value;
  if (message.role === 'assistant') {
    const messageText = textFromOpenCodeContent(message.content || message.parts || message.text);
    if (messageText.trim()) directMessages.push(messageText);
  }

  const part = value.part && typeof value.part === 'object' ? value.part : null;
  if (part && (part.type === 'text' || typeof part.text === 'string')) {
    const text = textFromOpenCodeContent(part);
    if (text.trim()) {
      const partID = String(part.id || value.id || '');
      if (partID && /updated|delta/i.test(String(value.type || ''))) {
        partUpdates.set(partID, text);
      } else {
        partChunks.push(text);
      }
    }
  }

  for (const key of ['response', 'result', 'output', 'text']) {
    if (typeof value[key] === 'string' && value[key].trim() && /assistant|result|complete|response|text/i.test(String(value.type || key))) {
      partChunks.push(value[key]);
    }
  }
}

function parseOpenCodeOutput(stdout) {
  const text = stdout.trim();
  if (!text) return { text: '(OpenCode CLI 没有返回内容)', sessionId: '' };

  const events = [];
  const lines = text.split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
  for (const line of lines) {
    try {
      events.push(JSON.parse(line));
    } catch {
      // OpenCode may fall back to formatted output when JSON is unavailable.
    }
  }
  if (events.length === 0) {
    try {
      events.push(JSON.parse(text));
    } catch {
      return { text, sessionId: '' };
    }
  }

  let sessionId = '';
  const directMessages = [];
  const partChunks = [];
  const partUpdates = new Map();
  for (const event of events) {
    sessionId = sessionId || extractOpenCodeSessionId(event);
    collectOpenCodeText(event, directMessages, partChunks, partUpdates);
  }

  const directText = directMessages.join('\n').trim();
  if (directText) return { text: directText, sessionId };

  const updatedText = Array.from(partUpdates.values()).join('').trim();
  if (updatedText) return { text: updatedText, sessionId };

  const chunkText = partChunks.join('').trim();
  if (chunkText) return { text: chunkText, sessionId };

  return { text, sessionId };
}

function lineEndIndex(text, start) {
  const nl = text.indexOf('\n', start);
  return nl === -1 ? text.length : nl + 1;
}

function lineText(text, start, end) {
  const raw = text.slice(start, end);
  return raw.endsWith('\n') ? raw.slice(0, -1).replace(/\r$/, '') : raw.replace(/\r$/, '');
}

function findFenceClose(text, start, preferLast) {
  let pos = start;
  let first = null;
  let last = null;
  while (pos < text.length) {
    const end = lineEndIndex(text, pos);
    const line = lineText(text, pos, end);
    if (/^ {0,3}`{3,}\s*$/.test(line)) {
      const found = { start: pos, end };
      if (!first) first = found;
      last = found;
      if (!preferLast) return found;
    }
    pos = end;
  }
  return preferLast ? last : first;
}

function extractFencedBlocks(text) {
  const blocks = [];
  let pos = 0;
  while (pos < text.length) {
    const end = lineEndIndex(text, pos);
    const line = lineText(text, pos, end);
    const open = line.match(/^ {0,3}`{3,}([^`]*)$/);
    if (!open) {
      pos = end;
      continue;
    }

    const language = (open[1] || '').trim().toLowerCase();
    const preferLastClose = language === 'markdown' || language === 'md';
    const close = findFenceClose(text, end, preferLastClose);
    if (!close) break;

    blocks.push({
      language,
      content: text.slice(end, close.start),
      start: pos,
      end: close.end,
    });
    pos = close.end;
  }
  return blocks;
}

function unwrapSingleMarkdownFence(text, fencedBlocks) {
  if (fencedBlocks.length !== 1) return null;
  const block = fencedBlocks[0];
  if (block.language !== 'markdown' && block.language !== 'md') return null;
  if (text.slice(0, block.start).trim() || text.slice(block.end).trim()) return null;
  return block.content.replace(/\s+$/, '');
}

function extractMarkdownDocumentFence(fencedBlocks) {
  for (const block of fencedBlocks) {
    if (block.language !== 'markdown' && block.language !== 'md') continue;
    const content = block.content.replace(/\s+$/, '');
    if (looksLikeMarkdownDocument(content)) return content;
  }
  return null;
}

function looksLikeMarkdownDocument(text) {
  const src = text.trim();
  if (src.length < 40) return false;
  const headingMatches = src.match(/^#{1,3}\s+\S.+$/gm) || [];
  if (headingMatches.length === 0) return false;
  if (headingMatches.length >= 2) return true;
  return /(^|\n)(?:[-*]\s+\S|\|.+\||```)/.test(src);
}

function markdownDocumentTitle(text) {
  const match = text.match(/^#\s+(.+)$/m);
  return match ? match[1].trim() : 'Document Preview';
}

function parseArtifacts(text) {
  if (typeof text !== 'string' || !text.trim()) return [];
  const artifacts = [];
  const documentLanguages = new Set(['markdown', 'md', 'txt', 'text', 'json', 'csv']);
  const fencedBlocks = extractFencedBlocks(text);
  const wrappedMarkdown = unwrapSingleMarkdownFence(text, fencedBlocks);
  if (wrappedMarkdown && looksLikeMarkdownDocument(wrappedMarkdown)) {
    return [{
      type: 'document',
      language: 'markdown',
      filename: '',
      title: markdownDocumentTitle(wrappedMarkdown),
      content: wrappedMarkdown,
    }];
  }
  const embeddedMarkdown = extractMarkdownDocumentFence(fencedBlocks);
  if (embeddedMarkdown) {
    return [{
      type: 'document',
      language: 'markdown',
      filename: '',
      title: markdownDocumentTitle(embeddedMarkdown),
      content: embeddedMarkdown,
    }];
  }
  if (looksLikeMarkdownDocument(text)) {
    const content = text.trim();
    return [{
      type: 'document',
      language: 'markdown',
      filename: '',
      title: markdownDocumentTitle(content),
      content,
    }];
  }

  // 1) 提取围栏代码块 ```lang\n...\n```
  // 第一行可能是 "// file: xxx" 或 "# file: xxx" 形式的文件名提示
  for (const block of fencedBlocks) {
    const language = block.language;
    let content = block.content;
    let filename = '';
    // 识别首行文件名注释：// file: xxx 、# file: xxx 、<!-- file: xxx -->
    const firstLine = content.split('\n', 1)[0] || '';
    const fileHint = firstLine.match(/^\s*(?:\/\/|#|<!--)\s*file:\s*(.+?)\s*(?:-->)?\s*$/i);
    if (fileHint) {
      filename = fileHint[1].trim();
      const nl = content.indexOf('\n');
      // 只在确有换行时剥离首行提示，否则整块就是提示行本身（无正文）
      content = nl === -1 ? '' : content.slice(nl + 1);
    }
    content = content.replace(/\s+$/, '');
    if (!content) continue;

    // language 为 html 时归类为 webpage（完整 HTML 文档），否则为 code
    if (language === 'html' && /<\s*(?:html|!doctype|body|head|div)\b/i.test(content)) {
      artifacts.push({ type: 'webpage', title: filename || 'HTML Preview', content });
    } else if (documentLanguages.has(language)) {
      artifacts.push({
        type: 'document',
        language: language || 'text',
        filename,
        title: filename || (language === 'json' ? 'JSON Document' : 'Document Preview'),
        content,
      });
    } else {
      artifacts.push({ type: 'code', language, filename, content });
    }
  }

  // 2) 提取裸 http/https URL（在代码块外，简单去重）
  // 先剔除围栏代码块，避免把代码里的 import/注释 URL 误当作网页产物
  let textOutsideFences = '';
  let lastIndex = 0;
  for (const block of fencedBlocks) {
    textOutsideFences += text.slice(lastIndex, block.start) + ' ';
    lastIndex = block.end;
  }
  textOutsideFences += text.slice(lastIndex);
  // 字符类排除空白、闭合括号/尖括号/引号，以及中文标点（。，、；）避免吞掉句末标点
  const urlRe = /\bhttps?:\/\/[^\s)\]<>"'，。、；：！？]+/g;
  const seen = new Set();
  let urlMatch;
  while ((urlMatch = urlRe.exec(textOutsideFences)) !== null) {
    const url = urlMatch[0].replace(/[.,;:!?]+$/, ''); // 去掉句末 ASCII 标点
    if (!url || seen.has(url)) continue;
    seen.add(url);
    artifacts.push({ type: 'webpage', url, title: '' });
  }

  return artifacts;
}

function runProcess(command, args, stdin, sessionId, cwd, extraEnv, meta = {}) {
  return new Promise((resolve, reject) => {
    const spec = processSpec(command, args);
    const detached = process.platform !== 'win32';
    const startedAt = Date.now();
    const child = spawn(spec.command, spec.args, {
      cwd: cwd || undefined,
      env: extraEnv ? { ...process.env, ...extraEnv } : undefined,
      windowsHide: true,
      stdio: ['pipe', 'pipe', 'pipe'],
      detached,
    });
    logFlow('info', 'process.spawn', {
      ...meta,
      command: spec.command,
      args_count: spec.args.length,
      stdin_len: typeof stdin === 'string' ? stdin.length : 0,
      session_id: sessionId,
      pid: child.pid,
    });
    if (sessionId) {
      activeSessions.set(sessionId, child);
      child.on('close', () => {
        if (activeSessions.get(sessionId) === child) {
          activeSessions.delete(sessionId);
        }
      });
    }
    let stdout = '';
    let stderr = '';
    const timer = setTimeout(() => {
      try {
        if (detached) {
          process.kill(-child.pid, 'SIGKILL');
        } else {
          child.kill();
        }
      } catch { /* ignore */ }
      logFlow('error', 'process.timeout', {
        ...meta,
        command: spec.command,
        pid: child.pid,
        session_id: sessionId,
        timeout_ms: EXEC_TIMEOUT_MS,
      });
      reject(new Error(`CLI execution timed out after ${EXEC_TIMEOUT_MS / 1000}s`));
    }, EXEC_TIMEOUT_MS);

    child.stdout.setEncoding('utf8');
    child.stderr.setEncoding('utf8');
    child.stdout.on('data', (chunk) => {
      stdout += chunk;
    });
    child.stderr.on('data', (chunk) => {
      stderr += chunk;
    });
    child.on('error', (error) => {
      clearTimeout(timer);
      logFlow('error', 'process.error', {
        ...meta,
        command: spec.command,
        pid: child.pid,
        session_id: sessionId,
        error: errorMessage(error),
      });
      reject(error);
    });
    child.on('close', (code) => {
      clearTimeout(timer);
      logFlow(code === 0 ? 'info' : 'warn', 'process.exit', {
        ...meta,
        command: spec.command,
        pid: child.pid,
        session_id: sessionId,
        exit_code: code,
        duration_ms: Date.now() - startedAt,
        stdout_len: stdout.length,
        stderr_len: stderr.length,
      });
      if (code === 0) {
        resolve({ stdout, stderr });
        return;
      }
      reject(new Error((stderr || stdout || `CLI exited with code ${code}`).trim()));
    });
    child.stdin.end(stdin || '');
  });
}

async function register(serverURL, apiKey) {
  const agents = scanAgents();
  logFlow('info', 'register.scan_completed', {
    machine_id: os.hostname(),
    agent_count: agents.length,
    agents: agents.map((agent) => `${agent.name}:${agent.cli_tool}`),
  });
  for (const agent of agents) {
    const version = agent.version ? ` ${String(agent.version).split('\n')[0].trim()}` : '';
    const skillCount = Array.isArray(agent.capabilities) ? agent.capabilities.length : 0;
    logFlow('info', 'register.agent_detected', {
      machine_id: os.hostname(),
      agent_name: agent.name,
      cli_tool: agent.cli_tool,
      version: version.trim(),
      skill_count: skillCount,
    });
  }
  const res = await requestJSON('POST', apiURL(serverURL, apiKey, '/daemon/register'), {
    machine_id: os.hostname(),
    agents,
  });
  if (res && res.data && res.data.daemon_token && !daemonConn.daemonToken) {
    daemonConn.daemonToken = res.data.daemon_token;
  }
  logFlow('info', 'register.completed', {
    machine_id: os.hostname(),
    agent_count: agents.length,
    daemon_token_received: Boolean(res && res.data && res.data.daemon_token),
  });
  logFlow('info', 'daemon.ready', { machine_id: os.hostname(), transport: WebSocket ? 'websocket' : 'polling' });
}

// 在任务执行期间定期发送心跳，告知 server 任务仍在进行中
function startHeartbeat(serverURL, apiKey, taskId) {
  const timer = setInterval(async () => {
    try {
      await requestJSON('POST', apiURL(serverURL, apiKey, `/daemon/tasks/${taskId}/heartbeat`), {});
    } catch (error) {
      logFlow('warn', 'task.heartbeat_failed', { task_id: taskId, error: errorMessage(error) });
    }
  }, HEARTBEAT_INTERVAL_MS);
  return () => clearInterval(timer);
}

async function pollTasks(serverURL, apiKey) {
  for (;;) {
    try {
      const response = await requestJSON('GET', apiURL(serverURL, apiKey, '/daemon/tasks'));
      const task = response ? response.data : null;
      if (!task) {
        await sleep(POLL_INTERVAL_MS);
        continue;
      }

      logFlow('info', 'poll.task_claimed', {
        task_id: task.id,
        cli_tool: task.cli_tool,
        agent_id: task.agent_id,
        conversation_id: task.conversation_id,
      });
      const stopHeartbeat = startHeartbeat(serverURL, apiKey, task.id);
      try {
        const result = await executeTaskOnce(task);
        if (result === null) {
          stopHeartbeat();
          continue;
        }
        stopHeartbeat();
        await requestJSON('POST', apiURL(serverURL, apiKey, `/daemon/tasks/${task.id}/complete`), { result });
        logFlow('info', 'poll.task_completed', {
          task_id: task.id,
          cli_tool: task.cli_tool,
          result_len: typeof result === 'string' ? result.length : 0,
        });
      } catch (error) {
        stopHeartbeat();
        await requestJSON('POST', apiURL(serverURL, apiKey, `/daemon/tasks/${task.id}/complete`), {
          error: error instanceof Error ? error.message : String(error),
        });
        logFlow('error', 'poll.task_failed', {
          task_id: task.id,
          cli_tool: task.cli_tool,
          error: errorMessage(error),
        });
      }
    } catch (error) {
      logFlow('warn', 'poll.failed', { error: errorMessage(error), retry_ms: POLL_INTERVAL_MS * 2 });
      await sleep(POLL_INTERVAL_MS * 2);
    }
  }
}
function stopAgentProcess(agent_id) {
  const entry = runningAgents.get(agent_id);
  if (!entry) return;
  try {
    if (process.platform === 'win32') {
      spawn('taskkill', ['/pid', String(entry.process.pid), '/T', '/F'], { windowsHide: true });
    } else {
      process.kill(-entry.process.pid, 'SIGKILL');
    }
    logFlow('info', 'agent.process_killed', {
      agent_id,
      session_id: entry.sessionId,
      pid: entry.process.pid,
      conversation_id: entry.currentConversationId,
    });
  } catch { /* already dead */ }
  runningAgents.delete(agent_id);
}

/**
 * Spawn a Claude Code process with stream-json transport.
 * Returns { child, sessionId, sendPrompt }.
 * If resume=true, uses --resume <sessionId>; otherwise --session-id <sessionId>.
 */
function spawnStreamJsonProcess(agentId, sessionId, systemPrompt, resume, conversationId, userId) {
  const command = resolveCommand('claude');
  const mcpArgs = buildPlatformMcpArgs(conversationId, userId, agentId);
  const effectiveSessionId = sessionId || crypto.randomUUID();

  const args = [
    '--dangerously-skip-permissions',
    '--output-format', 'stream-json',
    '--input-format', 'stream-json',
    '--verbose',
    ...mcpArgs,
    resume ? '--resume' : '--session-id',
    effectiveSessionId,
  ];
  if (systemPrompt) {
    args.push('--system-prompt', systemPrompt);
  }

  const spec = processSpec(command, args);
  const child = spawn(spec.command, spec.args, {
    detached: process.platform !== 'win32',
    stdio: ['pipe', 'pipe', 'pipe'],
    windowsHide: true,
  });
  logFlow('info', 'agent.process_spawn', {
    agent_id: agentId,
    conversation_id: conversationId,
    user_id: userId,
    command: spec.command,
    args_count: spec.args.length,
    session_id: effectiveSessionId,
    resume,
    mcp_enabled: mcpArgs.length > 0,
    system_prompt_len: typeof systemPrompt === 'string' ? systemPrompt.length : 0,
    pid: child.pid,
  });

  let stdoutBuf = '';
  let resultResolver = null;

  child.stdout.setEncoding('utf8');
  child.stdout.on('data', (chunk) => {
    stdoutBuf += chunk;
    const lines = stdoutBuf.split('\n');
    stdoutBuf = lines.pop();
    for (const line of lines) {
      if (!line.trim()) continue;
      try {
        const event = JSON.parse(line);
        if (event.type === 'assistant') {
          agentTurnStates.set(agentId, 'active');
        }
        if (event.type === 'result') {
          agentTurnStates.set(agentId, 'idle');
          const text = typeof event.result === 'string' ? event.result : JSON.stringify(event.result);
          logFlow(event.is_error || event.subtype === 'error_during_execution' ? 'warn' : 'info', 'agent.turn_result', {
            agent_id: agentId,
            conversation_id: conversationId,
            session_id: effectiveSessionId,
            is_error: Boolean(event.is_error || event.subtype === 'error_during_execution'),
            subtype: event.subtype,
            result_len: typeof text === 'string' ? text.length : 0,
          });
          if (resultResolver) {
            const r = resultResolver;
            resultResolver = null;
            if (event.is_error || event.subtype === 'error_during_execution') {
              r({ error: text || 'Agent execution failed' });
            } else {
              r({ result: text || '' });
            }
          }
        }
      } catch { /* ignore non-JSON lines */ }
    }
  });

  child.stderr.setEncoding('utf8');
  child.stderr.on('data', (chunk) => {
    logFlow('warn', 'agent.stderr', {
      agent_id: agentId,
      conversation_id: conversationId,
      session_id: effectiveSessionId,
      message: truncateStr(chunk.trim(), 500),
    });
  });

  // Reject any pending resultResolver if process exits mid-turn
  child.on('close', (code) => {
    const hadPendingTurn = Boolean(resultResolver);
    if (resultResolver) {
      const r = resultResolver;
      resultResolver = null;
      r({ error: `Agent process exited (code=${code})` });
    }
    agentTurnStates.delete(agentId);
    logFlow(code === 0 ? 'info' : 'warn', 'agent.process_close', {
      agent_id: agentId,
      conversation_id: conversationId,
      session_id: effectiveSessionId,
      pid: child.pid,
      exit_code: code,
      pending_turn: hadPendingTurn,
    });
  });

  let queueTail = Promise.resolve();
  const sendPromptRaw = (prompt) => new Promise((resolve, reject) => {
    if (child.exitCode !== null) {
      reject(new Error('Agent process not running'));
      return;
    }
    resultResolver = resolve;
    logFlow('info', 'agent.prompt_sent', {
      agent_id: agentId,
      conversation_id: conversationId,
      session_id: effectiveSessionId,
      prompt_len: typeof prompt === 'string' ? prompt.length : 0,
    });
    const msg = JSON.stringify({
      type: 'user',
      message: { role: 'user', content: [{ type: 'text', text: prompt }] },
    });
    child.stdin.write(msg + '\n');
    const timer = setTimeout(() => {
      if (resultResolver === resolve) {
        resultResolver = null;
        logFlow('error', 'agent.turn_timeout', {
          agent_id: agentId,
          conversation_id: conversationId,
          session_id: effectiveSessionId,
          timeout_ms: EXEC_TIMEOUT_MS,
        });
        reject(new Error('Agent task timed out (400s)'));
      }
    }, EXEC_TIMEOUT_MS);
    timer.unref(); // Don't keep event loop alive for timeout timer
  });

  const sendPrompt = (prompt) => {
    const run = () => sendPromptRaw(prompt);
    queueTail = queueTail.then(run, run);
    return queueTail;
  };

  return { child, sessionId: effectiveSessionId, sendPrompt };
}

/**
 * Unified claude dispatch: per-agent process slot with conversation isolation.
 * - Same conversation → stdin inject (fast path)
 * - Cross-conversation → kill + --resume restart
 * - No process → spawn fresh
 */
async function dispatchToClaudeSlot(ws, agentId, conversationId, userId, prompt, systemPrompt) {
  const sessionKey = `${agentId}:${conversationId}`;
  const savedSessionId = conversationSessions.get(sessionKey) || null;
  const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
  const validSessionId = savedSessionId && UUID_RE.test(savedSessionId) ? savedSessionId : null;
  const slot = runningAgents.get(agentId);

  // Fast path: same conversation or unbound agent (null = accept any), process running
  if (slot?.sendPrompt && (slot.currentConversationId === conversationId || slot.currentConversationId === null)) {
    if (slot.currentConversationId === null) {
      logFlow('info', 'agent.dispatch_fast_path', {
        agent_id: agentId,
        conversation_id: conversationId,
        previous_conversation_id: 'unbound',
        session_id: slot.sessionId,
      });
      slot.currentConversationId = conversationId;
    } else {
      logFlow('info', 'agent.dispatch_fast_path', {
        agent_id: agentId,
        conversation_id: conversationId,
        previous_conversation_id: slot.currentConversationId,
        session_id: slot.sessionId,
      });
    }
    const response = await slot.sendPrompt(prompt);
    if (response.error) throw new Error(response.error);
    return response.result;
  }

  // 如果已有持久进程在运行且属于同一会话，直接复用（fast path）。
  // 不同对话也复用同一进程——stream-json 模式支持多 turn，
  // 杀进程会导致其他对话的任务失败。
  if (slot?.sendPrompt) {
    logFlow('info', 'agent.reuse_slot', {
      agent_id: agentId,
      conversation_id: conversationId,
      current_conversation_id: slot.currentConversationId,
      same_conv: slot.currentConversationId === conversationId,
    });
    // 更新当前对话 ID（用于日志追踪）
    slot.currentConversationId = conversationId;
    // 复用已有进程执行 prompt
    const response = await slot.sendPrompt(userPrompt);
    if (response.error) throw new Error(response.error);
    return response.result;
  }

  // 没有已有进程——spawn 新的
  let result;
  if (validSessionId) {
    try {
      result = spawnStreamJsonProcess(agentId, validSessionId, systemPrompt, true, conversationId, userId);
      // Wait briefly to detect immediate resume failure
      await sleep(2000);
      if (result.child.exitCode !== null) {
        throw new Error('Resume failed');
      }
    } catch {
      // Resume failed — spawn fresh with new session ID
      logFlow('warn', 'agent.resume_failed', {
        agent_id: agentId,
        conversation_id: conversationId,
        session_id: validSessionId,
      });
      result = spawnStreamJsonProcess(agentId, null, systemPrompt, false, conversationId, userId);
    }
  } else {
    result = spawnStreamJsonProcess(agentId, null, systemPrompt, false, conversationId, userId);
  }

  const { child, sessionId, sendPrompt } = result;

  // Handle process exit
  child.on('close', (code) => {
    const entry = runningAgents.get(agentId);
    if (entry?.process === child) {
      runningAgents.delete(agentId);
    }
    agentTurnStates.delete(agentId);
    logFlow(code === 0 ? 'info' : 'warn', 'agent.process_exit', {
      agent_id: agentId,
      conversation_id: conversationId,
      session_id: sessionId,
      pid: child.pid,
      exit_code: code,
    });
    safeSend(currentDaemonWs, JSON.stringify({ type: 'agent.stopped', data: { agent_id: agentId, exit_code: code } }));
  });

  // Register in runningAgents with conversation tracking
  runningAgents.set(agentId, {
    process: child,
    sessionId,
    currentConversationId: conversationId,
    cliTool: 'claude',
    sendPrompt,
  });
  idleAgentConfigs.set(agentId, { cliTool: 'claude', sessionId, systemPrompt: systemPrompt || '' });
  agentTurnStates.set(agentId, 'idle');

  // Persist session mapping
  conversationSessions.set(sessionKey, sessionId);
  saveSessionMap();

  logFlow('info', 'agent.slot_spawned', {
    agent_id: agentId,
    conversation_id: conversationId,
    session_id: sessionId,
    pid: child.pid,
    resumed: Boolean(validSessionId),
  });

  // Send the prompt
  const response = await sendPrompt(prompt);
  if (response.error) throw new Error(response.error);
  return response.result;
}

function enqueueAgentStart(ws, payload) {
  agentStartQueue.push({ ws, payload });
  logFlow('info', 'agent.start_queued', {
    agent_id: payload && payload.agent_id,
    cli_tool: payload && payload.cli_tool,
    queue_len: agentStartQueue.length,
  });
  processStartQueue();
}

function processStartQueue() {
  if (agentStartQueue.length === 0) return;
  const now = Date.now();
  const elapsed = now - lastAgentStartAt;
  if (elapsed < START_QUEUE_INTERVAL_MS) {
    setTimeout(processStartQueue, START_QUEUE_INTERVAL_MS - elapsed);
    return;
  }
  const item = agentStartQueue.shift();
  if (item) {
    lastAgentStartAt = Date.now();
    handleAgentStart(item.ws, item.payload);
    if (agentStartQueue.length > 0) {
      setTimeout(processStartQueue, START_QUEUE_INTERVAL_MS);
    }
  }
}

async function handleAgentStart(ws, payload) {
  const { agent_id, cli_tool, system_prompt } = payload;
  if (!agent_id || !cli_tool) return;

  logFlow('info', 'agent.start_requested', {
    agent_id,
    cli_tool,
    system_prompt_len: typeof system_prompt === 'string' ? system_prompt.length : 0,
  });
  stopAgentProcess(agent_id);

  if (cli_tool !== 'claude') {
    logFlow('warn', 'agent.start_unsupported', { agent_id, cli_tool });
    safeSend(ws, JSON.stringify({ type: 'agent.started', data: { agent_id, error: `${cli_tool} does not support persistent mode` } }));
    return;
  }

  try {
    const result = spawnStreamJsonProcess(agent_id, null, system_prompt, false);
    const { child, sessionId, sendPrompt } = result;

    // Wait briefly to detect immediate startup failure (same pattern as dispatchToClaudeSlot)
    await sleep(2000);
    if (child.exitCode !== null) {
      throw new Error(`Agent process exited immediately (code=${child.exitCode})`);
    }

    child.on('error', (err) => {
      logFlow('error', 'agent.process_error', { agent_id, cli_tool, session_id: sessionId, pid: child.pid, error: errorMessage(err) });
    });

    // Handle process exit — cleanup and notify backend
    child.on('close', (code) => {
      if (runningAgents.get(agent_id)?.process === child) {
        runningAgents.delete(agent_id);
      }
      agentTurnStates.delete(agent_id);
      logFlow(code === 0 ? 'info' : 'warn', 'agent.process_exit', {
        agent_id,
        cli_tool,
        session_id: sessionId,
        pid: child.pid,
        exit_code: code,
      });
      safeSend(ws, JSON.stringify({ type: 'agent.stopped', data: { agent_id, exit_code: code } }));
    });

    runningAgents.set(agent_id, {
      process: child,
      sessionId,
      cliTool: cli_tool,
      sendPrompt,
      currentConversationId: null,
    });
    idleAgentConfigs.set(agent_id, { cliTool: cli_tool, sessionId, systemPrompt: system_prompt || '' });
    agentTurnStates.set(agent_id, 'idle');

    logFlow('info', 'agent.started', { agent_id, cli_tool, session_id: sessionId, pid: child.pid });
    safeSend(ws, JSON.stringify({ type: 'agent.started', data: { agent_id } }));
  } catch (error) {
    logFlow('error', 'agent.start_failed', { agent_id, cli_tool, error: errorMessage(error) });
    safeSend(ws, JSON.stringify({ type: 'agent.started', data: { agent_id, error: errorMessage(error) } }));
  }
}

function handleAgentStop(ws, payload) {
  const { agent_id } = payload;
  if (!agent_id) return;
  stopAgentProcess(agent_id);
  runningAgents.delete(agent_id);
  idleAgentConfigs.delete(agent_id);
  agentTurnStates.delete(agent_id);
  logFlow('info', 'agent.stopped', { agent_id });
  safeSend(ws, JSON.stringify({ type: 'agent.stopped', data: { agent_id } }));
}

function handleAgentRestart(ws, payload) {
  handleAgentStop(ws, payload);
  handleAgentStart(ws, payload);
}

// ---------------------------------------------------------------------------
// task.dispatch 处理器（从 connectWS 内联代码提取）
// ---------------------------------------------------------------------------

async function handleTaskDispatch(ws, data) {
  const task = {
    id: data.task_id,
    cli_tool: data.cli_tool,
    prompt: data.prompt,
    context_messages: data.context_messages,
    agent_id: data.agent_id,
    conversation_id: data.conversation_id,
    user_id: data.user_id,
  };
  if (!task.id) {
    logFlow('warn', 'task.dispatch_invalid', { reason: 'missing task_id' });
    return true;
  }

  const { systemPrompt, userPrompt } = buildPromptParts(task);

  logFlow('info', 'task.dispatch_received', {
    task_id: task.id,
    cli_tool: task.cli_tool || 'unknown',
    agent_id: task.agent_id,
    conversation_id: task.conversation_id,
    user_id: task.user_id,
    prompt_len: typeof task.prompt === 'string' ? task.prompt.length : 0,
    context_len: typeof task.context_messages === 'string' ? task.context_messages.length : 0,
    system_prompt_len: systemPrompt.length,
    user_prompt_len: userPrompt.length,
  });
  try {
    let result;
    if (
      task.cli_tool === 'claude' &&
      task.agent_id &&
      task.conversation_id &&
      process.env.AGENTHUB_DAEMON_DISABLE_STREAM_SLOT !== '1'
    ) {
      logFlow('info', 'task.execution_start', {
        task_id: task.id,
        cli_tool: task.cli_tool,
        agent_id: task.agent_id,
        conversation_id: task.conversation_id,
        mode: 'claude_slot',
      });
      result = await dispatchToClaudeSlot(ws, task.agent_id, task.conversation_id, task.user_id, userPrompt, systemPrompt);
    } else {
      logFlow('info', 'task.execution_start', {
        task_id: task.id,
        cli_tool: task.cli_tool,
        agent_id: task.agent_id,
        conversation_id: task.conversation_id,
        mode: 'legacy_spawn',
      });
      result = await executeTaskOnce(task);
      if (result === null) return true;
    }
    const artifacts = parseArtifacts(result);
    bus.emit('task.completed', {
      task_id: task.id,
      cli_tool: task.cli_tool,
      agent_id: task.agent_id,
      conversation_id: task.conversation_id,
      result,
      artifacts,
    });
  } catch (error) {
    bus.emit('task.failed', {
      task_id: task.id,
      cli_tool: task.cli_tool,
      agent_id: task.agent_id,
      conversation_id: task.conversation_id,
      error: error instanceof Error ? error.message : String(error),
    });
  }
  return true;
}

// ---------------------------------------------------------------------------
// 注册 WS 消息处理器——新增消息类型只需在此处加一行 registerWsHandler。
// ---------------------------------------------------------------------------

registerWsHandler('pong', () => true); // no-op
registerWsHandler('ping', (ws) => { safeSend(ws, JSON.stringify({ type: 'pong' })); return true; });
registerWsHandler('agent.start', (ws, data) => {
  bus.emit('agent.start_request', { ws, data });
  return true;
});
registerWsHandler('agent.stop', (ws, data) => {
  bus.emit('agent.stop_request', { ws, data });
  return true;
});
registerWsHandler('agent.restart', (ws, data) => {
  bus.emit('agent.restart_request', { ws, data });
  return true;
});
registerWsHandler('task.dispatch', (ws, data) => {
  bus.emit('task.dispatch', { ws, data });
  return true;
});

// ---------------------------------------------------------------------------
// 事件订阅者——业务逻辑与 WS 接收解耦
// ---------------------------------------------------------------------------

// agent 生命周期：start/stop/restart 请求 → 实际执行
bus.on('agent.start_request', ({ ws, data }) => {
  logFlow('info', 'ws.control_received', { type: 'agent.start', agent_id: data.agent_id, cli_tool: data.cli_tool });
  enqueueAgentStart(ws, data);
});
bus.on('agent.stop_request', ({ ws, data }) => {
  logFlow('info', 'ws.control_received', { type: 'agent.stop', agent_id: data.agent_id });
  handleAgentStop(ws, data);
});
bus.on('agent.restart_request', ({ ws, data }) => {
  logFlow('info', 'ws.control_received', { type: 'agent.restart', agent_id: data.agent_id, cli_tool: data.cli_tool });
  handleAgentRestart(ws, data);
});

// task 执行：dispatch → execute → completed/failed
bus.on('task.dispatch', async ({ ws, data }) => {
  await handleTaskDispatch(ws, data);
});

// task 结果回传——独立于执行逻辑，方便未来加重试/metrics
bus.on('task.completed', (info) => {
  sendTaskComplete({
    task_id: info.task_id,
    result: info.result,
    artifacts: info.artifacts,
  });
});
bus.on('task.failed', (info) => {
  sendTaskComplete({
    task_id: info.task_id,
    error: info.error,
  });
});

async function connectWS(serverURL, apiKey) {
  if (!WebSocket) {
    logFlow('warn', 'ws.unavailable_fallback_to_polling', {
      reason: 'WebSocket not available. Please install ws or use Node.js 22+',
    });
    return pollTasks(serverURL, apiKey);
  }

  const url = new URL(serverURL);
  const wsPath = `${url.pathname.replace(/\/$/, '')}/daemon/ws`;
  const protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsURL = `${protocol}//${url.host}${wsPath}?token=${encodeURIComponent(apiKey)}`;

  let reconnectAttempts = 0;

  function connect() {
    logFlow('info', 'ws.connect_start', {
      server: `${protocol}//${url.host}${wsPath}`,
      reconnect_attempt: reconnectAttempts,
      machine_id: os.hostname(),
    });
    const ws = new WebSocket(wsURL);
    let pingTimer = null;
    let watchdogTimer = null;

    function resetWatchdog() {
      if (watchdogTimer) clearTimeout(watchdogTimer);
      watchdogTimer = setTimeout(() => {
        logFlow('warn', 'ws.inbound_watchdog_timeout', {
          timeout_ms: INBOUND_WATCHDOG_MS,
          machine_id: os.hostname(),
        });
        try { ws.close(); } catch { /* ignore */ }
      }, INBOUND_WATCHDOG_MS);
    }

    onWebSocket(ws, 'open', () => {
      currentDaemonWs = ws;
      reconnectAttempts = 0;
      logFlow('info', 'ws.connected', { machine_id: os.hostname(), server: `${protocol}//${url.host}${wsPath}` });
      resetWatchdog();
      // Send register message over WS
      const agents = scanAgents();
      ws.send(JSON.stringify({
        type: 'daemon.register',
        data: { machine_id: os.hostname(), agents },
      }));
      logFlow('info', 'ws.register_sent', {
        machine_id: os.hostname(),
        agent_count: agents.length,
        agents: agents.map((agent) => `${agent.name}:${agent.cli_tool}`),
      });
      flushPendingTaskCompletions();
      // Start ping interval
      pingTimer = setInterval(() => {
        if (ws.readyState === WebSocket.OPEN) {
          ws.send(JSON.stringify({ type: 'ping' }));
        }
      }, WS_PING_INTERVAL_MS);
    });

    onWebSocket(ws, 'message', async (data) => {
      resetWatchdog();
      let envelope;
      try {
        envelope = JSON.parse(data.toString());
      } catch {
        logFlow('warn', 'ws.message_parse_failed', { bytes: Buffer.byteLength(data) });
        return;
      }

      if (dispatchWsMessage(ws, envelope)) return;

      logFlow('warn', 'ws.unknown_message', { type: envelope.type });
    });

    onWebSocket(ws, 'close', (code, reason) => {
      if (currentDaemonWs === ws) currentDaemonWs = null;
      if (pingTimer) clearInterval(pingTimer);
      if (watchdogTimer) clearTimeout(watchdogTimer);
      // Keep running agent processes alive across short control-channel reconnects.
      // In-flight task completions are buffered and flushed after reconnect.
      reconnectAttempts += 1;
      logFlow('warn', 'ws.closed', {
        code,
        reason: reason ? reason.toString() : '',
        reconnect_attempt: reconnectAttempts,
        retry_ms: WS_RECONNECT_DELAY_MS,
        running_agent_count: runningAgents.size,
        pending_completion_count: pendingTaskCompletions.size,
      });
      setTimeout(connect, WS_RECONNECT_DELAY_MS);
    });

    onWebSocket(ws, 'error', (error) => {
      logFlow('error', 'ws.error', { error: errorMessage(error), reconnect_attempt: reconnectAttempts });
      // close handler will trigger reconnect
    });
  }

  connect();

  // Keep process alive
  return new Promise(() => {});
}

// ── MCP 模式 ──────────────────────────────────────────────────────────────
// 以 stdio JSON-RPC（换行分隔）对外暴露一个 agenthub-platform MCP server，
// 让本机 Agent（Claude Code / Codex 等）可通过 MCP 工具操作 AgentHub 平台。
// 协议要求 stdout 只承载 JSON-RPC 报文，因此本模式所有日志改走 stderr。

const MCP_PROTOCOL_VERSION = '2024-11-05';

let cachedAgentJWT = null;
let cachedAgentJWTExp = 0;

// getAgentJWT 用机器 api-key 向后端换取 agent_management scoped JWT，并缓存到临近过期。
async function getAgentJWT(serverURL, apiKey, force) {
  const now = Date.now();
  if (!force && cachedAgentJWT && now < cachedAgentJWTExp - 30000) {
    return cachedAgentJWT;
  }
  const res = await requestJSON('GET', apiURL(serverURL, apiKey, '/daemon/agent-token'));
  const data = res && res.data ? res.data : res;
  if (!data || !data.token) {
    throw new Error('agent-token 响应缺少 token');
  }
  cachedAgentJWT = data.token;
  cachedAgentJWTExp = data.expires_at ? Date.parse(data.expires_at) : now + 4 * 60 * 1000;
  return cachedAgentJWT;
}

// apiURLWithToken 用指定 JWT 作为 token query 调用 /api（鉴权中间件支持 query token）。
function apiURLWithToken(serverURL, jwt, pathname, query) {
  const url = new URL(serverURL);
  url.pathname = `${url.pathname.replace(/\/$/, '')}${pathname}`;
  url.searchParams.set('token', jwt);
  if (query) {
    for (const [key, value] of Object.entries(query)) {
      if (value !== undefined && value !== null) url.searchParams.set(key, String(value));
    }
  }
  url.hash = '';
  return url;
}

// callApi 携带 scoped JWT 调用平台 REST API，401 时刷新一次 token 重试。
async function callApi(serverURL, apiKey, method, pathname, options = {}) {
  let lastError;
  for (let attempt = 0; attempt < 2; attempt += 1) {
    const jwt = await getAgentJWT(serverURL, apiKey, attempt === 1);
    const url = apiURLWithToken(serverURL, jwt, pathname, options.query);
    try {
      return await requestJSON(method, url, options.body);
    } catch (error) {
      lastError = error;
      if (attempt === 0 && /HTTP 401/.test(error.message)) {
        cachedAgentJWT = null;
        continue;
      }
      throw error;
    }
  }
  throw lastError;
}

// callMcpApi 用 daemon token（非 JWT、非 machine API key）调用 /mcp/... 端点。
// daemon token 从 CLI --daemon-token 或环境变量获取，与后端 config daemon.token 一致。
async function callMcpApi(serverURL, daemonToken, method, pathname, options = {}, userId) {
  const url = new URL(serverURL);
  url.pathname = `${url.pathname.replace(/\/$/, '')}${pathname}`;
  if (options.query) {
    for (const [key, value] of Object.entries(options.query)) {
      if (value !== undefined && value !== null) url.searchParams.set(key, String(value));
    }
  }
  if (userId) url.searchParams.set('user_id', userId);
  return requestJSON(method, url, options.body, daemonToken);
}

const MCP_TOOLS = [
  {
    name: 'list_conversations',
    description: '列出当前用户的所有会话（私聊与群聊）。',
    inputSchema: { type: 'object', properties: {}, additionalProperties: false },
    run: (args, ctx) => ctx.callApi('GET', '/api/conversations'),
  },
  {
    name: 'get_messages',
    description: '读取指定会话的历史消息，用于获取上下文。',
    inputSchema: {
      type: 'object',
      properties: {
        conversation_id: { type: 'string', description: '会话 ID' },
        limit: { type: 'integer', description: '返回条数，默认 50' },
      },
      required: ['conversation_id'],
      additionalProperties: false,
    },
    run: (args, ctx) => ctx.callApi(
      'GET',
      `/api/conversations/${encodeURIComponent(args.conversation_id)}/messages`,
      { query: { limit: args.limit || 50 } },
    ),
  },
  {
    name: 'create_group',
    description: '创建一个群聊。',
    inputSchema: {
      type: 'object',
      properties: {
        name: { type: 'string', description: '群名称（1-50 字符）' },
        member_ids: { type: 'array', items: { type: 'string' }, description: '初始成员用户 ID 列表（可选）' },
      },
      required: ['name'],
      additionalProperties: false,
    },
    run: (args, ctx) => ctx.callMcpApi('POST', '/mcp/groups', {
      body: { name: args.name, member_ids: args.member_ids || [] },
    }),
  },
  {
    name: 'list_agents',
    description: '列出当前用户可用的 Agent。',
    inputSchema: { type: 'object', properties: {}, additionalProperties: false },
    run: (args, ctx) => ctx.callMcpApi('GET', '/mcp/agents'),
  },
  {
    name: 'get_agent_skill',
    description: '查看当前 Agent 已分配平台 Skill 的详细内容。先根据提示词中的 Skill 索引选择 name，再调用本工具渐进加载 detail。',
    inputSchema: {
      type: 'object',
      properties: {
        name: { type: 'string', description: '平台 Skill 名称，必须属于当前 Agent' },
      },
      required: ['name'],
      additionalProperties: false,
    },
    run: async (args, ctx) => {
      const name = typeof args.name === 'string' ? args.name.trim() : '';
      if (!name) throw new Error('name is required');
      const agent = await resolveCurrentAgent(ctx);
      if (!agent) throw new Error('current agent not found');
      const skills = parsePlatformSkills(agent.custom_skills);
      const skill = skills.find((item) => item.name.toLowerCase() === name.toLowerCase());
      if (!skill) throw new Error(`skill not found for current agent: ${name}`);
      return skill;
    },
  },
  {
    name: 'list_agent_candidates',
    description: '列出当前用户电脑上扫描到的 Agent 底座候选。',
    inputSchema: { type: 'object', properties: {}, additionalProperties: false },
    run: (args, ctx) => ctx.callMcpApi('GET', '/mcp/daemon/agent-candidates'),
  },
  {
    name: 'list_conversation_agents',
    description: '列出指定会话中的智能体，用于了解当前会话可分派的 Agent。',
    inputSchema: {
      type: 'object',
      properties: {
        conversation_id: { type: 'string', description: '会话 ID（默认为当前会话）' },
      },
      additionalProperties: false,
    },
    run: async (args, ctx) => {
      const convId = args.conversation_id || ctx.conversationId || '';
      if (!convId) throw new Error('conversation_id is required (no conversation context)');
      const res = await ctx.callApi('GET', `/api/conversations/${encodeURIComponent(convId)}/agents`);
      if (!res || !Array.isArray(res.data)) return res;
      res.data = res.data.map(a => ({
        name: a.name,
        role: a.role,
        status: a.status,
        tags: a.tags || '',
      }));
      return res;
    },
  },
  {
    name: 'list_group_agents',
    description: '列出指定群聊中的智能体，包括名称、角色、状态和标签，用于 orchestrator 了解群内可分派 Agent。',
    inputSchema: {
      type: 'object',
      properties: {
        conversation_id: { type: 'string', description: '群聊会话 ID（默认为当前会话）' },
      },
      additionalProperties: false,
    },
    run: async (args, ctx) => {
      const convId = args.conversation_id || ctx.conversationId || '';
      const res = await ctx.callApi('GET', `/api/conversations/${encodeURIComponent(convId)}/agents`);
      if (!res || !Array.isArray(res.data)) return res;
      // 只返回 orch 需要的关键字段
      res.data = res.data.map(a => ({
        name: a.name,
        role: a.role,
        status: a.status,
        tags: a.tags || '',
      }));
      return res;
    },
  },
  // ── 任务管理（通过 /mcp/ 端点，daemon token 鉴权） ──
  {
    name: 'list_tasks',
    description: '列出任务。默认列出当前会话的任务，可指定 conversation_id 和 status 过滤。',
    inputSchema: {
      type: 'object',
      properties: {
        conversation_id: { type: 'string', description: '会话 ID（默认为当前会话）' },
        status: { type: 'string', description: '按状态过滤（todo/in_progress/done/cancelled）' },
      },
      additionalProperties: false,
    },
    run: (args, ctx) => {
      const query = {};
      query.conversation_id = args.conversation_id || ctx.conversationId || '';
      if (args.status) query.status = args.status;
      return ctx.callMcpApi('GET', '/mcp/tasks', { query });
    },
  },
  {
    name: 'create_task',
    description: '创建一个任务。默认关联到当前会话。',
    inputSchema: {
      type: 'object',
      properties: {
        title: { type: 'string', description: '任务标题' },
        description: { type: 'string', description: '任务描述（可选）' },
        conversation_id: { type: 'string', description: '会话 ID（默认为当前会话）' },
        assignee_id: { type: 'string', description: '指派给用户的 ID（可选）' },
        agent_id: { type: 'string', description: '关联的 Agent ID（可选）' },
        priority: { type: 'string', description: '优先级（low/medium/high，默认 medium）' },
      },
      required: ['title'],
      additionalProperties: false,
    },
    run: (args, ctx) => {
      const body = { title: args.title };
      body.conversation_id = args.conversation_id || ctx.conversationId || '';
      if (args.description) body.description = args.description;
      if (args.assignee_id) body.assignee_id = args.assignee_id;
      if (args.agent_id) body.agent_id = args.agent_id;
      if (args.priority) body.priority = args.priority;
      return ctx.callMcpApi('POST', '/mcp/tasks', { body });
    },
  },
  {
    name: 'update_task',
    description: '更新任务属性（标题、描述、优先级、指派人等）。',
    inputSchema: {
      type: 'object',
      properties: {
        id: { type: 'string', description: '任务 ID' },
        title: { type: 'string', description: '新标题' },
        description: { type: 'string', description: '新描述' },
        priority: { type: 'string', description: '优先级（low/medium/high）' },
        assignee_id: { type: 'string', description: '指派人 ID' },
        agent_id: { type: 'string', description: '关联 Agent ID' },
      },
      required: ['id'],
      additionalProperties: false,
    },
    run: (args, ctx) => {
      const body = {};
      if (args.title) body.title = args.title;
      if (args.description) body.description = args.description;
      if (args.priority) body.priority = args.priority;
      if (args.assignee_id) body.assignee_id = args.assignee_id;
      if (args.agent_id) body.agent_id = args.agent_id;
      return ctx.callMcpApi('PUT', `/mcp/tasks/${encodeURIComponent(args.id)}`, { body });
    },
  },
  {
    name: 'move_task_status',
    description: '移动任务状态。',
    inputSchema: {
      type: 'object',
      properties: {
        id: { type: 'string', description: '任务 ID' },
        status: { type: 'string', description: '目标状态（todo/in_progress/done/cancelled）' },
      },
      required: ['id', 'status'],
      additionalProperties: false,
    },
    run: (args, ctx) => ctx.callMcpApi(
      'POST',
      `/mcp/tasks/${encodeURIComponent(args.id)}/status`,
      { body: { status: args.status } },
    ),
  },
  {
    name: 'delete_task',
    description: '删除一个任务。',
    inputSchema: {
      type: 'object',
      properties: {
        id: { type: 'string', description: '任务 ID' },
      },
      required: ['id'],
      additionalProperties: false,
    },
    run: (args, ctx) => ctx.callMcpApi('DELETE', `/mcp/tasks/${encodeURIComponent(args.id)}`),
  },
  // ── 群组信息 ──
  {
    name: 'get_group_info',
    description: '获取群聊信息。默认获取当前会话对应的群组。',
    inputSchema: {
      type: 'object',
      properties: {
        group_id: { type: 'string', description: '群组 ID（默认为当前会话 ID）' },
      },
      additionalProperties: false,
    },
    run: (args, ctx) => {
      const gid = args.group_id || ctx.conversationId || '';
      if (!gid) throw new Error('group_id is required (no conversation context)');
      return ctx.callMcpApi('GET', `/mcp/groups/${encodeURIComponent(gid)}`);
    },
  },
  {
    name: 'list_group_members',
    description: '列出群聊成员。默认列出当前会话对应群组的成员。',
    inputSchema: {
      type: 'object',
      properties: {
        group_id: { type: 'string', description: '群组 ID（默认为当前会话 ID）' },
      },
      additionalProperties: false,
    },
    run: (args, ctx) => {
      const gid = args.group_id || ctx.conversationId || '';
      if (!gid) throw new Error('group_id is required (no conversation context)');
      return ctx.callMcpApi('GET', `/mcp/groups/${encodeURIComponent(gid)}/members`);
    },
  },
  {
    name: 'list_machines',
    description: '列出当前用户连接的电脑（daemon 机器）。',
    inputSchema: { type: 'object', properties: {}, additionalProperties: false },
    run: (args, ctx) => ctx.callMcpApi('GET', '/mcp/daemon/machines'),
  },
  // ── Agent 管理 ──
  {
    name: 'get_agent_detail',
    description: '查询单个 Agent 的完整详情，包括名称、类型、CLI 工具、系统提示词、工具配置、状态、版本、机器名称等。',
    inputSchema: {
      type: 'object',
      properties: {
        agent_id: { type: 'string', description: 'Agent ID' },
      },
      required: ['agent_id'],
      additionalProperties: false,
    },
    run: (args, ctx) => ctx.callMcpApi('GET', `/mcp/agents/${encodeURIComponent(args.agent_id)}`),
  },
  {
    name: 'update_agent_prompt',
    description: '更新 Agent 的系统提示词。会先获取当前完整信息，再只修改 system_prompt 字段。',
    inputSchema: {
      type: 'object',
      properties: {
        agent_id: { type: 'string', description: 'Agent ID' },
        system_prompt: { type: 'string', description: '新的系统提示词' },
      },
      required: ['agent_id', 'system_prompt'],
      additionalProperties: false,
    },
    run: async (args, ctx) => {
      const agentId = args.agent_id;
      if (!agentId) throw new Error('agent_id is required');
      const systemPrompt = args.system_prompt;
      if (!systemPrompt) throw new Error('system_prompt is required');
      // 先获取当前完整信息
      const res = await ctx.callMcpApi('GET', `/mcp/agents`);
      const agents = res && Array.isArray(res.data) ? res.data : Array.isArray(res) ? res : [];
      const agent = agents.find((a) => a && a.id === agentId);
      if (!agent) throw new Error(`agent not found: ${agentId}`);
      // 只改 system_prompt，其他字段原样传回
      return ctx.callMcpApi('PUT', `/mcp/agents/${encodeURIComponent(agentId)}`, {
        body: {
          name: agent.name,
          cli_tool: agent.cli_tool,
          system_prompt: systemPrompt,
          tools_config: agent.tools_config,
          capabilities_json: agent.capabilities_json,
          custom_skills: agent.custom_skills,
          enable_management_tools: agent.enable_management_tools,
        },
      });
    },
  },
  {
    name: 'start_agent',
    description: '启动指定的 Agent。',
    inputSchema: {
      type: 'object',
      properties: {
        agent_id: { type: 'string', description: 'Agent ID' },
      },
      required: ['agent_id'],
      additionalProperties: false,
    },
    run: (args, ctx) => ctx.callMcpApi('POST', `/mcp/agents/${encodeURIComponent(args.agent_id)}/start`),
  },
  {
    name: 'stop_agent',
    description: '停止指定的 Agent。',
    inputSchema: {
      type: 'object',
      properties: {
        agent_id: { type: 'string', description: 'Agent ID' },
      },
      required: ['agent_id'],
      additionalProperties: false,
    },
    run: (args, ctx) => ctx.callMcpApi('POST', `/mcp/agents/${encodeURIComponent(args.agent_id)}/stop`),
  },
  // ── 知识库 ──
  {
    name: 'list_knowledge_bases',
    description: '列出当前用户的知识库，包含 ID、名称、描述、可见性、文件数量等信息。',
    inputSchema: { type: 'object', properties: {}, additionalProperties: false },
    run: (args, ctx) => ctx.callMcpApi('GET', '/mcp/knowledge-bases'),
  },
  {
    name: 'list_knowledge_files',
    description: '列出指定知识库中的文件，包含文件名、大小、类型、预览文本等信息。',
    inputSchema: {
      type: 'object',
      properties: {
        knowledge_base_id: { type: 'string', description: '知识库 ID' },
      },
      required: ['knowledge_base_id'],
      additionalProperties: false,
    },
    run: (args, ctx) => ctx.callMcpApi('GET', `/mcp/knowledge-bases/${encodeURIComponent(args.knowledge_base_id)}/files`),
  },
  {
    name: 'search_knowledge',
    description: '在指定知识库中按关键词搜索文件，基于文件的 preview_text 字段进行匹配过滤。',
    inputSchema: {
      type: 'object',
      properties: {
        knowledge_base_id: { type: 'string', description: '知识库 ID' },
        keyword: { type: 'string', description: '搜索关键词' },
      },
      required: ['knowledge_base_id', 'keyword'],
      additionalProperties: false,
    },
    run: async (args, ctx) => {
      const kbId = args.knowledge_base_id;
      const keyword = args.keyword;
      if (!kbId) throw new Error('knowledge_base_id is required');
      if (!keyword) throw new Error('keyword is required');
      const res = await ctx.callMcpApi('GET', `/mcp/knowledge-bases/${encodeURIComponent(kbId)}/files`);
      const files = res && Array.isArray(res.data) ? res.data : Array.isArray(res) ? res : [];
      const keywordLower = keyword.toLowerCase();
      return files.filter((f) => {
        const preview = typeof f.preview_text === 'string' ? f.preview_text : '';
        return preview.toLowerCase().includes(keywordLower);
      });
    },
  },
  {
    name: 'read_knowledge_file',
    description: '读取知识库文件的抽取文本内容。适合在搜索命中文件后按 file_id 获取完整可用上下文。',
    inputSchema: {
      type: 'object',
      properties: {
        knowledge_base_id: { type: 'string', description: '知识库 ID' },
        file_id: { type: 'string', description: '文件 ID' },
      },
      required: ['knowledge_base_id', 'file_id'],
      additionalProperties: false,
    },
    run: async (args, ctx) => {
      const kbId = args.knowledge_base_id;
      const fileId = args.file_id;
      if (!kbId) throw new Error('knowledge_base_id is required');
      if (!fileId) throw new Error('file_id is required');
      return ctx.callMcpApi('GET', `/mcp/knowledge-bases/${encodeURIComponent(kbId)}/files/${encodeURIComponent(fileId)}/text`);
    },
  },
  // ── 平台 Skills ──
  {
    name: 'list_platform_skills',
    description: '列出所有平台 Skill，包含名称、分类、描述和触发场景，用于为 Agent 分配 Skill。',
    inputSchema: { type: 'object', properties: {}, additionalProperties: false },
    run: async (args, ctx) => {
      const res = await ctx.callMcpApi('GET', '/mcp/platform-skills');
      const skills = res && Array.isArray(res.data) ? res.data : Array.isArray(res) ? res : [];
      return skills.map((s) => ({ name: s.name, category: s.category, description: s.description, trigger: s.trigger }));
    },
  },
  // ── Agent 自建 ──
  {
    name: 'create_agent',
    description: '创建自建 Agent。需要提供名称和系统提示词，可选指定工具模板、CLI 工具和标签。',
    inputSchema: {
      type: 'object',
      properties: {
        name: { type: 'string', description: 'Agent 名称（必填）' },
        system_prompt: { type: 'string', description: '系统提示词（必填）' },
        toolset: { type: 'string', description: '工具模板名（none/basic/tasks/orchestrator/agent_builder/agent_manager/knowledge），默认 none' },
        cli_tool: { type: 'string', description: 'CLI 工具名，默认 claude' },
        tags: { type: 'string', description: '标签' },
      },
      required: ['name', 'system_prompt'],
      additionalProperties: false,
    },
    run: async (args, ctx) => {
      const name = args.name;
      if (!name) throw new Error('name is required');
      const systemPrompt = args.system_prompt;
      if (!systemPrompt) throw new Error('system_prompt is required');
      const cliTool = args.cli_tool || 'claude';
      const toolset = args.toolset || 'none';
      const tpl = TOOLSET_TEMPLATES[toolset] || [];
      const toolsConfig = JSON.stringify({ toolset, allowed_tools: tpl });
      const body = { name, cli_tool: cliTool, system_prompt: systemPrompt, tools_config: toolsConfig };
      if (args.tags) body.tags = args.tags;
      const candidatesRes = await ctx.callMcpApi('GET', '/mcp/daemon/agent-candidates');
      const candidates = candidatesRes && Array.isArray(candidatesRes.data) ? candidatesRes.data : Array.isArray(candidatesRes) ? candidatesRes : [];
      const candidate = candidates.find((item) => item && item.cli_tool === cliTool);
      if (candidate && candidate.id) {
        return ctx.callMcpApi('POST', `/mcp/daemon/agent-candidates/${encodeURIComponent(candidate.id)}/add`, { body });
      }
      return ctx.callMcpApi('POST', '/mcp/agents', { body });
    },
  },
  {
    name: 'update_agent',
    description: '更新 Agent 配置，只改传入的字段。可修改名称、系统提示词、工具模板、自定义工具列表、平台 Skill 分配和标签。',
    inputSchema: {
      type: 'object',
      properties: {
        agent_id: { type: 'string', description: 'Agent ID（必填）' },
        name: { type: 'string', description: '新名称' },
        system_prompt: { type: 'string', description: '新系统提示词' },
        toolset: { type: 'string', description: '切换工具模板' },
        allowed_tools: { type: 'array', items: { type: 'string' }, description: '自定义工具列表' },
        skills: { type: 'array', items: { type: 'string' }, description: '平台 Skill 名称列表，传入后覆盖当前 Agent 的 Skill 分配。传空数组清空所有 Skill' },
        tags: { type: 'string', description: '新标签' },
      },
      required: ['agent_id'],
      additionalProperties: false,
    },
    run: async (args, ctx) => {
      const agentId = args.agent_id;
      if (!agentId) throw new Error('agent_id is required');
      // Fetch current agent
      const res = await ctx.callMcpApi('GET', `/mcp/agents/${encodeURIComponent(agentId)}`);
      const agent = res && res.data ? res.data : res;
      if (!agent || typeof agent !== 'object') throw new Error(`agent not found: ${agentId}`);
      const body = {
        name: agent.name,
        cli_tool: agent.cli_tool,
        system_prompt: agent.system_prompt,
        tools_config: agent.tools_config,
        capabilities_json: agent.capabilities_json,
        custom_skills: agent.custom_skills,
        enable_management_tools: agent.enable_management_tools,
      };
      // Override only provided fields
      if (args.name) body.name = args.name;
      if (args.system_prompt) body.system_prompt = args.system_prompt;
      if (args.tags) body.tags = args.tags;
      if (args.toolset) {
        const tpl = TOOLSET_TEMPLATES[args.toolset] || [];
        body.tools_config = JSON.stringify({ toolset: args.toolset, allowed_tools: tpl });
      }
      if (Array.isArray(args.allowed_tools) && args.allowed_tools.length > 0) {
        const tools = args.allowed_tools.filter((t) => typeof t === 'string' && t);
        if (tools.length > 0) {
          body.tools_config = JSON.stringify({ toolset: '', allowed_tools: tools });
        }
      }
      // Skills: fetch platform skills, filter by name, build custom_skills
      if (Array.isArray(args.skills)) {
        const skillNames = new Set(args.skills.filter((s) => typeof s === 'string' && s));
        const psRes = await ctx.callMcpApi('GET', '/mcp/platform-skills');
        const allSkills = psRes && Array.isArray(psRes.data) ? psRes.data : Array.isArray(psRes) ? psRes : [];
        const matched = allSkills.filter((s) => skillNames.has(s.name));
        body.custom_skills = JSON.stringify(matched);
      }
      return ctx.callMcpApi('PUT', `/mcp/agents/${encodeURIComponent(agentId)}`, { body });
    },
  },
  {
    name: 'delete_agent',
    description: '删除自建 Agent。',
    inputSchema: {
      type: 'object',
      properties: {
        agent_id: { type: 'string', description: 'Agent ID（必填）' },
      },
      required: ['agent_id'],
      additionalProperties: false,
    },
    run: (args, ctx) => ctx.callMcpApi('DELETE', `/mcp/agents/${encodeURIComponent(args.agent_id)}`),
  },
  {
    name: 'list_toolsets',
    description: '列出可用的工具模板及其描述，用于创建或更新 Agent 时选择合适的工具配置。',
    inputSchema: { type: 'object', properties: {}, additionalProperties: false },
    run: () => [
      { name: 'none', label: '无工具', description: '不分配任何平台工具' },
      { name: 'basic', label: '基础群聊', description: '包含群 Agent 列表、消息读取、Skill 查看等基础工具' },
      { name: 'tasks', label: '任务协作', description: '包含任务看板的完整增删改查能力' },
      { name: 'orchestrator', label: 'Orchestrator', description: '编排器模板，包含会话、任务、群组管理和知识库搜索' },
      { name: 'agent_builder', label: 'Agent 创建', description: 'Agent 发现、详情查询、创建和更新工具' },
      { name: 'agent_manager', label: 'Agent 管理', description: 'Agent 详情、配置更新、提示词修改、启停和删除' },
      { name: 'knowledge', label: '知识库', description: '知识库列表、文件列表和关键词搜索' },
    ],
  },
  {
    name: 'deploy_artifact',
    description: '将当前会话中的 artifact（代码/网页/文档）部署为可公开访问的预览页面。通过内网穿透(tunnel)生成临时公网 URL。不指定 artifact_name 时部署最新 artifact。注意：webpage 类型的 artifact 需要包含完整的 HTML 内容（content 字段）才能正确部署预览，仅包含 localhost URL 的产物无法通过公网访问。',
    inputSchema: {
      type: 'object',
      properties: {
        artifact_name: {
          type: 'string',
          description: '要部署的 artifact 名称（匹配 filename 或 title），不指定则部署最新',
        },
      },
      additionalProperties: false,
    },
    run: async (args, ctx) => {
      return ctx.callApi('POST', '/api/deployments/deploy', {
        body: {
          conversation_id: ctx.conversationId,
          artifact_name: args.artifact_name || '',
          mode: 'preview',
        },
      });
    },
  },
  {
    name: 'deploy_artifact_github',
    description: '将 artifact 永久发布到 GitHub Pages。需要后端配置 GitHub Token。不指定 artifact_name 时部署最新 artifact。',
    inputSchema: {
      type: 'object',
      properties: {
        artifact_name: {
          type: 'string',
          description: '要发布的 artifact 名称（匹配 filename 或 title），不指定则发布最新',
        },
      },
      additionalProperties: false,
    },
    run: async (args, ctx) => {
      return ctx.callApi('POST', '/api/deployments/deploy', {
        body: {
          conversation_id: ctx.conversationId,
          artifact_name: args.artifact_name || '',
          mode: 'github',
        },
      });
    },
  },
];

const DEFAULT_AGENT_TOOLS = [
  'list_group_agents',
  'get_messages',
  'get_agent_skill',
  'list_tasks',
  'create_task',
  'update_task',
  'move_task_status',
];
const NO_AGENT_TOOLS = [];
const MANAGEMENT_TOOL_NAMES = ['create_agent', 'update_agent', 'delete_agent'];

// TOOLSET_TEMPLATES is populated from the backend API at startup (see fetch below).
const TOOLSET_TEMPLATES = {};

function normalizeToolName(value) {
  return typeof value === 'string' ? value.trim() : '';
}

function uniqueToolNames(values) {
  return [...new Set(values.map(normalizeToolName).filter(Boolean))];
}

function parseToolsConfig(raw) {
  if (!raw || typeof raw !== 'string') return { ok: false, config: null };
  try {
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return { ok: false, config: null };
    }
    return { ok: true, config: parsed };
  } catch {
    return { ok: false, config: null };
  }
}

function allowedToolsFromConfig(raw) {
  const parsed = parseToolsConfig(raw);
  if (!parsed.ok) return NO_AGENT_TOOLS;
  const config = parsed.config;
  if (!config) return NO_AGENT_TOOLS;
  if (Array.isArray(config.allowed_tools) && config.allowed_tools.length > 0) return uniqueToolNames(config.allowed_tools);
  if (Array.isArray(config.tools) && config.tools.length > 0) return uniqueToolNames(config.tools);
  if (typeof config.toolset === 'string' && Object.prototype.hasOwnProperty.call(TOOLSET_TEMPLATES, config.toolset)) {
    return TOOLSET_TEMPLATES[config.toolset];
  }
  return NO_AGENT_TOOLS;
}

async function resolveCurrentAgent(ctx) {
  if (!ctx.agentId) return null;
  if (ctx.currentAgent !== undefined) return ctx.currentAgent;
  const res = await ctx.callMcpApi('GET', `/mcp/agents/${encodeURIComponent(ctx.agentId)}`);
  const agent = res && res.data ? res.data : (res && typeof res === 'object' && !Array.isArray(res) ? res : null);
  ctx.currentAgent = (agent && agent.id === ctx.agentId) ? agent : null;
  return ctx.currentAgent;
}

function parsePlatformSkills(raw) {
  if (!raw || typeof raw !== 'string') return [];
  try {
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed
      .filter((item) => item && typeof item === 'object' && typeof item.name === 'string' && item.name.trim())
      .map((item) => ({
        name: item.name.trim(),
        description: typeof item.description === 'string' ? item.description : '',
        trigger: typeof item.trigger === 'string' ? item.trigger : '',
        detail: typeof item.detail === 'string' ? item.detail : '',
      }));
  } catch {
    return [];
  }
}

async function resolveAllowedTools(ctx) {
  if (!ctx.agentId) return NO_AGENT_TOOLS;
  if (ctx.allowedTools !== null) return ctx.allowedTools;
  const agent = await resolveCurrentAgent(ctx);
  if (!agent) {
    ctx.allowedTools = NO_AGENT_TOOLS;
    return ctx.allowedTools;
  }
  let tools = allowedToolsFromConfig(agent.tools_config);
  // enable_management_tools 为 true 时自动追加管理类工具
  if (agent.enable_management_tools) {
    const toolSet = new Set(tools);
    for (const mt of MANAGEMENT_TOOL_NAMES) {
      toolSet.add(mt);
    }
    tools = [...toolSet];
  }
  ctx.allowedTools = tools;
  return ctx.allowedTools;
}

async function isToolAllowed(ctx, toolName) {
  const allowed = await resolveAllowedTools(ctx);
  return allowed.includes(toolName);
}

function writeMcp(message) {
  process.stdout.write(`${JSON.stringify(message)}\n`);
}

async function handleMcpMessage(line, toolMap, ctx) {
  let msg;
  try {
    msg = JSON.parse(line);
  } catch {
    return;
  }
  const { id, method, params } = msg;

  if (method === 'initialize') {
    writeMcp({
      jsonrpc: '2.0',
      id,
      result: {
        protocolVersion: MCP_PROTOCOL_VERSION,
        capabilities: { tools: {} },
        serverInfo: { name: 'agenthub-platform', version: '0.1.0' },
      },
    });
    return;
  }
  if (method === 'notifications/initialized' || method === 'initialized') {
    return;
  }
  if (method === 'ping') {
    writeMcp({ jsonrpc: '2.0', id, result: {} });
    return;
  }
  if (method === 'tools/list') {
    const allowed = await resolveAllowedTools(ctx);
    const tools = MCP_TOOLS
      .filter((tool) => allowed.includes(tool.name))
      .map(({ name, description, inputSchema }) => ({ name, description, inputSchema }));
    writeMcp({
      jsonrpc: '2.0',
      id,
      result: { tools },
    });
    return;
  }
  if (method === 'tools/call') {
    const toolName = params && params.name;
    const tool = toolMap.get(toolName);
    if (!tool) {
      writeMcp({ jsonrpc: '2.0', id, error: { code: -32602, message: `未知工具: ${toolName}` } });
      return;
    }
    if (!(await isToolAllowed(ctx, toolName))) {
      writeMcp({
        jsonrpc: '2.0',
        id,
        result: { content: [{ type: 'text', text: `工具未授权: ${toolName}` }], isError: true },
      });
      return;
    }
    try {
      const res = await tool.run((params && params.arguments) || {}, ctx);
      const data = res && Object.prototype.hasOwnProperty.call(res, 'data') ? res.data : res;
      writeMcp({
        jsonrpc: '2.0',
        id,
        result: { content: [{ type: 'text', text: JSON.stringify(data, null, 2) }] },
      });
    } catch (error) {
      writeMcp({
        jsonrpc: '2.0',
        id,
        result: { content: [{ type: 'text', text: `调用失败: ${error.message}` }], isError: true },
      });
    }
    return;
  }
  if (id !== undefined && id !== null) {
    writeMcp({ jsonrpc: '2.0', id, error: { code: -32601, message: `方法不存在: ${method}` } });
  }
}

async function runMcpServer(serverURL, apiKey) {
  const daemonToken = readArg('--daemon-token') || process.env.AGENTHUB_DAEMON_TOKEN || '';
  const ctx = {
    conversationId: readArg('--conversation-id') || process.env.AGENTHUB_CONVERSATION_ID || null,
    userId: readArg('--user-id') || process.env.AGENTHUB_USER_ID || null,
    agentId: readArg('--agent-id') || process.env.AGENTHUB_AGENT_ID || null,
    allowedTools: null,
    currentAgent: undefined,
    callApi: (method, pathname, options) => callApi(serverURL, apiKey, method, pathname, options),
    callMcpApi: (method, pathname, options) => callMcpApi(serverURL, daemonToken, method, pathname, options, ctx.userId),
  };
  const toolMap = new Map(MCP_TOOLS.map((tool) => [tool.name, tool]));

  // 从后端拉取工具集模板到 TOOLSET_TEMPLATES，失败时保持为空对象（工具集解析将回退到 NO_AGENT_TOOLS）
  try {
    const templatesRes = await callMcpApi(serverURL, daemonToken, 'GET', '/api/tools/builtin-templates', {}, ctx.userId);
    const data = templatesRes && templatesRes.data ? templatesRes.data : templatesRes;
    if (data && Array.isArray(data)) {
      for (const tpl of data) {
        if (tpl && typeof tpl.name === 'string' && Array.isArray(tpl.tool_names)) {
          TOOLSET_TEMPLATES[tpl.name] = tpl.tool_names;
        }
      }
      logFlow('info', 'mcp.templates_fetched', { count: data.length });
    }
  } catch (err) {
    logFlow('warn', 'mcp.templates_fetch_failed', { error: errorMessage(err), fallback: 'hardcoded' });
  }

  let buffer = '';
  process.stdin.setEncoding('utf8');
  process.stdin.on('data', (chunk) => {
    buffer += chunk;
    let index = buffer.indexOf('\n');
    while (index >= 0) {
      const line = buffer.slice(0, index).trim();
      buffer = buffer.slice(index + 1);
      if (line) void handleMcpMessage(line, toolMap, ctx);
      index = buffer.indexOf('\n');
    }
  });
  process.stdin.on('end', () => process.exit(0));
  logFlow('info', 'mcp.ready', { tool_count: MCP_TOOLS.length });
}

async function main() {
  const serverURL = readArg('--server-url');
  const apiKey = readArg('--api-key');
  if (!serverURL || !apiKey) {
    logFlow('error', 'cli.usage_error', { usage: 'npx @hust-agenthub/daemon --server-url <url> --api-key <key> [--mcp]' });
    process.exit(2);
  }

  if (process.argv.includes('--mcp')) {
    await runMcpServer(serverURL, apiKey);
    return;
  }

  daemonConn.serverURL = serverURL;
  daemonConn.apiKey = apiKey;
  daemonConn.daemonToken = readArg('--daemon-token') || process.env.AGENTHUB_DAEMON_TOKEN || '';
  loadSessionMap();
  await register(serverURL, apiKey);
  ensureGlobalMcpConfigs(serverURL, apiKey);
  await connectWS(serverURL, apiKey);
}

if (require.main === module) {
  main().catch((error) => {
    logFlow('error', 'daemon.failed', { error: errorMessage(error) });
    process.exit(1);
  });
}

module.exports = {
  commandForTask,
  conversationSessions,
  executeTaskOnce,
  ensureOpenCodeMcpConfig,
  onWebSocket,
  parseOpenCodeOutput,
};
