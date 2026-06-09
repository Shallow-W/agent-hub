# implement pdf gap skill tool assignment

## Goal

对照 `doc/AgentHub-_多Agent协作平台设计.pdf` 中核心功能 1-4，在暂不考虑桌面客户端/移动端的前提下，补齐 Web UI 版自建 Agent 的 skill 分配与工具集分配闭环，并做端到端验证。

## PDF Scope Notes

PDF 的核心 1-4 为：

1. IM 聊天式交互：对话列表、单聊/群聊、消息类型、消息操作、上下文管理。
2. Orchestrator：群聊模式下拆解、分派、聚合，支持并行与失败降级。
3. 多 Agent 接入：统一适配器、至少两个 Agent 平台、用户自建 Agent（System Prompt + 工具集）、联系人显示头像/名称/能力标签。
4. 产物预览与编辑：内联卡片、全屏预览/代码编辑器，P2 Diff/版本历史等。

本任务聚焦用户点名的第 3 点缺口：skill 分配设计、工具集分配设计和测试。

## What I already know

* `agents.tools_config` 已存在，并且 Go/Node daemon MCP 都有 per-agent allowed tools 过滤逻辑。
* `agents.custom_skills` 已存在，前端 AgentProfile 已可编辑平台 Skills。
* 当前 Agent 创建弹窗只支持底座、名称、System Prompt，不支持创建时分配工具集和 Skills。
* 前端工具目录、后端 normalize 目录、Go daemon MCP、Node daemon MCP 存在重复硬编码且不完全一致。
* Go daemon MCP 缺少 `create_group` 工具实现，而 Node daemon 已有。

## Requirements

* 创建 Agent 时可选择工具集模板、具体工具，并填写初始平台 Skills。
* 创建 Agent 成功后保存 `tools_config` 和 `custom_skills`，后续 Agent 详情页可继续编辑。
* 前端工具目录包含后端/daemon 已支持工具，不漏掉 `create_group`、`delete_task`、`list_conversation_agents`。
* 后端 `AddCandidateAgent` 接口支持接收并规范化 `tools_config`，保存 `custom_skills`。
* Go daemon MCP 支持 `create_group`，工具模板与后端/前端目录对齐。
* 补测试覆盖工具配置规范化、候选 Agent 创建保存工具配置、MCP 工具授权/空授权。
* 做 UI 版端到端验证：登录 `wjc / 123456`，创建/编辑 Agent 工具集与 Skills，确认 UI 和 API 数据一致。

## Acceptance Criteria

* [x] Agent 创建弹窗包含 System Prompt + 工具集 + 初始 Skills。
* [x] 新建候选 Agent 后，`tools_config` 和 `custom_skills` 持久化并在详情页显示。
* [x] 工具集 UI 可选工具与 runtime 允许工具目录一致。
* [x] per-agent MCP `tools/list` 只返回允许工具，未授权工具调用被拒绝。
* [x] 后端测试、daemon 测试、前端构建通过。
* [x] 端到端 UI/API 验证通过。

## Verification

* `bash scripts/test.sh`
* `go test ./... -count=1` in `src/daemon`
* `bash scripts/build.sh`
* WebUI E2E smoke on `http://localhost:5173` with `wjc / 123456`: create Agent with System Prompt + toolset + platform Skills, verify API persistence, edit toolset to `none`, verify API returns `{"toolset":"none","allowed_tools":[]}`, then delete the temporary test Agent.

## Out of Scope

* 不实现桌面端和移动端。
* 不实现完整“对话式创建向导”的多轮 Agent 对话流程。
* 不新增 Agent description 数据库字段。
* 不重做产物预览/部署模块。
