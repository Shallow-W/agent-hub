# CLAUDE.md
规则 1——先想后写。 杜绝隐蔽假设。明确声明你的前提假设，阐明方案权衡。有疑问先确认，切忌盲目猜测。若存在更简单的实现路径，应果断提出异议。
规则 2——简单至上。 仅以最少代码解决问题。不添加"以防万一"的冗余功能，不为仅用一次的代码强行设计抽象层。若资深工程师会认为其过度复杂，则立即简化。
规则 3——外科手术式修改。仅改动绝对必要的部分。切勿"顺手优化"相邻代码、注释或格式排版。未出问题的代码绝不重构。严格贴合项目现有代码风格。
规则 4——目标驱动。 明确定义成功标准（验收条件）。持续迭代直至验证通过。不要规定具体操作步骤，只需清晰描述"成功的最终形态"，交由模型自主探索与迭代。
This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project: AgentHub - 多Agent协作平台

一个以IM聊天为核心交互范式的多Agent协作平台。用户通过类似飞书/微信的对话界面与多个AI Agent（Claude Code、Codex、OpenCode等）交互，支持单聊、群聊、任务分派和产物预览。

### 核心概念

- **IM聊天范式**：对话列表 + 单聊/群聊 + 富媒体消息，是整个平台的交互核心
- **Orchestrator（协调器）**：群聊模式下理解用户意图，将任务拆解并分派给子Agent，聚合结果
- **Agent适配器层**：统一抽象不同Agent平台的API差异（Claude Code、Codex、OpenCode等）
- **自建Agent**：用户通过对话式创建，设定System Prompt + 工具集
- **产物系统**：Agent回复中内联预览卡片（网页iframe、代码Diff、文件附件等）

### 优先级

- **P0**：IM聊天核心体验、单聊/群聊、多Agent接入（≥2个）、Orchestrator
- **P1**：产物预览卡片、上下文管理（pin消息）、多会话并行
- **P2**：部署发布、Diff/版本历史、PPT浏览、多端支持

### 交付要求

- 30%权重在AI协作能力（需沉淀Spec、Skill、Rules协作规范）
- 需产出：产品设计文档 + 技术文档 + 可运行Demo + AI协作开发记录 + 3分钟Demo视频

### 需求文档

完整需求见 `doc/需求文档.md`，原始PDF见 `doc/AgentHub-_多Agent协作平台设计.pdf`

### 任务列表

任务拆解与进度跟踪见 `doc/TASKLIST.md`

## 开发规范

- Git分支/提交规范：`doc/harness/git-conventions.md`
- 前端React/TS + 后端Go编码规范：`doc/harness/coding-conventions.md`
- Monorepo目录结构与文件命名：`doc/harness/project-structure.md`

## 工作流

1. **无明确指令时**，按 `doc/TASKLIST.md` 顺序解决未完成任务（按依赖关系，优先最短跑通路径）
2. **提出新任务时**，在 `doc/TASKLIST.md` 添加索引行，在 `doc/task/` 下创建详情文件，遵循 `doc/harness/` 中的文件命名和格式规范
3. **完成任务时**，将 TASKLIST.md 中对应状态改为 `[x]`
4. **每次任务完成后，自行判断是否需要提交代码。有实质性改动则直接 commit，严格遵守 `doc/harness/git-conventions.md` 中的 commit 格式：`type(scope): 中文描述`。不要主动询问用户是否提交**
5. **提交后自动 Review**：commit 后启动 2 个并行 sub-agent 对该 commit 进行 code review（一个侧重代码质量/逻辑缺陷，一个侧重安全性/规范合规）。最多迭代 3 轮，收敛（两个 reviewer 均无 critical 问题）即提前停止。review 修复产生的 commit 不再触发 Rule 5（避免反馈循环）
