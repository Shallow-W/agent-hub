# 会话类型与消息派发分流

> 适合人群：刚接手 AgentHub 项目的工程师，需要理解用户消息如何触发 Agent 调用。
>
> 阅读时长：6 分钟。
>
> 关键问题：用户发一条消息，后端怎么判断"该派给哪个 Agent"？单聊和群聊的派发逻辑有什么区别？为什么有时候必须 @，有时候又不需要？

---

## 一句话总结

AgentHub 用 **`conversation.type` 字段**决定消息派发路径，而不是用 @mention：

- `type=single`（人 ↔ 人）→ **不派发**，只持久化 + 推送
- `type=agent`（人 ↔ 单个 Agent）→ **直接派给该会话绑定的 Agent**，不需要 @
- `type=group`（人 ↔ 多 Agent）→ **必须 @mention 才派发**，没 @ 当闲聊处理

@mention 不是"决定走哪条路"的开关，而是"群聊里指定谁干活"的选择器。

---

## 1. 三种会话类型

`conversations` 表有一个 `type` 字段，三种取值：

| type | 含义 | 创建时机 | 成员结构 |
|---|---|---|---|
| `"single"` | 人和人单聊 | 用户发起的对人私聊 | 用户 + 用户 |
| `"agent"` | 人和单个 Agent 的专属对话 | 用户把 Agent 加为好友（`handler/conversation.go:49 GetOrCreatePrivate`） | 用户 + 1 个 Agent |
| `"group"` | 多 Agent 群聊 | 用户建群并拉多个 Agent（`conversation_agents` 表） | 用户 + N 个 Agent |

**一旦创建，type 固定不变**——不会因为消息内容改变。所以 `conversation.type` 是系统的"地基字段"。

---

## 2. 派发分流的真实代码

位置：`src/backend/internal/service/message.go:461`

```go
// Agent dispatch routing based on conversation type
switch conv.Type {
case "agent":
    // Single/agent chat — direct dispatch via agentID
    resolvedAgentID := strings.TrimSpace(agentID)
    if resolvedAgentID == "" {
        resolvedAgentID = s.resolveAgentConversationAgentID(ctx, convID, userID)
    }
    if resolvedAgentID != "" {
        go s.asyncAgentReply(convID, userID, resolvedAgentID, content, msg.Attachments, &msg.ID)
    }

case "group":
    // Group chat — mention routing via Orchestrator
    if s.orchSvc != nil {
        parsedMentions := ParseMentions(content)
        if len(mentions) > 0 || len(parsedMentions) > 0 {
            go s.asyncMentionDispatch(convID, userID, msg.ID, content, msg.Attachments)
        }
    }

default:
    // "single" or other types — no agent dispatch
}
```

**三件事要看清**：

1. 分流依据是 `conv.Type`，**不是** mention
2. `case "agent"`：会话本身绑定了 Agent（`conversation_agents` 表），不需要 @，直接派
3. `case "group"`：必须有 @mention 才进 `asyncMentionDispatch` → `RouteMention`，否则跳过

---

## 3. 三种会话的派发对比

### 3.1 `type=single`（人 ↔ 人）

```
用户 A 发 "你好"
   └─→ switch default 分支
       └─→ 不派发
       └─→ 只持久化 + WS 推送给对方
```

后端不调任何 Agent，纯 IM 消息流转。

### 3.2 `type=agent`（人 ↔ 单个 Agent）

```
用户发 "写个 hello world"
   └─→ switch case "agent"
       └─→ resolveAgentConversationAgentID 从会话查到 Agent ID
       └─→ asyncAgentReply → createAgentReply
       └─→ 不需要 @，因为这个会话只为这一个 Agent 存在
```

会话创建时 `conversation_agents` 表就记好了"这个会话对应哪个 Agent"，所以后端能直接查出来。

### 3.3 `type=group`（多 Agent 群聊）

```
用户发 "你好"（无 @）
   └─→ switch case "group"
       └─→ len(mentions)==0 → 不派发
       └─→ 只持久化 + 推送给群里所有成员看

用户发 "@AgentA 写个 hello world"
   └─→ switch case "group"
       └─→ len(mentions)>0 → asyncMentionDispatch
           └─→ RouteMention → Router.Resolve → 拆出 DispatchTarget
```

**关键**：群聊里的 @ 不是路由开关，是**"指定谁干活"的选择器**。

---

## 4. 一图流：派发分流全景

```
                    用户发消息
                         │
                         ▼
           SendMessageWithReply (message.go:411)
                         │
                查 conv.Type 字段
                         │
          ┌──────────────┼──────────────┐
          │              │              │
       "single"       "agent"        "group"
       (人单聊)      (Agent 单聊)     (群聊)
          │              │              │
          ▼              ▼              ▼
    ┌─────────┐   ┌────────────┐  ┌─────────────────┐
    │ 不派发  │   │ 直接派发   │  │ 检查 @mention    │
    │ 只持久化│   │ 该会话绑定 │  │                  │
    │ + 推送  │   │ 的 Agent   │  │  有 @  │  无 @   │
    └─────────┘   │            │  └───┬────┴────┬────┘
                  │            │      │        │
                  │            │      ▼        ▼
                  │            │  RouteMention  不派发
                  │            │  解析每个 @     只推送
                  │            │  → worker/orch
                  │            │
                  ▼            ▼
              createAgentReply（单聊和群聊 worker 在这里汇流）
                  │            │
                  └────┬───────┘
                       ▼
               共用 SetupStreamingPipeline
                       │
                       ▼
               daemonHub.SendToMachine(task.dispatch)
```

---

## 5. 群聊里 @ 和不 @ 的完整对照

假设群里有 `AgentA`、`AgentB`、`Orch`（orchestrator 角色）三个成员：

| 场景 | 消息内容 | mentions 解析 | 派发结果 |
|---|---|---|---|
| 1 | `@AgentA 帮我查 X` | `[AgentA]` | 1 个 worker target，**只派给 AgentA** |
| 2 | `@AgentA 查 X，@AgentB 验证 Y` | `[AgentA, AgentB]` | 2 个 worker target，**并行派发**（不同 Agent） |
| 3 | `@Orch 帮我分析并协调 X` | `[Orch]` | 1 个 orchestrator target，**派给 Orch 让它自己再分** |
| 4 | `你好啊`（无 @） | `[]` | **不派发**，闲聊消息 |
| 5 | `@AgentA 查 X，@Orch 综合` | `[AgentA, Orch]` | 2 个 target，一个 worker 一个 orchestrator，**并行** |

@谁就派谁，不@就闲聊——就这么简单。

---

## 6. 一个反直觉的点

很多人第一次看会以为"无 mention = 单聊，有 mention = 群聊"——**这个映射是反过来的**：

|  | 无 @ | 有 @ |
|---|---|---|
| `type=agent`（Agent 单聊） | ✅ 直接派给该 Agent | 不常见（Agent 单聊里 @ 没意义，没别的 Agent 可选） |
| `type=group`（群聊） | ❌ 不派发（闲聊） | ✅ 派给被 @ 的 Agent |
| `type=single`（人单聊） | ❌ 不派发 | N/A（群里没 Agent 可 @） |

记忆口诀：
- **Agent 单聊**：会话只为一个 Agent 存在，**默认全派**，不需要 @
- **群聊**：会话有多个 Agent，**默认不派**，必须 @ 指定谁干

---

## 7. 相关表结构速查

### `conversations`
```
id | user_id | type     | title | created_at
---+--------+----------+-------+-----
1  | u1     | single   | -     | ...
2  | u1     | agent    | -     | ...   ← type=agent
3  | u1     | group    | 群1   | ...   ← type=group
```

### `conversation_agents`（type=agent / group 都用）
```
conversation_id | agent_id | role           | ...
----------------+----------+----------------+-----
2               | agentA   | member         | ...   ← type=agent：唯一一行
3               | agentA   | member         | ...
3               | agentB   | member         | ...
3               | orch     | orchestrator   | ...   ← type=group：多行
```

`resolveAgentConversationAgentID` 就是查这张表（type=agent 时只返回唯一一行）。
`Router.Resolve` 也是查这张表判定 worker vs orchestrator 角色（`ConversationAgent.IsOrchestrator()`）。

---

## 8. 关键代码位置

| 功能 | 文件:行号 |
|---|---|
| 入口 HTTP handler | `src/backend/internal/handler/message.go:43 Send` |
| 派发分流主体 | `src/backend/internal/service/message.go:411 SendMessageWithReply` |
| 三种 type 的 switch | `src/backend/internal/service/message.go:461` |
| Agent 单聊派发 | `src/backend/internal/service/message.go:1387 asyncAgentReply` |
| 群聊 mention 派发 | `src/backend/internal/service/message.go:1354 asyncMentionDispatch` |
| resolveAgentConversationAgentID | `src/backend/internal/service/message.go:1492` |
| mention 解析 | `src/backend/internal/service/mention_parser.go:50 ParseMentions` |
| Orchestrator 入口 | `src/backend/internal/service/orchestrator.go:290 RouteMention` |
| Router 解析 | `src/backend/internal/service/dispatcher_router.go:75 Resolve` |
| 创建 type=agent 会话 | `src/backend/internal/handler/conversation.go:49 GetOrCreatePrivate` |

---

## 9. 设计要点回顾

1. **派发依据是 `conversation.type`，不是 @mention**——这是反直觉但正确的判断
2. **type=agent 会话"默认全派"**——因为会话只为一个 Agent 存在，没别的选择
3. **type=group 会话"默认不派"**——因为群里多个 Agent，必须显式指定
4. **@mention 是"群聊选择器"，不是"派发开关"**——这个语义区分理解了，整个分流逻辑就通了
5. **type=single（人单聊）完全不派发**——AgentHub 是 IM 框架，人与人对话也支持，只是不调 Agent

---

## 10. 进一步阅读

- 群聊 mention 拆解细节 → `doc/architecture/agent-slot-session.md`
- 派发后流式渲染管线 → 待补（streaming pipeline 文档）
- 卡片数据流 → `doc/architecture/cards-dataflow.md`
- 系统整体架构 → `doc/architecture/overview.md`
