#!/usr/bin/env node

const { execFileSync, spawn } = require('node:child_process');
const fs = require('node:fs');
const http = require('node:http');
const https = require('node:https');
const os = require('node:os');
const path = require('node:path');
const POLL_INTERVAL_MS = 1500;
const EXEC_TIMEOUT_MS = 120000;

const CANDIDATES = [
  {
    name: 'Claude Code',
    cli_tool: 'claude',
    capabilities: ['coding', 'review', 'orchestration'],
  },
  {
    name: 'Codex',
    cli_tool: 'codex',
    capabilities: ['coding', 'review'],
  },
  {
    name: 'OpenCode',
    cli_tool: 'opencode',
    capabilities: ['coding'],
  },
  {
    name: 'OpenClaw',
    cli_tool: 'openclaw',
    capabilities: ['coding'],
  },
];

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
      return {
        name: candidate.name,
        cli_tool: candidate.cli_tool,
        version,
        capabilities: candidate.capabilities,
      };
    })
    .filter(Boolean);
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
