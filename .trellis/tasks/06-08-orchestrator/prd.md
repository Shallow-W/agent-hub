# 优化 orchestrator 上下文结构

## Goal

优化群聊 orchestrator 的 prompt 构造，让上下文按固定大括号区块组织，并把可用 Agent 详情从后端真实数据注入到 prompt 中，同时改进 orchestrator 的分派指令，减少模型执行任务本身或编造 Agent 能力的概率。

## What I already know

* 用户希望 prompt 使用 `{区块标题 ...}` 的结构包裹各部分。
* 当前群聊信息需要包含群聊名称、可用 Agent 列表，以及由后端查询得到的 Agent 简介、标签等详情。
* 群聊最近动态、用户消息、orchestrator 指令需要作为独立区块。
* orchestrator 指令需要比当前版本更清晰，重点约束“分析、拆解、分派”，不要替代被分派 Agent 执行任务。

## Assumptions

* 后端已有可用 Agent 的数据结构或查询结果；本任务优先复用现有字段。
* 如果某些 Agent 缺少简介或标签，prompt 构造只做空值兜底，不自行生成虚假描述。
* 输出格式沿用当前系统已有的 @Agent 分派协议，除非代码中已有结构化协议。

## Requirements

* 将 orchestrator prompt 分成固定区块：当前群聊、可用 Agent 详情、群聊最近动态、用户消息、Orchestrator 指令。
* 可用 Agent 详情必须来源于后端现有 Agent 数据。
* 优化 Orchestrator 指令，使其明确角色、决策规则、分派边界、输出格式。
* 保持现有调用链兼容，不引入新的外部依赖。

## Acceptance Criteria

* [x] 生成的 orchestrator prompt 包含清晰的大括号区块边界。
* [x] 可用 Agent 详情使用真实 Agent 字段，未配置字段显示为空/未配置，而不是编造。
* [x] 用户要求多 Agent 协作时，指令鼓励按 @Agent 分派任务。
* [x] 相关测试或现有检查通过。

## Out of Scope

* 不新增 Agent 管理后台字段。
* 不改变实际 Agent 执行协议，除非现有代码必须同步适配。
* 不实现完整结构化 JSON 分派协议。

## Technical Notes

* `src/backend/internal/service/orchestrator_prompt.go` 构造 orchestrator prompt。
* `src/backend/internal/service/orchestrator.go` 从 `ConversationAgent` 映射 prompt 详情，并截断长字段。
* `src/backend/internal/repository/conversation.go` 的 `ListAgents` 查询现在返回 `agents.tags`。
* 验证命令：`go test ./internal/service ./internal/repository`。
