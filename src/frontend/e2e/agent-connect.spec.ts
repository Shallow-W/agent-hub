import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process';
import { expect, test, type APIRequestContext, type Page } from '@playwright/test';

const apiBaseURL = 'http://127.0.0.1:5173';
const backendURL = 'http://127.0.0.1:8080';
const username = '121';
const password = '121';

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

async function startDaemon(command: string): Promise<ChildProcessWithoutNullStreams> {
  const child = spawn(command, {
    shell: true,
    cwd: '../..',
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
    }, 30000);

    const onData = (chunk: Buffer) => {
      output += chunk.toString('utf8');
      if (!settled && output.includes('AgentHub daemon is running')) {
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
  const token = await loginByAPI(request);
  await cleanupE2EData(request, token);

  await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: 'http://127.0.0.1:5173' });
  await loginByUI(page);
  await page.getByRole('menuitem', { name: 'Agent' }).click();
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
    daemon = await startDaemon(copiedCommand);

    await expect(dialog.getByText('Connected computers')).toBeVisible({ timeout: 10000 });
    await expect(dialog.getByText(machineName, { exact: true })).toBeVisible();
    await expect(dialog.getByText('Claude Code', { exact: true })).toBeVisible({ timeout: 10000 });

    const codexRow = dialog.getByText(new RegExp(`claude · ${machineName}`))
      .locator('xpath=ancestor::div[contains(@class,"candidateItem")][1]');
    await codexRow.getByRole('textbox').fill('AgentHub UI E2E A');
    await codexRow.getByRole('button', { name: '添加 Agent' }).click();
    await expect(page.getByText('AgentHub UI E2E A 已添加')).toBeVisible();

    await codexRow.getByRole('textbox').fill('AgentHub UI E2E B');
    await codexRow.getByRole('button', { name: '添加 Agent' }).click();
    await expect(page.getByText('AgentHub UI E2E B 已添加')).toBeVisible();

    await codexRow.getByRole('textbox').fill('AgentHub UI E2E C');
    await codexRow.getByRole('button', { name: '添加 Agent' }).click();
    await expect(page.getByText('AgentHub UI E2E C 已添加')).toBeVisible();

  await page.keyboard.press('Escape');
  await expect(page.getByRole('button', { name: /AgentHub UI E2E A/ })).toBeVisible();
  await expect(page.getByRole('button', { name: /AgentHub UI E2E B/ })).toBeVisible();
  await expect(page.getByRole('button', { name: /AgentHub UI E2E C/ })).toBeVisible();

  const agentCard = page.getByRole('button', { name: /AgentHub UI E2E A/ });
  await agentCard.getByRole('button').click();
  await page.getByRole('button', { name: /删\s*除/ }).click();
  await expect(page.getByRole('button', { name: /AgentHub UI E2E A/ })).toHaveCount(0);
  await expect(page.getByRole('button', { name: /AgentHub UI E2E B/ })).toBeVisible();

  const chatTitle = `AgentHub UI E2E Chat ${Date.now()}`;
  const headers = { Authorization: `Bearer ${token}` };
  const conversationResponse = await request.post(`${apiBaseURL}/api/conversations`, {
    headers,
    data: { type: 'single', title: chatTitle },
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
  await page.getByRole('menuitem', { name: '对话' }).click();
  await page.getByText(chatTitle, { exact: true }).click();
  await expect(page.getByText('AgentHub UI E2E B', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '添加 Robot' }).click();
  const addRobotDialog = page.getByRole('dialog', { name: '添加 Robot 到当前对话' });
  await addRobotDialog.getByLabel('选择要加入的 Robot').click();
  await page.getByText('AgentHub UI E2E C ·').click();
  await page.keyboard.press('Escape');
  await addRobotDialog.locator('.ant-modal-footer .ant-btn-primary').click();
  await expect(page.getByText('AgentHub UI E2E C', { exact: true })).toBeVisible();
  const chatInput = page.getByPlaceholder('输入消息... (Enter 发送, Shift+Enter 换行)');
  await chatInput.click();
  await chatInput.fill('Reply with exactly: AgentHub-claude-ok');
  await expect(chatInput).toHaveValue('Reply with exactly: AgentHub-claude-ok');
  await expect(page.getByRole('button', { name: 'send' })).toBeEnabled();
  const messageRequest = page.waitForResponse((response) => (
    response.url().includes('/messages') && response.request().method() === 'POST'
  ), { timeout: 10000 });
  await page.locator('button').filter({ has: page.locator('.anticon-send') }).click();
  await expect(page.getByText('Reply with exactly: AgentHub-claude-ok', { exact: true })).toBeVisible();
  await expect(page.getByText('Agent 正在思考...', { exact: true })).toBeVisible();
  await messageRequest;
  await expect(page.getByText('AgentHub-claude-ok', { exact: true })).toBeVisible({ timeout: 120000 });

  await page.getByRole('menuitem', { name: 'Agent' }).click();
  await page.getByRole('button', { name: '连接电脑' }).click();
  const deleteDialog = page.getByRole('dialog', { name: 'CONNECT COMPUTER' });
  const machineRow = deleteDialog.getByText(machineName).locator('xpath=ancestor::div[contains(@class,"machineItem")][1]');
  await machineRow.getByRole('button').click();
  await page.getByRole('button', { name: /删\s*除/ }).click();
  await expect(deleteDialog.getByText(machineName, { exact: true })).toHaveCount(0);
  await page.keyboard.press('Escape');
  await expect(page.getByRole('button', { name: /AgentHub UI E2E B/ })).toHaveCount(0);

  await cleanupE2EData(request, token);
  } finally {
    if (daemon && !daemon.killed) {
      daemon.kill();
    }
  }
});
