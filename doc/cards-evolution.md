# 交互式卡片系统演进文档

本文记录 AgentHub 交互式卡片（尤其是 plan 方案选择卡）从初始设计到多问题翻页的完整演进过程，作为后续维护与扩展的参考。

## 目录

1. [架构总览](#1-架构总览)
2. [第一阶段：单问题 plan 卡](#2-第一阶段单问题-plan-卡)
3. [踩过的坑](#3-踩过的坑)
4. [第二阶段：命名统一重构](#4-第二阶段命名统一重构)
5. [第三阶段：多问题翻页](#5-第三阶段多问题翻页)
6. [数据结构演进对照](#6-数据结构演进对照)
7. [扩展指南](#7-扩展指南)

---

## 1. 架构总览

卡片系统横跨三层，由 `render_card` MCP 工具驱动：

```
Agent 调 render_card(card_type, ...)
  ↓ daemon 工具产出卡片 JSON
TaskContext collector 收集（临时文件 IPC）
  ↓ task.completed 事件携带 cards[]
后端 handleTaskComplete 反序列化 Cards
  ↓ createAgentReply 写 cards_json
WS message.complete 推送 cards_json
  ↓ 前端 useWebSocket 透传
MessageBubble → CardRegistry 查 CardSpec → 组件渲染
  ↓ 用户交互
CardSpec.reduceAction → PATCH cards_json 持久化
```

**单一事实源**：`card_type` 字符串是协议契约，必须三层一致：
- daemon 工具 inputSchema + run 分支
- 后端 `context_agent_config.go` 提示词
- 前端 `types/card.ts` CardType union + `CardRegistry.tsx` 注册 key

### 命名约定（第二阶段确立）

```
type key  = 接口名去 Card 后缀的小写 = 组件名去 Card 后缀

type key      接口            组件文件
plan          PlanCard        PlanCard.tsx
approval      ApprovalCard    ApprovalCard.tsx
progress      ProgressCard    ProgressCard.tsx
info          InfoCard        InfoCard.tsx
```

读到任何一个名字立刻知道其余两个，无需猜映射。

---

## 2. 第一阶段：单问题 plan 卡

### 数据结构

plan 卡片最初只支持**单个问题**，结构扁平：

```jsonc
{
  "type": "plan",
  "id": "<uuid>",
  "title": "部署方案选择",
  "options": [
    { "id": "opt1", "label": "SaaS", "description": "...", "recommended": true },
    { "id": "opt2", "label": "私有化", "description": "..." }
  ],
  "selected_option": "opt1",   // 用户选择后写入
  "state": "resolved"          // resolved 后只读
}
```

### 交互

- 单选 Radio.Group，选完点「确认选择」
- `onAction(card.id, 'select_plan', { option_id })` → CardSpec reducer 标记 resolved

### 局限

- 一张卡只能问一个问题，Agent 想问多个就得发多张卡，无法在一个上下文里组织多个关联选择

---

## 3. 踩过的坑

这一路踩了三个结构性 bug，都是"分层开发没对齐契约"的典型后果。

### 3.1 命名错位（plan vs plan_selection）—— 卡片不渲染

**现象**：daemon 日志显示 `cards=[4 张卡片满载]`，前端一片空白。

**根因**：daemon 工具 emit 的 `card.type` 是 `plan_selection` / `approval` / `task_status` / `info`（4 种），但前端 CardRegistry 注册的 key 是 `plan` / `progress` / `confirm`（3 种）——**4 vs 3 全错位**，且少一个。`renderCards` 的 `if (!spec) return null` 把所有卡片静默丢弃。

**教训**：协议字符串（type key）是跨层契约，必须单一事实源。分层开发时没把"工具 emit 什么"和"registry 认什么"对齐。

### 3.2 HTTP 方法不匹配（PUT vs PATCH）—— 状态刷新后丢失

**现象**：用户选择后前端显示"已选择"，刷新浏览器状态消失。

**根因**：前端 `updateMessageCards` 发 **PUT**，后端路由注册的是 **PATCH**：
```
后端: convRoutes.PATCH("/:id/messages/:messageId/cards", ...)
前端: return put<...>(`/api/conversations/.../cards`, ...)
```
PUT 打 PATCH 路由 → Gin 找不到匹配 → **404**，PATCH 从没执行，DB 里还是未解决状态。前端是乐观更新所以 UI 显示对了，但数据没落库。

**修复**：加 `patch` helper，`updateMessageCards` 改用 `patch`。

**教训**：乐观更新会掩盖后端失败——用户操作看似成功，实际没持久化。这类 bug 只有刷新才暴露。

### 3.3 历史消息 cards 数组恒空

**现象**：刷新后历史消息的卡片数据读不回来。

**根因**：`Message.Cards`（`db:"-"`）只在 `Create` 时填充，`ListByConversation`/`GetByID` 等历史读路径不反序列化 `cards_json`。

**修复**：加 `fillCards()` 在 `fillAttachmentsAndReply` 里统一反序列化。

---

## 4. 第二阶段：命名统一重构

把"工具 emit / registry 认 / 类型定义"三套不一致的命名，统一成单一事实源。

### 改动

| 旧 | 新 | 说明 |
|---|---|---|
| `plan_selection` | `plan` | type key 简化 |
| `task_status` | `progress` | 避免与 `move_task_status` 工具混淆 |
| `PlanSelectionCard` | `PlanCard` | 接口名对齐 |
| `TaskStatusCard` | `ProgressCard` | 接口名对齐 |
| `ConfirmCardView` | `ApprovalCard` | 组件名 = 接口名，去 View 后缀 |
| `ProgressCardView` | `ProgressCard` | 同上 |
| （无） | `InfoCard` + `InfoCard.tsx` | 补全第 4 种卡片的渲染器 |

### 兼容策略

旧 key 通过 `CARD_TYPE_ALIASES` 别名表兼容历史 DB 数据：
```ts
const CARD_TYPE_ALIASES = { task_status: 'progress' };  // plan_selection 后续移除
```
`getCardSpec` 查不到时回退查别名，旧卡片继续渲染无需数据迁移。

---

## 5. 第三阶段：多问题翻页

### 需求

一张 plan 卡支持**多个问题**，用户翻页逐个选择，全部答完后统一提交。

### 决策

1. **数据模型**：plan 卡内置 `questions[]`，每个 question 有自己的 `options` + `selected_option`
2. **提交**：统一全部提交——底部一个按钮，所有问题都选完才可提交
3. **兼容**：不兼容旧 `options` 单问题结构，强制迁移（删除 `plan_selection` 别名）

### 新数据结构

```jsonc
{
  "type": "plan",
  "id": "<uuid>",
  "title": "方案选择",              // 卡片总标题
  "questions": [                    // 多个问题
    {
      "id": "q1",
      "title": "部署方案？",
      "options": [
        { "id": "opt1", "label": "SaaS", "recommended": true }
      ],
      "selected_option": "opt1",    // 提交后写入
      "state": "resolved"           // question 级状态
    },
    {
      "id": "q2",
      "title": "数据库选型？",
      "options": [...]
    }
  ],
  "state": "resolved"               // 卡片级状态（全部提交后）
}
```

### 交互设计

**未提交态**（`card.state !== 'resolved'`）：
- 头部分页导航：当前问题标题 + `2/3` 指示 + ‹ › 翻页按钮
- 每题单选，临时选择存本地 `answers`，未提交前可改
- 底部：
  - 非最后一页：「上一题」+「下一题」（本题未选则禁用）
  - 最后一页：「上一题」+「提交全部选择」（有任一未选则禁用）

**已提交态**（`card.state === 'resolved'`，含刷新后历史）：
- 整卡只读，各题显示已选答案，仍可翻页查看
- 底部显示「已提交 N/M」

**提交动作**：`onAction(card.id, 'submit_plan', { answers: { q1: 'opt1', q2: 'opt2' } })`
一次性把所有答案交给 reducer，reducer 把卡片和每个 question 都标记 resolved。

### 三层改动

| 层 | 文件 | 改动 |
|---|---|---|
| daemon | `agenthub-daemon.js` | inputSchema `options`→`questions`（嵌套）；run 分支规整化每个 question |
| 后端 | `context_agent_config.go` | plan 提示词行改引导 `questions=[...]` |
| 前端类型 | `types/card.ts` | 新增 `PlanQuestion` 接口；`PlanCard.options`→`questions` |
| 前端组件 | `PlanCard.tsx` | 重写：翻页 + 统一提交 + 历史只读 |
| 前端注册 | `CardRegistry.tsx` | spec 适配 `submit_plan`（reducer 写每个 question 的 selected_option） |
| 前端样式 | `Cards.module.css` | 新增 `.planPager` / `.planNavBtn` 等分页样式 |
| 清理 | `CardRegistry.tsx` | 删除 `plan_selection` 别名（强制迁移） |

---

## 6. 数据结构演进对照

### plan 卡片 type key

| 阶段 | type key | 说明 |
|---|---|---|
| 初始 | `plan_selection` | 工具 emit，registry 不认（错位 bug） |
| 第二阶段 | `plan` | 统一命名 |

### plan 卡片数据结构

**旧（单问题）**：
```jsonc
{ "type": "plan", "options": [...], "selected_option": "opt1", "state": "resolved" }
```

**新（多问题）**：
```jsonc
{
  "type": "plan",
  "questions": [
    { "id": "q1", "title": "...", "options": [...], "selected_option": "...", "state": "resolved" }
  ],
  "state": "resolved"
}
```

### 状态字段语义

| 字段 | 层级 | 值 | 含义 |
|---|---|---|---|
| `card.state` | 卡片级 | `resolved` | 所有问题已提交 |
| `question.state` | 问题级 | `resolved` | 该问题已选（提交后） |

---

## 7. 卡片联调检查清单（核心章节）

> **卡片系统是三层联动系统**——daemon（工具协议）↔ 后端（传输+存储）↔ 前端（类型+渲染+交互）。
> 改任何一处卡片，都必须按下面的清单核对所有相关触点。**只改单独卡片不联调，是前几轮所有 bug 的根因。**

### 7.1 协议契约：单一事实源

`card_type` 字符串是跨层契约，**必须四处一致**。任何一处不一致 → 卡片静默不渲染（`renderCards` 里 `if (!spec) return null`，前端看不到但后端已落库）：

| # | 层 | 文件 | 位置 | 内容 |
|---|---|---|---|---|
| 1 | daemon | `src/daemon-npm/bin/agenthub-daemon.js` | run() 分支（~L3321） | `if (args.card_type === 'xxx')` |
| 2 | 后端提示词 | `src/backend/internal/service/context_agent_config.go` | L66-69 | `render_card(card_type="xxx", ...)` |
| 3 | 前端类型 | `src/frontend/src/types/card.ts` | L28 CardType union | `'xxx'` 成员 |
| 4 | 前端注册 | `src/frontend/src/components/chat/cards/CardRegistry.tsx` | registerCard 调用 | `registerCard('xxx', {...})` |

**联调自检**：改完后 grep 确认四处字符串一致。

### 7.2 新增一种 card_type —— 必改 5 处

| # | 文件 | 位置 | 改什么 |
|---|---|---|---|
| ① | `src/daemon-npm/bin/agenthub-daemon.js` | inputSchema（~L3253）+ run 分支（~L3321）+ description（L3244） | 加新 payload 字段 schema；加 `else if` 填充分支；description 列举新类型 |
| ② | `src/backend/internal/service/context_agent_config.go` | L66-69 段 | 加一行 `sb.WriteString("- xxx：render_card(card_type=\"xxx\", ...)\n")` |
| ③ | `src/frontend/src/types/card.ts` | L28 union + 新接口 + L102 联合 | 加 `'xxx'` 到 CardType；加 `XxxCard extends BaseCard`；加进 InteractiveCard |
| ④ | `src/frontend/src/components/chat/cards/XxxCard.tsx` | 新文件 | 写组件（文件名=导出名=接口名，见命名约定） |
| ⑤ | `src/frontend/src/components/chat/cards/CardRegistry.tsx` | 末尾 | `import` + `registerCard<XxxCard>('xxx', {component, reduceAction?, actionToMessage?})` |

**不需要改**（弱类型透传层，自动支持任何 card_type）：
- 后端 `handleTaskComplete`（`handler/daemon.go`，`[]map[string]any`）
- 后端 `createAgentReply` / `fillCards` / `UpdateMessageCards`（`service/message.go` + `repository/message.go`）
- 后端 `UpdateCard` handler + 路由（`handler/message.go` + `router.go`）
- 后端 ToolSpec `platform.go` / `main.go` 注册
- 前端 `MessageBubble.tsx`（通过 CardSpec 委托，零 hardcode）
- 前端 `useWebSocket.ts` / `api/message.ts`（透传 cards_json）
- DB migration（cards_json 是无 schema JSON，L054）

### 7.3 已有 card_type 新增一个 action —— 必改 2-3 处

| # | 文件 | 改什么 |
|---|---|---|
| ① | `src/frontend/src/types/card.ts` | 如需新字段（如 `skipped?`）加到对应接口 |
| ② | `src/frontend/src/components/chat/cards/CardRegistry.tsx` | 对应 `registerCard` 的 `reduceAction` 加分支 + `actionToMessage` 加翻译 |
| ③ | `src/frontend/src/components/chat/cards/XxxCard.tsx` | 新按钮 → `onAction(card.id, 'new_action', {...})` |

**不需要改**：MessageBubble（`handleCardAction` 设计为查 CardSpec 委托，不感知具体 action）、后端、daemon、DB。

### 7.4 改卡片数据结构（如 plan 的 options→questions）—— 全链路

涉及数据结构变更时，除 7.2 的 5 处外，还要检查：
- **daemon run() 分支**：规整化新结构的字段
- **前端组件**：重写渲染逻辑适配新结构
- **CardRegistry reducer**：适配新结构的持久化
- **兼容性决策**：旧数据是否迁移？（加 CARD_TYPE_ALIASES 别名 / 强制迁移删除别名）
- **历史数据**：cards_json 里旧结构的卡片刷新后如何显示？

### 7.5 完整数据流（改完后的验证路径）

改完任何卡片后，按此链路验证端到端：

```
Agent 调 render_card(card_type, ...)
  → daemon run() 填充 payload，写 --card-file 临时文件
  → TaskContext collector finalize('cards')
  → task.completed 事件携带 cards[]
  → 后端 handleTaskComplete 反序列化 Cards（弱类型透传）
  → createAgentReply 存 cards_json + 广播
  → WS message.complete 推送 cards_json
  → 前端 useWebSocket 透传 → store
  → MessageBubble parsedCards → renderCards → CardRegistry 查 CardSpec → 组件渲染
  → 用户交互 → CardSpec.reduceAction → PATCH cards_json 持久化
  → 刷新 → fillCards 历史读取 → 恢复已解决状态
```

### 7.6 触点速查表（全量）

#### daemon（工具协议）
| 触点 | 文件:行 | 说明 |
|---|---|---|
| 工具定义 | `daemon-npm/bin/agenthub-daemon.js:3242-3359` | description + inputSchema + run() 分发 |
| IPC 通道 | `daemon-npm/bin/agenthub-daemon.js:88-115` | cards collector（临时文件） |
| 工具名清单 | `daemon-npm/bin/agenthub-daemon.js:3375` | PLATFORM_TOOL_NAMES（强制注入） |

#### 后端（传输+存储，弱类型透传）
| 触点 | 文件:行 | 说明 |
|---|---|---|
| 系统提示词 | `internal/service/context_agent_config.go:63-70` | **唯一需要改的后端文件**（加 card_type 说明） |
| ToolSpec | `internal/service/tool_specs/platform.go` | 工具元数据，改 card_type 通常不动 |
| 注册 | `cmd/server/main.go:487-488` | mustRegister，新增工具才改 |
| WS 反序列化 | `internal/handler/daemon.go:294-324` | Cards 字段，弱类型 |
| WS 类型 | `pkg/ws/daemon_hub.go:18-24` | TaskResult.Cards |
| 落库 | `internal/service/message.go:1024-1043` | createAgentReply 存 cards_json |
| 历史读 | `internal/repository/message.go:504-519` | fillCards 反序列化 |
| 用户回写 | `internal/handler/message.go:302-328` | UpdateCard handler |
| 路由 | `internal/router/router.go:116` | `PATCH /:id/messages/:messageId/cards` |

#### 前端（类型+渲染+交互）
| 触点 | 文件:行 | 说明 |
|---|---|---|
| 类型定义 | `types/card.ts:28,102` | CardType union + InteractiveCard 联合 |
| 注册表 | `components/chat/cards/CardRegistry.tsx` | registerCard + getCardSpec + renderCards |
| 各组件 | `components/chat/cards/*.tsx` | PlanCard/ApprovalCard/ProgressCard/InfoCard |
| 消费端 | `components/chat/MessageBubble.tsx:423-465,694-698` | parsedCards + handleCardAction（零 hardcode） |
| WS 投递 | `hooks/useWebSocket.ts:95-113` | cards_json 透传 |
| API client | `api/message.ts:112-121` | updateMessageCards（PATCH） |
| 工具门控 | `components/agent/AgentCreateModal.tsx:41,81,124,128-133` | render_card 强制选中 |

#### DB
| 触点 | 文件 | 说明 |
|---|---|---|
| cards 列 | `migrations/054_add_message_cards.sql` | cards_json TEXT 列（无 schema） |
| 工具 seed | `migrations/055_seed_render_card_tool.sql` | render_card 工具注册（改 card_type 不动） |

### 7.7 常见陷阱清单

- ❌ 改了 type key 没同步三层 → 卡片静默不渲染（见 3.1）
- ❌ 用 PUT 打 PATCH 路由 → 状态不持久化（见 3.2）
- ❌ 历史读路径不反序列化 cards_json → 刷新后卡片丢失（见 3.3）
- ❌ 组件名 ≠ 接口名 → 读代码靠猜映射
- ❌ 只改组件不改工具 inputSchema → Agent 调不出新字段
- ❌ 只改工具不改提示词 → Agent 不知道有新类型
- ✅ 改协议字符串前，grep 确认三层引用都更新
- ✅ 改完后按 7.5 数据流端到端验证

---

## 8. v3：文本路径回归（fenced JSON block）

**时间**：2026-06

**动机**：v2 的 `render_card` MCP 工具返回 `{rendered:true}` 零信息，但为维持 tool_use 语义堆了 300+ 行 IPC 基础设施（cardFile / `--card-file` / collector 生命周期），且有并发隐患（读-改-写非原子，长驻 slot 场景下卡片丢失）。

**变更**：

- 删除 `render_card` MCP 工具
- 删除 `cardFile` 跨进程文件 IPC（agentId 固定路径的 tmp 文件）
- 删除 `--card-file` spawn 参数注入
- 删除 cards collector（reset/drain/cleanup 生命周期）
- Agent 在回复正文写 ` ```json {"cards":[...]} ``` ` fenced block
- 后端 `extractCardsFromContent` 解析 + 剥离 block + 替换为 `[CARD:id]` 占位符
- daemon 内部卡片（`deploy_project` info 卡）改走 `TaskContext.daemonCards` 内存队列，task.completed 时 emit
- 后端合并 daemon emitted cards + 文本解析 cards 为单一数组
- 修复 `UpdateMessageCards` 不广播的 bug（新增 `UpdateMessageCardsAndBroadcast`）

**收益**：

- 净代码 -200 行
- 消除 cardFile 并发隐患
- Agent prompt 反而更简单（"输出 fenced block" 比 "调 render_card 工具" 更直接）
- 卡片定义单一事实源：`context_agent_config.go` prompt + `types/card.ts`（2 处而非 3 处，daemon 不再参与）

**代价**：

- 结构可靠性从 "LLM API 强制 JSON Schema" 降级到 "LLM 遵守 prompt 输出 fenced block"
- 缓解：parser 静默丢弃 + 跑批验证遵循率 + 失败时文字回复仍可见
