# AgentHub — 多 Agent 协作平台

AgentHub 是一个以**电脑**为管理单元的多 Agent 协作平台。用户在每台电脑上运行 Daemon 守护进程，平台通过统一的适配层自动发现并对接该机器上安装的 Agent CLI（Claude Code、Codex、OpenCode、OpenClaw 等），将其注册为平台内的可用 Agent。

用户可以为每个 Agent 独立配置 System Prompt、工具集和 Skills，按需组合成不同的能力组合。平台以 IM 聊天为核心交互方式，支持单聊（1v1 对话）、群聊（多 Agent 由 Orchestrator 自动协调分工）、实时流式输出，以及代码 Diff、网页预览等富媒体产物卡片。同时内置 MCP Server，让 Agent 能够反向操作平台对象（创建 Agent、管理群聊等），实现 Agent 自治。

## 核心特点

- **以电脑为管理单元** — 每台电脑运行一个 Daemon，自动扫描并注册本机所有 Agent CLI，无需手动配置
- **统一适配层** — 一套接口对接 Claude Code、Codex、OpenCode、OpenClaw 等不同 CLI，屏蔽调用差异
- **灵活的 Agent 配置** — 为每个 Agent 独立分配 System Prompt、工具集（29 种内置工具）、平台 Skills，支持模板保存与复用
- **Orchestrator 群聊编排** — 群聊模式下自动解析意图、拆分任务、并行分派多 Agent，聚合结果统一回复
- **进程槽位与会话隔离** — Daemon 为每个 Agent 维护独立进程，支持上下文连续和多轮迭代，同一 Agent 服务不同对话时互不干扰
- **MCP 双向集成** — Agent 通过 MCP Server 反向操作平台（创建 Agent、管理群聊、部署发布等），实现 Agent 自治
- **一键部署** — 聊天中直接触发静态站点部署，生成预览 URL，MCP 工具驱动

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go 1.26, Gin, PostgreSQL, Redis |
| 前端 | React 18, TypeScript, Vite, Ant Design, Zustand |
| Daemon | Node.js (npm 包 `@hust-agenthub/daemon`), WebSocket |
| 桌面端 | Electron（可选打包） |
| 实时通信 | WebSocket（用户端 + Daemon 双通道） |
| 测试 | Go 单元测试, Playwright E2E |

## 快速启动

### 环境要求

- Go 1.26+
- Node.js 18+
- PostgreSQL 15+
- Redis 7（可选，缺失时自动降级）

### 一键开发环境

```bash
# 克隆项目
git clone https://github.com/your-org/agent-hub.git
cd agent-hub

# 安装前端依赖
cd src/frontend && npm install && cd ../..

# 启动 PostgreSQL + 后端 + 前端
bash scripts/dev.sh
```

`dev.sh` 会自动启动 PostgreSQL（docker compose）、运行数据库迁移、启动后端和前端开发服务器。

### 单独启动

```bash
# 后端
cd src/backend
cp config/config.example.yaml config/config.yaml  # 编辑数据库密码和 JWT 密钥
go run ./cmd/server/

# 前端（Vite 代理 /api → :8080, /ws → ws://:8080）
cd src/frontend
npm run dev

# 构建
bash scripts/build.sh  # 产出 bin/server + src/frontend/dist/
```

### 桌面端（Electron）

桌面端作为独立客户端，连接远程服务器使用，无需本地部署后端：

```bash
# 开发模式
bash scripts/desktop-dev.sh

# 生产构建（打包为目录）
bash scripts/desktop-build.sh
```

桌面端连接用户指定的远程 AgentHub 服务器，支持无边框窗口、本地文件访问。首次启动时配置服务器地址和认证信息。

### Daemon 接入

用户在自己的电脑上运行 Daemon，将本地 Agent CLI 接入平台：

```bash
npx @hust-agenthub/daemon --server-url http://<server-ip>:8080 --api-key <machine-api-key>
```

Daemon 会自动扫描本机可用的 Agent CLI（Claude Code、Codex、OpenCode 等），通过 WebSocket 注册到平台。

## 架构概览

```
┌──────────────┐       ┌──────────────────────────────────┐       ┌──────────────┐
│              │  HTTP │                                  │  WS   │              │
│   Frontend   │◄─────►│          Backend (Go)            │◄─────►│   Daemon     │
│  React SPA   │  WS   │  Gin + PostgreSQL + Redis        │       │  (Node.js)   │
│              │◄─────►│                                  │       │              │
└──────────────┘       └──────────────────────────────────┘       └──────────────┘
                              │                                        │
                              │ DB                                     │ 本地 CLI
                              ▼                                        ▼
                       ┌──────────┐                           ┌──────────────┐
                       │PostgreSQL│                           │ claude /     │
                       │ + Redis  │                           │ codex /      │
                       └──────────┘                           │ opencode ... │
                                                              └──────────────┘
```

平台由四个核心模块组成：

1. **Frontend** — React SPA，提供 IM 聊天界面、Agent 管理、产物预览
2. **Backend** — Go 后端，处理 API、WebSocket、消息路由、Agent 编排
3. **Daemon** — 运行在用户电脑上的 Node.js 守护进程，管理本地 Agent CLI 进程
4. **Database** — PostgreSQL 存储会话、消息、Agent 配置等持久化数据

### 信息流

```
用户发送消息
    │
    ▼
Frontend ──WS──► Backend ──WS──► Daemon ──spawn──► Claude Code / Codex / ...
    │                │                │
    │                ▼                │
    │           写入数据库            │
    │                │                │
    │                │           Agent 执行完毕
    │                │                │
    │             ◄──WS── 结果回传 ──WS─┘
    │
    ▼
Frontend 实时渲染消息 + 产物卡片
```

- 用户在 Frontend 发送消息，通过 WebSocket 传递到 Backend
- Backend 创建 Daemon 任务，通过 WebSocket 推送到用户电脑上的 Daemon
- Daemon 调用对应的 Agent CLI（claude/codex/opencode 等）执行任务
- 执行结果沿反向链路回传，Frontend 实时渲染文本、代码 Diff、网页预览等产物

在群聊模式下，Backend 内的 **Orchestrator** 自动解析用户意图，将任务拆分并分派给多个 Agent，聚合结果后统一返回。

## 模块介绍

### Backend（Go）

后端采用分层架构，所有依赖注入在 `cmd/server/main.go` 中完成：

- **handler** — HTTP/WS 请求处理，参数校验
- **service** — 业务逻辑层，包括消息服务、Agent 服务、Orchestrator 编排服务
- **repository** — 数据访问层，封装 SQL 操作和内存任务队列
- **model** — 数据模型定义
- **middleware** — JWT 认证、CORS、错误处理
- **pkg/ws** — WebSocket Hub，管理用户端和 Daemon 端双通道连接

### Client（Web + Desktop）

客户端提供 **Web 端**和**桌面端（Electron）**两种使用方式，共享同一套 React UI。

- **会话列表** — 左侧 IM 风格会话管理，支持置顶、搜索、归档
- **聊天窗口** — 消息流渲染，支持文本、代码块、Diff 视图、网页 iframe 预览卡片、文件附件
- **Agent 管理** — 查看/创建/配置 Agent，设置 System Prompt、工具集（29 种内置工具按分类筛选）、平台 Skills 分配
- **电脑管理** — 管理已连接的 Daemon 机器，查看在线状态，配置 API Key
- **任务看板** — 对话内的任务卡片列表，跟踪 Agent 执行进度
- **知识库** — 上传文件，作为 Agent 长期上下文引用
- **设置页** — 用户信息、密码修改

Web 端通过浏览器直接访问，Vite 开发服务器代理 API 和 WebSocket 请求。桌面端（Electron）作为独立客户端连接远程服务器，支持无边框窗口和本地文件系统访问，首次启动时配置服务器地址。

状态管理使用 Zustand，WebSocket 通过自定义 Hook 维持实时连接，消息采用 JSON 协议双向通信。

### Daemon（Node.js）

以 npm 包形式分发（`@hust-agenthub/daemon`），运行在用户电脑上：

- **Agent 扫描** — 自动检测 PATH 中可用的 Agent CLI 及其版本
- **Skills 发现** — 扫描 `.claude/skills`、`.codex/skills` 等目录下的 SKILL.md 文件
- **进程管理** — 为每个 Agent 维护独立进程槽位，支持会话隔离和上下文连续
- **MCP Server** — 暴露 `agenthub-platform` MCP 工具，让 Agent 能够操作平台对象
- **任务执行** — 接收 Backend 推送的任务，调用对应 CLI 执行并回传结果

### Orchestrator（编排器）

群聊模式下的核心调度组件，内嵌在 Backend service 层：

- 自动解析用户意图，判断需要哪些 Agent 参与
- 将复杂任务拆分为子任务，并行/串行分派给对应 Agent
- 支持失败降级和超时处理
- 聚合各 Agent 的产出，生成统一回复

## 功能特性

### IM 聊天交互

- **基础 IM 功能** — 完整覆盖即时通讯核心能力：联系人列表 + 好友申请/搜索、单聊（用户↔用户、用户↔Agent）和群聊（多成员 + owner/admin/member 角色）创建与管理、会话置顶/归档、未读数与已读同步、消息回复/转发/复制/撤回（2 分钟时限）、消息内 Pin 与会话黑板（长期上下文）、文件附件上传预览、消息搜索、正在输入提示
- **双通道 WebSocket Hub** — 用户从浏览器连接用户 Hub，每台电脑的 Daemon 连接 Daemon Hub，Backend 作为中点路由消息。消息按 conversation ID 路由到对应 Room，仅推送给该对话的订阅者，避免跨对话串流
- **流式消息协议** — Agent 输出 → Daemon 解析为 chunk → 经 Daemon Hub 推送 → Backend 转发到 User Hub → 前端增量拼接渲染，用户看到的不是等待整段结果，而是逐字浮现的实时回复
- **群聊 @mention 自动分派** — 用户在群聊中发送含 `@Agent名` 的消息 → Backend 解析 mentions 列表 → 自动触发 Orchestrator 编排流程 → 拆分任务并分派给对应 Agent，无需用户手动切换模式
- **多会话并行** — 每台电脑运行一个 Daemon 守护进程，扫描本机所有 Agent CLI 作为底座；用户为每个 Agent 配置独立 System Prompt 和工具集后创建实例，Daemon 为其分配独立进程槽位；当同一 Agent 收到来自不同群聊的消息时，通过会话切换串行处理，每个对话保持独立的上下文链路

### 多 Agent 接入

- **统一命令规范** — 用户在前端为 Agent 选择 CLI 底座 → 配置存入 Backend → Daemon 收到任务时调用 `commandForTask()` 根据底座类型生成对应的命令、参数和输入格式，上层调度完全屏蔽 CLI 差异
- **Claude Code（默认底座）** — 启动后通过 stdin 建立 `--stream-json` 双向持久通信，prompt 实时注入、`--session-id` 持久化上下文。选作默认是因为其流式输出和 MCP 原生支持最完善，多轮迭代体验最佳
- **Codex** — 采用 `exec` 子命令一次性执行模式，每次任务 spawn 新进程，输出写入临时文件后读取。选择 exec 而非交互模式是因为 Codex 的交互流不稳定；启动前用 `codex login status` 预检，未登录则跳过注册
- **OpenCode / OpenClaw** — 分别用 `run --format json` 和 `agent --local --json` 启动，输出统一解析为 JSON 事件流，作为补充底座覆盖更多用户偏好
- **MCP 平台反向集成** — Daemon 启动时按各 CLI 原生方式写入 `agenthub-platform` MCP Server 配置（Claude 用 `--mcp-config`、Codex 写 `config.toml`、OpenCode 写 `opencode.json`），Agent 执行任务时自动加载，可反向调用平台工具创建 Agent、管理群聊、触发部署，实现 Agent 自治

### Agent 进程管理

- **进程槽位** — Daemon 维护以 `agent_id` 为键的 `runningAgents` Map，每个 Agent 至多持有一个活跃进程。当多个对话同时调用同一 Agent 时，任务进入队列串行执行，避免本机资源失控
- **会话隔离** — 会话 ID 由 `agent_id + conversation_id` 经 UUID v5 哈希确定性生成，同一 Agent 服务对话 A 和对话 B 时使用完全不同的 session，历史上下文互不可见
- **三路调度** — 收到任务时分三种情况处理：① 同一对话连续提问，直接通过 stdin 注入 prompt 复用进程，毫秒级响应；② Agent 当前服务于其他对话，杀掉旧进程后以 `--resume <目标sessionId>` 恢复目标会话历史；③ 首次调用，以 `--session-id` 启动新进程。兼顾响应速度和上下文隔离
- **断线保活** — WebSocket 断开时 Daemon 不杀 Agent 进程，任务继续执行，结果暂存到 `pendingTaskCompletions`；Daemon 重连 Backend 后自动回传缓存结果，避免长任务因网络抖动而丢失

### 产物预览

- **智能产物提取** — Agent 输出原始 Markdown → Daemon 的 `parseArtifacts` 扫描代码围栏（识别 `// file:` 文件名提示）、HTML 标签、裸 URL、Markdown 文档 → 分类为 `code` / `webpage` / `document` / `file` 上报 Backend 持久化。设计目标：让 Agent 自由输出，平台自动结构化
- **产物工作台** — 用户点击消息中的产物卡片 → 打开全屏模态，提供 Preview / Code / Meta 三标签页：Preview 中 webpage 走 iframe 沙箱渲染、markdown 经 GFM 渲染（含交互式 checkbox）、代码块语法高亮；Code 查看原始源码；Meta 展示产物类型、版本、来源 Agent
- **版本链路** — 同一产物的多次迭代通过 `root_id` 关联，用户可在工作台中追溯演进历史，对比不同版本的差异

### 部署发布

- **双模式部署** — Preview 模式将静态站点写入本地磁盘，通过 Cloudflare Tunnel 暴露临时公网 URL，适合快速分享；GitHub Pages 模式推送到指定仓库生成永久 URL，适合正式发布。用户根据场景选择
- **零配置隧道** — 首次部署时后端自动检测并下载 `cloudflared` 二进制，启动 quick tunnel 并从 stderr 捕获分配的 `*.trycloudflare.com` URL，用户无需配置域名、证书或防火墙
- **服务端统一渲染** — `renderSiteFiles()` 在后端将各类产物统一转为可访问 HTML：webpage 直接落盘、markdown 经 goldmark 渲染为自包含页面（含 GFM 支持）、代码包装为深色主题代码页，确保产物离线可看、跨设备一致
- **聊天指令 + MCP 工具双触发** — ① 用户在对话中输入 `/deploy`、`部署` 等关键词，Backend 拦截消息后自动部署当前对话最新产物并回复状态卡片；② Agent 在执行过程中通过 `deploy_artifact` MCP 工具主动发起部署。覆盖人工触发和 Agent 自治两种场景

### Orchestrator 群聊编排

群聊模式下，Orchestrator 作为主 Agent 自动协调多个子 Agent 协作。核心流程是一个多轮自动循环（Orch Loop）：

```
用户消息 → Orch 解析意图 → @mention 分派子任务 → Worker 并行执行
     ↑                                                    │
     │              Orch 汇总结果 → 判断是否需要下一轮 ←────┘
     │                         │
     │              不含 @mention → 输出最终结果，循环结束
     └── 含 @mention → 进入下一轮分派
```

- **@mention 分派协议** — Orch 通过 `@Agent名 任务描述` 格式分派任务，支持并行（空行分隔）和顺序执行（`→` 前缀标记依赖关系）
- **多轮循环** — Worker 执行完毕后，Orch 自动汇总结果并判断是否需要继续。如果汇总回复中包含 @mention 则自动进入下一轮，不含则循环结束
- **最大轮次保护** — 设置最大循环轮次上限，防止无限循环
- **多层上下文** — Worker 执行时注入三层上下文：群聊背景（最近消息摘要）、调度指令（Orch 分配的具体任务）、依赖输出（前置 Worker 的执行结果）
- **会话黑板（Blackboard）** — 用户可 Pin 关键消息作为群聊长期上下文，所有轮次自动携带
- **并行 Worker** — 同一轮的多个 Worker 通过 goroutine 并行执行，WaitGroup 同步等待全部完成后再触发汇总
- **CAS 状态机** — OrchTask 通过 `workers_running → summarizing → evaluating → completed/failed` 状态流转，CAS 原子操作防止并发竞争
- **自定义人格** — 支持配置 Orchestrator 的 System Prompt，改变其分派风格和决策策略

### 平台管理

- 电脑（Daemon）管理 — 查看在线状态、管理 API Key
- Agent 模板管理 — 保存和复用工具集/Skills 配置模板
- 群聊角色设置 — 配置 Orchestrator 的人格和分派策略
- 用户认证 — JWT Token，支持多用户隔离

## 项目结构

```
src/
  backend/                    Go 后端
    cmd/server/main.go        入口，DI 组装
    internal/
      handler/                HTTP/WS 处理器
      service/                业务逻辑
      repository/             数据访问
      model/                  数据模型
      middleware/             中间件
    pkg/
      ws/                     WebSocket Hub
      redis/                  Redis 客户端
    config/                   配置文件
    migrations/               SQL 迁移脚本
  frontend/                   React 前端
    src/
      api/                    后端 API 封装
      components/             UI 组件
      hooks/                  自定义 Hooks
      store/                  Zustand 状态管理
      views/                  页面视图
      types/                  TypeScript 类型
      layout/                 布局组件
    e2e/                      Playwright E2E 测试
    electron/                 Electron 桌面端配置
  daemon/                     Go Daemon（备用）
  daemon-npm/                 npm Daemon 包
    bin/agenthub-daemon.js    主入口
scripts/
  dev.sh                      开发环境启动
  build.sh                    构建
  test.sh                     测试
```

## AI 协作开发

本项目使用 **Trellis**（文件驱动的 AI 协作框架）管理开发流程，将 AI Agent 的编码过程结构化为可追溯的任务生命周期。

### 协作框架

本项目使用 **Trellis**（文件驱动的 AI 协作框架）管理开发流程。Trellis 的核心思路是"对话会被压缩，文件不会"——将 AI 编码过程的所有中间状态持久化为文件，使不同 AI 会话之间可以无缝接力。

**一个任务的完整生命周期：**

```
1. 用户描述需求 → AI 逐轮提问澄清，产出 prd.md
2. AI 调研技术方案 → 写入 research/ 目录
3. 人工确认需求 → AI 配置 implement.jsonl（指定 sub-agent 需要读取哪些编码规范）
4. 激活任务 → 主会话派发 trellis-implement sub-agent 编码
5. trellis-check sub-agent 对照规范审查 + 自动修复
6. 循环 4-5 直到质量检查通过
7. 新发现的编码规范沉淀到 .trellis/spec/
8. 提交代码，归档任务
```

**Hook 驱动的上下文注入** 是整个流程的自动化引擎。三组 Hook 分别在不同时机向 AI 注入结构化上下文：

- **SessionStart** — 每次新会话/清除/压缩后，自动注入当前任务状态、Git 分支、活跃任务列表、工作流指引和编码规范索引，让 AI 无需人工复述即可接手工作
- **UserPromptSubmit** — 每轮对话注入轻量级面包屑，根据当前任务阶段（planning / in_progress / completed）提醒 AI 下一步该做什么，防止跳过必要步骤
- **PreToolUse** — 在派发 sub-agent 前拦截，自动将 `implement.jsonl` / `check.jsonl` 中引用的规范文件内容注入到 sub-agent 的 prompt 中，确保编码和审查 Agent 都能读取到项目约定

**主从分工**：主会话（Claude Code）负责需求分析、上下文组装和提交决策，不直接编辑代码；`trellis-implement` sub-agent 读取 PRD + 注入的规范后编码；`trellis-check` sub-agent 对照规范审查并自动修复。角色分离避免自我审批。

**跨平台适配**：通过平台检测（环境变量 + Hook 路径）自动适配 Claude Code、Codex、Cursor、OpenCode、Gemini 等 12 种 AI 编码工具。支持 sub-agent 模式（Claude Code/Cursor 等）和 inline 模式（Codex 等），同一套任务规范和 PRD 在不同平台间通用。

### 端到端测试环境

为了让 AI 不止"写完即走"，平台为 Agent 配备了一套基于 MCP 的浏览器测试环境，让 AI 在编码后能直接驱动浏览器完成自我验收，把"能跑通编译"提升到"能跑通真实场景"。

- **双 MCP 浏览器工具** — 同时接入 `chrome-devtools` 和 `playwright` 两个 MCP Server。前者基于 Chrome DevTools Protocol 提供深度调试能力（a11y 快照、控制台日志、网络抓包、Performance trace、Lighthouse 审计），后者基于 Playwright 提供脚本化端到端自动化（跨浏览器导航、元素交互、表单填写、截图对比）。两者互补：chrome-devtools 用于诊断"为什么不对"，playwright 用于验证"是否真的对了"
- **可观测的 UI 修改** — Agent 改完前端代码后，直接通过 MCP 驱动浏览器打开本地 dev server，对改动前后截图对比、读取真实渲染的 DOM、捕获控制台报错与网络异常，所见即所得地确认 UI 渲染符合预期，避免"代码看起来对、运行时白屏"这类编译通过但功能失效的常见陷阱
- **端到端功能验证** — Agent 按用户故事编排完整操作流（登录 → 创建群聊 → @mention 触发 Orchestrator → 校验产物卡片渲染与部署链接），覆盖从前端交互到后端落库的完整链路；配合项目内置的 Playwright E2E 测试套件，在提交前完成自我验收
- **闭环修复反馈** — 测试中捕获的异常（控制台 error、网络 4xx/5xx、断言失败、a11y 问题）自动回流到 Agent 上下文，驱动下一轮修复迭代，形成"编码 → 浏览器验证 → 修复"的完整闭环

### 协作成果

AgentHub 项目本身即是 AI 协作的产物。在约三周的开发周期内，通过 Trellis 管理了 **29 个任务**、**500+ 次提交**，核心模块（Orchestrator 群聊编排、Daemon 多 CLI 适配、产物智能提取与部署）均由 AI 主导实现。协作过程中沉淀出以下可复用资产：

**编码规范体系**（`.trellis/spec/`）— 按后端/前端/通用三层组织，在 AI 编码时自动注入：

- 后端规范：context.Context 首参约定、`%w` 错误链、Daemon 存活时序合约、禁止 `init()` 函数
- 前端规范：Zustand 状态分类（local/global UI/server metadata/URL）、禁止 `any` 类型、REST 请求经 `api/` 模块、产预览组件合约
- 通用规范：中文注释英文命名、文件 300 行上限、搜索优先于新建代码、跨层数据流边界检查

**工作流文档**（`.trellis/workflow.md`）— 693 行的完整工作流定义，包含每个阶段的步骤说明、平台差异标记、`[required]` / `[optional]` 标注，作为 AI 行为的"操作系统"

**开发者日志**（`.trellis/workspace/`）— 每次会话自动记录变更摘要、Git 提交、测试状态和下一步计划，形成连续的开发日志链

**Bug 经验库**（`doc/task/Bugfix-测试发现的Bug.md`）— 13 轮深度测试沉淀的 **110+ 个真实 Bug**（按 P0–P3 分级），覆盖后端、前端、性能、安全等维度，归纳出高频复现模式：

- **层边界 Bug** — 前后端响应结构不匹配（后端返回 `{user_message, agent_message}` 但前端按 `Message` 解析，导致幽灵气泡）、UI 存在但调用链断开（`sendMessage` 未传 `reply_to`）
- **编译通过 ≠ 功能正确** — 多个 Bug 明确指出 `go build` + `tsc` 通过掩盖了死代码、被忽略的参数和未发送的字段
- **生命周期与幂等性** — `close of closed channel` panic、RateLimiter goroutine 泄漏、`INSERT` 缺少 `ON CONFLICT`、缓存写入未失效
- **Zustand 反模式** — 选择器内联 `?? []` 触发无限渲染、订阅整个对象导致全量 re-render、`Set`/`Map` 破坏不可变性

**过程经验沉淀**（`doc/conventions/process-lessons.md` + `doc/retrospective/`）— 6 篇根因分析文档，每篇按"问题 → 案例 → 规则 → 根本原因"结构组织，例如 Bash 命令的 `cd` 必须放在 `command` 参数中、后端改动必须重启服务、API 必须返回空数组而非 `null`

### Trellis 目录结构

```
.trellis/
├── workflow.md                      工作流定义（三阶段流程）
├── config.yaml                      项目配置
├── spec/                            编码规范（注入到 AI 上下文）
│   ├── backend/                     后端规范（quality / database / error-handling ...）
│   ├── frontend/                    前端规范（state-management / artifact-preview ...）
│   └── guides/                      通用思考指南（code-reuse / cross-layer ...）
├── tasks/                           任务目录
│   ├── MM-DD-task-name/
│   │   ├── prd.md                   需求文档
│   │   ├── research/                调研产物
│   │   ├── implement.jsonl          实现 sub-agent 上下文配置
│   │   └── check.jsonl              检查 sub-agent 上下文配置
│   └── archive/YYYY-MM/             已归档任务
├── workspace/<developer>/           开发者日志与会话记录
└── .runtime/sessions/               每会话活跃任务指针

# Hook 脚本位于 .claude/hooks/
.claude/hooks/
├── session-start.py                 会话启动注入
├── inject-workflow-state.py         每轮面包屑注入
└── inject-subagent-context.py       sub-agent 上下文注入
```

## API 概览

| 路径 | 说明 |
|------|------|
| `POST /api/auth/register` | 用户注册 |
| `POST /api/auth/login` | 用户登录 |
| `GET /ws?token=` | 用户 WebSocket 连接 |
| `GET /api/conversations` | 对话列表 |
| `POST /api/conversations/private` | 创建/获取私聊 |
| `GET /api/conversations/:id/messages` | 消息历史 |
| `POST /api/conversations/:id/messages` | 发送消息 |
| `POST /api/groups` | 创建群聊 |
| `POST /api/upload` | 文件上传 |
| `GET /api/agents` | Agent 列表 |
| `POST /api/agents` | 创建 Agent |
| `GET /daemon/ws?token=` | Daemon WebSocket 连接 |
| `GET /health` | 健康检查 |

## 配置

见 `src/backend/config/config.example.yaml`。`config.yaml` 被 gitignore，需从示例文件复制并填入实际值。

## License

MIT
