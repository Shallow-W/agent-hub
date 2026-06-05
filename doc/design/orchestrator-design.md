# Orchestrator 设计文档

## 概述

群聊多 Agent 协作需要一个协调者（Orchestrator，简称 Orch），负责分析用户意图、拆解任务、分派给合适的 Agent、聚合结果并回复用户。

## 核心概念

### 两种模式

#### 模式 1 — 固定 Orch（显式）

群聊设置中指定一个 Agent 为固定 Orch，所有消息都由它协调分派。

适用场景：团队有明确的协调者，任务分派策略稳定。

#### 模式 2 — 动态 Orch（按消息自动，默认）

用户在消息中 `@agent-1 @agent-2 @agent-3` 同时 @ 多个 Agent，没有 @ 固定 Orch 时：
- **第一个被 @ 的 Agent 自动成为该轮任务的 Orch**
- 它负责分析意图、拆解任务、分派给后续被 @ 的 Agent
- 下一轮消息 @ 顺序变了，Orch 随之切换

```
消息 1: @Claude-Code 请分析这个架构 @Codex 写测试
        → Claude-Code 是这轮 Orch，协调分派给 Codex

消息 2: @Codex 重构这个函数 @Claude-Code review
        → Codex 是这轮 Orch，协调分派给 Claude-Code
```

适用场景：快速协作，不同任务由不同 Agent 主导。

### 判定优先级

```
1. 消息是否 @ 了固定 Orch？
   → 是：固定 Orch 协调所有被 @ 的 Agent
   → 否：进入第 2 步

2. 消息是否 @ 了多个 Agent？
   → 是：第一个被 @ 的 Agent 担任该轮 Orch
   → 否（只 @ 了一个）：该 Agent 直接执行，无需协调
```

### Orch 的职责

```
用户发消息（@ 多个 Agent）
        ↓
   Orch 收到消息（带完整上下文 + 可用 Agent 列表）
        ↓
   Orch 分析意图，决定分派策略
        ↓
   ┌──────────┼──────────┐
   ↓          ↓          ↓
自己处理   分派 worker-1  分派 worker-2
   ↓          ↓          ↓
   └──────────┼──────────┘
              ↓
   Orch 收集结果，聚合回复用户
```

## 数据模型

### conversation_agents 表变更

扩展 `role` 字段：

| 值 | 含义 |
|----|------|
| `'orchestrator'` | 固定 Orch（显式模式） |
| `'worker'` | 工作节点（默认角色） |

```sql
ALTER TABLE conversation_agents
  DROP CONSTRAINT IF EXISTS conversation_agents_role_check;

ALTER TABLE conversation_agents
  ADD CONSTRAINT conversation_agents_role_check
  CHECK (role IN ('orchestrator', 'worker'));
```

每个群聊最多一个 `role='orchestrator'` 的 Agent。动态模式下所有 Agent 均为 `role='worker'`。

### 不新增表

动态 Orch 是运行时逻辑（看消息 @ 顺序），不需要持久化。固定 Orch 通过 `conversation_agents.role` 标识。

## 运行时行为

### 固定 Orch 模式

1. 用户发消息 @ 多个 Agent
2. 后端识别该群聊有固定 Orch
3. 将消息 + @ 列表 + 可用 worker 信息发送给 Orch
4. Orch 决定分派策略，通过平台消息机制调度 worker
5. Worker 执行完毕后结果回传 Orch
6. Orch 聚合结果，回复用户

### 动态 Orch 模式

1. 用户发消息 `@Agent-A @Agent-B @Agent-C`
2. 后端解析 @ 顺序：`[Agent-A, Agent-B, Agent-C]`
3. Agent-A 成为本轮 Orch（第一个被 @ 的）
4. 将消息 + worker 列表（Agent-B, Agent-C）+ 协调指令注入 Orch 的 system prompt
5. Orch 分派，Worker 执行，结果聚合
6. Orch 回复用户

### 单 Agent 场景

只 @ 了一个 Agent 时，无论是否为固定 Orch，都直接执行，不走协调流程。

### Orch 被移除

固定 Orch 被移出群聊时：
- 如果群内还有其他 Agent，将 `joined_at` 最早的 Agent 提升为 Orch
- 如果只剩一个 Agent，它自动成为 Orch（也直接处理）

## 上下文注入

确定 Orch 后，后端为 Orch 构造增强 system prompt：

```
你是本轮的协调者（Orchestrator）。用户发送了一条消息，需要多个 Agent 协作完成。

可用 Agent：
- @Codex (worker): 擅长代码编写和测试
- @OpenCode (worker): 擅长代码实现

用户消息：{原始消息}

请分析用户需求，决定任务拆解和分派策略：
1. 如果你自己就能完成，直接回复
2. 如果需要其他 Agent 协作，说明分派计划
3. 收到所有 Worker 结果后，聚合并回复用户
```

Worker 收到的 system prompt 只包含执行指令：

```
你是群聊中的工作 Agent。Orchestrator 分派了以下任务给你：

任务：{具体子任务描述}
上下文：{相关背景信息}

请完成该任务并返回结果。
```

## API 变更

### 新增接口

| 方法 | 路径 | 说明 |
|------|------|------|
| PUT | `/api/conversations/:id/agents/:agentId/role` | 切换 Agent 角色（设为/取消 Orch） |

请求体：
```json
{ "role": "orchestrator" | "worker" }
```

规则：设为 Orch 时，原 Orch 自动降级为 worker。

### 修改接口

`POST /api/conversations/:id/agents` 添加 Agent 时：
- 群聊第一个 Agent 自动设置 `role='orchestrator'`
- 后续 Agent 默认 `role='worker'`

## 前端交互

### 群聊设置页
- Agent 列表中固定 Orch 显示 `Orch` 标签
- 长按/右键 Agent 可选「设为 Orch」/「取消 Orch」
- 取消固定 Orch 后切换为动态模式

### 发消息时
- `@` 列表中固定 Orch 显示 `Orch` 后缀
- 动态模式下 `@` 顺序暗示 Orch（第一个）
- 可选：`@` 列表旁加 `👑` 按钮，让用户手动指定谁是这轮的 Orch（覆盖默认顺序）

### 消息流展示
- Orch 的回复中可折叠展示各 Worker 的子任务结果
- 每个 Worker 的执行状态实时显示（等待中/执行中/已完成）

## 不做的事

- 不做"无 Orch"广播模式——Orch 本身可以选择广播分派
- 不做 Orch 嵌套（Orch of Orch）
- 不做 Orch 自动选举/轮换——按 @ 顺序或固定指定即可
- 不做 Orch 策略可配置——先硬编码在 system prompt 中，后续再抽象

## 实现优先级

1. **P0**: 动态 Orch（按 @ 顺序）— 零配置，开箱即用
2. **P1**: 固定 Orch 设置 + 切换 API — 显式控制
3. **P2**: 上下文注入模板优化 — 提升协调质量
4. **P3**: 前端 Orch 标签和交互完善
