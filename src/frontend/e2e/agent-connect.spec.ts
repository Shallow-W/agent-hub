import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { expect, test, type APIRequestContext, type Locator, type Page } from '@playwright/test';

const apiBaseURL = process.env.AGENTHUB_E2E_BASE_URL || 'http://127.0.0.1:5173';
const backendURL = process.env.AGENTHUB_E2E_BACKEND_URL || 'http://127.0.0.1:8080';
const username = process.env.AGENTHUB_E2E_USERNAME || 'yzk';
const password = process.env.AGENTHUB_E2E_PASSWORD || '123456';

interface APIResponse<T> {
  code: number;
  message: string;
  data: T;
}

interface LoginData {
  token: string;
}

interface AgentData {
  id: string;
  name: string;
  machine_id?: string;
}

interface MachineData {
  id: string;
  name: string;
}

interface ConversationData {
  id: string;
  title: string;
}

async function loginByAPI(request: APIRequestContext): Promise<string> {
  const response = await request.post(`${backendURL}/api/auth/login`, {
    data: { username, password },
  });
  expect(response.ok()).toBeTruthy();
  const body = await response.json() as APIResponse<LoginData>;
  return body.data.token;
}

async function cleanupE2EData(request: APIRequestContext, token: string): Promise<void> {
  const headers = { Authorization: `Bearer ${token}` };
  const agentsResponse = await request.get(`${apiBaseURL}/api/agents`, { headers });
  const agentsBody = await agentsResponse.json() as APIResponse<AgentData[] | null>;
  for (const agent of agentsBody.data ?? []) {
    if (agent.name.startsWith('AgentHub UI E2E')) {
      await request.delete(`${apiBaseURL}/api/agents/${agent.id}`, { headers });
    }
  }

  const machinesResponse = await request.get(`${apiBaseURL}/api/daemon/machines`, { headers });
  const machinesBody = await machinesResponse.json() as APIResponse<MachineData[] | null>;
  for (const machine of machinesBody.data ?? []) {
    if (machine.name.startsWith('ui-e2e-computer-')) {
      await request.delete(`${apiBaseURL}/api/daemon/machines/${machine.id}`, { headers });
    }
  }

  const conversationsResponse = await request.get(`${apiBaseURL}/api/conversations`, { headers });
  const conversationsBody = await conversationsResponse.json() as APIResponse<ConversationData[] | null>;
  for (const conversation of conversationsBody.data ?? []) {
    if (conversation.title.startsWith('AgentHub UI E2E Chat')) {
      await request.delete(`${apiBaseURL}/api/conversations/${conversation.id}`, { headers });
    }
  }
}

async function loginByUI(page: Page): Promise<void> {
  await page.goto(`${apiBaseURL}/login`);
  await page.getByPlaceholder('请输入用户名').fill(username);
  await page.getByPlaceholder('请输入密码').fill(password);
  await page.getByRole('button', { name: '登录' }).click();
  await expect(page.getByText('AgentHub')).toBeVisible();
}

async function confirmPopoverDelete(page: Page): Promise<void> {
  await page.locator('.ant-popover:visible').getByRole('button', { name: /^删\s*除$/ }).click();
}

async function addAgentFromCandidate(page: Page, candidateRow: Locator, name: string): Promise<void> {
  await candidateRow.getByRole('button').click();
  const createDialog = page.getByRole('dialog', { name: /Agent/ });
  await expect(createDialog).toBeVisible();
  await expect(createDialog.locator('[class*=toolGrid]')).toBeVisible();
  await createDialog.locator('input[maxlength="100"]').fill(name);
  const createResponse = page.waitForResponse((response) => (
    response.url().includes('/api/daemon/agent-candidates/')
      && response.url().endsWith('/add')
      && response.request().method() === 'POST'
  ));
  await createDialog.locator('.ant-btn-primary').click();
  expect((await createResponse).ok()).toBeTruthy();
  await expect(createDialog).toBeHidden();
}

async function createFakeClaudeBin(): Promise<string> {
  const binDir = await fs.mkdtemp(path.join(os.tmpdir(), 'agenthub-e2e-bin-'));
  const scriptPath = path.join(binDir, process.platform === 'win32' ? 'claude.cmd' : 'claude');
  const script = process.platform === 'win32'
    ? '@echo off\r\necho AgentHub-claude-ok\r\n'
    : '#!/bin/sh\necho AgentHub-claude-ok\n';
  await fs.writeFile(scriptPath, script, { mode: 0o755 });
  return binDir;
}

async function startDaemon(command: string, extraPath: string): Promise<ChildProcessWithoutNullStreams> {
  const daemonPackagePath = command.match(/@agenthub\/daemon@file:([^"]+)/)?.[1];
  const serverURL = command.match(/--server-url\s+"([^"]+)"/)?.[1];
  const apiKey = command.match(/--api-key\s+"([^"]+)"/)?.[1];
  if (!daemonPackagePath || !serverURL || !apiKey) {
    throw new Error(`invalid daemon command: ${command}`);
  }

  const child = spawn(process.execPath, [
    path.join(daemonPackagePath, 'bin', 'agenthub-daemon.js'),
    '--server-url',
    serverURL,
    '--api-key',
    apiKey,
  ], {
    cwd: daemonPackagePath,
    env: {
      ...process.env,
      PATH: `${extraPath}${path.delimiter}${process.env.PATH ?? ''}`,
      Path: `${extraPath}${path.delimiter}${process.env.Path ?? process.env.PATH ?? ''}`,
      AGENTHUB_DAEMON_DISABLE_STREAM_SLOT: '1',
    },
    windowsHide: true,
  });
  let output = '';
  let settled = false;

  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      if (settled) return;
      settled = true;
      child.kill();
      reject(new Error(`daemon did not start in time: ${output}`));
    }, 60000);

    const onData = (chunk: Buffer) => {
      output += chunk.toString('utf8');
      if (!settled && (/AgentHub daemon (is running|正在运行)/.test(output) || /stage=daemon\.ready/.test(output))) {
        settled = true;
        clearTimeout(timer);
        resolve(child);
      }
    };

    child.stdout.on('data', onData);
    child.stderr.on('data', onData);
    child.on('error', (error) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      reject(error);
    });
    child.on('exit', (code) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      reject(new Error(`daemon exited early with code ${code}: ${output}`));
    });
  });
}

test.afterEach(async ({ request }) => {
  const token = await loginByAPI(request);
  await cleanupE2EData(request, token);
});

test('connect computer, detect CLI tools, add and delete agents', async ({ page, request, context }) => {
  test.setTimeout(150000);
  let daemon: ChildProcessWithoutNullStreams | null = null;
  let fakeClaudeBin = '';
  const token = await loginByAPI(request);
  await cleanupE2EData(request, token);

  await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: apiBaseURL });
  await loginByUI(page);
  await page.getByRole('button', { name: /智能体/ }).click();
  await page.getByRole('button', { name: '连接电脑' }).click();

  const dialog = page.getByRole('dialog', { name: 'CONNECT COMPUTER' });
  const machineName = `ui-e2e-computer-${Date.now()}`;
  await dialog.getByRole('textbox').first().fill(machineName);
  await dialog.getByRole('button', { name: '创建连接' }).click();
  await expect(dialog.getByText('CONNECT COMMAND')).toBeVisible();

  await dialog.getByRole('button', { name: 'copy' }).click();
  const copiedCommand = await page.evaluate(() => navigator.clipboard.readText());
  expect(copiedCommand).toContain('npx');
  expect(copiedCommand).toContain('@agenthub/daemon@file:');
  expect(copiedCommand).toContain('--server-url');
  expect(copiedCommand).toContain('--api-key');

  try {
    fakeClaudeBin = await createFakeClaudeBin();
    daemon = await startDaemon(copiedCommand, fakeClaudeBin);

    await expect(dialog.getByText('Connected computers')).toBeVisible({ timeout: 10000 });
    await expect(dialog.getByText(machineName, { exact: true })).toBeVisible();

    const codexRow = dialog.getByText(new RegExp(`claude · ${machineName}`))
      .locator('xpath=ancestor::div[contains(@class,"candidateItem")][1]');
    await expect(codexRow.getByText('Claude Code', { exact: true })).toBeVisible({ timeout: 10000 });
    await addAgentFromCandidate(page, codexRow, 'AgentHub UI E2E A');
    await addAgentFromCandidate(page, codexRow, 'AgentHub UI E2E B');
    await addAgentFromCandidate(page, codexRow, 'AgentHub UI E2E C');

  await page.keyboard.press('Escape');
  const machineButton = page.getByRole('button', { name: new RegExp(machineName) });
  await machineButton.click();
  if (await page.getByRole('button', { name: /AgentHub UI E2E A/ }).count() === 0) {
    await machineButton.click();
  }
  await expect(page.getByRole('button', { name: /AgentHub UI E2E A/ }).first()).toBeVisible();
  await expect(page.getByRole('button', { name: /AgentHub UI E2E B/ }).first()).toBeVisible();
  await expect(page.getByRole('button', { name: /AgentHub UI E2E C/ }).first()).toBeVisible();

  const agentCard = page.getByRole('button', { name: /AgentHub UI E2E A/ }).first();
  await agentCard.getByRole('button').click();
  await confirmPopoverDelete(page);
  await expect(page.getByRole('button', { name: /AgentHub UI E2E A/ })).toHaveCount(0);
  await expect(page.getByRole('button', { name: /AgentHub UI E2E B/ }).first()).toBeVisible();

  const chatTitle = `AgentHub UI E2E Chat ${Date.now()}`;
  const headers = { Authorization: `Bearer ${token}` };
  const conversationResponse = await request.post(`${apiBaseURL}/api/conversations`, {
    headers,
    data: { type: 'group', title: chatTitle },
  });
  expect(conversationResponse.ok()).toBeTruthy();
  const conversation = (await conversationResponse.json()).data;
  const agentsResponse = await request.get(`${apiBaseURL}/api/agents`, { headers });
  expect(agentsResponse.ok()).toBeTruthy();
  const agents = (await agentsResponse.json()).data as Array<{ id: string; name: string }>;
  const chatAgent = agents.find((agent) => agent.name === 'AgentHub UI E2E B');
  expect(chatAgent).toBeTruthy();
  const addRobotResponse = await request.post(`${apiBaseURL}/api/conversations/${conversation.id}/agents`, {
    headers,
    data: { agent_id: chatAgent?.id },
  });
  expect(addRobotResponse.ok()).toBeTruthy();

  await page.reload();
  await page.getByRole('button', { name: /消息/ }).click();
  await page.getByText(chatTitle, { exact: true }).click();
  const chatInput = page.getByPlaceholder('发送至当前对话');
  await expect(chatInput).toBeVisible();
  await chatInput.click();
  await chatInput.fill('@AgentHubUIE2EB Reply with exactly: AgentHub-claude-ok');
  await expect(chatInput).toHaveValue('@AgentHubUIE2EB Reply with exactly: AgentHub-claude-ok');
  await expect(page.getByRole('button', { name: 'send' })).toBeEnabled();
  const messageRequest = page.waitForResponse((response) => (
    response.url().includes('/messages') && response.request().method() === 'POST'
  ), { timeout: 10000 });
  await page.locator('button').filter({ has: page.locator('.anticon-send') }).click();
  await expect(page.getByText('@AgentHubUIE2EB Reply with exactly: AgentHub-claude-ok', { exact: true })).toBeVisible();
  await expect(page.getByText(/AgentHub UI E2E B 正在思考\.\.\./)).toBeVisible();
  await messageRequest;
  await expect(page.getByRole('paragraph').filter({ hasText: /^AgentHub-claude-ok$/ })).toBeVisible({ timeout: 120000 });

  await page.getByRole('button', { name: /智能体/ }).click();
  await page.getByRole('button', { name: '连接电脑' }).click();
  const deleteDialog = page.getByRole('dialog', { name: 'CONNECT COMPUTER' });
  const machineRow = deleteDialog.getByText(machineName).locator('xpath=ancestor::div[contains(@class,"machineItem")][1]');
  await machineRow.getByRole('button').click();
  await confirmPopoverDelete(page);
  await expect(deleteDialog.getByText(machineName, { exact: true })).toHaveCount(0);
  await page.keyboard.press('Escape');
  await expect(page.getByRole('button', { name: /AgentHub UI E2E B/ })).toHaveCount(0);

  await cleanupE2EData(request, token);
  } finally {
    if (daemon && !daemon.killed) {
      daemon.kill();
    }
    if (fakeClaudeBin) {
      await fs.rm(fakeClaudeBin, { recursive: true, force: true });
    }
  }
});
