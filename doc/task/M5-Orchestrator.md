# M5 Orchestrator 主 Agent 协调器

## 目标

实现群聊模式下的 Orchestrator，能够理解用户意图、拆解任务、并行调度多个 Agent、聚合结果。

## 子任务

### M5-1 Orchestrator System Prompt 设计

- 设计专用的 Orchestration System Prompt
- 引导 LLM 输出结构化的任务拆解方案
- 输出格式示例：

```json
{
  "analysis": "用户需要...",
  "subtasks": [
    {"id": 1, "description": "...", "agent": "claude-code", "reason": "..."},
    {"id": 2, "description": "...", "agent": "codex", "reason": "..."}
  ]
}
```

### M5-2 意图分析 + 任务拆解

- 接收用户群聊消息
- 通过 Claude Code CLI（带 Orchestration Prompt）分析意图
- 解析 LLM 输出的任务拆解方案
- 校验拆解结果（agent 是否可用、子任务是否明确）

### M5-3 并行调度

- 根据拆解方案，将子任务同时分派给对应 Agent
- 每个子任务通过守护进程启动对应 CLI 进程
- 实时收集各 Agent 的流式输出
- Agent 状态在聊天流中展示（"Agent A 正在处理..."）

### M5-4 结果聚合

- 等待所有子任务完成（或超时）
- 合并各 Agent 产出一个连贯的回复
- 在聊天流中依次展示各 Agent 的贡献
- 失败降级：某个 Agent 失败时，返回已完成部分 + 失败说明

## 验收标准

- [ ] Orchestrator 能正确分析 "帮我用Claude Code写前端，用Codex写后端" 类型的指令
- [ ] 拆解后的子任务被分派给正确的 Agent
- [ ] 多个 Agent 并行执行，结果合并展示
- [ ] 单个 Agent 失败不影响整体流程

## 依赖

- M4-3（适配器接口）
- M4-4（Claude Code 适配器，Orchestrator 复用它做意图分析）
- M2-1（WebSocket 推送流式结果）

## 技术要点

- Orchestrator 本身是一个特殊的 Agent（role=orchestrator）
- 它复用 Claude Code CLI + 自定义 System Prompt 来做意图理解
- 调度逻辑在 Go 后端实现，不在 CLI 内
