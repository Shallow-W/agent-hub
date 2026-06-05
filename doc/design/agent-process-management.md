# Agent 进程管理与 MCP 集成设计

## 背景

当前 Agent 和 Machine 强耦合：连接 Machine 后 Agent 自动可用，Agent 进程生命周期跟着 task 走（一次任务一个进程）。MCP 需要手动注入，Claude Code 以 `dontAsk` 权限模式运行导致无法使用 Bash 等工具。

本设计将 Machine 和 Agent 解耦，引入 Agent 进程生命周期管理，并在 Machine 连接时自动启动 MCP。

## 核心设计

### 1. Machine 与 Agent 解耦

```
┌─────────────────────────────────────────────────────┐
│  Machine（电脑）                                      │
│  状态: connected / offline                           │
│  职责: 管理本机进程、上报心跳、执行控制指令              │
│                                                     │
│  ┌───────────┐  ┌───────────┐  ┌───────────┐       │
│  │ Agent A    │  │ Agent B    │  │ Agent C    │       │
│  │ stopped    │  │ running    │  │ stopped    │       │
│  │ (claude)   │  │ (claude)   │  │ (openclaw) │       │
│  └───────────┘  └───────────┘  └───────────┘       │
└─────────────────────────────────────────────────────┘
```

**状态流转：**

```
Agent 候选（candidate）──用户添加──▶ Agent 实例（stopped）
                                        │
                            用户点击"启动" │ daemon startAgent()
                                        ▼
                                   Agent 实例（running）
                                   可接收聊天任务
                                        │
                  ┌─────────────────────┤──────────────────────┐
                  │                     │                      │
        用户点击"重启"         用户点击"停止"          进程异常退出
        restartAgent()        stopAgent()         上报 status=error
                  │                     │                      │
                  ▼                     ▼                      ▼
            running(stopped            stopped               stopped
            → 启动新进程)           (杀掉进程)
```

**关键规则：**
- Machine 连接不等于 Agent 启动
- 只有 `running` 状态的 Agent 才接收聊天任务
- Agent 进程常驻运行，不是每个任务一个进程
- 任务通过 session 机制复用已有进程（`--resume` 或 `--session-id`）

### 2. Agent 启动流程

用户在前端点击"启动 Agent"时：

```
前端 POST /api/agents/:id/start
    │
    ▼
后端 AgentService.StartAgent()
    ├── 校验所有权
    ├── 校验 machine 在线（MachineTracker.IsOnline）
    ├── 创建控制指令 task（type=start, agentID, cliTool, systemPrompt, capabilitiesJSON）
    ├── 等待 daemon 确认（WS 推送后回包 / 或 poll 短轮询）
    └── 更新 DB: agent.status = "running"
    │
    ▼
Daemon 收到 start 指令
    ├── 构建 CLI 启动命令:
    │   claude --dangerously-skip-permissions
    │         --output-format text
    │         --mcp-config { agenthub-platform: ... }
    │         --allowedTools mcp__agenthub-platform
    │         --system-prompt "..."
    │         --session-id "agenthub-{agentID}"
    │
    ├── spawn CLI 进程（detached, stdin=pipe, stdout/stderr=pipe）
    ├── 记录到 runningAgents: Map<agentID, ChildProcess>
    └── 上报 agent.status = "running"
```

**Claude Code 启动参数：**

```bash
claude \
  --dangerously-skip-permissions \   # 自动批准所有工具，不阻塞
  --output-format text \
  --mcp-config '{"mcpServers":{"agenthub-platform":{...}}}' \
  --allowedTools 'mcp__agenthub-platform' \
  --system-prompt "系统指令..." \
  --session-id "agenthub-{agentID}"
```

**关键变更：** `--permission-mode dontAsk` → `--dangerously-skip-permissions`

| Flag | 行为 |
|------|------|
| `--permission-mode dontAsk`（旧） | 不弹审批，但**跳过**需要权限的工具 → Bash/Edit/Write 不可用 |
| `--dangerously-skip-permissions`（新） | 不弹审批，**自动批准**所有工具 → Bash/Edit/Write 全部可用 |

### 3. Agent 进程管理

Daemon 维护 `runningAgents` Map：

```javascript
const runningAgents = new Map(); // agentID → { child, sessionId, cliTool }

async function startAgent(agentID, config) {
  // 如果已有进程，先停掉
  await stopAgent(agentID);

  const sessionId = `agenthub-${agentID}`;
  const { command, args } = buildAgentCommand(config);
  const child = spawn(command, args, {
    detached: true,
    stdio: ['pipe', 'pipe', 'pipe'],
  });

  runningAgents.set(agentID, { child, sessionId, cliTool: config.cliTool });
  // 上报状态
  await reportAgentStatus(agentID, 'running');
}

async function stopAgent(agentID) {
  const entry = runningAgents.get(agentID);
  if (!entry) return;
  try {
    process.kill(-entry.child.pid, 'SIGKILL');
  } catch { /* already dead */ }
  runningAgents.delete(agentID);
  await reportAgentStatus(agentID, 'stopped');
}

async function restartAgent(agentID) {
  const entry = runningAgents.get(agentID);
  if (!entry) return;
  await stopAgent(agentID);
  // 用原有配置重新启动
  await startAgent(agentID, entry.config);
}
```

### 4. MCP 自动启动

Daemon 启动时（npx 连接 server）自动 fork MCP 子进程：

```
npx @agenthub/daemon --server-url ... --api-key ...
    │
    ├── 主进程: 注册机器 → 轮询/WS → 任务分发 → 进程管理
    │
    └── MCP 子进程: node agenthub-daemon.js --mcp
         （常驻 stdio MCP server，供本机 Agent 调用平台工具）
```

**启动时机：** daemon `main()` 中，`register()` 之后立即 fork：

```javascript
async function main() {
  // ... 解析参数 ...

  // 1. 注册机器 + 扫描 Agent 候选
  await register(serverURL, apiKey);

  // 2. 启动常驻 MCP server（供 Claude Code 注入用）
  mcpChild = spawn('node', [__filename, '--server-url', serverURL, '--api-key', apiKey, '--mcp'], {
    stdio: ['pipe', 'pipe', 'pipe'],
    detached: false,
  });

  // 3. 进入任务循环（轮询 或 WS）
  await pollTasks(serverURL, apiKey);
}
```

**MCP 配置注入：** `buildPlatformMcpArgs()` 改为指向已运行的 MCP 子进程（通过 stdin/stdout pipe），而非每次启动新的。

### 5. 任务派发流程（重构后）

```
用户发消息 → Orchestrator
    ├── 查找目标 Agent
    ├── 检查 Agent.status === "running"
    │   ├── 是: 继续派发
    │   └── 否: 返回错误 "Agent 未启动，请先启动 Agent"
    ├── 创建任务（内存队列）
    ├── 通过 WS 推送给 Daemon（或等待 poll）
    └── Daemon 收到任务
        ├── 从 runningAgents 取出已有进程
        ├── 通过 stdin pipe 发送 prompt（或 --resume 恢复 session）
        └── 收集 stdout 输出 → 流式上报结果
```

**关键变化：** 任务不再每次启动新进程，而是复用已运行的 Agent 进程。

### 6. 前端交互

**Machine 详情页（ComputerProfile）：**
- Machine 状态（connected / offline）
- Agent 列表，每个 Agent 显示状态 badge（running / stopped / error）
- 操作按钮：启动 / 停止 / 重启（根据状态显示不同按钮）
- 只有 running 的 Agent 可以在聊天中被 @

**Agent 状态展示：**

| 状态 | Badge 颜色 | 可用操作 |
|------|-----------|---------|
| stopped | default（灰色） | 启动 |
| running | green | 停止、重启 |
| error | red | 重启、停止 |

### 7. 后端 API 变更

```
POST /api/agents/:id/start     → 创建 start 控制指令，等待 daemon 执行
POST /api/agents/:id/stop      → 创建 stop 控制指令
POST /api/agents/:id/restart   → 创建 restart 控制指令
GET  /api/agents/:id/status    → 查询实时状态（优先从 MachineTracker 读）
```

控制指令通过现有的 daemon task 机制下发（type 字段区分 chat task 和 control task）。

### 8. 前置依赖

本设计依赖以下已完成/进行中的工作：

- [x] Daemon 任务队列内存化（已合并）
- [x] MCP Server 实现（已合并）
- [x] `--dangerously-skip-permissions`（待改）
- [ ] Daemon 轮询 → WS 重构（进行中）
- [ ] 流式输出（WS 后可支持）

## 风险与注意事项

1. **`--dangerously-skip-permissions` 安全性**：Agent 进程拥有完全的 Bash 权限，无审批。适用于用户自有机器上的自主 Agent 场景。后续可考虑细粒度权限控制（允许的工具白名单）。

2. **进程泄漏**：Daemon 异常退出时 runningAgents 中的进程可能变成孤儿进程。使用 `detached: true` + 进程组 kill 可缓解，但极端情况需系统级清理。

3. **MCP 子进程生命周期**：MCP 子进程跟随 daemon 主进程。daemon 退出时 MCP 子进程应自动退出（`detached: false` + stdin close 触发）。

4. **Session 复用**：Agent 常驻后通过 `--session-id` 复用会话上下文。需处理 session 过期/损坏的回退逻辑（已有三级 fallback 机制）。
