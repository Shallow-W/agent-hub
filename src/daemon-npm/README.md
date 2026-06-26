# @hust-agenthub/daemon

AgentHub daemon — 连接 AgentHub 后端（HTTP + WebSocket），在本机 spawn agent CLI（Claude Code / Codex / OpenCode / OpenClaw），把本机算力接入 AgentHub 网关。

典型场景：你在 A 机器上跑 AgentHub 后端 + 数据库，想在 B / C 机器上用 `claude` / `codex` CLI 但能复用同一套会话历史、卡片、审批流。每台机器跑一个 daemon，连回 A 的后端即可。

## 前置条件

- **Node.js ≥ 18**（用 `node -v` 验证）
- **至少装一个目标 agent CLI**，并完成登录：
  - Claude Code: `npm install -g @anthropic-ai/claude-code` 然后 `claude` 跑一次登录
  - Codex: 参考 OpenAI 官方指引
  - OpenCode / OpenClaw: 视你需要的能力按需安装
- **后端可达**：你能从这台机器 ping 通跑 AgentHub server 的机器（同 LAN / Tailscale / 反向代理都行）

## 快速上手（LAN 部署示例）

假设 A 机器（跑 AgentHub 后端）LAN IP 是 `10.11.211.178`，后端端口 `8080`。在 B 机器：

```bash
npx @hust-agenthub/daemon \
  --server-url http://10.11.211.178:8080 \
  --api-key <你的 daemon api key>
```

看到日志 `daemon.registered` + `ws.connected` 就说明接入了。后端派任务给这台 daemon 时，会自动 spawn 对应 CLI 执行。

> ℹ️ 首次跑 `npx` 会问你确认安装这个包，回车确认即可。后续执行不会重复询问。

## 获取 API key

API key 由后端签发。在 A 机器（后端所在机器）：

- 如果你是 admin，进后端的管理界面或 DB 直接生成一个 daemon 专用 key
- 或者用现有的用户 API key（取决于后端的鉴权策略）

`--api-key` 决定了这个 daemon 以哪个用户身份注册任务、上报结果。多台 daemon 用同一个 key = 共享同一身份；用不同 key = 各自独立身份。

## 命令行参数 & 环境变量

| 参数 | 环境变量 | 必填 | 说明 |
|------|---------|------|------|
| `--server-url <url>` | — | ✅ | 后端 HTTP 根地址（如 `http://10.11.211.178:8080`） |
| `--api-key <key>` | — | ✅ | 后端 API key |
| `--daemon-token <tok>` | `AGENTHUB_DAEMON_TOKEN` | ❌ | 调 MCP 内部接口（emitCard / task-cards 队列）用的 token；不传则不能发卡片到任务面板 |
| `--conversation-id <id>` | `AGENTHUB_CONVERSATION_ID` | ❌ | MCP 模式下绑死到某会话 |
| `--user-id <id>` | `AGENTHUB_USER_ID` | ❌ | MCP 模式下绑死到某用户 |
| `--agent-id <id>` | `AGENTHUB_AGENT_ID` | ❌ | MCP 模式下绑死到某 agent |
| `--task-id <id>` | `AGENTHUB_TASK_ID` | ❌ | MCP 模式下绑死到某 task（emitCard 依赖） |
| `--mcp` | — | ❌ | 以 MCP server 模式跑（stdin/stdout 协议），不连 WS |

可以混用：`AGENTHUB_DAEMON_TOKEN=xxx npx @hust-agenthub/daemon --server-url http://...:8080 --api-key yyy`。

## 故障排查

- **`Cannot find module '../cli/...'`** —— 0.1.0 的已知 bug，0.2.0+ 已修。`npx @hust-agenthub/daemon@latest` 强制拉新版。
- **连不上后端** —— 先 `curl http://<server-url>/healthz` 验证可达；再检查防火墙是否放行 `:8080`。
- **daemon 起来了但拿不到任务** —— 后端按 agent_id 派活，确认 daemon 用了正确的 `--api-key`（对应身份有权限接到目标 agent 的任务）。
- **spawn claude 失败** —— 在 daemon 这台机器上跑 `claude -p "hi"`，确认能交互。CLI 没装或没登录是最常见原因。
- **卡片不显示** —— 缺 `--daemon-token` 或 `AGENTHUB_DAEMON_TOKEN`，emitCard 调用会被后端拒绝（daemon 日志会打 `card.emit_failed`）。

## 作为常驻进程跑

B 机器上想一直挂着，用 launchd / systemd / pm2 / nohup 都行。最简单的 nohup：

```bash
nohup npx @hust-agenthub/daemon \
  --server-url http://10.11.211.178:8080 \
  --api-key <key> \
  > ~/agenthub-daemon.log 2>&1 &
```

## 版本

当前版本见 `package.json`。破坏性变更会 bump minor/major，修 bug 走 patch。
