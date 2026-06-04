# 群聊 Agent 上下文工程与 Orchestrator 调度

## 背景

AgentHub 群聊中需要多个 Agent 协作完成任务。用户通过 @mention 与特定 Agent 交互，或通过 Orchestrator Agent 拆解复杂任务并分派给子 Agent。

## 核心设计

### 两种 @mention 模式

**模式一：直连模式（Direct）**
```
用户 @AgentA "帮我设计数据库"
  → 直接 dispatch AgentA，无 Orchestrator 介入
  → AgentA 回复发到群聊
```

**模式二：编排模式（Orchestrated）**
```
用户 @Orch "设计认证系统"
  → Orch 拆解任务 → @AgentA @AgentB
  → 后端解析，dispatch 各 Agent
  → Agent 完成 → @Orch 验收
  → Orch 审核/汇总 → 发到群聊
```

### 路由机制

- **@mention 是统一路由原语**，不区分用户消息和 Agent 消息
- Orchestrator 本身就是一个普通 Agent（type=orchestrator），只是 system prompt 不同
- 后端解析消息中的 @mention，匹配群聊中的 Agent 成员，创建 DaemonTask

### 上下文三层架构

| 层 | 管理者 | 内容 | 持久性 |
|----|--------|------|--------|
| Layer 1: CC Session | Claude Code CLI | Agent 自身的历史交互、推理过程 | 随 session 持久 |
| Layer 2: Dispatch Prompt | 后端组装 | 群聊背景摘要 + 具体任务 + 依赖结果 | 每次按需生成 |
| Layer 3: Orchestrator Session | Orch 的 CC session | 完整编排上下文、所有 worker 结果 | 随 session 持久 |

**Layer 1 — CC Session（零成本）**
- 每个 Agent 有独立 CC session：UUID v5(conversationID + agentID)
- Agent 记得自己在该群聊中的所有历史交互
- Agent 之间互相看不到对方的 session（隔离）

**Layer 2 — Dispatch Prompt（按需注入）**
- 后端为每个被 @ 的 Agent 构建独立 prompt
- 每个 Agent 只拿到分配给自己的任务描述，不是 Orch 的完整输出
- 结构：
  ```
  [群聊背景]
  - 最近 N 条消息的压缩摘要

  [调度指令]
  Orch @你，分配了以下任务：
  {具体任务描述}

  [依赖输出]（如有）
  AgentX 已完成 {task}，结果摘要：
  {compressed_result}

  请完成这个任务并在回复末尾 @Orch 表示完成。
  ```

**Layer 3 — Orchestrator Session**
- Orch 的 CC session 跨越整个编排过程
- Orch 记得自己拆了什么任务、分给了谁
- Agent @Orch 验收时，Orch 不需要额外注入历史上下文

### Orch 输出格式约定

Orch 分派任务时，system prompt 约定以下格式：

```
我来拆解这个任务：

@AgentA 设计用户认证的数据库 schema，包括 users 表和 refresh_tokens 表

@AgentB 编写认证相关的 RESTful API 接口
```

**串行依赖标记**（默认并行，`→` 表示串行）：
```
@AgentA 设计数据库 schema

→ @AgentB 根据 @AgentA 的结果编写 API 接口
```

### 解析规则

1. 正则匹配 `@AgentName`，提取每段 @mention 到下一个 @mention（或文末）之间的文本
2. 每个 @mention 前有 `→` → 有依赖，串行；无 `→` → 并行
3. 依赖目标从任务描述中提取 `@AgentName` 引用
4. 非 @mention 开头的文本段落 = Orch 对群聊的说明，不触发分发

### 每个 Agent 的上下文区分

后端为每个被 @ 的 Agent 构建独立的 DaemonTask：
- `prompt` = 群聊背景 + 该 Agent 专属的任务描述 + 依赖输出
- `context_messages` = 群聊最近 N 条消息的压缩
- 不同 Agent 收到不同的任务描述，互不干扰

### 并行 vs 串行执行

**并行**（默认）：后端同时创建多个 DaemonTask，各自独立执行

**串行**（Orch 标记 `→`）：
1. 先创建前置 Agent 的 DaemonTask
2. 前置 Agent 完成 → @Orch
3. Orch 验收满意 → Orch 输出新 dispatch → 后端创建后续 Agent 的 DaemonTask
4. 后续 Agent 的 prompt 注入前置 Agent 的结果摘要

串行的关键：**Orch 掌控调度权**——前置做得不好可以让它重做，不用浪费后续 Agent 调用。

### 完整流程示例

```
Turn 1: 用户 @Orch "设计认证系统"
         → 后端 dispatch Orch
         → Orch 输出：@AgentA 设计DB，@AgentB 写API
         → 后端解析 → 并行创建 AgentA、AgentB 的 DaemonTask

Turn 2: AgentA 完成 → 回复含 @Orch
         → 后端 dispatch Orch 验收
         → Orch 的 CC session 已有记忆，判断 AgentA 结果

Turn 3: AgentB 完成 → 回复含 @Orch
         → 后端 dispatch Orch 验收
         → Orch 判断全部完成，输出最终汇总

Turn 4: Orch 最终汇总发到群聊
```

### 防护机制

| 风险 | 防护 |
|------|------|
| Orch ↔ Agent 死循环 | 单次编排最大 round-trip 10 轮 |
| 编排超时 | 整体超时 5 分钟 |
| 并发冲突 | 同一群聊同时只允许 1 个编排流程 |
| Agent 不响应 | DaemonTask 已有 120s 超时机制 |

## 实现范围

### 后端改动

1. **@mention 路由器**（新增）
   - 解析消息中的 @AgentName，匹配群聊 Agent 成员
   - 区分直连模式和编排模式
   - 路径：`internal/service/orchestrator.go`

2. **Orch 输出解析器**（新增）
   - 正则提取 @mention + 任务描述 + 依赖关系
   - 路径：`internal/service/orchestrator.go`

3. **Dispatch prompt 构建器**（修改）
   - 启用现有 `buildAgentHandoffs()`，改造为 Layer 2 上下文构建
   - 填充 `DaemonTask.ContextMessages`
   - 路径：`internal/service/message.go`

4. **编排状态跟踪**（新增）
   - 跟踪当前编排的 round-trip 计数、超时
   - 路径：`internal/service/orchestrator.go`

### 前端改动

- 无强制改动，@mention 已有 UI 支持
- 可选：Orch 调度过程的可视化（步骤指示器）

### Orchestrator Agent 配置

- type = "orchestrator"
- system prompt 包含：分派格式约定、验收流程、汇总模板
- 与普通 Agent 共享 CC session 管理机制

## 与现有代码的对接点

| 现有代码 | 改动 |
|----------|------|
| `buildAgentHandoffs()` (message.go:480-541) | 启用并改造为 dispatch 上下文构建 |
| `DaemonTask.ContextMessages` 字段 | 填充 Layer 2 上下文 |
| `createAgentReply()` (message.go:544-595) | 增加 @mention 路由分支 |
| `message.mentions` 字段 | 用于触发路由而非仅存储 |
| CC session 持久化 (已实现) | 直接复用 |
| Daemon task 轮询 (已实现) | 直接复用 |
| WebSocket push (已实现) | 直接复用 |
