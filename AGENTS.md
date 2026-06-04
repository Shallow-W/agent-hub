<!-- TRELLIS:START -->
# Trellis Instructions

These instructions are for AI assistants working in this project.

This project is managed by Trellis. The working knowledge you need lives under `.trellis/`:

- `.trellis/workflow.md` — development phases, when to create tasks, skill routing
- `.trellis/spec/` — package- and layer-scoped coding guidelines (read before writing code in a given layer)
- `.trellis/workspace/` — per-developer journals and session traces
- `.trellis/tasks/` — active and archived tasks (PRDs, research, jsonl context)

If a Trellis command is available on your platform (e.g. `/trellis:finish-work`, `/trellis:continue`), prefer it over manual steps. Not every platform exposes every command.

If you're using Codex or another agent-capable tool, additional project-scoped helpers may live in:
- `.agents/skills/` — reusable Trellis skills
- `.codex/agents/` — optional custom subagents

Managed by Trellis. Edits outside this block are preserved; edits inside may be overwritten by a future `trellis update`.

<!-- TRELLIS:END -->

## 项目简介
AgentHub 是一个以 IM 聊天为核心交互范式的多 Agent 协作平台，用户通过类似飞书/微信的对话界面与多个 AI Agent（Claude Code、Codex、OpenCode 等）交互，支持单聊、群聊、任务分派和产物预览。

## 核心概念

- **IM聊天范式**：对话列表 + 单聊/群聊 + 富媒体消息，是整个平台的交互核心
- **Orchestrator（协调器）**：群聊模式下理解用户意图，将任务拆解并分派给子Agent，聚合结果
- **Agent适配器层**：统一抽象不同Agent平台的API差异（Claude Code、Codex、OpenCode等）
- **自建Agent**：用户通过对话式创建，设定System Prompt + 工具集
- **产物系统**：Agent回复中内联预览卡片（网页iframe、代码Diff、文件附件等）

## 交付要求

- 30%权重在AI协作能力（需沉淀Spec、Skill、Rules协作规范）
- 需产出：产品设计文档 + 技术文档 + 可运行Demo + AI协作开发记录 + 3分钟Demo视频

## 技术栈
| 层 | 技术 |
|----|------|
| 前端 | React + TypeScript + Vite + Zustand + React Router v6 + CSS Modules |
| 后端 | Go + Gin + pgx/sqlx + go-redis + nhooyr/websocket + koanf + slog |
| 数据库 | PostgreSQL |
| 守护进程 | Go（本地 Agent 扫描、进程管理、适配器层） |

## 快速导航
| 你想做什么 | 去哪里看 |
|-----------|---------|
| 了解产品需求 | `doc/需求文档.md`，原始PDF：`doc/AgentHub-_多Agent协作平台设计.pdf` |
| 了解系统架构 | `doc/architecture/overview.md` |
| 了解项目目录结构 | `doc/conventions/project-structure.md` |
| 了解前端编码规范 | `doc/conventions/frontend-conventions.md` |
| 了解后端编码规范 | `doc/conventions/backend-conventions.md` |
| 了解 Git 分支和提交规范 | `doc/conventions/git-conventions.md` |
| 了解文档编写规范 | `doc/conventions/doc-conventions.md` |
| 了解开发流程经验教训 | `doc/conventions/process-lessons.md` |
| 了解 API 接口设计 | `doc/reference/api.md` |
| 了解模块任务详情 | `doc/task/M0-基础设施.md` ~ `doc/task/M10-Pin上下文.md` |
| 了解当前任务进度 | `doc/TASKLIST.md` |

## 优先级
- **P0**：IM 聊天核心体验、单聊/群聊、多 Agent 接入（≥2 个）、Orchestrator
- **P1**：产物预览卡片、上下文管理（pin 消息）、多会话并行
- **P2**：部署发布、Diff/版本历史、PPT 浏览、多端支持

## 硬性规则（必须遵守）

### 通用
1. 注释语言：中文（说明"为什么"），命名语言：英文
2. 换行：LF，编码：UTF-8
3. 单文件不超过 300 行（前端组件 / Go 文件），超过则拆分
4. 禁止提交敏感信息（API Key、密码、`.env`）
5. 新增代码必须有对应测试

### 前端
6. 禁止使用 `any`，用 `unknown` 或具体类型替代
7. 所有 REST 请求通过 `api/` 模块发出，组件内禁止直接调用 `fetch`/`axios`
8. WebSocket 通过自定义 Hook 消费，不直接操作 WebSocket 实例
9. 样式使用 CSS Modules，类名 camelCase，禁止内联样式

### 后端
10. 所有跨函数调用传递 `context.Context` 作为第一个参数
11. 错误使用 `%w` 包装以保留堆栈，handler 层统一处理错误响应
12. 禁止使用 `init()` 函数或包级全局变量管理依赖
13. 依赖注入在 `cmd/server/main.go` 中统一组装
14. 接口在消费方定义，保持小（1-3 个方法）

### 工作流
15. **无明确指令时**，按 `doc/TASKLIST.md` 顺序解决未完成任务（按依赖关系，优先最短跑通路径）
16. **提出新任务时**，在 `doc/TASKLIST.md` 添加索引行，在 `doc/task/` 下创建详情文件，遵循 `doc/conventions/` 中的文件命名和格式规范
17. **完成任务时**，将 TASKLIST.md 中对应状态改为 `[x]`
18. **每次任务完成后，自行判断是否需要提交代码。有实质性改动则直接 commit**，严格遵守 `doc/conventions/git-conventions.md` 中的 commit 格式：`type(scope): 中文描述`。不要主动询问用户是否提交
19. **提交后自动 Review + 测试**：commit 后启动 3 个并行 sub-agent：（a）代码质量/逻辑缺陷审查——侧重代码可读性、潜在 bug、边界处理、SOLID 原则；（b）功能验证——扮演产品经理角色，验证功能是否完整实现、交互流程是否合理、用户体验是否达标、是否有遗漏场景；（c）端到端测试——根据变更内容编写并执行实际测试（API 调用、UI 操作），验证核心流程可跑通，发现的 bug 记录到 `doc/TASKLIST.md`。最少迭代 3 轮，最多 8 轮，收敛即提前停止。review/测试修复产生的 commit 不再触发本条规则（避免反馈循环）。**测试环节必须覆盖正常路径和边界情况，不得仅依赖编译通过作为验证手段**
20. **Review 通过后更新文档**：检查本次任务是否涉及需要文档化的内容（新增结构体/接口、API 变更、架构调整等），如有则同步更新 `doc/` 下的设计文档、API 文档或数据模型文档。文档更新产生的 commit 不再触发步骤 19
