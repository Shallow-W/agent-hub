# Daemon-Backend 通信架构重构：从轮询到推送

## 问题

当前 Daemon（电脑端守护进程）与 Backend 之间的任务分发采用 **HTTP 轮询** 模式：

```
Daemon: for (;;) { GET /daemon/tasks → 无任务则 sleep → 再查 }
Backend (Orchestrator): 创建 DaemonTask 后 for { sleep 600ms; 查 DB 等结果 }
```

存在三个核心问题：

1. **DB 被当消息队列用** — 两层轮询（daemon 领任务 + orchestrator 等结果），空闲时产生无意义的 DB 查询
2. **延迟高** — 任务从创建到被 daemon 领取，最差等一个完整 poll 周期（~3s）
3. **不支持流式输出** — 必须等 CLI 执行完毕才能拿到完整结果，前端无法实时展示 Agent 输出

## 现有代码偏离了原始设计

`doc/task/M4-1` 和 `M6` 的设计文档描述的是 WebSocket 推送架构：

> M4-1: "守护进程启动时通过 WebSocket 主动连接 Go 后端 → 心跳保活，断连自动重连"
> M6: "后端将任务通过 WebSocket 转发给守护进程 → CLI 流式输出 → 守护进程转发 → 后端 → WebSocket → 前端"

但实际实现中：
- Go 版 daemon (`src/daemon/main.go`) 连 WS 发完注册消息就退出
- NPM 版 daemon (`src/daemon-npm/bin/agenthub-daemon.js`) 注册走 HTTP，之后用轮询领任务
- Orchestrator 用 `waitDaemonTask()` 轮询 DB 等待结果

## 目标架构

```
┌──────────┐                    ┌───────────┐                   ┌────────────┐
│ Frontend  │◄──── WS ────────►│  Backend   │◄──── WS ────────►│  Daemon    │
│           │   /ws (用户)      │            │   /daemon/ws      │ (长驻进程)  │
└──────────┘                    └───────────┘                   └────────────┘
```

### 连接生命周期

```
1. Daemon 启动 → 连接 /daemon/ws?token=<api_key>
2. 发送 {"type":"daemon.register", "data":{machine_id, agents[]}}
3. Backend 注册成功 → Hub 记录 machineID → WS 连接
4. 进入 readLoop，ping/pong 保活
5. 收到 task.execute → 执行 CLI → 流式回传 task.chunk → 最终 task.done
6. 断连 → Hub 标记 machine offline → 重连后恢复
```

### 单次对话数据流

```
1. 用户 @ClaudeCode 发消息
   Frontend ──WS──► Backend  (type: "chat")

2. Backend 解析 mention → 找到 Agent → 找到 machine_id
   → 通过 Hub 查找该 machine 的 daemon WS 连接

3. Backend 推送任务到 Daemon
   Backend ──WS──► Daemon  {"type":"task.execute", "data":{
     task_id, cli_tool, prompt, context_messages, system_prompt
   }}

4. Daemon 执行 CLI，流式回传
   Daemon: spawn("claude", ["--print", prompt])
   Daemon ──WS──► Backend  {"type":"task.chunk", "content":"..."}  (逐 chunk)
   Daemon ──WS──► Backend  {"type":"task.done", "result":"..."}

5. Backend 转发给前端
   Backend ──WS──► Frontend  {"type":"stream.message", ...}

6. DB 异步持久化（不阻塞主流程）
   Backend: goroutine 写入 Message + DaemonTask 记录
```

## 改动范围

### Backend (Go)

| 文件 | 改动 |
|------|------|
| `pkg/ws/hub.go` | 新增 daemon 连接管理：`RegisterDaemon(machineID, conn)`、`SendToMachine(machineID, msg)` |
| `handler/daemon.go` | `Handle()` 中维持 WS 长连接 + readLoop 处理 `task.chunk`/`task.done` |
| `service/orchestrator.go` | `Dispatch()` 改为通过 Hub 推送任务，删除 `waitDaemonTask()` 轮询 |
| `handler/websocket.go` | 接收 daemon 回传的 chunk，转发到对应 conversation 的 room |

### Daemon (NPM)

| 文件 | 改动 |
|------|------|
| `agenthub-daemon.js` | 注册改走 WS；删除 `pollTasks()` 轮询；改为 WS readLoop 接收 `task.execute` |

### 可删除的代码

| 文件 | 说明 |
|------|------|
| `handler/daemon.go` ClaimTask | 不再需要 HTTP 领取 |
| `handler/daemon.go` CompleteTask | 结果通过 WS 回传 |
| `handler/daemon.go` Heartbeat | WS 自带 ping/pong |
| `daemon/client/client.go` | Go 版 daemon 的 HTTP client，改为 WS 长连接 |

## Hub 扩展设计

```go
// Hub 新增字段
type Hub struct {
    // 现有：用户 WS 连接
    clients    map[string]*Client   // userID → Client

    // 新增：Daemon WS 连接
    daemons    map[string]*DaemonConn  // machineID → DaemonConn
    daemonMu   sync.RWMutex
}

type DaemonConn struct {
    MachineID string
    Conn      *websocket.Conn
    sendCh    chan []byte
}

// 新增方法
func (h *Hub) RegisterDaemon(dc *DaemonConn)
func (h *Hub) UnregisterDaemon(machineID string)
func (h *Hub) SendToMachine(machineID string, msg WSMessage) error
func (h *Hub) GetDaemonConn(machineID string) (*DaemonConn, bool)
```

## Daemon WS 消息协议

```jsonc
// Server → Daemon
{"type":"task.execute", "data":{"task_id":"...", "cli_tool":"claude", "prompt":"...", "system_prompt":"...", "context_messages":"..."}}

// Daemon → Server
{"type":"task.chunk",  "data":{"task_id":"...", "content":"部分输出"}}
{"type":"task.done",   "data":{"task_id":"...", "result":"完整输出"}}
{"type":"task.error",  "data":{"task_id":"...", "error":"错误信息"}}

// 保活
{"type":"ping"}  →  {"type":"pong"}
```

## 降级策略

- Daemon 断连时：Hub 检测到断开 → 标记 machine offline → 前端显示 Agent 离线
- Daemon 重连时：重新走 register 流程 → Hub 恢复映射 → Agent 恢复在线
- 任务执行中断连：Backend 检测到 daemon 断开 → 标记 DaemonTask failed → 通知前端
- 长时间输出（>5min）：daemon 定期发送 heartbeat chunk，Backend 据此判断任务仍活跃

## 与现有功能的兼容

- 前端 ConnectComputerModal 的 3s 轮询可保留（仅在弹窗打开时），或改为通过用户 WS 接收 `machine.status_changed` 事件
- Orchestrator 的群聊编排逻辑不受影响，只是底层的 dispatch 通道从 DB 换成 WS
- AgentCandidate 的扫描和注册流程不变，只是从一次性变为长连接生命周期的一部分
