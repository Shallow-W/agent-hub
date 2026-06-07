# Daemon-Backend WS 推送架构 — 现状记录

> 更新于 2026-06-05：经代码审查确认，Backend 侧 WS 推送已完整实现，无需改动。

## 已实现的架构

```
┌──────────┐                    ┌───────────┐                   ┌────────────┐
│ Frontend  │◄──── WS ────────►│  Backend   │◄──── WS ────────►│  Daemon    │
│           │   /ws (Hub)       │            │   /daemon/ws      │ (NPM 长驻)  │
│           │                   │            │   (DaemonHub)     │            │
└──────────┘                    └───────────┘                   └────────────┘
```

## 核心组件

### DaemonHub (`pkg/ws/daemon_hub.go`)

独立于用户 Hub，专门管理 daemon WS 连接：

- `Register(client)` / `Unregister(client)` — 按 machineID 索引 daemon 连接
- `SendToMachine(machineID, msg)` — 推送 `task.dispatch` 到指定机器
- `IsConnected(machineID)` — 检查 daemon 是否在线
- `RegisterTaskPromise(taskID)` / `AwaitTaskResult(taskID)` / `ResolveTask(taskID, result)` — channel 模式的任务结果等待
- 独立事件循环 `Run(ctx)` + 优雅 `Shutdown`

### DaemonHandler (`handler/daemon.go`)

- `Handle()` 创建 `DaemonClient` 并注册到 DaemonHub
- `readLoop()` 处理入站消息：
  - `daemon.register` → 注册机器 + 上报 agents
  - `task.complete` → 调用 `daemonHub.ResolveTask()` 解除 Orchestrator 等待
  - `agent.start` / `agent.stop` / `agent.restart` → 持久进程管理
  - `ping` → 回复 `pong`
- 断连时 `Unregister` → 标记 machine offline

### Orchestrator (`service/orchestrator.go`)

- `dispatchSingleAgent()` / `dispatchWorker()` / `handleOrchestratedDispatch()`：
  - 检查 `daemonHub.IsConnected(machineID)` → 不在线直接报错
  - `daemonHub.SendToMachine()` 推送 `task.dispatch`
  - `daemonHub.RegisterTaskPromise()` + `AwaitTaskResult()` channel 等待（120s 超时）
- **不再有 DB 轮询** — 完全通过 channel 回调

### Daemon (NPM) (`src/daemon-npm/bin/agenthub-daemon.js`)

- `connectWS()` 长驻 WS 连接，监听 `task.dispatch`，回传 `task.complete`
- `pollTasks()` 仅作 WebSocket 库不可用时的降级
- 支持 persistent process（agent.start/stop/restart）
- MCP 模式（`--mcp`）作为 stdio server 给 CLI 提供平台工具

## 数据流

```
1. 用户 @Agent 发消息 → Frontend WS → Backend websocket.go (chat)
2. Message service 解析 mention → Orchestrator.RouteMention()
3. Orchestrator → daemonHub.SendToMachine(machineID, {type:"task.dispatch", ...})
4. Daemon 收到 → 执行 CLI → WS 回传 {type:"task.complete", ...}
5. DaemonHandler.readLoop() → daemonHub.ResolveTask(taskID, result)
6. Orchestrator.waitDaemonTask() channel 收到结果 → 创建 assistant message
7. Hub.PushToConversation() → Frontend WS → 渲染回复
```
