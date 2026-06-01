#!/usr/bin/env node

const { execFileSync, spawn } = require('node:child_process');
const fs = require('node:fs');
const http = require('node:http');
const https = require('node:https');
const os = require('node:os');
const path = require('node:path');
const POLL_INTERVAL_MS = 1500;
const EXEC_TIMEOUT_MS = 120000;
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

function buildPrompt(task) {
  return [
    '你是 AgentHub 中被用户选中的机器人，请直接回答用户当前消息。',
    '不要修改文件，不要执行破坏性操作；如果需要说明限制，请简洁说明。',
    '',
    `用户消息：${task.prompt}`,
  ].join('\n');
}

function commandForTask(task) {
  const prompt = buildPrompt(task);
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
        prompt,
      ],
      outputFile,
    };
  }
  if (task.cli_tool === 'claude') {
    return {
      command,
      args: [
        '-p',
        '--permission-mode',
        'dontAsk',
        '--output-format',
        'text',
      ],
      stdin: prompt,
    };
  }
  if (task.cli_tool === 'openclaw') {
    const sessionID = `agenthub-${String(task.agent_id || task.id).replace(/[^a-zA-Z0-9_-]/g, '-')}`;
    return {
      command,
      args: [
        'agent',
        '--local',
        '--session-id',
        sessionID,
        '--message',
        prompt,
        '--json',
        '--thinking',
        'off',
      ],
      resultFormat: 'openclaw-json',
    };
  }
  return { command, args: [prompt] };
}

async function executeTask(task) {
  if (task.cli_tool === SKILL_SYNC_TOOL) {
    return syncSkillFiles(task.prompt);
  }
  if (task.cli_tool === OPEN_PATH_TOOL) {
    return openSkillLocation(task.prompt);
  }
  const spec = commandForTask(task);
  const output = await runProcess(spec.command, spec.args, spec.stdin);
  if (spec.outputFile && fs.existsSync(spec.outputFile)) {
    const text = fs.readFileSync(spec.outputFile, 'utf8').trim();
    fs.rmSync(spec.outputFile, { force: true });
    if (text) return text;
  }
  if (spec.resultFormat === 'openclaw-json') {
    return parseOpenClawOutput(output.stdout);
  }
  const text = `${output.stdout || ''}${output.stderr ? `\n${output.stderr}` : ''}`.trim();
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
    if (typeof parsed.finalAssistantVisibleText === 'string' && parsed.finalAssistantVisibleText.trim()) {
      return parsed.finalAssistantVisibleText.trim();
    }
    if (typeof parsed.finalAssistantRawText === 'string' && parsed.finalAssistantRawText.trim()) {
      return parsed.finalAssistantRawText.trim();
    }
    if (Array.isArray(parsed.payloads)) {
      const payloadText = parsed.payloads
        .map((payload) => (typeof payload?.text === 'string' ? payload.text : ''))
        .filter(Boolean)
        .join('\n')
        .trim();
      if (payloadText) return payloadText;
    }
  } catch {
    return text;
  }
  return text;
}

function runProcess(command, args, stdin) {
  return new Promise((resolve, reject) => {
    const spec = processSpec(command, args);
    const child = spawn(spec.command, spec.args, {
      windowsHide: true,
      stdio: ['pipe', 'pipe', 'pipe'],
    });
    let stdout = '';
    let stderr = '';
    const timer = setTimeout(() => {
      child.kill();
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
      try {
        const result = await executeTask(task);
        await requestJSON('POST', apiURL(serverURL, apiKey, `/daemon/tasks/${task.id}/complete`), { result });
        console.log(`AgentHub daemon task ${task.id} completed.`);
      } catch (error) {
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
