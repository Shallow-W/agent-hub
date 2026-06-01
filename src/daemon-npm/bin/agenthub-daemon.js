#!/usr/bin/env node

const { execFileSync, spawn } = require('node:child_process');
const crypto = require('node:crypto');
const fs = require('node:fs');
const http = require('node:http');
const https = require('node:https');
const os = require('node:os');
const path = require('node:path');
const POLL_INTERVAL_MS = 1500;
const EXEC_TIMEOUT_MS = 120000;
const HEARTBEAT_INTERVAL_MS = 30000;

const activeSessions = new Map();

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
  return names.map((name) => ({ name, auto: true }));
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

function commandVersion(command) {
  try {
    const spec = processSpec(command, ['--version']);
    return execFileSync(spec.command, spec.args, {
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'ignore'],
      timeout: 5000,
      windowsHide: true,
    }).trim();
  } catch {
    return null;
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

function resolveCommand(cliTool) {
  if (cliTool === 'codex') return codexExtensionPath() || 'codex';
  return cliTool;
}

function scanAgents() {
  return CANDIDATES
    .map((candidate) => {
      const command = resolveCommand(candidate.cli_tool);
      const version = commandVersion(command);
      if (version === null) return null;
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
  if (lines[0]?.trim() !== '---') return skill;
  for (let i = 1; i < lines.length; i += 1) {
    const line = lines[i].trim();
    if (line === '---') break;
    const separator = line.indexOf(':');
    if (separator === -1) continue;
    const key = line.slice(0, separator).trim();
    const value = line.slice(separator + 1).trim().replace(/^['"]|['"]$/g, '');
    if (key === 'name' && value) skill.name = value;
    if (key === 'description') skill.description = value;
  }
  return skill;
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
    const sessionId = task.conversation_id && task.agent_id
      ? makeSessionId(task.conversation_id, task.agent_id)
      : null;
    const args = [
      '-p',
      '--permission-mode',
      'dontAsk',
      '--output-format',
      'text',
    ];
    if (systemPrompt) {
      args.push('--system-prompt', systemPrompt);
    }
    return {
      command,
      args,
      stdin: userPrompt,
      sessionId,
    };
  }
  if (task.cli_tool === 'openclaw') {
    const sessionId = task.conversation_id && task.agent_id
      ? makeSessionId(task.conversation_id, task.agent_id)
      : `agenthub-${String(task.agent_id || task.id).replace(/[^a-zA-Z0-9_-]/g, '-')}`;
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
  if (process.platform === 'win32') {
    command = 'explorer.exe';
    args = [`/select,${sourcePath}`];
  } else if (process.platform === 'darwin') {
    command = 'open';
    args = ['-R', sourcePath];
  }
  const child = spawn(command, args, {
    detached: true,
    stdio: 'ignore',
    windowsHide: true,
  });
  child.unref();
  return `Opened ${sourcePath}`;
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
  console.log(JSON.stringify(agents, null, 2));
  await requestJSON('POST', apiURL(serverURL, apiKey, '/daemon/register'), {
    machine_id: os.hostname(),
    agents,
  });
  console.log(`AgentHub daemon reported ${agents.length} candidate agent(s).`);
  console.log('AgentHub daemon is running. Keep this terminal open to execute chat tasks.');
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

async function main() {
  const serverURL = readArg('--server-url');
  const apiKey = readArg('--api-key');
  if (!serverURL || !apiKey) {
    console.error('Usage: npx @agenthub/daemon --server-url <url> --api-key <key>');
    process.exit(2);
  }

  await register(serverURL, apiKey);
  await pollTasks(serverURL, apiKey);
}

main().catch((error) => {
  console.error(`AgentHub daemon failed: ${error.message}`);
  process.exit(1);
});
