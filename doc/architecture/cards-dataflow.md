# 卡片信息流（Cards Data Flow）

> 本文聚焦「卡片从产生到渲染的完整信息流」，覆盖 6 种卡片类型，重点讲 fenced JSON block 文本协议 + `[CARD:id]` 占位符 + diff 卡片的 workDir 解耦设计。卡片系统的演进史见 [`doc/cards-evolution.md`](../cards-evolution.md)。

## 目录

1. [全链路总览](#1-全链路总览)
2. [阶段一：Agent 在回复正文写 fenced JSON block](#2-阶段一agent-在回复正文写-fenced-json-block)
3. [阶段二：后端解析与剥离](#3-阶段二后端解析与剥离)
4. [阶段三：daemon → 后端 → 数据库](#4-阶段三daemon--后端--数据库)
5. [阶段四：前端拆段 + 占位符渲染](#5-阶段四前端拆段--占位符渲染)
6. [阶段五：diff 卡片的二次数据流（workDir 解耦）](#6-阶段五diff-卡片的二次数据流workdir-解耦)
7. [踩过的坑与关键约束](#7-踩过的坑与关键约束)

---

## 1. 全链路总览

```
┌─────────┐  回复正文写 ```json {"cards":[...]} ```  ┌──────────────┐
│  Agent   │ ─────────────────────────────────────▶ │  daemon MCP  │
│ (子进程)  │                                         │  子进程        │
└─────────┘                                         └──────┬───────┘
       │                                                     │
       │  （daemon 内部卡片如 deploy_project info 卡            │
       │   走 TaskContext.daemonCards 内存队列）                │
       │                                                     ▼
       │                       ┌──────────────────────────────────┐
       │                       │ daemon task.completed             │
       │                       │   { result, cards: daemonCards }  │
       │                       └──────────────┬───────────────────┘
       │                                      ▼
       │         ┌────────────────────────────────────────────────┐
       └───────▶ │ 后端 createAgentReply                           │
                 │   extractCardsFromContent(result.Result)        │
                 │     → 解析正文 fenced block → agentCards        │
                 │     → 剥离 block，替换为 [CARD:id] 占位符        │
                 │   allCards = daemon cards + agentCards          │
                 │   msg.Content = 剥离后正文                        │
                 │   msgRepo.UpdateMessageCardsAndBroadcast         │
                 │     → msg.CardsJSON（DB）+ msg.Cards（WS）       │
                 └────────────────────┬───────────────────────────┘
                                      │ WS message.complete { cards_json }
                                      ▼
                 ┌────────────────────────────────────────────────┐
                 │ 前端 MessageBubble.splitByCardPlaceholder       │
                 │   按 [CARD:id] 占位符把 content 拆成段           │
                 │   → CardRegistry 按 type 查 CardSpec → 渲染      │
                 │   unmatchedCards 兜底渲染到末尾                   │
                 └────────────────────────────────────────────────┘
```

**关键设计原则**：

- **agent 只产数据，不产渲染**——agent 在正文写 fenced JSON block 上报结构化数据，绝不输出卡片 HTML/markdown
- **block 位置即渲染位置**——后端剥离 block 时自动替换为 `[CARD:id]` 占位符，卡片自然落在 agent 写 block 的位置
- **路径生产与浏览解耦**——agent 上报 `workDir`（绝对路径），前端浏览时用 `workDir` 反查 git，互不依赖

---

## 2. 阶段一：Agent 在回复正文写 fenced JSON block

### 提示词约定（`src/backend/internal/service/context_agent_config.go`）

系统提示词里有一段 `[卡片——重要]`，教 agent：

1. 卡片**必须**通过 fenced JSON block 产生，不能在文字里描述
2. block 的标准格式：

   ````markdown
   ```json
   {"cards":[{"type":"diff","id":"diff-1","title":"...","workDir":"/abs/path","files":["App.tsx"]}]}
   ```
   ````

3. 6 种 `card_type` 及其参数：
   - `plan` / `approval` / `progress` / `info`——交互/展示卡
   - `diff`——文件变更卡，参数 `workDir`（绝对路径）+ `files`（相对路径数组）
   - `project`——项目目录卡，参数 `workDir` + `summary?`

### block 位置即默认渲染位置

**关键约定**：agent 在正文哪里写 fenced block，后端剥离时就**在该位置插入 `[CARD:id]` 占位符**。例如：

````markdown
我来分析下变更：

```json
{"cards":[{"type":"diff","id":"d1","workDir":"/repo","files":["App.tsx"]}]}
```

接下来我会继续……
````

后端剥离后正文变为：

```markdown
我来分析下变更：

[CARD:d1]

接下来我会继续……
```

前端 `splitByCardPlaceholder` 见到 `[CARD:d1]` 就在该位置渲染 diff 卡片。

### 跨段引用（agent 手写占位符）

agent 也可以先在正文写占位符、再在任意位置写 block：

```markdown
看下面的变更 [CARD:diff-1]：

```json
{"cards":[{"type":"diff","id":"diff-1","workDir":"/repo","files":["App.tsx"]}]}
```
```

后端解析时若发现正文已含 `[CARD:id]`（与 block 内 id 匹配），则**不再插入新占位符**，保留 agent 手写位置。这一约定让 agent 可以在复杂回复里精确控制卡片渲染位置。

> ⚠️ **id 字段是占位符方案的核心**：block 内传 `id="diff-1"`，后端剥离时替换为 `[CARD:diff-1]`。不传 id 则后端生成随机 UUID，卡片只能落末尾（`unmatchedCards` 兜底）。

---

## 3. 阶段二：后端解析与剥离

后端 `createAgentReply` 收到 daemon `task.completed` 后，调用 `extractCardsFromContent` 解析正文 fenced block，剥离 block 替换为 `[CARD:id]` 占位符，合并 daemon emitted cards（见阶段三）。

```go
// 伪代码
agentCards, stripped := extractCardsFromContent(result.Result)
//   - 扫描 ```json fenced block
//   - 校验 JSON 结构 + card_type 合法性
//   - 剥离 block，在原位置插入 [CARD:id]
//   - 静默丢弃格式错误的 block（不影响回复可见性）

allCards := append(result.Cards, agentCards...)  // daemon cards + 文本 cards
msg.Content = stripped                            // 存剥离后正文
```

**关键点**：

- **解析容错**：parser 静默丢弃格式错误的 block，agent 的文字回复仍然可见
- **合并而非二选一**：daemon 内部卡片（`TaskContext.daemonCards`）与 agent 文本卡片合并为单一数组，互不覆盖
- **剥离与占位符同源**：同一个函数既剥离 block 又生成占位符，保证位置语义一致

---

## 4. 阶段三：daemon → 后端 → 数据库

### daemon 内部卡片收集

daemon 自身产生的卡片（如 `deploy_project` 的 info 卡）写入 `TaskContext.daemonCards` 内存队列，task 完成时一并 emit：

```js
// daemon 主进程
const cards = taskCtx.finalize('cards');           // daemonCards 队列
bus.emit('task.completed', { ..., result, cards });
// → sendTaskComplete({ ..., cards: info.cards || [] })
```

### 后端存储（`message.go` createAgentReply）

```go
// 1. 解析 agent 正文 fenced block + 剥离
agentCards, stripped := extractCardsFromContent(result.Result)
msg.Content = stripped

// 2. 合并 daemon emitted cards + 文本解析 cards
allCards := append(result.Cards, agentCards...)

// 3. 落库 + 广播
if len(allCards) > 0 {
    cardsJSON, _ := json.Marshal(allCards)
    s.msgRepo.UpdateMessageCardsAndBroadcast(ctx, msg.ID, string(cardsJSON))
    msg.CardsJSON = string(cardsJSON)  // DB 列
    msg.Cards = allCards               // 结构体（WS 广播用）
}
```

**合并逻辑**：`allCards = result.Cards（daemon emitted）+ agentCards（文本解析）`。两条来源互不依赖，任一缺失都不会使另一条丢失。

**剥离与占位符**：`msg.Content` 存**剥离 block 后**的正文，block 原位置已替换为 `[CARD:id]`。前端据此拆段渲染。

### WS 推送与多用户同步

`UpdateMessageCardsAndBroadcast` 修复了旧版 `UpdateMessageCards` 不广播的 bug——多用户场景下，A 用户 PATCH 卡片状态后，B 用户必须收到 WS 推送才能看到同步状态。新 API 在落库后**立即广播**，保证多端一致。

`message.complete` 事件携带 `cards_json`，前端 `useWebSocket` 透传到 Zustand store。

---

## 5. 阶段四：前端拆段 + 占位符渲染

### splitByCardPlaceholder（`MessageBubble.tsx` L61）

这是占位符方案的核心。把消息正文按 `[CARD:id]` 占位符拆成有序段：

```ts
function splitByCardPlaceholder(content, cards) {
  // 用 CARD_PLACEHOLDER_REGEX 扫描正文
  // 每个匹配的占位符：
  //   - 前面的文本 → markdown 段
  //   - 占位符本身 → card 段（从 cards 里找 id 匹配的卡片）
  // 末尾剩余文本 → markdown 段
  // 返回 { segments, unmatchedCards }
}
```

**占位符来源**：前端零改动，只消费后端剥离 block 时自动生成的占位符（或 agent 手写的占位符）。前端不关心占位符是 agent 写的还是后端生成的。

**三种渲染结果**：

| 场景 | segments | unmatchedCards | 渲染 |
|------|----------|----------------|------|
| 正文有 `[CARD:diff1]` + cards 含 diff1 | `[md, card, md]` | `[]` | 卡片嵌在占位符位置 ✓ |
| 正文无占位符 + 有 cards | `[md]` | 全部 cards | 末尾渲染卡片（兜底）|
| 正文有占位符但 cards 无匹配 | `[md]`（占位符当普通文本）| 全部 cards | 占位符显示成字面文本，卡片落末尾 |

### CardRegistry 分发（`CardRegistry.tsx`）

`Map<CardType, CardSpec>` 注册表，按 `card.type` 查到对应组件渲染。`renderCards(cards, agentId)` 透传 `agentId`（来自 `message.artifacts.agent_id`），供 diff/project 卡发 RPC 用。

---

## 6. 阶段五：diff 卡片的二次数据流（workDir 解耦）

diff/project 卡片渲染后，还有一条**独立的二次数据流**：用 agent 上报的 `workDir` 反查 git 状态和 diff。

### 解耦设计

```
Agent 上报（fenced block）            前端浏览（点击卡片时）
─────────────────────                ─────────────────────────
card.workDir = "/path/to/repo"       → fileStatus(agentId, workDir, files)
card.files   = ["App.tsx"]           → fileDiff(agentId, workDir, filePath)
                                     ↑ 不依赖 agent，直接查 git
```

**agent 只报路径，不报状态/内容**。状态（added/modified/deleted）和前后对比内容，都由前端调 RPC → daemon 跑 git 查询。

### RPC 链路（同步，20s 超时）

```
前端 fileStatus/fileDiff
  → GET /api/agents/:id/files/browse?action=status|diff&work_dir=...&files=...
  → 后端 BrowseAgentFiles（RegisterTaskPromise）
  → SendToMachine(task.dispatch { task_id: ... })  ← 字段名必须是 task_id
  → daemon handleTaskDispatch → browseFiles(action, workDir, files)
  → runGitSync / gitChangedFiles / parseGitStatus
  → task.complete → ResolveTask → 后端返回 JSON
```

**⚠️ 字段名契约**：后端 `SendToMachine` 必须用 `task_id`（不是 `id`）。daemon 读 `data.task_id`，字段名不匹配会导致 daemon 静默返回 → 后端超时 → 文件浏览器空白。

### daemon browseFiles action 路由（`agenthub-daemon.js` L692）

| action | 输入 | 输出 | 备注 |
|--------|------|------|------|
| `tree` | workDir | repoRoot + rootEntries | 目录树（project 卡用）|
| `status` | workDir + files[] | `[{path, status}]` | 复用 `gitChangedFiles`，按 wanted 过滤 |
| `diff` | workDir + path | `{oldContent, newContent}` | 默认工作区 vs HEAD |
| `list/read/log/show/zip` | workDir + path | 各异 | 文件抽屉的其他操作 |

### 前端预取优化（`DiffCard.tsx`）

diff 卡片 `useEffect` 里**同时**发：
- `fileStatus(agentId, workDir, files)` —— 查状态，渲染文件列表
- 每个文件的 `fileDiff(agentId, workDir, f)` —— **预取**前后内容，存 `useRef<Map>`

点击文件时，`DiffViewer` 优先用预取缓存（`initialDiff` prop），秒开，不发新 RPC。预取失败则 DiffViewer 兜底重新请求。

---

## 7. 踩过的坑与关键约束

### render_card 工具返回值零信息 → 过度工程化（已修）

**症状**：`render_card` MCP 工具返回 `{rendered:true}` 对 agent 零价值，但为维持 tool_use 语义堆了 300+ 行 IPC 基础设施（cardFile / --card-file 注入 / collector 生命周期），且有并发隐患（读-改-写非原子）。

**根因**：tool_use 应该用于「读取信息」或「改变世界 + 返回关键 id」，fire-and-forget 的副作用型输出（如卡片）不该用工具封装。返回值零信息意味着 agent 无法基于返回值决策，工具调用沦为「记录事件」的副作用通道。

**修复**：改为 agent 直接在回复正文写 fenced JSON block，后端解析。删除 `render_card` 工具 + `cardFile` 跨进程 IPC + `--card-file` spawn 参数注入 + collector（reset/drain/cleanup）生命周期。

**教训**：工具调用不是万能抽象。副作用型输出（卡片、日志、通知）用结构化文本（fenced block / sentinel）更轻。

### cardFile fast-path 丢失 [已废弃]

> 此坑记录的是 v2 cardFile 机制下的 bug。cardFile 整套机制已删除（见 [`doc/cards-evolution.md`](../cards-evolution.md) v3 节），此条仅作历史记录。

原症状：同一 agent 连续发两条消息，第二条（长驻 slot）的卡片丢失。根因：cardFile 绑 taskId 与长驻进程的 `--card-file` 参数不一致。

### parseGitStatus 只读 porcelain 第 0 列（已修）

**症状**：未暂存改动（` M`/` D`）状态全错。

**根因**：git porcelain 格式是两列 `XY`（X=staged，Y=工作区）。agent 改动通常未暂存，真实状态在 Y 列，但代码只读 X 列。

**修复**：`parseGitStatus` 接收两列 `line.slice(0,2)`，合并判定（删除>新增>修改优先级）。

### runGitSync 的 .trim() 吃掉首行空格（已修）

**症状**：status 查询里按字母序排第一的文件永远匹配不到。

**根因**：`execFileSync(...).trim()` 会 trim 整个输出字符串的首尾空白，导致 porcelain 首行 ` M App.tsx` 的行首空格被吞 → 变成 `M App.tsx` → `line[2]` 不再是空格 → 跳过该行。

**修复**：`runGitSync` 改用 `.replace(/\r?\n$/, '')` 只去尾部换行。

### status action 被共享 path 校验挡住（已修）

**症状**：status action 返回 "Invalid or out-of-root path"。

**根因**：`browseFiles` 有个共享校验 `safePathWithin(baseRoot, payload.path)`，但 status 用的是 `files` 数组而非单文件 `path`，`payload.path` 为空 → 校验失败。

**修复**：status 提前走自己的分支（在共享校验之前），绕过 path 校验。

### antd v6 Modal class 名变化（非 bug，诊断教训）

**症状**：误判 DiffViewer 内容空白。

**根因**：antd v5 的 Modal 内容在 `.ant-modal-content`，v6 改为 `.ant-modal-container`。测试脚本用旧选择器查不到，误以为没渲染。

**教训**：写 antd 相关的 DOM 断言前，先确认 antd 版本的 class 契约。

---

## 扩展指南：新增一种卡片类型

1. **后端提示词**：`src/backend/internal/service/context_agent_config.go` 的 `[卡片——重要]` 段补一行 card_type 说明（告诉 agent 该 card_type 的字段 schema）
2. **前端类型**：`src/frontend/src/types/card.ts` 加 `'xxx'` 到 `CardType` union，加 `interface XxxCard extends BaseCard`，加入 `InteractiveCard` 联合类型
3. **前端组件**：`src/frontend/src/components/chat/cards/` 新建 `XxxCard.tsx`，实现 `CardProps<XxxCard>`（文件名 = 导出名 = 接口名）
4. **注册**：`CardRegistry.tsx` 底部 `registerCard<XxxCard>('xxx', { component, reduceAction?, actionToMessage? })`

无需改：
- **后端 message.go**（弱类型透传，自动支持任何 card_type）
- **daemon**（不再参与卡片收集，agent 自己在正文写 block）
