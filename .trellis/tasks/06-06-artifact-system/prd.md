# Artifact System 产物预览与编辑

## Goal

把 Agent 回复里的代码 / 网页 / 文档 / 文件等输出，从「纯 Markdown 文本」升级为**结构化产物（Artifact）一等对象**：在聊天流中以内联卡片预览，点击可展开全屏 Preview / Code 工作区，后续支持编辑、Diff、版本历史与对话式局部修改。对标 Claude Artifacts / ChatGPT Canvas / v0，参考 Slock「任务作为一等对象 + Socket 实时同步」的形态。

## What I already know

### 产品需求（用户原话）
- Agent 回复中内联产物预览卡片（网页 iframe、文档渲染、【P2】PPT 浏览）
- 点击卡片展开全屏预览 / 代码编辑器
- 【P2】Diff 视图、版本历史、对话式局部修改（选中代码 → 在聊天中描述修改）

### 竞品调研结论（已验证可引用的部分）
- **Slock** (app.slock.ai)：任务/产物作为一等对象，前端包内有 tasks 路由、LegacyTaskPanel、`task:created/updated/deleted` 实时事件 → 大概率「一等对象 + Socket 实时同步」，不是聊天消息里塞 HTML。真实 /tasks 页面需登录，未能看到完整 UI。
- **Claude Artifacts**：代码/文档/可视化放独立工作区迭代。
- **ChatGPT Canvas**：选中局部内容让模型修改、直接编辑、版本历史、React/HTML 沙箱渲染。
- **v0**：Preview / Code 标签、浏览器内编辑器、Diff 视图、split view、文件树。

### 本地现状（已核实，非拍脑袋）
- ✅ `messages.artifacts_json` 字段存在：[model/message.go:20](../../../src/backend/internal/model/message.go#L20)（`string`，`db:"artifacts_json"`）。
- ⚠️ **该字段已被占用**：当前存的是 agent 元信息 `{agent_id, agent_name, cli_tool}`，由 [service/agent.go:433-434](../../../src/backend/internal/service/agent.go#L433) 写入，前端类型 `MessageArtifacts` [types/message.ts:46-50](../../../src/frontend/src/types/message.ts#L46)。**前端关键路径依赖它**：[MessageBubble.tsx:286-288](../../../src/frontend/src/components/chat/MessageBubble.tsx#L286) 用 `JSON.parse(artifacts_json).agent_name` 显示 Agent 头像与「Agent」徽章 [:423](../../../src/frontend/src/components/chat/MessageBubble.tsx#L423)。→ **A1 数据契约必须先解决字段占用冲突**，否则会打挂 Agent 名字显示。
- ✅ **代码卡片已做一大半**：[MessageBubble.tsx:149-189](../../../src/frontend/src/components/chat/MessageBubble.tsx#L149) 的 `CodeBlock` 已有 highlight.js 语法高亮、复制按钮、长内容折叠。→ 代码卡片**不用重造**，复用即可。
- 真正的空白：结构化产物提取、网页 iframe 预览、文档渲染、全屏工作区、编辑器、Diff/版本。

## Assumptions (temporary)

- 产物来源主要是 Agent（assistant 消息），而非用户消息。
- 产物随消息持久化（刷新后仍在），实时推送走现有 WebSocket（[useWebSocket.ts](../../../src/frontend/src/hooks/useWebSocket.ts)）。
- MVP 优先级：内联卡片 + 全屏只读预览 > 编辑/Diff/版本（后者 P2）。

## Open Questions

- **Q1 (Preference)**：`artifacts_json` 字段占用冲突怎么解决？（见 Research Notes 的 3 个方案）
- **Q2 (Preference)**：产物提取在哪一层做？daemon adapter 解析 vs backend 解析 vs Agent 主动结构化输出。
- **Q3 (Scope)**：MVP 卡片类型边界——code / webpage / document / file 哪些进 MVP，哪些 P2。

## Requirements (MVP = code + webpage)

- [契约] 独立 `artifacts` 表（关联 message_id + version），`artifacts_json` 不动只放 meta（D1）。
- [提取] JS daemon 从 CLI 输出解析 code / webpage 产物，随 assistant 消息上行持久化（D2）。
- [卡片-code] 聊天流内联渲染代码卡，复用现有 CodeBlock（高亮+复制+折叠），升级为结构化产物。
- [卡片-webpage] 内联网页卡，sandbox iframe 预览，含 loading/error + 权限限制。
- [工作区] 点击卡片展开全屏 Preview / Code / Meta 视图（先支持 code/webpage）。

## Acceptance Criteria (MVP)

- [ ] Agent 产出代码 → 内联代码卡（复用现有高亮+复制+折叠），点击可全屏 Code 视图。
- [ ] Agent 产出网页 URL/HTML → iframe 卡片，sandbox 限权，可全屏 Preview。
- [ ] 产物落独立 artifacts 表，刷新后卡片仍在。
- [ ] 群聊多 Agent 产物带来源 Agent 标识（复用 meta.agent_name）。
- [ ] **不回归**：现有 Agent 头像/名字/「Agent」徽章显示正常（artifacts_json 不动，零副作用）。
- [ ] iframe 恶意/跨域内容被 sandbox 限制，CSP 拦截有 error 兜底。

## Definition of Done (team quality bar)

- 后端单测（artifact 解析/持久化）+ 前端组件渲染验证。
- Lint / typecheck / build green。
- 字段迁移有兼容策略（旧数据不丢 agent 元信息）。
- 行为变化更新 doc/task/M9-产物预览.md。

## Out of Scope (explicit / 后置)

- **document 文档卡（markdown/text/pdf/docx 渲染）** — 实现量大，MVP 后第一优先补。
- **file 文件附件卡（下载）** — 需 daemon 传文件内容/路径，MVP 后补。
- 代码编辑器（Monaco/CodeMirror）与下载、选区上下文（P2）。
- Diff 视图、版本历史、回滚（P2，artifacts 表已预留 version 利于此）。
- 对话式局部修改（选中代码 → patch）（P2）。
- PPT/PPTX 浏览（P2）。

## Research References

- 见本会话已完成的竞品调研（Slock / Claude Artifacts / ChatGPT Canvas / v0）。
- 待补：daemon adapter 输出格式（产物提取 hook 点）。

## Research Notes

### Q1 — artifacts_json 字段占用，3 个方案

**方案 A：包一层（Recommended）**
- `artifacts_json` 改为 `{ meta: {agent_id, agent_name, cli_tool}, artifacts: [...] }`。
- Pros：单字段、单次迁移、前端读 `.meta.agent_name`、产物读 `.artifacts`。
- Cons：需同时改后端写入点 + 前端读取点 + 兼容旧数据（旧数据是裸 meta 对象）。

**方案 B：产物单开 DB 列 / 表**
- 新增 `messages.artifacts`（或独立 `artifacts` 表关联 message_id），`artifacts_json` 保持只放 meta。
- Pros：彻底解耦，产物可独立查询/版本化（利于 P2 版本历史）。
- Cons：迁移 + repository/handler 改动更大。

**方案 C：复用同一数组、用 type 区分**
- Pros：改动小。Cons：语义混乱，meta 不是产物，不推荐。

### Q2 — 产物提取层

- **daemon adapter 层**（[src/daemon/adapter](../../../src/daemon/adapter)）：贴近 CLI 原始输出，能拿到文件创建等信号。
- **backend 层**：集中、易测，但只能从 Markdown 文本正则提取。
- **Agent 结构化输出**：最准但依赖 Agent 配合（prompt/协议）。
- 倾向：MVP 用 backend/adapter 正则提取（代码块/URL/"Created file:"），P2 再上结构化协议。

### Feasible approaches（待用户定）
见 Q2。

## Decision (ADR-lite)

**[D1] 产物存储 — 方案 B：产物单开列/表（已定）**
- Context：`artifacts_json` 已被 agent 元信息占用，前端关键路径依赖 `.agent_name`。
- Decision：产物**不**塞回 `artifacts_json`。`artifacts_json` 保持只放 meta（零改动、零回归风险）；产物另存。
- 子决策待定：新增 `messages.artifacts` JSON 列 vs 独立 `artifacts` 表（关联 message_id + version）。**推荐独立表**——为 P2 版本历史/独立查询铺路，A8 几乎零额外成本。
- Consequences：迁移 + repository/handler/service 改动比方案 A 大，但语义干净、可扩展、对现有 Agent 名字显示零回归。

**[D2] 提取层 — daemon 层（已定，含关键架构修正）**
- Context：产物准确性 vs 改动复杂度。
- Decision：在 daemon 层提取产物（贴近 CLI 原始输出，能拿文件创建/工具调用等结构化信号），而非 backend 正则。
- ⚠️ **架构修正（已核实）**：活路径是 **JS daemon `src/daemon-npm/bin/agenthub-daemon.js`**，它直接 spawn Claude Code/Codex CLI（[:152-195](../../../src/daemon-npm/bin/agenthub-daemon.js#L152)）。Go 的 `src/daemon/adapter/adapter.go` 里那个 `Artifact` struct + `StreamChunk.Artifact` 字段是**死代码/遗留脚手架，全库无一处填充**——不要往那儿写，否则白做。
- 真正 hook 点：JS daemon 拿到 CLI 结果文本处（[:818-826](../../../src/daemon-npm/bin/agenthub-daemon.js#L818) `runProcess` 后、`parseOpenClawOutput`），在回传 assistant 消息给 backend 前解析产物。
- 提升准确度的路子：Claude Code 现在用 `--output-last-message`（只拿最终文本）。若改/并用 `--output-format stream-json`（[:726](../../../src/daemon-npm/bin/agenthub-daemon.js#L726) 已有 `--output-format` 用法），可拿到 `tool_use`（Write/Edit 文件创建）等结构化事件 → 文件类产物提取更准。
- Consequences：提取放 JS daemon；需打通 daemon→backend 产物字段（随 assistant 消息上行，落到 D1 的 artifacts 表）。Go adapter 不动。

## Refined Task List (A0–A10, 修正版)

图例：✅ 完成 · 🔨 进行中 · ⬜ 待开始 · 🅿️ 后置

| ID | 状态 | 优先级 | 模块 | 交付物 |
|----|------|--------|------|--------|
| A0 | ✅ | - | 竞品/现状调研 | Slock 可验证线索 + Claude/Canvas/v0 模式 + 本地现状核实（含 artifacts_json 占用、CodeBlock 已有、JS daemon 为活路径） |
| A1 | ✅ | **P0** | 产物契约 + 独立表 | 新建 `artifacts` 表（message_id+version+type+content...）；Artifact schema；artifacts_json 不动（D1） |
| A2 | ✅ | P1 | daemon 产物解析 | JS daemon 从 CLI 输出解析 code/webpage 产物，随 assistant 消息上行（D2，活路径在 src/daemon-npm） |
| A3 | ✅ | P1 | 聊天内联卡片 | ArtifactCard：CodeBlock 抽共享组件复用（code）+ webpage 卡 |
| A4 | ✅ | P1 | 全屏预览工作区 | ArtifactWorkspace：Preview / Code / Meta 三视图 |
| A5 | ✅ | P1 | 网页 iframe 沙箱 | WebpageFrame：sandbox=allow-scripts + loading/error + 8s 超时 + CSP 兜底 |
| A6 | 🅿️ | 后置 | 文档渲染 | markdown/text/pdf/docx 基础预览（MVP 后第一优先） |
| A7 | 🅿️ | 后置 | 文件附件卡 | 文件名+图标+下载（需 daemon 传文件内容） |
| A8 | 🅿️ | P2 | 代码编辑器 | Monaco/CodeMirror 编辑、下载、选区上下文 |
| A9 | 🅿️ | P2 | Diff 与版本历史 | 版本列表、diff view、回滚（artifacts 表已预留 version） |
| A10 | 🅿️ | P2 | 对话式局部修改 | 选中代码 → selection context → patch |
| A11 | 🅿️ | P2 | PPT 浏览 | PPT/PPTX 转预览页/图片序列 |

> 进度跟踪：本表 = 唯一真源。每完成一项更新「状态」列；实现期同步用会话内 todo。

### MVP 验证记录（2026-06-06，主会话独立验证）

- ✅ 后端 `go build ./...` + `go test`（service/repository/handler 全 ok，含新增 artifact 持久化测试）
- ✅ 前端 `tsc -b && vite build`（3454 modules，类型零报错）
- ✅ daemon `parseArtifacts` 真实样本 7/7：code+多URL、文件名提示剥离、HTML→webpage、代码块内 URL 不误判、空/纯文本/CJK 句末 URL
- ✅ 迁移 024 跑真实 PG：11 列建表 + FK→messages 确认
- ✅ 真实数据往返：插入 code+webpage → 按 sort_order 读回 → 删消息级联清产物（0 孤儿）→ 清理
- ✅ 跨层字段契约 5 层贯通（daemon→WS handler→service→广播→前端 msg.data.artifacts）；artifacts_json 零回归
- ⏳ **唯一待人工**：真·UI 点击流（前端+后端+daemon+已连电脑跑真实 Claude Code → 发消息 → 看卡片渲染），需活体 agent 硬件/凭据，无法 headless 自动驱动。

## Technical Notes

- 影响文件（初判）：
  - 后端：[model/message.go](../../../src/backend/internal/model/message.go)、[service/message.go](../../../src/backend/internal/service/message.go)、[repository/message.go](../../../src/backend/internal/repository/message.go)、[handler/message.go](../../../src/backend/internal/handler/message.go)、migrations。
  - 前端：[MessageBubble.tsx](../../../src/frontend/src/components/chat/MessageBubble.tsx)、[types/message.ts](../../../src/frontend/src/types/message.ts)、[messageStore.ts](../../../src/frontend/src/store/messageStore.ts)、新增 Artifact 卡片/工作区组件。
  - daemon：[src/daemon/adapter](../../../src/daemon/adapter)（如在 adapter 层提取）。

### Artifact 字段契约（三层对齐真源）

A1+A2 已落地。daemon(JS) 产出的 artifact JSON、backend Go model、以及（下一批次的）前端 TS 类型必须严格用以下字段名（snake_case 的 json tag）：

| 字段 | 类型 | 说明 | code | webpage |
|------|------|------|------|---------|
| `type` | string | `"code"` \| `"webpage"` | 必填 | 必填 |
| `language` | string | 代码语言（go/ts/python…），小写 | 可选 | — |
| `filename` | string | 文件名（从 `// file: xxx` 首行提示提取） | 可选 | — |
| `title` | string | 标题（HTML 文档用文件名/默认值） | — | 可选 |
| `url` | string | 网页链接（裸 http/https URL） | — | 二选一 |
| `content` | string | 源码 / 完整 HTML 文档 | 必填 | 二选一 |
| `version` | int | 版本，默认 1（仅后端持久化，daemon 不传） | 后端补 | 后端补 |

落地位置：
- **daemon 解析**：`src/daemon-npm/bin/agenthub-daemon.js` 的 `parseArtifacts(text)`，在 `task.complete` 上行 payload 加 `artifacts` 数组（围栏代码块→code/webpage、裸 URL→webpage）。
- **WS 契约**：`pkg/ws/daemon_hub.go` 的 `TaskResult.Artifacts []ArtifactResult`；handler `handleTaskComplete` 接收并透传。
- **持久化**：独立 `artifacts` 表（migration `024_create_artifacts.sql`），`model.Artifact` + `repository/ArtifactRepo`。assistant 消息落库后由 `MessageService.createAgentReply` / `OrchestratorService.persistArtifacts` 调用 `msgRepo.SaveArtifacts` 写入。
- **查询回传**：`MessageRepo.fillArtifacts` 按 message_id 批量加载，挂到 `Message.Artifacts`（`db:"-"`），随消息 API/WS 自动返回。`artifacts_json` 未改动，Agent 名字显示零回归。
