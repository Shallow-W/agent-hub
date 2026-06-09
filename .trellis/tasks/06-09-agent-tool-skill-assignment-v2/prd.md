# 完善 Agent 工具集与 Skill 分配设计

## Goal

让用户能够针对不同自建 Agent 独立分配平台工具和平台 Skills。工具继续通过 AgentHub MCP 暴露，但必须按 Agent 授权控制 `tools/list` 和 `tools/call`；Skills 先做平台级分配和运行时渐进式提示词加载，后续再扩展为同步到对应电脑的原生 Skill 文件。

## What I Already Know

- 用户希望不同 Agent 拥有不同工具调用权限，而不是所有 Agent 共用同一组 MCP 工具。
- 用户希望不同 Agent 可以分配不同 Skills，并考虑两种实现路径：
  - 下载/安装到对应电脑，让底层 CLI 原生发现。
  - 平台在提示词中做渐进式加载适配。
- 现有工具集基础已经存在：
  - `agents.tools_config` 存储 `{"toolset": string, "allowed_tools": string[]}`。
  - daemon MCP runtime 已按 `agent_id` 过滤 `tools/list` 和 `tools/call`。
  - 前端创建/详情页已有工具模板和勾选 UI。
- 现有 Skill 基础已经存在：
  - daemon 会扫描本机真实 `SKILL.md`，写入 `capabilities_json`。
  - 用户可在 Agent 详情页编辑 `custom_skills`。
  - 本任务将 `custom_skills` 扩展为保留 `name`、`description`、`trigger`、`detail`，用于运行时渐进式加载。

## Recommended Design

### Toolsets: MCP Allowlist As The Source Of Truth

继续使用 MCP 作为工具执行面。平台只做三件事：

- 配置面：每个 Agent 独立保存 `tools_config`。
- 授权面：daemon MCP server 以 `agent_id` 查询该 Agent 配置，并过滤 `tools/list`。
- 执行面：`tools/call` 再次检查工具是否授权，未授权直接拒绝。

设计原则：

- 空配置、未知 Agent、无法解析旧配置均 fail-closed，不授予工具。
- 模板只是 UI 辅助，真正授权以 `allowed_tools` 为准。
- 工具 catalog 需要前端、后端、Go daemon、Node daemon 保持一致，后续应收敛到一个可生成或可校验的共享清单。

### Skills: Start With Platform Progressive Loading

MVP 不直接把用户配置的 Skills 下载到对应电脑。原因：

- 不同 CLI 的 Skill 目录、插件机制、热加载策略不同，直接写文件容易产生跨平台和权限问题。
- 平台已经掌握对话上下文、Agent 配置和群聊黑板，更适合先做运行时 prompt 适配。
- `custom_skills` 作为用户分配结果更容易在 WebUI 里编辑、审计、回滚。

MVP 的 Skill 运行时模型：

- `capabilities_json`：本机扫描到的底座原生 Skills，只读展示，作为候选能力来源。
- `custom_skills`：用户分配给该 Agent 的平台 Skills，可编辑。
- 调度 Agent 时，不把所有 Skill 详情一股脑塞进 prompt。
- Prompt 里注入一个短 Skill Index：
  - Skill 名称
  - 简短描述
  - 何时使用
- 当任务明显命中某个 Skill 时，调度层再注入该 Skill 的 detail，形成“渐进式加载”。

### Future: Optional Native Skill Sync

后续可以增加“同步到电脑”模式，但应作为显式开关：

- 适合场景：Skill 需要本地文件、脚本、模板、CLI 原生加载能力。
- 同步对象：只同步用户明确选择的 Skills。
- 同步位置：每台电脑/每个 CLI 使用独立受管目录，例如 AgentHub managed skills root。
- 安全边界：同步前展示 diff 或来源；支持撤销；避免覆盖用户手写原生 Skill。

## Requirements

- 用户能在 Agent 创建和详情页为单个 Agent 分配工具集和具体工具。
- 用户能在 Agent 创建和详情页为单个 Agent 分配平台 Skills。
- 工具权限在 MCP runtime 强制生效，而不是仅靠提示词约束。
- Skill 分配结果在 Agent 运行 prompt 中可见，但默认只注入摘要索引。
- Skill detail 只在需要时注入，避免上下文膨胀。
- 原生 Skill 同步先不作为本阶段默认路径。

## Acceptance Criteria

- [x] WebUI 中每个自建 Agent 的工具集配置能独立保存和回显。
- [x] WebUI 中每个自建 Agent 的平台 Skills 能独立保存和回显。
- [x] MCP `tools/list` 只返回该 Agent 被授权的工具。
- [x] MCP `tools/call` 调用未授权工具时返回明确拒绝。
- [x] Agent 运行时 prompt 包含该 Agent 的平台 Skill Index。
- [x] 至少一个 E2E 验证：Agent A 有工具 X/Skill A，Agent B 没有工具 X/Skill A，二者运行时看到的工具/Skill 上下文不同。
- [x] 使用账号 `wjc` / `123456` 通过浏览器完成端到端测试：创建或编辑测试 Agent、分配工具集和平台 Skills、刷新后回显、API 数据一致、运行时权限/上下文一致。

## Out Of Scope

- 暂不实现跨电脑自动安装/同步原生 `SKILL.md` 文件。
- 暂不实现 Skill marketplace。
- 暂不实现复杂的 Skill 版本依赖、冲突解决和文件资产同步。

## Technical Notes

- 相关前端文件：
  - `src/frontend/src/components/agent/toolAssignments.ts`
  - `src/frontend/src/components/agent/AgentCreateModal.tsx`
  - `src/frontend/src/components/agent/AgentProfile.tsx`
  - `src/frontend/src/components/agent/AgentSkillsPanel.tsx`
- 相关后端文件：
  - `src/backend/internal/service/tool_config.go`
  - `src/backend/internal/service/agent_skill.go`
  - `src/backend/internal/service/agent.go`
  - `src/backend/internal/repository/agent.go`
  - `src/backend/internal/repository/conversation.go`
- 相关 daemon 文件：
  - `src/daemon/mcp/tool_permissions.go`
  - `src/daemon/mcp/handlers.go`
  - `src/daemon-npm/bin/agenthub-daemon.js`
- 本阶段复用 `custom_skills` 字段承载结构化平台 Skill；后续如需版本、市场、资产文件，再拆成独立表。

## Verification

- `bash scripts/test.sh`
- `cd src/daemon && go test ./... -count=1`
- `node --check src/daemon-npm/bin/agenthub-daemon.js`
- `bash scripts/build.sh`
- Playwright E2E：登录 `wjc` / `123456`，创建临时 `E2E工具Skill-*` Agent，验证工具集、结构化平台 Skills、刷新回显与 API 数据一致，最后删除临时 Agent。

## E2E Test Plan

- 用 Playwright 登录 `http://localhost:5173`。
- 进入智能体页面，选择一个测试自建 Agent 或创建临时 Agent。
- 设置工具集为自定义，仅勾选一组可区分工具。
- 设置平台 Skills，包含 `name`、`description`、`trigger`、`detail`。
- 保存后刷新页面，确认 UI 回显一致。
- 调用 API 确认 `tools_config`、`custom_skills` 持久化一致。
- 通过 daemon MCP 或可测试 helper 确认授权工具列表只包含该 Agent 的允许工具。
- 创建/编辑另一个 Agent 为无工具/无该 Skill，确认两者配置隔离。
- 清理临时测试 Agent，避免污染用户数据。
