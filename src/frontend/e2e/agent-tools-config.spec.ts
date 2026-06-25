import { expect, test, type APIRequestContext, type Page } from '@playwright/test';

const FRONTEND_URL = process.env.AGENTHUB_E2E_BASE_URL || 'http://127.0.0.1:5173';
const BACKEND_URL = process.env.AGENTHUB_E2E_BACKEND_URL || 'http://127.0.0.1:8080';
const USERNAME = process.env.AGENTHUB_E2E_USERNAME || 'yzk';
const PASSWORD = process.env.AGENTHUB_E2E_PASSWORD || '123456';

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
  cli_tool: string;
  tools_config?: string;
}

async function loginByAPI(request: APIRequestContext): Promise<string> {
  const response = await request.post(`${BACKEND_URL}/api/auth/login`, {
    data: { username: USERNAME, password: PASSWORD },
  });
  expect(response.ok()).toBeTruthy();
  const body = await response.json() as APIResponse<LoginData>;
  return body.data.token;
}

async function loginByUI(page: Page): Promise<void> {
  await page.goto(`${FRONTEND_URL}/login`);
  await page.getByPlaceholder('请输入用户名').fill(USERNAME);
  await page.getByPlaceholder('请输入密码').fill(PASSWORD);
  await page.getByRole('button', { name: '登录' }).click();
  await expect(page.getByText('AgentHub')).toBeVisible();
}

async function cleanupAgents(request: APIRequestContext, token: string): Promise<void> {
  const headers = { Authorization: `Bearer ${token}` };
  const response = await request.get(`${BACKEND_URL}/api/agents`, { headers });
  const body = await response.json() as APIResponse<AgentData[] | null>;
  for (const agent of body.data ?? []) {
    if (agent.name.startsWith('AgentHub Tools E2E')) {
      await request.delete(`${BACKEND_URL}/api/agents/${agent.id}`, { headers });
    }
  }
}

async function expandUnboundMachine(page: Page): Promise<void> {
  const globalMachine = page.getByRole('button', { name: /未绑定电脑/ });
  await expect(globalMachine).toBeVisible({ timeout: 15000 });
  if ((await globalMachine.getAttribute('aria-expanded')) !== 'true') {
    await globalMachine.locator('xpath=ancestor::div[contains(@class,"machineCard")][1]')
      .locator('[class*="machineActions"]')
      .click();
  }
}

function parseAllowedTools(raw?: string): string[] {
  if (!raw) return [];
  const parsed = JSON.parse(raw) as { allowed_tools?: unknown };
  return Array.isArray(parsed.allowed_tools)
    ? parsed.allowed_tools.filter((tool): tool is string => typeof tool === 'string')
    : [];
}

test.afterEach(async ({ request }) => {
  const token = await loginByAPI(request);
  await cleanupAgents(request, token);
});

test('agent tool assignment survives save, refresh, and tab switches', async ({ page, request }) => {
  test.setTimeout(90000);
  const token = await loginByAPI(request);
  await cleanupAgents(request, token);
  const headers = { Authorization: `Bearer ${token}` };
  const agentName = `AgentHub Tools E2E ${Date.now()}`;

  const createResponse = await request.post(`${BACKEND_URL}/api/agents`, {
    headers,
    data: {
      name: agentName,
      cli_tool: 'codex',
      system_prompt: 'tools e2e',
      tools_config: '{"toolset":"none","allowed_tools":[]}',
    },
  });
  expect(createResponse.ok()).toBeTruthy();
  const created = (await createResponse.json() as APIResponse<AgentData>).data;

  await loginByUI(page);
  await page.getByRole('button', { name: /智能体/ }).click();
  await expandUnboundMachine(page);

  await page.getByText(agentName, { exact: true }).click();
  await page.getByRole('button', { name: /^工具/ }).click();

  const staleToolCard = page.locator('[class*="toolCard"]', { hasText: 'list_group_agents' });
  const targetTool = await staleToolCard.count() > 0 ? 'list_group_agents' : 'list_conversation_agents';
  const toolCard = page.locator('[class*="toolCard"]', { hasText: targetTool }).first();
  await expect(toolCard, `tool card ${targetTool} should be visible`).toBeVisible();
  await toolCard.click();
  await expect(toolCard).toHaveClass(/toolCardSelected/);

  const saveResponsePromise = page.waitForResponse((response) => (
    response.url().includes(`/api/agents/${created.id}/tools-config`)
      && response.request().method() === 'PUT'
  ));
  await page.getByRole('button', { name: '保存' }).click();
  const saveResponse = await saveResponsePromise;
  expect(saveResponse.ok()).toBeTruthy();
  const saveBody = await saveResponse.json() as APIResponse<AgentData>;
  expect(parseAllowedTools(saveBody.data.tools_config)).toContain(targetTool);

  const getResponse = await request.get(`${BACKEND_URL}/api/agents`, { headers });
  const listBody = await getResponse.json() as APIResponse<AgentData[] | null>;
  const persisted = (listBody.data ?? []).find((agent) => agent.id === created.id);
  expect(persisted, 'agent should still exist after save').toBeTruthy();
  expect(parseAllowedTools(persisted?.tools_config)).toContain(targetTool);

  await page.getByRole('button', { name: '概览' }).click();
  await page.getByRole('button', { name: /^工具/ }).click();
  await expect(toolCard).toHaveClass(/toolCardSelected/);

  await page.reload();
  await page.getByRole('button', { name: /智能体/ }).click();
  await expandUnboundMachine(page);
  await page.getByText(agentName, { exact: true }).click();
  await page.getByRole('button', { name: /^工具/ }).click();
  await expect(page.locator('[class*="toolCard"]', { hasText: targetTool }).first()).toHaveClass(/toolCardSelected/);
});
