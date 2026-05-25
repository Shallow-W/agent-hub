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
| 了解产品需求 | `doc/需求文档.md` |
| 了解系统架构和目录结构 | `doc/harness/project-structure.md` |
| 了解前端编码规范 | `doc/harness/frontend-conventions.md` |
| 了解后端编码规范 | `doc/harness/backend-conventions.md` |
| 了解 Git 分支和提交规范 | `doc/harness/git-conventions.md` |
| 了解文档编写规范 | `doc/harness/doc-conventions.md` |
| 了解当前任务进度 | `doc/TASKLIST.md` |
| 了解 API 设计 | `doc/design/api-*.md` |
| 了解数据模型 | `doc/design/data-model.md` |

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
15. 有实质性改动则直接 commit，格式：`type(scope): 中文描述`
16. Commit 后自动启动 2 个并行 sub-agent review，最少 3 轮最多 8 轮
17. Review 修复 commit 不再触发 review（避免反馈循环）
18. 文档更新 commit 不触发 review
