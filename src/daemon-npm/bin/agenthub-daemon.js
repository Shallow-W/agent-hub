#!/usr/bin/env node

const { execFileSync, spawn, spawnSync } = require('node:child_process');
const crypto = require('node:crypto');
const fs = require('node:fs');
const http = require('node:http');
const https = require('node:https');
const os = require('node:os');
const path = require('node:path');
const POLL_INTERVAL_MS = 1500;
const EXEC_TIMEOUT_MS = 120000;
const HEARTBEAT_INTERVAL_MS = 30000;
const WS_RECONNECT_DELAY_MS = 3000;
const WS_PING_INTERVAL_MS = 30000;
const INBOUND_WATCHDOG_MS = 70000;
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

const activeSessions = new Map();
const runningAgents = new Map(); // agentID → { process, sessionId, cliTool, sendPrompt, _queue }
const idleAgentConfigs = new Map(); // agentID → { cliTool, sessionId, systemPrompt }
const agentTurnStates = new Map(); // agentID → 'idle' | 'active'
let currentDaemonWs = null;
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

// ensureGlobalMcpConfigs 为不支持按次注入的 CLI（openclaw/codex）在启动时幂等写入
// 全局 MCP 配置，把本 daemon 以 --mcp 模式注册为 agenthub-platform server。
// 仅对本机实际安装的 CLI 生效，失败仅告警、不影响轮询。
function ensureGlobalMcpConfigs(serverURL, apiKey) {
  const mcpArgs = [__filename, '--server-url', serverURL, '--api-key', apiKey, '--mcp'];
  if (daemonConn.daemonToken) mcpArgs.push('--daemon-token', daemonConn.daemonToken);
  registerOpenClawMcp(mcpArgs);
  registerCodexMcp(mcpArgs);
}

function registerOpenClawMcp(mcpArgs) {
  const command = 'openclaw';
  if (commandVersion(command) === null) return;
  const value = JSON.stringify({ command: 'node', args: mcpArgs });
  const spec = processSpec(command, ['mcp', 'set', 'agenthub-platform', value]);
  const result = spawnSync(spec.command, spec.args, {
    encoding: 'utf8', timeout: 15000, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'],
  });
  if (result.status === 0) {
    logFlow('info', 'mcp_config.openclaw_configured', { server: 'agenthub-platform' });
  } else {
    logFlow('warn', 'mcp_config.openclaw_failed', { error: firstLine(result.stderr || result.stdout) });
  }
}

function registerCodexMcp(mcpArgs) {
  const command = resolveCommand('codex');
  if (commandVersion(command) === null) return;
  // 幂等：先移除旧条目（忽略不存在的报错），再新增。
  const remove = processSpec(command, ['mcp', 'remove', 'agenthub-platform']);
  spawnSync(remove.command, remove.args, { timeout: 15000, windowsHide: true, stdio: 'ignore' });
  const add = processSpec(command, ['mcp', 'add', 'agenthub-platform', '--', 'node', ...mcpArgs]);
  const result = spawnSync(add.command, add.args, {
    encoding: 'utf8', timeout: 15000, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'],
  });
  if (result.status === 0) {
    logFlow('info', 'mcp_config.codex_configured', { server: 'agenthub-platform' });
  } else {
    logFlow('warn', 'mcp_config.codex_failed', { error: firstLine(result.stderr || result.stdout) });
  }
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

const CANDIDATES = [
  {
    name: 'Claude Code',
    cli_tool: 'claude',
    capabilities: defaultSkills(['coding', 'review', 'orchestration']),
  },
  {
    name: 'Codex',
    cli_tool: 'codex',
    capabilities: defaultSkills(['coding', 'review']),
  },
  {
    name: 'OpenCode',
    cli_tool: 'opencode',
    capabilities: defaultSkills(['coding']),
  },
  {
    name: 'OpenClaw',
    cli_tool: 'openclaw',
    capabilities: defaultSkills(['coding']),
  },
];

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

function isCodexAuthenticated(command) {
  const status = codexLoginStatus(command);
  return status !== null && /\blogged in\b/i.test(status);
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

function resolveCommand(cliTool) {
  if (cliTool === 'codex') {
    return resolveCodexCommand();
  }
  return cliTool;
}

function scanAgents() {
  return CANDIDATES
    .map((candidate) => {
      const command = resolveCommand(candidate.cli_tool);
      if (candidate.cli_tool === 'codex') {
        console.log(`Codex command resolved: ${command}`);
      }
      const version = commandVersion(command);
      if (version === null) return null;
      if (candidate.cli_tool === 'codex' && !isCodexAuthenticated(command)) return null;
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
    return pkg.name === '@agenthub/daemon';
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
  const roots = [];
  const cwd = process.cwd();
  const home = os.homedir();
  const includeProjectRoots = !isAgentHubWorkspace(cwd);
  if (cliTool === 'claude') {
    if (includeProjectRoots) addRoot(roots, path.join(cwd, '.claude', 'skills'));
    if (home) addRoot(roots, path.join(home, '.claude', 'skills'));
    if (home) addRoot(roots, path.join(home, '.claude', 'plugins', 'marketplaces'));
    if (home) addRoot(roots, path.join(home, '.claude', 'plugins', 'cache'));
  } else if (cliTool === 'codex') {
    if (includeProjectRoots) addRoot(roots, path.join(cwd, '.agents', 'skills'));
    if (home) addRoot(roots, path.join(home, '.codex', 'skills'));
  } else if (cliTool === 'opencode' || cliTool === 'openclaw') {
    if (includeProjectRoots) {
      addRoot(roots, path.join(cwd, '.opencode', 'skills'));
      addRoot(roots, path.join(cwd, '.openclaw', 'skills'));
    }
    if (home) addRoot(roots, path.join(home, '.opencode', 'skills'));
    if (home) addRoot(roots, path.join(home, '.openclaw', 'skills'));
    if (home) addRoot(roots, path.join(home, '.openclaw', 'plugin-skills'));
    if (home) {
      for (const root of openClawInstallSkillRoots(home)) {
        addRoot(roots, root);
      }
    }
  }
  return roots;
}

function parseSkillFile(fallbackName, sourcePath, content) {
  const skill = {
    name: fallbackName,
    detail: content,
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

function commandForTask(task) {
  const { systemPrompt, userPrompt } = buildPromptParts(task);
  const command = resolveCommand(task.cli_tool);
  if (task.cli_tool === 'codex') {
    const outputFile = path.join(os.tmpdir(), `agenthub-task-${task.id}.txt`);
    const codexMcpFallback = [
      '[Codex MCP 适配]',
      '你正在执行 AgentHub 平台派发的聊天任务，不是在当前文件夹内做代码开发或项目诊断。',
      '不要读取或遵循当前工作目录的 AGENTS.md/项目说明来改写用户意图；只把下面的 AgentHub prompt 当作任务来源。',
      '以下适配优先于后续工具调用说明：除非用户明确要求测试 MCP 工具本身，否则不要主动调用 agenthub-platform MCP 工具。',
      '如果 agenthub-platform MCP 工具调用被取消、不可用或无法返回，请不要回复“工具调用被取消”。',
      '本次任务的 AgentHub 上下文已经包含在 prompt 中，请直接基于这些上下文继续完成任务。',
      '',
    ].join('\n');
    const effectivePrompt = systemPrompt
      ? `${codexMcpFallback}[系统指令]\n${systemPrompt}\n\n${userPrompt}`
      : `${codexMcpFallback}${userPrompt}`;
    const globalArgs = [
      '--ask-for-approval',
      'never',
    ];
    const execArgs = [
      '--skip-git-repo-check',
      '--sandbox',
      'read-only',
      '--color',
      'never',
      '--output-last-message',
      outputFile,
    ];
    return {
      command,
      args: [...globalArgs, 'exec', ...execArgs, effectivePrompt],
      outputFile,
      cwd: ensureTaskWorkdir(task),
      env: { CODEX_HOME: ensureAgentHubCodexHome() },
    };
  }
  if (task.cli_tool === 'claude') {
    const sessionId = task._sessionId || (task.conversation_id && task.agent_id
      ? makeSessionId(task.conversation_id, task.agent_id)
      : null);
    // Check if this agent is in persistent mode (registered via agent.start)
    const persistent = task.agent_id && runningAgents.has(task.agent_id);
    const args = [
      '-p',
      '--output-format',
      'text',
      ...buildPlatformMcpArgs(task.conversation_id, task.user_id, task.agent_id),
    ];
    if (persistent) {
      args.push('--dangerously-skip-permissions');
    } else {
      args.push('--permission-mode', 'dontAsk');
    }
    if (systemPrompt) {
      args.push('--system-prompt', systemPrompt);
    }
    // For persistent agents, use the registered sessionId
    const effectiveSessionId = persistent
      ? runningAgents.get(task.agent_id).sessionId
      : sessionId;
    return {
      command,
      args,
      stdin: userPrompt,
      sessionId: effectiveSessionId,
    };
  }
  if (task.cli_tool === 'openclaw') {
    const sessionId = task._sessionId || (task.conversation_id && task.agent_id
      ? makeSessionId(task.conversation_id, task.agent_id)
      : `agenthub-${String(task.agent_id || task.id).replace(/[^a-zA-Z0-9_-]/g, '-')}`);
    return {
      command,
      args: [
        'agent',
        '--local',
        '--session-id',
        sessionId,
        '--message',
        userPrompt,
        '--json',
        '--thinking',
        'off',
      ],
      resultFormat: 'openclaw-json',
    };
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
  const text = `${stdout || ''}${stderr ? `\n${stderr}` : ''}`.trim();
  logFlow('info', 'task.result_ready', { ...taskMeta, source: 'stdio', result_len: text.length });
  return text || '(Agent CLI 没有返回内容)';
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
        const result = await executeTask(task);
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
        reject(new Error('Agent task timed out (120s)'));
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

  // Kill existing process if serving a different conversation
  if (slot?.process) {
    logFlow('info', 'agent.conversation_switch', {
      agent_id: agentId,
      from_conversation_id: slot.currentConversationId,
      to_conversation_id: conversationId,
      previous_session_id: slot.sessionId,
    });
    stopAgentProcess(agentId);
    await sleep(500);
  }

  // Spawn with --resume if we have a saved session, otherwise fresh
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

    ws.on('open', () => {
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

    ws.on('message', async (data) => {
      resetWatchdog();
      let envelope;
      try {
        envelope = JSON.parse(data.toString());
      } catch {
        logFlow('warn', 'ws.message_parse_failed', { bytes: Buffer.byteLength(data) });
        return;
      }

      if (envelope.type === 'pong') return;

      if (envelope.type === 'ping') {
        safeSend(ws, JSON.stringify({ type: 'pong' }));
        return;
      }

      if (envelope.type === 'agent.start') {
        logFlow('info', 'ws.control_received', {
          type: envelope.type,
          agent_id: envelope.data && envelope.data.agent_id,
          cli_tool: envelope.data && envelope.data.cli_tool,
        });
        enqueueAgentStart(ws, envelope.data);
        return;
      }
      if (envelope.type === 'agent.stop') {
        logFlow('info', 'ws.control_received', {
          type: envelope.type,
          agent_id: envelope.data && envelope.data.agent_id,
        });
        handleAgentStop(ws, envelope.data);
        return;
      }
      if (envelope.type === 'agent.restart') {
        logFlow('info', 'ws.control_received', {
          type: envelope.type,
          agent_id: envelope.data && envelope.data.agent_id,
          cli_tool: envelope.data && envelope.data.cli_tool,
        });
        handleAgentRestart(ws, envelope.data);
        return;
      }

      if (envelope.type === 'task.dispatch') {
        const d = envelope.data || {};
        const task = {
          id: d.task_id,
          cli_tool: d.cli_tool,
          prompt: d.prompt,
          context_messages: d.context_messages,
          agent_id: d.agent_id,
          conversation_id: d.conversation_id,
          user_id: d.user_id,
        };
        if (!task.id) {
          logFlow('warn', 'task.dispatch_invalid', { reason: 'missing task_id' });
          return;
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
          if (task.cli_tool === 'claude' && task.agent_id && task.conversation_id) {
            // Unified stream-json slot path with conversation isolation
            logFlow('info', 'task.execution_start', {
              task_id: task.id,
              cli_tool: task.cli_tool,
              agent_id: task.agent_id,
              conversation_id: task.conversation_id,
              mode: 'claude_slot',
            });
            result = await dispatchToClaudeSlot(ws, task.agent_id, task.conversation_id, task.user_id, userPrompt, systemPrompt);
          } else {
            // Non-claude or missing info — use legacy per-task spawn
            logFlow('info', 'task.execution_start', {
              task_id: task.id,
              cli_tool: task.cli_tool,
              agent_id: task.agent_id,
              conversation_id: task.conversation_id,
              mode: 'legacy_spawn',
            });
            result = await executeTask(task);
          }
          const artifacts = parseArtifacts(result);
          sendTaskComplete({ task_id: task.id, result, artifacts });
          logFlow('info', 'task.execution_completed', {
            task_id: task.id,
            cli_tool: task.cli_tool,
            agent_id: task.agent_id,
            conversation_id: task.conversation_id,
            result_len: typeof result === 'string' ? result.length : 0,
            artifact_count: artifacts.length,
          });
        } catch (error) {
          sendTaskComplete({
            task_id: task.id,
            error: error instanceof Error ? error.message : String(error),
          });
          logFlow('error', 'task.execution_failed', {
            task_id: task.id,
            cli_tool: task.cli_tool,
            agent_id: task.agent_id,
            conversation_id: task.conversation_id,
            error: errorMessage(error),
          });
        }
        return;
      }

      logFlow('warn', 'ws.unknown_message', { type: envelope.type });
    });

    ws.on('close', (code, reason) => {
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

    ws.on('error', (error) => {
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
    run: (args, ctx) => ctx.callApi('POST', '/api/groups', {
      body: { name: args.name, member_ids: args.member_ids || [] },
    }),
  },
  {
    name: 'list_agents',
    description: '列出当前用户可用的 Agent。',
    inputSchema: { type: 'object', properties: {}, additionalProperties: false },
    run: (args, ctx) => ctx.callApi('GET', '/api/agents'),
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

const TOOLSET_TEMPLATES = {
  none: [],
  basic: ['list_group_agents', 'get_messages', 'get_agent_skill'],
  tasks: DEFAULT_AGENT_TOOLS,
  orchestrator: [
    ...DEFAULT_AGENT_TOOLS,
    'list_conversation_agents',
    'list_conversations',
    'get_group_info',
    'list_group_members',
  ],
  agent_builder: [
    'list_agents',
    'list_group_agents',
    'get_agent_skill',
    'list_agent_candidates',
    'list_machines',
  ],
};

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
  if (Array.isArray(config.allowed_tools)) return uniqueToolNames(config.allowed_tools);
  if (Array.isArray(config.tools)) return uniqueToolNames(config.tools);
  if (typeof config.toolset === 'string' && Object.prototype.hasOwnProperty.call(TOOLSET_TEMPLATES, config.toolset)) {
    return TOOLSET_TEMPLATES[config.toolset];
  }
  return NO_AGENT_TOOLS;
}

async function resolveCurrentAgent(ctx) {
  if (!ctx.agentId) return null;
  if (ctx.currentAgent !== undefined) return ctx.currentAgent;
  const res = await ctx.callApi('GET', '/api/agents');
  const agents = res && Array.isArray(res.data) ? res.data : Array.isArray(res) ? res : [];
  ctx.currentAgent = agents.find((item) => item && item.id === ctx.agentId) || null;
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
  ctx.allowedTools = agent ? allowedToolsFromConfig(agent.tools_config) : NO_AGENT_TOOLS;
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
    conversationId: readArg('--conversation-id') || null,
    userId: readArg('--user-id') || null,
    agentId: readArg('--agent-id') || null,
    allowedTools: null,
    currentAgent: undefined,
    callApi: (method, pathname, options) => callApi(serverURL, apiKey, method, pathname, options),
    callMcpApi: (method, pathname, options) => callMcpApi(serverURL, daemonToken, method, pathname, options, ctx.userId),
  };
  const toolMap = new Map(MCP_TOOLS.map((tool) => [tool.name, tool]));

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
    logFlow('error', 'cli.usage_error', { usage: 'npx @agenthub/daemon --server-url <url> --api-key <key> [--mcp]' });
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

main().catch((error) => {
  logFlow('error', 'daemon.failed', { error: errorMessage(error) });
  process.exit(1);
});
