# PRD: Backend Daemon WS 推送对接

## 背景

Daemon (NPM) 已完整实现 WS 推送接收（`connectWS()` 监听 `task.dispatch`，回传 `task.complete`），但 Backend 缺少发送侧，导致实际走 HTTP 轮询降级路径。本任务补齐 Backend 侧，让 Orchestrator 通过 WS 直接推送任务到目标机器的 daemon。

## 需求

### 1. Hub 扩展：Daemon 连接管理

在 `pkg/ws/hub.go` 中新增 daemon 连接注册和索引：

- `RegisterDaemon(machineID, *DaemonConn)` — daemon WS 连接建立时调用
- `UnregisterDaemon(machineID)` — 断连时清理
- `SendToMachine(machineID, msg)` — 向指定机器推送消息
- `GetDaemonConn(machineID)` — 查询连接是否存在

### 2. DaemonHandler 改造：注册连接到 Hub

`handler/daemon.go` 的 `Handle()` 中：

- daemon WS 连接建立后，将连接注册到 Hub（按 machineID 索引）
- `readLoop()` 新增处理 `task.complete` 消息（daemon 回传任务结果）
- 断连时从 Hub 注销，触发 machine offline 状态更新

### 3. Orchestrator 改造：WS 推送替代 DB 轮询

`service/orchestrator.go` 的 dispatch 流程：

- `Dispatch()` 检查目标 machine 是否有活跃 WS 连接
  - 有 → 通过 Hub.SendToMachine 推送 `task.dispatch`
  - 无 → 保留现有 DB 写入 + HTTP ClaimTask 降级路径
- 结果等待：从 `waitDaemonTask()` DB 轮询改为 channel 回调
  - daemon 通过 WS 回传 `task.complete` → DaemonHandler 写入 channel → Orchestrator 收到结果
- DB 持久化保留（异步写入，不阻塞主流程）

### 4. 兼容性

- HTTP 端点（ClaimTask / CompleteTask / Heartbeat）保留，作为 WS 不可用时的降级
- Daemon 端零改动
- 前端零改动

## 验收标准

- [ ] daemon 连上 `/daemon/ws` 后，Hub 按 machineID 索引该连接
- [ ] Orchestrator dispatch 时，通过 WS 推送 `task.dispatch` 到目标 daemon
- [ ] daemon 执行 CLI 后，通过 WS 回传 `task.complete`
- [ ] Backend 收到结果后创建 assistant 消息，推送给前端
- [ ] daemon 断连时 machine 标记 offline，重连后恢复 online
- [ ] WS 不可用时自动降级到 HTTP 轮询路径
- [ ] 现有单聊/群聊 Agent 对话不受影响

## 改动范围

| 文件 | 改动类型 |
|------|---------|
| `pkg/ws/hub.go` | 新增 daemon 连接管理 |
| `handler/daemon.go` | 注册到 Hub + 处理 task.complete |
| `service/orchestrator.go` | WS 推送 + channel 等待 |
| `service/machine_tracker.go` | 可能需要配合 offline 检测 |

## 不改动的部分

- Daemon NPM 代码（已实现）
- 前端代码
- HTTP 降级端点（保留）
