# Agent ID × Slot × Session 三层抽象与交互

> 适合人群：刚接手 AgentHub 项目的工程师，需要理解 daemon 侧的核心数据模型。
>
> 阅读时长：10 分钟。
>
> 关键问题：用户在多个 conversation（单聊/群聊）里使用同一个 Agent 时，daemon 内部怎么区分？session 怎么复用？跨机器会怎样？

---

## 一句话总结

AgentHub 在 daemon 侧用三层抽象来管理 AI 助手的进程与上下文：

- **Agent ID**（DB 实体，全局唯一）：标识"是哪个 AI 助手"
- **Slot**（daemon 内存，易失）：标识"这个 AI 助手的进程槽位"，同 agent 同时只能 1 个
- **Session ID**（本地文件持久化）：标识"这个 AI 助手在这个 conversation 里的上下文"，按 `agent × conversation` 组合区分

三层粒度不同、生命周期不同、归属也不同。

---

## 1. 三层抽象

| 概念 | 粒度 | 唯一性 | 持久化 | 用途 |
|---|---|---|---|---|
| **Agent ID** | 全局 | 全局唯一（UUID） | DB `agents` 表 | 标识"哪个 AI 助手" |
| **Slot** | agent 维度 | 同 agent 同时 1 个 | daemon 内存（易失） | 进程槽位，复用 CLI 子进程 |
| **Session ID** | agent × conversation | 每个 conversation 1 个 | 本地文件 `~/.agenthub/sessions.json`（持久） | claude CLI 上下文隔离 |

### 关键点

- **Agent 加入多个 conversation，Agent ID 永远不变**。"加入群聊"在 DB 里是往 `conversation_members` 表插一行，不是创建新 Agent
- **同一 Agent 同时只能跑一个 turn**（daemon 串行化），不同 Agent 可以并行
- **Session ID 按 `agent × conversation` 组合区分**，保证不同 conversation 上下文互不污染

---

## 2. 静态视角：三层的物理位置

```
╔══════════════════════════════════════════════════════════════════╗
║  位置 1：PostgreSQL 数据库（服务器侧，持久化）                    ║
║  ─────────────────────────────────────────────                   ║
║  agents 表                                                       ║
║  ┌────────────────────────────────┬────────┬─────────┐           ║
║  │ id                             │ name   │ cli_tool│           ║
║  ├────────────────────────────────┼────────┼─────────┤           ║
║  │ 46f93269-f786-4f2e-...         │ 开发人员2│ claude │           ║
║  │ c8a39d74-8d58-...              │ 浏览文件│ claude │           ║
║  └────────────────────────────────┴────────┴─────────┘           ║
║                                                                  ║
║  conversations 表（部分）                                        ║
║  ┌────────────────────────────────┬────────┐                     ║
║  │ id                             │ type   │                     ║
║  ├────────────────────────────────┼────────┤                     ║
║  │ conv-aaa                       │ single │                     ║
║  │ conv-bbb                       │ group  │                     ║
║  │ conv-ccc                       │ single │                     ║
║  └────────────────────────────────┴────────┘                     ║
╚══════════════════════════════════════════════════════════════════╝
                              ↑↓
                              │ 查询 / 写回
                              │
╔══════════════════════════════════════════════════════════════════╗
║  位置 2：daemon 进程内存（Node.js Map，易失）                    ║
║  ─────────────────────────────────────────────                   ║
║                                                                  ║
║  runningAgents: Map<agent_id, AgentSlot>                         ║
║  ┌──────────────────────────────────────────────────────────┐    ║
║  │ key: 46f93269-f786-4f2e-...                              │    ║
║  │ value: AgentSlot {                                       │    ║
║  │   process: <ChildProcess>,                               │    ║
║  │   sessionId: "2a57ab1c-09ab-...",     ← 当前 active      │    ║
║  │   currentConversationId: "conv-aaa",  ← 当前 active      │    ║
║  │   sendPrompt: fn,                                        │    ║
║  │   eventRef: {current: fn},                               │    ║
║  │   _queue: PromiseQueue                                   │    ║
║  │ }                                                        │    ║
║  └──────────────────────────────────────────────────────────┘    ║
║                                                                  ║
║  agentTurnStates: Map<agent_id, 'idle'|'active'>                 ║
║  ┌────────────────────────────────┬─────────┐                    ║
║  │ 46f93269-f786-4f2e-...              │ active  │ ← conv-aaa 在跑    ║
║  │ c8a39d74-8d58-...              │ idle    │                    ║
║  └────────────────────────────────┴─────────┘                    ║
║                                                                  ║
║  conversationSessions: Map<"agent:conv", sessionId>              ║
║  ┌───────────────────────────────────────────┬──────────────┐    ║
║  │ 46f93269:conv-aaa                         │ 2a57ab1c-... │    ║
║  │ 46f93269:conv-bbb                         │ 1dcd40d5-... │    ║
║  │ 46f93269:conv-ccc                         │ f3e8c1a0-... │    ║
║  │ c8a39d74:conv-aaa                         │ b7d2e5f9-... │    ║
║  └───────────────────────────────────────────┴──────────────┘    ║
╚══════════════════════════════════════════════════════════════════╝
                              ↑↓
                              │ 启动加载 / 变更写回
                              │
╔══════════════════════════════════════════════════════════════════╗
║  位置 3：用户本地文件 ~/.agenthub/sessions.json（持久化）        ║
║  ─────────────────────────────────────────────                   ║
║  {                                                               ║
║    "46f93269:conv-aaa": "2a57ab1c-09ab-4631-9dcf-d61a8925ce70", ║
║    "46f93269:conv-bbb": "1dcd40d5-ed66-4bb1-b52f-022317367257", ║
║    "46f93269:conv-ccc": "f3e8c1a0-...",                          ║
║    "c8a39d74:conv-aaa": "b7d2e5f9-...",                          ║
║    ...                                                           ║
║  }                                                               ║
╚══════════════════════════════════════════════════════════════════╝
                              ↑↓
                              │ --resume / --session-id
                              │
╔══════════════════════════════════════════════════════════════════╗
║  位置 4：claude CLI session 存储 ~/.claude/sessions/             ║
║  ─────────────────────────────────────────────                   ║
║  每个 sessionId 对应一个 JSON 文件，存对话历史：                 ║
║                                                                  ║
║  ~/.claude/sessions/                                             ║
║    ├── 2a57ab1c-09ab-...json  ← agent46 + conv-aaa 的历史        ║
║    ├── 1dcd40d5-ed66-...json  ← agent46 + conv-bbb 的历史        ║
║    ├── f3e8c1a0-...json        ← agent46 + conv-ccc 的历史       ║
║    └── b7d2e5f9-...json        ← agentC8 + conv-aaa 的历史       ║
╚══════════════════════════════════════════════════════════════════╝
```

---

## 3. 数据归属与边界

| 谁负责 | 做什么 |
|---|---|
| **Server** | 存 users / agents / conversations / messages 等业务数据 |
| **Daemon** | 存 `agent × conversation → claude session` 映射（本地） |
| **Claude CLI** | 存每个 session 的对话历史 JSON（本地 `~/.claude/sessions/`） |

**三层独立，互不干涉**。Server 完全不感知 session_id 的存在（`grep external_session_id → 0 匹配`）。

---

## 4. Resume vs Session-ID 判断逻辑

### 决策流程

```
dispatchToPersistentSlot(agentId, conversationId, ...)
    ↓
查 conversationSessions[`${agentId}:${conversationId}`]
    ↓
UUID 格式校验
    ↓
              ┌───┴───┐
              │       │
        有 validSessionId   无
              │       │
              ↓       ↓
   spawn(--resume=<id>)  spawn(--session-id=<newUUID>)
              │
              ↓
        等 2 秒看进程是否退出
              │
        ┌─────┴─────┐
        │           │
    还活着         立即退出
        │           │
   resume 成功    fallback 新建
                    │
                spawn(--session-id=<newUUID>)
    ↓
写回 conversationSessions[`${agentId}:${conversationId}`] = sessionId
    ↓
saveSessionMap() → 全量覆盖写 ~/.agenthub/sessions.json
```

### 决策表

| 场景 | validSessionId | 走哪条路 | 命令行 |
|---|---|---|---|
| 首次发消息（Map 里没记录） | null | 新建 | `--session-id <randomUUID>` |
| 进程被 kill 后再发消息 | 有 | resume | `--resume <saved>` |
| daemon 重启后发消息 | 有（从 sessions.json 加载） | resume | `--resume <saved>` |
| sessions.json 损坏 | null（校验失败） | 新建 | `--session-id <randomUUID>` |
| resume 2 秒内失败 | 有但损坏 | fallback 新建 | `--session-id <randomUUID>` |

---

## 5. 读写时机

### 读 sessions.json
```
daemon 启动时（loadSessionMap）：
  读 ~/.agenthub/sessions.json
  → 解析为 conversationSessions Map
  → 之后整个生命周期都在内存用这个 Map
```

### 写 sessions.json
```
每次 spawn 成功后（saveSessionMap）：
  ① cold start 新建 session（new UUID）→ 写入
  ② resume 失败 fallback 到新 session → 覆盖
  ③ session 损坏被清理 → 删除

写策略：全量覆盖写（不是增量）
  read entire Map → JSON.stringify → writeFileSync
```

**注意**：不是每次 sendPrompt 都写。只在**新建/切换 session 时**写。所以写频率很低（一个 agent 一个 conv 只写一次）。

---

## 6. 设计决策：为什么 Server 不管 session？

### 理由 1：安全隔离
session_id 是 claude CLI 的私有标识，能反查 `~/.claude/sessions/<id>.json` 里的完整对话历史。留在 daemon 本地 = 留在用户机器 = 用户掌控。

### 理由 2：耦合度
如果 server 存 session_id，每次 dispatch 都要在 payload 里带上。daemon 还得 fallback 处理 server 数据丢失。把状态放在最贴近 claude CLI 的地方（daemon 本地），逻辑闭环。

### 理由 3：多机场景天然隔离
用户可能有 2 台机器（公司 + 家）。每台机器跑独立 daemon，有独立的 sessions.json 和 ~/.claude/sessions/。server 不需要知道"哪台机器对应哪个 session"——它只管"消息发给哪个 agent"。

### 理由 4：claude CLI 升级容错
Claude CLI 升级后 session 格式可能变。如果 server 存了旧版本 session_id，升级后失效。放本地，daemon 自己处理版本兼容。

---

## 7. 动态视角：同一 Agent 三个 conversation 的时间线

场景：用户有 3 个对话（conv-aaa / bbb / ccc），都用了"开发人员2"（agent 46f93269）

### T0: daemon 启动
```
runningAgents:        (空)
conversationSessions: (从 sessions.json 加载)
                        "46f93269:conv-aaa" → "2a57ab1c-..."
                        "46f93269:conv-bbb" → "1dcd40d5-..."
                        "46f93269:conv-ccc" → "f3e8c1a0-..."
agentTurnStates:      (空)
```

### T1: 用户在 conv-aaa 发消息"写诗"
```
dispatchToPersistentSlot("46f93269", "conv-aaa", ...)

  ① 查 conversationSessions["46f93269:conv-aaa"]
     → savedSessionId = "2a57ab1c-..."
     → UUID 格式合法 → validSessionId = "2a57ab1c-..."

  ② 查 runningAgents["46f93269"] → 空（cold start）

  ③ spawn claude --resume 2a57ab1c-...
     等 2 秒看进程是否立刻退出
     → 进程还活着 → resume 成功 ✓

  ④ 注册 slot
     runningAgents["46f93269"] = {
       process: <claude PID 1234>,
       sessionId: "2a57ab1c-...",
       currentConversationId: "conv-aaa",  ← 绑定
       ...
     }
     agentTurnStates["46f93269"] = 'active'

  ⑤ slot.sendPrompt("写诗")
     claude 在 session 2a57ab1c 上下文里生成回复

  ⑥ 流式输出完成，agentTurnStates["46f93269"] = 'idle'
```

### T2: 用户在 conv-bbb 发消息（同一 Agent，不同 conversation）
```
dispatchToPersistentSlot("46f93269", "conv-bbb", ...)

  ① 查 conversationSessions["46f93269:conv-bbb"]
     → savedSessionId = "1dcd40d5-..."

  ② 查 runningAgents["46f93269"] → 有 slot！

  ③ Fast path 检查：
     slot.currentConversationId === "conv-aaa"  ≠ "conv-bbb"
     → 不走 fast path，但 slot 还活着不 kill

  ④ 切换 session：
     slot 里 sessionId 从 "2a57ab1c" 切换到 "1dcd40d5"
     slot.currentConversationId = "conv-bbb"
     slot.sendPrompt 时附带 --resume 1dcd40d5（通过 stdin NDJSON 协议）
     claude 进程切换到 session 1dcd40d5 上下文

  ⑤ 流式输出 + 完成
```

### T3: daemon 重启（所有内存丢失）
```
runningAgents:        (空)  ← 进程都没了
agentTurnStates:      (空)
conversationSessions: (从 sessions.json 重新加载，数据不丢)
```

### T4: 用户在 conv-aaa 再发消息
```
dispatchToPersistentSlot("46f93269", "conv-aaa", ...)

  ① 查 conversationSessions["46f93269:conv-aaa"]
     → "2a57ab1c-..."（重启后从 sessions.json 恢复）

  ② 查 runningAgents["46f93269"] → 空（重启后 cold）

  ③ spawn claude --resume 2a57ab1c-...
     claude 加载 ~/.claude/sessions/2a57ab1c-....json 历史
     → 用户感觉"上下文还在" ✓
```

### T5: 假设用户手工删了 ~/.claude/sessions/2a57ab1c-....json
```
dispatchToPersistentSlot("46f93269", "conv-aaa", ...)

  ① validSessionId = "2a57ab1c-..."（daemon 不知道文件被删）

  ② spawn claude --resume 2a57ab1c-...
     claude 找不到 session 文件 → 进程立即退出（exitCode ≠ null）

  ③ 等 2 秒：child.exitCode !== null → "Resume failed"

  ④ Fallback：spawn claude --session-id <newUUID>
     生成 e.g. "9f8a7b6c-..."
     → 全新 session，没有历史

  ⑤ 写回 sessions.json：
     conversationSessions["46f93269:conv-aaa"] = "9f8a7b6c-..."
     → 覆盖旧的损坏映射
     → saveSessionMap() 持久化

  用户感知：上下文丢失，但对话能继续
```

---

## 8. 群聊场景

```
群聊 conv-groupX 里有 3 个 Agent：A、B、C

conversationSessions 表里会有：
  ┌─────────────────────────────────────────┬────────────────┐
  │ "agentA:conv-groupX"                    │ sessionA_X     │
  │ "agentB:conv-groupX"                    │ sessionB_X     │
  │ "agentC:conv-groupX"                    │ sessionC_X     │
  └─────────────────────────────────────────┴────────────────┘

用户 @A @B @C 同时发消息时：

runningAgents 表（同时 3 个槽位）：
  ┌────────────┬─────────────────────────────────────────┐
  │ agentA     │ slot{ sessionId: sessionA_X, conv: X }  │
  │ agentB     │ slot{ sessionId: sessionB_X, conv: X }  │
  │ agentC     │ slot{ sessionId: sessionC_X, conv: X }  │
  └────────────┴─────────────────────────────────────────┘

3 个 claude 子进程并行跑（不同 PID，不同 session，互不干扰）
3 条流式输出独立显示在前端
```

**关键**：群聊不创造新的 Agent ID，只是同一 Agent 在同一 conversation 里被使用。每个 Agent × 群聊组合有独立 Session ID。

---

## 9. 三层"生命周期"对比

|  | DB Agent | Slot | Session 映射 | Claude session 文件 |
|---|---|---|---|---|
| 创建时机 | 用户注册时 | dispatch 时 | 第一次 spawn 成功后 | spawn `--session-id` 时 |
| 存活时长 | 永久 | 进程寿命 | 永久（直到清理） | 直到用户清理 |
| 重启后 | ✓ 保留 | ✗ 丢失 | ✓ 从文件恢复 | ✓ 文件保留 |
| 数量级 | 每用户几个 | 每 agent 最多 1 | 每 conv×agent 1 | 每 session 1 |
| 内容 | Agent 元信息 | 进程 + stdin 通道 | key→value 映射 | 对话历史 JSON |

---

## 10. 三层"粒度"对比

|  | Agent ID | Slot | Session ID |
|---|---|---|---|
| 全局唯一 | ✓ | ✗ | ✗ |
| 粒度 | 1 个 Agent | 1 个 Agent | 1 个 Agent × 1 个 Conversation |
| 同一 Agent 多 conv | 相同 ID | 同一 slot（切换 session） | 不同 sessionId |
| 跨用户 | 不同 | 不同 | 不同 |

**核心要点**：
- **Agent ID**：标识"是哪个 AI 助手"——跨 conversation 不变
- **Slot**：标识"这个 AI 助手的进程槽位"——同一 agent 同时 1 个
- **Session ID**：标识"这个 AI 助手在这个 conversation 里的上下文"——按组合区分

---

## 11. 多机场景的坑

假设用户 wjc 有 2 台机器：

```
机器 A（公司）                              机器 B（家）
─────────                                  ─────────
daemon A                                   daemon B
~/.agenthub/sessions.json                  ~/.agenthub/sessions.json
{ "agent:conv-X": "session-aaa" }          { "agent:conv-X": "session-bbb" }

~/.claude/sessions/                        ~/.claude/sessions/
  session-aaa.json                           session-bbb.json
```

**同一 conversation conv-X 在两台机器上有不同的 claude session**！

因为：
- 用户在公司机器上和 agent 聊 → 机器 A 的 daemon 新建 session-aaa
- 用户回家，在机器 B 上继续 conv-X → 机器 B 的 daemon 没记录 → 新建 session-bbb
- 上下文不连贯

这是 daemon-side 管理的**代价**。

**当前架构的取舍**：
- 接受这个限制（"换机器就丢上下文"）
- 因为换机器场景少，且修复成本高（要么 server 同步、要么用 conversation_id 作为 claude session_id）

---

## 12. 容易混淆的边界

| 问题 | 答案 |
|---|---|
| Agent ID 会变吗？ | ❌ 永远不变（DB 主键） |
| Slot 数量 = Agent 数量吗？ | ❌ Slot 数 ≤ Agent 数（不活跃的 Agent 没 slot） |
| Session ID 数 = Conversation 数吗？ | ❌ Session 数 = Agent 数 × Conversation 数（每个组合 1 个） |
| 同一 Agent 在不同群聊里的"人设"一样吗？ | ✓ 一样（同一个 Agent，但上下文不同） |
| 群聊里同一 Agent 被 5 人同时 @，会并发吗？ | ❌ daemon 串行化同 agent，排队处理 |
| Server 知道 session_id 吗？ | ❌ 完全不知道，daemon 本地管理 |
| sessions.json 损坏怎么办？ | ✓ try/catch 兜底，当作空 Map，所有 conv 走 cold start |
| 用户删了 sessions.json 会怎样？ | ✓ 历史映射丢失，所有 conv 重新 cold start，但 claude 历史文件还在（daemon 找不到映射） |

---

## 13. 关键代码位置

| 关注点 | 文件 |
|---|---|
| SESSIONS_FILE 路径定义 | `src/daemon-npm/bin/agenthub-daemon.js:62` |
| `loadSessionMap()` 启动加载 | `src/daemon-npm/bin/agenthub-daemon.js:1055` |
| `saveSessionMap()` 写回文件 | `src/daemon-npm/bin/agenthub-daemon.js:1065` |
| Resume vs session-id 决策 | `src/daemon-npm/bin/agenthub-daemon.js:2513-2598` |
| args 拼装（--resume / --session-id） | `src/daemon-npm/cli/claude.js:309-331` |
| 2 秒 resume 失败检测 | `src/daemon-npm/bin/agenthub-daemon.js:2585-2598` |

---

## 14. 同 Agent 串行化机制

### 14.1 问题场景

不同群聊 / 单聊同时给同一个 agent 发消息时，daemon 必须保证：
- 同 agent 内 prompt **按到达顺序串行处理**（claude CLI stdin 不支持并行）
- 不同 agent 之间**互不阻塞**

实现机制是 **Slot 内的 Promise 链表（queueTail）**，4 行代码完成串行化，无需显式消息队列。

### 14.2 静态结构：Slot 内部

```
┌──────────────────────────────────────────────────────────────────────┐
│  slot[agentA]（daemon 内存）                                          │
│  ─────────────────────────────────                                   │
│                                                                      │
│  ┌─────────────────┐    ┌──────────────────┐                        │
│  │ child           │    │ queueTail        │  ← Promise 链尾指针    │
│  │ (claude 子进程) │    │ (链表)           │                        │
│  │                 │    │                  │                        │
│  │ stdin ──────┐   │    │  初始:           │                        │
│  │ stdout ─────┼───┼──→ │  Promise.resolve │                        │
│  │ pid         │   │    │                  │                        │
│  └─────────────┘   │    └──────────────────┘                        │
│                    │                                                 │
│  sendPrompt 函数 ──┘                                                 │
│  ────────────────                                                    │
│  function sendPrompt(prompt) {                                       │
│    const run = () => sendPromptRaw(prompt);                          │
│    queueTail = queueTail.then(run, run);                             │
│    return queueTail;                                                 │
│  }                                                                   │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
```

### 14.3 动态流程：3 个 sendPrompt 调用如何排队

场景：conv-X 和 conv-Y 同时给同 agent 发消息，加上 conv-Z 排队中。

#### T0: 初始状态
```
slot[agentA].queueTail = Promise.resolve()  ← 空 Promise（resolved）
                          ↓ (next 指针)
                         null

child.stdin 空闲
claude idle
```

#### T1: sendPrompt(prompt_X) 被调用（来自 conv-X 的 dispatch）
```
1. 创建 run_X = () => sendPromptRaw(prompt_X)
2. queueTail = queueTail.then(run_X, run_X)

   queueTail 链：
   Promise.resolve() ──then(run_X)──→ [新 Promise X]
                                          ↑
                                      queueTail 指向这里

3. JS event loop 立即触发 run_X（因为 Promise.resolve 已 resolved）：
   - resultResolver_X = resolve_X
   - 写 child.stdin: {type:'user', message:{content:[{text:"prompt_X"}]}}
   - 设 400s 超时 timer

4. claude 收到 prompt_X，开始生成
   stdout 流出 thinking_delta / text_delta ...
   → task.progress → backend → 前端 conv-X 显示流式
```

#### T2: sendPrompt(prompt_Y) 被调用（来自 conv-Y 的 dispatch，X 还在跑）
```
1. 创建 run_Y = () => sendPromptRaw(prompt_Y)
2. queueTail = queueTail.then(run_Y, run_Y)

   queueTail 链：
   Promise.resolve() ──then(run_X)──→ [Promise X (pending)]
                                              ↓
                                          then(run_Y)
                                              ↓
                                         [Promise Y (pending)]
                                              ↑
                                          queueTail 指向这里

3. run_Y **不立即执行**——Promise X 还 pending
   run_Y 排在链里等
```

#### T3: sendPrompt(prompt_Z) 被调用（来自 conv-Z 的 dispatch，X 还在跑）
```
1. 创建 run_Z = () => sendPromptRaw(prompt_Z)
2. queueTail = queueTail.then(run_Z, run_Z)

   queueTail 链：
   Promise.resolve() ──then(run_X)──→ [Promise X (pending)]
                                              ↓
                                          then(run_Y)
                                              ↓
                                         [Promise Y (pending)]
                                              ↓
                                          then(run_Z)
                                              ↓
                                         [Promise Z (pending)]
                                              ↑
                                          queueTail 指向这里

   3 个 prompt 排队，等 X 完成
```

#### T4: claude 完成 prompt_X，emit result event
```
1. claude 输出 result event 到 stdout
2. parseStreamEvent 转成 turnEndEvent
3. daemon 触发 resolve_X()（在 spawnPersistent 的 stdout 处理里）
   → Promise X 状态变 fulfilled
4. Promise X 的 .then(run_Y, run_Y) 立即触发：
   - 创建 run_Y 执行上下文
   - resultResolver_Y = resolve_Y
   - 写 child.stdin: prompt_Y
   - claude 开始处理 prompt_Y
```

#### T5-T6: 后续按链顺序执行
```
T5: claude 完成 prompt_Y → resolve_Y() → run_Z 触发
T6: claude 完成 prompt_Z → resolve_Z() → slot idle
```

### 14.4 一图总览：链表形态

```
                    ┌──────────────────────────────────┐
                    │       queueTail Promise 链        │
                    │       （FIFO 顺序执行）           │
                    │                                  │
  初始 ────────────► │  Promise.resolve (fulfilled)    │
                    │         │                        │
                    │         ▼ .then(run_X)           │
                    │  Promise X (pending → fulfilled) │
                    │         │                        │
                    │         ▼ .then(run_Y)           │
                    │  Promise Y (pending → fulfilled) │
                    │         │                        │
                    │         ▼ .then(run_Z)           │
                    │  Promise Z (pending → fulfilled) │
                    │         │                        │
                    │         ▼                        │
                    │       (链尾，slot idle)          │
                    └──────────────────────────────────┘
                                       │
                                       │ 每个 run 执行时
                                       ▼
                    ┌──────────────────────────────────┐
                    │     child.stdin（单一管道）       │
                    │     ──────────────────────        │
                    │     T1: {user prompt_X}           │
                    │     T4: {user prompt_Y}           │
                    │     T5: {user prompt_Z}           │
                    │                                   │
                    │     严格按链顺序写入              │
                    └──────────────────────────────────┘
                                       │
                                       ▼
                    ┌──────────────────────────────────┐
                    │     claude CLI 子进程             │
                    │     ──────────────────────        │
                    │     串行处理 prompt_X → Y → Z     │
                    └──────────────────────────────────┘
```

### 14.5 跨 Slot 视角：不同 agent 并行

```
┌─────────────────────────────────────────────────────────────────────┐
│  daemon 内存                                                        │
│                                                                     │
│  slot[agentA]                       slot[agentB]                    │
│  ──────────────                     ──────────────                   │
│  queueTail_A:                       queueTail_B:                     │
│  Promise.resolve                                                     │
│     │ .then(run_X1)                  Promise.resolve                 │
│     ▼                                 │ .then(run_Y1)                │
│  [Promise X1]                         ▼                             │
│     │ .then(run_X2)                  [Promise Y1]                   │
│     ▼                                 │ .then(run_Y2)               │
│  [Promise X2]                         ▼                             │
│     │                                [Promise Y2]                   │
│     ▼                                                                │
│  (链尾)                                                              │
│                                                                     │
│  child_A (PID 1234)                 child_B (PID 5678)              │
│  ─────────────────                  ─────────────────                │
│  stdin: prompt_X1                   stdin: prompt_Y1                │
│         prompt_X2 (排队)                    prompt_Y2 (排队)        │
│                                                                     │
│  ↑ 两条链独立并行，互不阻塞 ↑                                        │
└─────────────────────────────────────────────────────────────────────┘
```

**关键**：
- 同 agent：链表内串行
- 不同 agent：不同链表，并行

### 14.6 sendPrompt 的精妙之处

```
sendPrompt(prompt)
       │
       ▼
   ┌─────────────────────────────────┐
   │  const run = () => sendPromptRaw│  ← 创建闭包，不立即执行
   └─────────────────────────────────┘
       │
       ▼
   ┌─────────────────────────────────┐
   │ queueTail = queueTail.then(run,run)│ ← 追加到链尾
   └─────────────────────────────────┘
       │
       ▼
   ┌─────────────────────────────────┐
   │  return queueTail               │ ← 调用方 await 这个
   └─────────────────────────────────┘
       │
       ▼
   调用方拿到 Promise，等链前面所有 run 完成 + 自己 run 完成
   才 resolve
```

**.then(run, run)** 第二个参数也是 run（不是 catch handler）：
- 前一个 run 失败（reject）→ 第二个 run 仍然执行（不阻塞队列）
- 前一个 run 成功（resolve）→ 第一个 run 执行
- 所以无论前一个成败，后续 run 都会跑

### 14.7 4 行代码的串行化（精华）

```js
let queueTail = Promise.resolve();                          // 1. 链头

const sendPrompt = (prompt) => {
  const run = () => sendPromptRaw(prompt);                  // 2. 包装成函数
  queueTail = queueTail.then(run, run);                     // 3. 追加链尾
  return queueTail;                                          // 4. 返回等结果
};
```

**对比"显式消息队列"方案**：
- 显式：`queue.push()` / `queue.shift()` / worker loop
- Promise 链：JS event loop 天然 FIFO，4 行代码

### 14.8 为什么不用显式消息队列？

显式队列（如 Bull / RabbitMQ）的代价：
- 引入依赖（redis / amqp）
- 持久化（重启后状态恢复复杂）
- 跨进程协调（daemon 是单进程，不需要）

Promise 链的优势：
- 零依赖（原生 JS）
- 进程内并发控制（够用）
- 简单可读（4 行代码）
- daemon 重启 = 所有 slot 丢失 = 队列自然清空（业务上可接受）

代价：
- daemon 崩溃 = 排队中的 prompt 丢失（但 backend 视为 task 失败，watchdog 兜底）
- 不能跨进程（但 daemon 本来就是单进程）

### 14.9 完整时序：群聊 + 单聊同时给同 agent

```
T0:
  - conv-X 用户发 "写诗"
  - conv-Y 用户发 "翻译这段"
  - 几乎同时到达 daemon

T0+ε: daemon 收到两个 task.dispatch
  ├─ dispatch X: 查 slot[agentA]
  │   ├─ slot 不存在 → enqueueAgentStart(spawn A)
  │   └─ 在 spawn queue 等
  └─ dispatch Y: 查 slot[agentA]
      ├─ slot 不存在（A 还在 spawn queue）→ enqueueAgentStart
      └─ 在 spawn queue 等

T0+3s: spawn A 完成（agentStartQueue 间隔 3 秒）
  - slot[agentA] = { process, sessionId, sendPrompt, queueTail, ... }
  - dispatch X 调 slot.sendPrompt(prompt_X)
    → queueTail.then(runX)
  - dispatch Y 调 slot.sendPrompt(prompt_Y)
    → queueTail.then(runY)  ← 排在 runX 后面

T0+3s ~ T0+8s: runX 执行
  - 写 stdin：{type:'user', message:{content:[{text:"写诗"}]}}
  - claude 在 sessionX 上下文生成诗
  - 流式输出 → task.progress → 前端 conv-X 显示
  - turn 结束 → runX resolve

T0+8s ~ T0+13s: runY 执行
  - 写 stdin：{type:'user', message:{content:[{text:"翻译这段"}]}}
  - 切换 session（slot.currentConversationId 改为 conv-Y）
  - claude 在 sessionY 上下文翻译
  - 流式输出 → 前端 conv-Y 显示
  - turn 结束 → runY resolve

用户视角：
  - conv-X 用户：等了几秒后看到诗（流式）
  - conv-Y 用户：等了十几秒后看到翻译（流式）
  - 都成功，但 conv-Y 用户感知到"延迟"
```

### 14.10 关键代码位置

| 关注点 | 文件:行号 |
|---|---|
| `queueTail` 定义 | `src/daemon-npm/cli/claude.js:463` |
| `sendPromptRaw` 实现（stdin 写入 + resultResolver） | `src/daemon-npm/cli/claude.js:464-488` |
| `sendPrompt` 串行化包装 | `src/daemon-npm/cli/claude.js:496-499` |
| `resultResolver` resolve 触发点（stdout 处理） | `src/daemon-npm/cli/claude.js:409` |
| turn 超时定时器 | `src/daemon-npm/cli/claude.js:472-484` |
| `agentStartQueue`（spawn 串行化） | `src/daemon-npm/bin/agenthub-daemon.js:1074` |
| `START_QUEUE_INTERVAL_MS = 3000` | `src/daemon-npm/bin/agenthub-daemon.js:60` |
| `enqueueAgentStart` | `src/daemon-npm/bin/agenthub-daemon.js:2655` |

### 14.11 一句话总结

**Slot 内的 `queueTail` 是一个 Promise 链表，每次 `sendPrompt` 把新的 run 追加到链尾，JS event loop 保证按链顺序串行执行。无需显式队列，无需锁，无需协调原语——纯 Promise.then 的链式调用天然 FIFO。**

---

## 15. 总结

**三层抽象的协作关系**：

```
用户操作（前端）
      ↓
Server（DB / 调度）
   知道: agent_id + conversation_id
   不知道: session_id
      ↓
Daemon（本机）
   查表: agent × conversation → session_id
   维护: 进程槽位（slot）
      ↓
Claude CLI（本机）
   使用: --resume <session_id>
   读写: ~/.claude/sessions/<id>.json
```

**一句话**：sessions.json 是 daemon 的本地"黑盒"——server 完全不管，daemon 自己读自己写自己用，承载"agent × conversation → claude session"的映射。设计哲学是：LLM 私有状态留在用户本机，server 只管业务数据。代价是多机场景下 session 不互通。
