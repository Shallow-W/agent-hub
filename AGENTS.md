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
| 用 Codegraph 查代码关系 | `doc/reference/codegraph.md` |

- **P0**：IM 聊天核心体验、单聊/群聊、多 Agent 接入（≥2 个）、Orchestrator
- **P1**：产物预览卡片、上下文管理（pin 消息）、多会话并行
- **P2**：部署发布、Diff/版本历史、PPT 浏览、多端支持

## 硬性规则

> 编码规范已迁移至 `.trellis/spec/`，由 Trellis SessionStart hook 自动注入。包括：
> - 通用规则 → `.trellis/spec/guides/general-conventions.md`
> - 前端规则 → `.trellis/spec/frontend/quality-guidelines.md`
> - 后端规则 → `.trellis/spec/backend/quality-guidelines.md`
> - 工作流规则 → `.trellis/spec/guides/workflow-rules.md`
> - 核心原则 → `.trellis/spec/guides/core-principles.md`
