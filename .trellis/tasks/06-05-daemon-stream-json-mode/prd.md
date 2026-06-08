# Daemon Stream-JSON 模式迁移

## Goal

将 daemon 的 per-task claude 交互从 `-p --output-format text` 切换到 spawn + `stream-json` 双向管道模式，引入 per-agent 进程槽位设计：同对话连续消息通过 stdin 直接注入（快路径），跨对话切换时重启进程并恢复对应 session（会话隔离）。

## Requirements

### 核心设计：Per-Agent 进程槽位 + 会话隔离

每个 agent 维护一个进程槽位（扩展现有 `runningAgents` Map），记录：
- `currentConversationId` — 当前正在服务的对话
- `sessionId` — 当前对话对应的 Claude Code session ID
- `process` — 已 spawn 的 Claude Code 子进程
- `sendPrompt` — 向 stdin 写入 JSON 消息的函数（复用现有 queueTail 串行队列）

#### 消息路由逻辑

```
task.dispatch arrives with { agent_id, conversation_id, prompt }
  → if running process exists AND conversation_id === currentConversationId:
      → stdin write JSON (快路径，零开销)
  → if running process exists BUT conversation_id !== currentConversationId:
      → kill process → spawn with --resume <convSessionId> (切对话)
  → if no running process:
      → spawn with --resume <convSessionId> or fresh session
      → stdin write JSON
```

#### Session 管理

- 每个 `(agent_id, conversation_id)` 组合对应一个 Claude Code session ID
- 首次遇到的对话：生成新的 sessionId（`agenthub-{agentId}-{convId}` 格式）
- 再次遇到的对话：`--resume <sessionId>` 恢复上下文
- sessionId 映射持久化到 `~/.agenthub/sessions.json`，daemon 重启后可恢复

### 传输协议

- Claude Code 启动参数：`--input-format stream-json --output-format stream-json --include-partial-messages --dangerously-skip-permissions --verbose`
- 消息输入：stdin JSON `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"..."}]}}`
- 结果解析：stdout JSON 流，提取 `result` 事件作为 turn 结束信号
- 结构化解析：从 stream-json 事件中提取 text 作为回复内容

### 崩溃恢复（复用现有能力）

- 进程异常退出时，保留 session 映射（复用 `idleAgentConfigs` 模式）
- 下次同对话任务到来时，`--resume` 恢复 session
- `--resume` 失败时降级为新建 session

### 并发任务排队（复用现有能力）

- 复用 `sendPrompt` 的 `queueTail` promise chain，串行化同 agent 的所有任务
- 排队中的任务如果目标是不同对话，会在自己执行时触发对话切换

### 超时处理（复用现有能力）

- 复用 120s 超时，超时 kill 进程并回传 error

### 兼容性

- codex / openclaw 等非 claude 工具保持现有行为（executeTask 路径不变）
- 后端不感知变化（task.dispatch → task.complete 协议不变）
- 现有 persistent 模式（agent.start 路径）可逐步合并到新统一路径（本次不改）

## Acceptance Criteria

- [ ] claude per-task 交互使用 stream-json 双向管道
- [ ] 同对话连续消息通过 stdin 直接注入，不重启进程
- [ ] 跨对话切换时 kill + --resume 重启，会话隔离
- [ ] 结果解析从 stdout 拼接改为 JSON 流 result 事件解析
- [ ] codex / openclaw 等工具不受影响
- [ ] 后端不感知变化（task.dispatch/task.complete 协议不变）
- [ ] daemon 重启后能恢复已有的 session 映射
- [ ] 进程崩溃后自动恢复（下次任务时 --resume）
- [ ] 并发任务串行排队处理

## Definition of Done

* Daemon 可正常执行 claude 任务并通过 stream-json 获取结果
* 同对话连续任务不重启进程
* 跨对话切换正确隔离上下文
* 崩溃恢复、超时处理、并发排队正常工作
* 无 breaking change

## Decision (ADR-lite)

**Context**: Per-task 模式使用 `-p --output-format text`，无流式输出、无结构化解析、无进程复用。需要与 persistent 模式统一并改进。

**Decision**: 方案 2 — per-agent 进程槽位 + 会话隔离。每个 agent 一个进程，同对话复用（stdin 注入），跨对话重启（--resume 恢复）。复用现有 queueTail 排队、idleAgentConfigs 自动重启、EXEC_TIMEOUT 超时。

**Consequences**:
- 同对话内零开销，跨对话切换 ~2-3秒重启
- 每个 conversation 独立 session，上下文隔离
- 为后续流式输出（task.chunk）奠定基础

## Out of Scope

* 后端改动（DaemonHub、Orchestrator）
* 前端改动
* 流式输出推送到前端（task.chunk）
* codex / openclaw 交互方式变更
* system prompt 注入优化（后续加 --append-system-prompt-file）
* 合并现有 agent.start persistent 路径

## Technical Notes

* 关键文件：`src/daemon-npm/bin/agenthub-daemon.js`
* 已有能力（复用）：`runningAgents` Map, `idleAgentConfigs`, `queueTail`, `EXEC_TIMEOUT_MS`
* 需新增：`currentConversationId` 追踪, `sessions.json` 持久化, stream-json 事件解析器（可从 handleAgentStart 提取）
* stream-json 事件类型：system(init/status/compact_boundary), assistant(thinking/text/tool_call), user(tool_result), result(success/error)
* 参考 slock 的 ClaudeDriver（stdin JSON 格式、事件解析）
