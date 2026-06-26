import { execSync } from 'node:child_process';
import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { randomUUID } from 'node:crypto';

/**
 * Diff 卡 + [CARD:id] 占位符 端到端测试。
 *
 * 测试策略（确定性，不依赖 LLM 行为）：
 *   1. 用 API 建一个 group 会话，把「产品经理」agent 加进去（diff 卡 fileStatus 需 agentId→machine）
 *   2. 直接 DB 注入一条 assistant 消息：正文含 [CARD:diff1] 占位符、cards_json 挂 diff 卡、
 *      artifacts_json 带 agent_id（卡片的 agentId prop 从这里取）
 *      —— 走 API Send 会被当作 agent dispatch 触发真实 LLM，故用 DB 注入保证确定性
 *   3. 用 UI 打开会话，断言：
 *      - 占位符位置渲染出 diff 卡（不是末尾兜底）
 *      - 卡片挂载后调 fileStatus，列出 3 个文件 + 正确状态徽标
 *      - 点击文件后打开 DiffViewer(Modal)，展示前后内容
 */

const FRONTEND_URL = process.env.AGENTHUB_E2E_BASE_URL || 'http://127.0.0.1:5173';
const BACKEND_URL = process.env.AGENTHUB_E2E_BACKEND_URL || 'http://127.0.0.1:8080';
const USERNAME = process.env.AGENTHUB_E2E_USERNAME || 'wjc';
const PASSWORD = process.env.AGENTHUB_E2E_PASSWORD || '123456';
const AGENT_NAME = process.env.AGENTHUB_E2E_AGENT_NAME || '产品经理';
// 测试 git 仓库（由测试前置脚本准备：含 modified/deleted/added 三种改动）
const TEST_WORKDIR = process.env.AGENTHUB_E2E_WORKDIR || '/tmp/agenthub-e2e-test/testrepo';
const TEST_FILES = ['App.tsx', 'style.css', 'newfile.ts'];
// Postgres 连接（与后端 config.yaml 一致）
const PG_PASSWORD = process.env.AGENTHUB_E2E_PG_PASSWORD || '123456';
const PG_HOST = process.env.AGENTHUB_E2E_PG_HOST || 'localhost';
const PG_USER = process.env.AGENTHUB_E2E_PG_USER || 'shallow';

interface APIResponse<T> { code: number; message: string; data: T; }

async function login(request: APIRequestContext): Promise<string> {
  const r = await request.post(`${BACKEND_URL}/api/auth/login`, { data: { username: USERNAME, password: PASSWORD } });
  expect(r.ok()).toBeTruthy();
  const body = await r.json() as APIResponse<{ token: string }>;
  return body.data.token;
}

async function getAgentId(request: APIRequestContext, token: string, name: string): Promise<string> {
  const r = await request.get(`${BACKEND_URL}/api/agents`, { headers: { Authorization: `Bearer ${token}` } });
  const body = await r.json() as APIResponse<Array<{ id: string; name: string }>>;
  const agent = (body.data ?? []).find((a) => a.name === name);
  expect(agent, `agent "${name}" must exist`).toBeTruthy();
  return agent!.id;
}

function psql(sql: string): string {
  // 用 stdin 传 SQL，避免 shell 转义；连接参数与后端 config.yaml 对齐。
  return execSync(`PGPASSWORD=${PG_PASSWORD} psql -h ${PG_HOST} -U ${PG_USER} -d agenthub -t -A`, {
    input: sql,
    env: { ...process.env, PGPASSWORD: PG_PASSWORD },
  }).toString();
}

/** 直接 DB 注入 assistant 消息（绕过 Send 的 agent dispatch），带占位符正文 + cards + artifacts。 */
function injectAssistantMessage(convId: string, agentId: string, agentName: string): string {
  const msgId = randomUUID();
  const cardId = 'diff1';
  const content = `我已完成本次改动的代码 review，具体文件差异见下方卡片：\n\n[CARD:${cardId}]\n\n如有问题随时反馈。`;
  const cards = [{ type: 'diff', id: cardId, title: '本次代码改动', workDir: TEST_WORKDIR, files: TEST_FILES }];
  const artifacts = { agent_id: agentId, agent_name: agentName, cli_tool: 'claude' };
  const esc = (s: string) => s.replace(/'/g, "''");
  const sql = `INSERT INTO messages (id, conversation_id, role, content, artifacts_json, cards_json) VALUES ('${msgId}', '${convId}', 'assistant', '${esc(content)}', '${esc(JSON.stringify(artifacts))}', '${esc(JSON.stringify(cards))}');`;
  psql(sql);
  return msgId;
}

test.describe.configure({ mode: 'serial' });

test('diff card renders at placeholder + fileStatus + fileDiff', async ({ page, request }) => {
  test.setTimeout(120000);
  const token = await login(request);
  const agentId = await getAgentId(request, token, AGENT_NAME);

  // 建一个独立 group 测试会话（group 才能加 agent；single 加 agent 会失败）
  const title = `DiffCard E2E ${Date.now()}`;
  const convResp = await request.post(`${BACKEND_URL}/api/conversations`, {
    headers: { Authorization: `Bearer ${token}` },
    data: { type: 'group', title },
  });
  expect(convResp.ok()).toBeTruthy();
  const conv = (await convResp.json()).data as { id: string };
  // 把 agent 加进会话（diff 卡 fileStatus 需要 agentId → machine）
  const addResp = await request.post(`${BACKEND_URL}/api/conversations/${conv.id}/agents`, {
    headers: { Authorization: `Bearer ${token}` },
    data: { agent_id: agentId },
  });
  expect(addResp.ok()).toBeTruthy();
  // DB 注入 assistant 消息（带占位符 + diff 卡 + agent artifacts）
  const msgId = injectAssistantMessage(conv.id, agentId, AGENT_NAME);

  // ---- UI 登录 ----
  await page.goto(`${FRONTEND_URL}/login`);
  await page.getByPlaceholder('请输入用户名').fill(USERNAME);
  await page.getByPlaceholder('请输入密码').fill(PASSWORD);
  await page.getByRole('button', { name: '登录' }).click();

  // 进入会话
  await page.getByRole('button', { name: /消息/ }).click();
  await page.getByText(title, { exact: true }).click();

  // CSS Modules 会把类名哈希成 diffCard_abc123，用 [class*=] 部分匹配。
  // 1) 断言：占位符位置的 diff 卡渲染出来了（标题可见）
  const diffCard = page.locator('[class*="diffCard"]', { hasText: '本次代码改动' });
  await expect(diffCard).toBeVisible({ timeout: 15000 });

  // 2) 断言：卡片挂载后调 fileStatus，列出 3 个文件 + 状态徽标
  const list = diffCard.locator('[class*="diffList"] [class*="diffItem"]');
  await expect(list).toHaveCount(3, { timeout: 15000 });
  await expect(diffCard.locator('[class*="diffItem"]', { hasText: 'App.tsx' })).toBeVisible();
  await expect(diffCard.locator('[class*="diffItem"]', { hasText: 'style.css' })).toBeVisible();
  await expect(diffCard.locator('[class*="diffItem"]', { hasText: 'newfile.ts' })).toBeVisible();
  // 状态徽标文本（中文：新增/修改/删除）
  await expect(diffCard.locator('[class*="diffItem"]', { hasText: 'style.css' }).locator('[class*="diffBadge"]')).toHaveText('删除');
  await expect(diffCard.locator('[class*="diffItem"]', { hasText: 'App.tsx' }).locator('[class*="diffBadge"]')).toHaveText('修改');
  await expect(diffCard.locator('[class*="diffItem"]', { hasText: 'newfile.ts' }).locator('[class*="diffBadge"]')).toHaveText('新增');

  // 3) 断言：点击文件打开 DiffViewer(Modal)，展示前后内容
  await diffCard.locator('[class*="diffItem"]', { hasText: 'App.tsx' }).click();
  const modal = page.locator('.ant-modal', { hasText: '本次代码改动' });
  await expect(modal).toBeVisible({ timeout: 15000 });
  // 新版本含 line2-modified
  await expect(modal.getByText('line2-modified')).toBeVisible({ timeout: 15000 });

  // 清理：会话级联删除消息
  await request.delete(`${BACKEND_URL}/api/conversations/${conv.id}`, { headers: { Authorization: `Bearer ${token}` } }).catch(() => {});
  void msgId;
});
