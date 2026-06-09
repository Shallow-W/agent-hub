# 实现 Agent 工具集权限

## Goal

实现可给不同 Agent 分配不同工具集的端到端能力。创建或更新 Agent 时可以配置工具集；Agent 运行时只能看到并调用自己被授权的 MCP 工具；后端需要在工具调用入口做权限校验，不能只依赖 prompt 或 MCP tools/list 隐藏工具。

## What I already know

* 现有 `agents` 表已有 `tools_config` 字段，但 MCP 工具目前主要是 daemon 侧全量静态注册。
* 用户希望重点先实现“工具集”部分，并且不同创建出来的 Agent 能分配不同工具使用。
* 后续对话式创建 Agent 会复用工具集能力。
* 需要端到端验证：配置 Agent 工具集 -> daemon 运行任务 -> MCP 工具列表/调用受限制。

## Assumptions

* MVP 可以先用 `agents.tools_config` 存储 Agent 的工具授权配置，避免新增复杂 UI 和多表模型。
* 工具目录第一版可以用代码内静态目录，不必先做 DB 表。
* 前端如果已有 Agent 创建/编辑表单，应提供工具集配置入口；若 UI 成本过大，至少保证 API 和 daemon E2E 可跑通。
* 运行时权限校验需要覆盖 tools/list 和 tools/call 两层。

## Requirements

* 定义平台 MCP 工具目录和预设工具集模板。
* Agent 创建/更新支持保存工具集配置。
* daemon MCP tools/list 根据当前 `agent_id` 的工具授权过滤。
* daemon MCP tools/call 调用前按 `agent_id + tool_name` 做权限校验。
* 默认行为兼容已有 Agent：未配置工具集时使用保守默认值，避免突然暴露高危工具。
* 提供测试覆盖，包括后端单测和至少一个端到端/集成验证路径。

## Acceptance Criteria

* [x] 不同 Agent 配置不同工具集后，MCP tools/list 返回不同工具列表。
* [x] Agent 调用未授权工具会被拒绝。
* [x] Agent 调用授权工具仍可正常工作。
* [x] 创建/更新 Agent 的配置能持久化工具集。
* [x] 端到端测试或脚本能验证上述流程。

## Out of Scope

* 不实现完整的图形化细粒度权限矩阵（若现有 UI 足够轻量可做基础入口）。
* 不实现审计日志和多租户高级策略。
* 不实现对话式创建向导状态机；本任务只打通工具集底座。

## Technical Notes

* 使用现有 `agents.tools_config` 字段存储 MVP 工具集配置，格式为 `{"toolset": string, "allowed_tools": string[]}`。
* npm daemon 通过 `--agent-id` 将当前 Agent 身份传入 MCP 子进程，并在 `tools/list` 与 `tools/call` 过滤授权工具。
* Go daemon 支持 `AGENTHUB_AGENT_ID`，同样过滤 `tools/list` 与 `tools/call`。
* 前端 Agent Profile 的 Tools Config tab 已改为模板 + 工具勾选，保存 JSON 配置。
* 验证命令：
  * `go test ./internal/service ./internal/handler ./internal/repository`
  * `go test ./...` in `src/daemon`
  * `npm run build` in `src/frontend`
  * `node --check src/daemon-npm/bin/agenthub-daemon.js`
  * npm daemon MCP mock E2E script
  * Playwright page smoke check for `http://127.0.0.1:5173/`
