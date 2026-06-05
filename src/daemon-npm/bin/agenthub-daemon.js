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
    }
  } catch { /* connection already closed */ }
}

const activeSessions = new Map();
const runningAgents = new Map(); // agentID → { process, sessionId, cliTool, sendPrompt, _queue }

// 轮询模式下的后端连接信息，供派发任务时给 Claude Code 注入平台 MCP server。
const daemonConn = { serverURL: '', apiKey: '' };

// buildPlatformMcpArgs 生成 Claude Code 的 MCP 注入参数：把本 daemon 以 --mcp
// 模式作为 stdio MCP server 挂上，让被派发的 claude 任务能直接调用平台工具。
// 仅在已知后端连接信息时生效；其它 CLI（openclaw/codex）无按次注入能力，返回空。
function buildPlatformMcpArgs() {
  if (!daemonConn.serverURL || !daemonConn.apiKey) return [];
  const mcpConfig = JSON.stringify({
    mcpServers: {
      'agenthub-platform': {
        // 用 'node'（PATH 解析）而非 process.execPath，避免 "Program Files" 空格在
        // 子进程参数转义中被拆断；daemon 始终在 node 下运行，PATH 必有 node。
        command: 'node',
        args: [__filename, '--server-url', daemonConn.serverURL, '--api-key', daemonConn.apiKey, '--mcp'],
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
    console.log('已为 OpenClaw 配置平台 MCP（agenthub-platform）。');
  } else {
    console.error(`OpenClaw MCP 配置失败: ${(result.stderr || result.stdout || '').toString().trim()}`);
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
    console.log('已为 Codex 配置平台 MCP（agenthub-platform）。');
  } else {
    console.error(`Codex MCP 配置失败: ${(result.stderr || result.stdout || '').toString().trim()}`);
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
  } catch { /* already dead */ }
  activeSessions.delete(sessionId);
}

const OPEN_PATH_TIMEOUT_MS = 5000;
const MIN_DESCRIPTION_CHARS = 6;
const SKILL_SYNC_TOOL = '__agenthub_skill_sync__';
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

function resolveCommand(cliTool) {
  if (cliTool === 'codex') {
    return codexExtensionPath() || 'codex';
  }
  return cliTool;
}

function scanAgents() {
  return CANDIDATES
    .map((candidate) => {
      const command = resolveCommand(candidate.cli_tool);
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

function requestJSON(method, url, body) {
  const data = body === undefined ? null : Buffer.from(JSON.stringify(body));
  const transport = url.protocol === 'https:' ? https : http;

  return new Promise((resolve, reject) => {
    const req = transport.request(url, {
      method,
      headers: data
        ? {
          'Content-Type': 'application/json',
          'Content-Length': data.length,
        }
        : undefined,
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

  const sysSection = remainingCtx.match(/^(\[系统指令\]\n[\s\S]*?)(?=\n\n\[可用工具\]|\n\n\[群聊背景\]|\n\n\[调度指令\]|\n\n\[依赖输出\]|$)/);
  if (sysSection) {
    systemPrompt += sysSection[1].replace('[系统指令]\n', '').trim();
    remainingCtx = remainingCtx.slice(sysSection[0].length);
  }

  const toolsSection = remainingCtx.match(/^(\[可用工具\]\n[\s\S]*?)(?=\n\n\[群聊背景\]|\n\n\[调度指令\]|\n\n\[依赖输出\]|$)/);
  if (toolsSection) {
    systemPrompt += (systemPrompt ? '\n\n' : '') + '# 可用工具\n' + toolsSection[1].replace('[可用工具]\n', '').trim();
    remainingCtx = remainingCtx.slice(toolsSection[0].length);
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

function commandForTask(task) {
  const { systemPrompt, userPrompt } = buildPromptParts(task);
  const command = resolveCommand(task.cli_tool);
  if (task.cli_tool === 'codex') {
    const outputFile = path.join(os.tmpdir(), `agenthub-task-${task.id}.txt`);
    return {
      command,
      args: [
        '--ask-for-approval',
        'never',
        'exec',
        '--skip-git-repo-check',
        '--sandbox',
        'read-only',
        '--color',
        'never',
        '--output-last-message',
        outputFile,
        userPrompt,
      ],
      outputFile,
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
      ...buildPlatformMcpArgs(),
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
  if (task.cli_tool === SKILL_SYNC_TOOL) {
    return syncSkillFiles(task.prompt);
  }
  if (task.cli_tool === OPEN_PATH_TOOL) {
    return openSkillLocation(task.prompt);
  }
  const spec = commandForTask(task);

  let stdout;
  let stderr;
  if (spec.sessionId) {
    killSessionProcess(spec.sessionId);
    await sleep(1000);

    try {
      ({ stdout, stderr } = await runProcess(
        spec.command,
        ['--resume', spec.sessionId, ...spec.args],
        spec.stdin,
        spec.sessionId,
      ));
    } catch (_err) {
      killSessionProcess(spec.sessionId);
      await sleep(500);
      try {
        ({ stdout, stderr } = await runProcess(
          spec.command,
          ['--session-id', spec.sessionId, ...spec.args],
          spec.stdin,
          spec.sessionId,
        ));
      } catch (_err2) {
        const freshId = `agenthub-${String(task.id || crypto.randomUUID()).replace(/[^a-zA-Z0-9_-]/g, '-')}`;
        ({ stdout, stderr } = await runProcess(
          spec.command,
          ['--session-id', freshId, ...spec.args],
          spec.stdin,
          freshId,
        ));
      }
    }
  } else {
    ({ stdout, stderr } = await runProcess(spec.command, spec.args, spec.stdin));
  }

  if (spec.outputFile && fs.existsSync(spec.outputFile)) {
    const text = fs.readFileSync(spec.outputFile, 'utf8').trim();
    fs.rmSync(spec.outputFile, { force: true });
    if (text) return text;
  }
  if (spec.resultFormat === 'openclaw-json') {
    return parseOpenClawOutput(stdout);
  }
  const text = `${stdout || ''}${stderr ? `\n${stderr}` : ''}`.trim();
  return text || '(Agent CLI 没有返回内容)';
}

function syncSkillFiles(prompt) {
  let payload = null;
  try {
    payload = JSON.parse(prompt);
  } catch {
    throw new Error('Invalid skill sync payload');
  }
  let skills = [];
  try {
    skills = JSON.parse(String(payload.capabilities_json || '[]'));
  } catch {
    throw new Error('Invalid skills JSON');
  }
  let count = 0;
  for (const skill of skills) {
    if (!skill || typeof skill !== 'object') continue;
    const sourcePath = String(skill.source_path || '').trim();
    if (!sourcePath) continue;
    if (path.basename(sourcePath) !== 'SKILL.md') {
      throw new Error(`Refuse to write non-skill file: ${sourcePath}`);
    }
    if (agentHubWorkspaceForPath(sourcePath)) {
      throw new Error('Refuse to write stale AgentHub workspace skill source. Reconnect this computer to refresh skills.');
    }
    if (!fs.existsSync(sourcePath) || !fs.statSync(sourcePath).isFile()) {
      throw new Error(`Skill file not found: ${sourcePath}`);
    }
    fs.writeFileSync(sourcePath, String(skill.detail || ''), 'utf8');
    count += 1;
  }
  return `Synced ${count} skill file(s).`;
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

function runProcess(command, args, stdin, sessionId) {
  return new Promise((resolve, reject) => {
    const spec = processSpec(command, args);
    const detached = process.platform !== 'win32';
    const child = spawn(spec.command, spec.args, {
      windowsHide: true,
      stdio: ['pipe', 'pipe', 'pipe'],
      detached,
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
      reject(error);
    });
    child.on('close', (code) => {
      clearTimeout(timer);
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
  console.log(`AgentHub daemon 发现 ${agents.length} 个 Agent：`);
  for (const agent of agents) {
    const version = agent.version ? ` ${String(agent.version).split('\n')[0].trim()}` : '';
    const skillCount = Array.isArray(agent.capabilities) ? agent.capabilities.length : 0;
    console.log(`  • ${agent.name} (${agent.cli_tool})${version} · ${skillCount} 个技能`);
  }
  await requestJSON('POST', apiURL(serverURL, apiKey, '/daemon/register'), {
    machine_id: os.hostname(),
    agents,
  });
  console.log('详细能力已上报，请在 AgentHub 网页端查看。');
  console.log('AgentHub daemon 正在运行，请保持此终端开启以处理聊天任务。');
}

// 在任务执行期间定期发送心跳，告知 server 任务仍在进行中
function startHeartbeat(serverURL, apiKey, taskId) {
  const timer = setInterval(async () => {
    try {
      await requestJSON('POST', apiURL(serverURL, apiKey, `/daemon/tasks/${taskId}/heartbeat`), {});
    } catch (error) {
      console.error(`AgentHub daemon task ${taskId} heartbeat failed: ${error.message}`);
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

      console.log(`AgentHub daemon task ${task.id}: ${task.cli_tool}`);
      const stopHeartbeat = startHeartbeat(serverURL, apiKey, task.id);
      try {
        const result = await executeTask(task);
        stopHeartbeat();
        await requestJSON('POST', apiURL(serverURL, apiKey, `/daemon/tasks/${task.id}/complete`), { result });
        console.log(`AgentHub daemon task ${task.id} completed.`);
      } catch (error) {
        stopHeartbeat();
        await requestJSON('POST', apiURL(serverURL, apiKey, `/daemon/tasks/${task.id}/complete`), {
          error: error instanceof Error ? error.message : String(error),
        });
        console.error(`AgentHub daemon task ${task.id} failed: ${error.message}`);
      }
    } catch (error) {
      console.error(`AgentHub daemon poll failed: ${error.message}`);
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
  } catch { /* already dead */ }
  runningAgents.delete(agent_id);
}

function handleAgentStart(ws, payload) {
  const { agent_id, cli_tool, system_prompt } = payload;
  if (!agent_id || !cli_tool) return;

  stopAgentProcess(agent_id);

  if (cli_tool !== 'claude') {
    console.log(`Agent ${agent_id}: persistent mode not supported for ${cli_tool}`);
    safeSend(ws, JSON.stringify({ type: 'agent.started', data: { agent_id, error: `${cli_tool} does not support persistent mode` } }));
    return;
  }

  const sessionId = `agenthub-${agent_id.replace(/[^a-zA-Z0-9_-]/g, '-')}`;
  const command = resolveCommand(cli_tool);
  const mcpArgs = buildPlatformMcpArgs();

  const args = [
    '--dangerously-skip-permissions',
    '--output-format', 'stream-json',
    '--input-format', 'stream-json',
    ...mcpArgs,
    '--session-id', sessionId,
  ];
  if (system_prompt) {
    args.push('--system-prompt', system_prompt);
  }

  try {
    const spec = processSpec(command, args);
    const child = spawn(spec.command, spec.args, {
      detached: process.platform !== 'win32',
      stdio: ['pipe', 'pipe', 'pipe'],
      windowsHide: true,
    });

    // Stdout line buffer for stream-json parsing
    let stdoutBuf = '';
    let resultResolver = null;

    child.stdout.setEncoding('utf8');
    child.stdout.on('data', (chunk) => {
      stdoutBuf += chunk;
      const lines = stdoutBuf.split('\n');
      stdoutBuf = lines.pop(); // keep incomplete last line
      for (const line of lines) {
        if (!line.trim()) continue;
        try {
          const event = JSON.parse(line);
          if (event.type === 'result') {
            // Turn complete — resolve pending promise
            const text = typeof event.result === 'string' ? event.result : JSON.stringify(event.result);
            if (event.is_error || event.subtype === 'error_during_execution') {
              if (resultResolver) {
                const r = resultResolver;
                resultResolver = null;
                r({ error: text || 'Agent execution failed' });
              }
            } else {
              if (resultResolver) {
                const r = resultResolver;
                resultResolver = null;
                r({ result: text || '' });
              }
            }
          }
        } catch { /* ignore non-JSON lines */ }
      }
    });

    child.stderr.setEncoding('utf8');
    child.stderr.on('data', (chunk) => {
      console.error(`[agent:${agent_id}:err] ${chunk.trim()}`);
    });

    child.on('close', (code) => {
      console.log(`Agent ${agent_id} process exited with code ${code}`);
      // Reject any pending result promise
      if (resultResolver) {
        const r = resultResolver;
        resultResolver = null;
        r({ error: `Agent process exited (code=${code})` });
      }
      if (runningAgents.get(agent_id)?.process === child) {
        runningAgents.delete(agent_id);
      }
      safeSend(ws, JSON.stringify({ type: 'agent.stopped', data: { agent_id, exit_code: code } }));
    });

    // sendPrompt: write user message to stdin, return promise that resolves on result event
    const sendPromptRaw = (prompt) => new Promise((resolve, reject) => {
      if (child.exitCode !== null) {
        reject(new Error('Agent process not running'));
        return;
      }
      resultResolver = resolve;
      const msg = JSON.stringify({
        type: 'user',
        message: { role: 'user', content: [{ type: 'text', text: prompt }] },
      });
      child.stdin.write(msg + '\n');
      // Timeout after 120s
      setTimeout(() => {
        if (resultResolver === resolve) {
          resultResolver = null;
          reject(new Error('Agent task timed out (120s)'));
        }
      }, 120000);
    });

    // Queue to serialize concurrent sendPrompt calls for the same agent.
    // Each call chains onto the previous promise so only one is active at a time.
    let queueTail = Promise.resolve();
    const sendPrompt = (prompt) => {
      const run = () => sendPromptRaw(prompt);
      queueTail = queueTail.then(run, run);
      return queueTail;
    };

    runningAgents.set(agent_id, {
      process: child,
      sessionId,
      cliTool: cli_tool,
      sendPrompt,
    });

    console.log(`Agent ${agent_id} started (pid=${child.pid}, session=${sessionId})`);
    safeSend(ws, JSON.stringify({ type: 'agent.started', data: { agent_id } }));
  } catch (error) {
    console.error(`Agent ${agent_id} start failed: ${error.message}`);
    safeSend(ws, JSON.stringify({ type: 'agent.started', data: { agent_id, error: error.message } }));
  }
}

function handleAgentStop(ws, payload) {
  const { agent_id } = payload;
  if (!agent_id) return;
  stopAgentProcess(agent_id);
  runningAgents.delete(agent_id);
  console.log(`Agent ${agent_id} stopped`);
  safeSend(ws, JSON.stringify({ type: 'agent.stopped', data: { agent_id } }));
}

function handleAgentRestart(ws, payload) {
  handleAgentStop(ws, payload);
  handleAgentStart(ws, payload);
}

async function connectWS(serverURL, apiKey) {
  if (!WebSocket) {
    console.error('WebSocket not available. Please install ws: npm install ws, or use Node.js 22+');
    console.log('Falling back to HTTP polling...');
    return pollTasks(serverURL, apiKey);
  }

  const url = new URL(serverURL);
  const wsPath = `${url.pathname.replace(/\/$/, '')}/daemon/ws`;
  const protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsURL = `${protocol}//${url.host}${wsPath}?token=${encodeURIComponent(apiKey)}`;

  let reconnectAttempts = 0;

  function connect() {
    console.log(`AgentHub daemon connecting to ${protocol}//${url.host}/daemon/ws ...`);
    const ws = new WebSocket(wsURL);
    let pingTimer = null;
    let watchdogTimer = null;

    function resetWatchdog() {
      if (watchdogTimer) clearTimeout(watchdogTimer);
      watchdogTimer = setTimeout(() => {
        console.warn(`No message from server for ${INBOUND_WATCHDOG_MS / 1000}s, closing WS to reconnect.`);
        try { ws.close(); } catch { /* ignore */ }
      }, INBOUND_WATCHDOG_MS);
    }

    ws.on('open', () => {
      reconnectAttempts = 0;
      console.log('AgentHub daemon WS connected.');
      resetWatchdog();
      // Send register message over WS
      const agents = scanAgents();
      ws.send(JSON.stringify({
        type: 'daemon.register',
        data: { machine_id: os.hostname(), agents },
      }));
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
        return;
      }

      if (envelope.type === 'pong') return;

      if (envelope.type === 'agent.start') {
        handleAgentStart(ws, envelope.data);
        return;
      }
      if (envelope.type === 'agent.stop') {
        handleAgentStop(ws, envelope.data);
        return;
      }
      if (envelope.type === 'agent.restart') {
        handleAgentRestart(ws, envelope.data);
        return;
      }

      if (envelope.type === 'task.dispatch') {
        const d = envelope.data;
        const task = {
          id: d.task_id,
          cli_tool: d.cli_tool,
          prompt: d.prompt,
          context_messages: d.context_messages,
          agent_id: d.agent_id,
          conversation_id: d.conversation_id,
        };
        if (!task.id) return;

        const runningAgent = runningAgents.get(task.agent_id);
        const { systemPrompt, userPrompt } = buildPromptParts(task);

        console.log(`AgentHub daemon task ${task.id}: ${task.cli_tool || 'unknown'}`);
        const stopHeartbeat = startHeartbeat(serverURL, apiKey, task.id);
        try {
          let result;
          if (runningAgent?.sendPrompt) {
            // Persistent process: send prompt via stdin, wait for result
            const response = await runningAgent.sendPrompt(userPrompt);
            if (response.error) {
              throw new Error(response.error);
            }
            result = response.result;
          } else {
            // No persistent process: spawn per task
            task._sessionId = runningAgent?.sessionId || null;
            result = await executeTask(task);
          }
          stopHeartbeat();
          safeSend(ws, JSON.stringify({
            type: 'task.complete',
            data: { task_id: task.id, result },
          }));
          console.log(`AgentHub daemon task ${task.id} completed.`);
        } catch (error) {
          stopHeartbeat();
          safeSend(ws, JSON.stringify({
            type: 'task.complete',
            data: {
              task_id: task.id,
              error: error instanceof Error ? error.message : String(error),
            },
          }));
          console.error(`AgentHub daemon task ${task.id} failed: ${error.message}`);
        }
        return;
      }

      console.log(`AgentHub daemon unknown WS message: ${envelope.type}`);
    });

    ws.on('close', (code, reason) => {
      if (pingTimer) clearInterval(pingTimer);
      if (watchdogTimer) clearTimeout(watchdogTimer);
      // Clean up all running agent entries on disconnect
      for (const [agentId] of runningAgents) {
        stopAgentProcess(agentId);
      }
      runningAgents.clear();
      console.log(`AgentHub daemon WS closed (code=${code}). Reconnecting in ${WS_RECONNECT_DELAY_MS / 1000}s...`);
      setTimeout(connect, WS_RECONNECT_DELAY_MS);
    });

    ws.on('error', (error) => {
      console.error(`AgentHub daemon WS error: ${error.message}`);
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
    name: 'send_message',
    description: '以当前用户身份向指定会话发送一条消息；可用 mentions 传入 agent ID 触发 @机器人。',
    inputSchema: {
      type: 'object',
      properties: {
        conversation_id: { type: 'string', description: '会话 ID' },
        content: { type: 'string', description: '消息内容' },
        mentions: { type: 'array', items: { type: 'string' }, description: '被 @ 的 agent ID 列表（可选）' },
      },
      required: ['conversation_id', 'content'],
      additionalProperties: false,
    },
    run: (args, ctx) => ctx.callApi(
      'POST',
      `/api/conversations/${encodeURIComponent(args.conversation_id)}/messages`,
      { body: { role: 'user', content: args.content, mentions: args.mentions || [] } },
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
];

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
    writeMcp({
      jsonrpc: '2.0',
      id,
      result: {
        tools: MCP_TOOLS.map(({ name, description, inputSchema }) => ({ name, description, inputSchema })),
      },
    });
    return;
  }
  if (method === 'tools/call') {
    const tool = toolMap.get(params && params.name);
    if (!tool) {
      writeMcp({ jsonrpc: '2.0', id, error: { code: -32602, message: `未知工具: ${params && params.name}` } });
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
  const ctx = {
    callApi: (method, pathname, options) => callApi(serverURL, apiKey, method, pathname, options),
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
  console.error(`AgentHub MCP server (stdio) 已就绪，暴露 ${MCP_TOOLS.length} 个工具。`);
}

async function main() {
  const serverURL = readArg('--server-url');
  const apiKey = readArg('--api-key');
  if (!serverURL || !apiKey) {
    console.error('Usage: npx @agenthub/daemon --server-url <url> --api-key <key> [--mcp]');
    process.exit(2);
  }

  if (process.argv.includes('--mcp')) {
    await runMcpServer(serverURL, apiKey);
    return;
  }

  daemonConn.serverURL = serverURL;
  daemonConn.apiKey = apiKey;
  await register(serverURL, apiKey);
  ensureGlobalMcpConfigs(serverURL, apiKey);
  await connectWS(serverURL, apiKey);
}

main().catch((error) => {
  console.error(`AgentHub daemon failed: ${error.message}`);
  process.exit(1);
});
