# 整合远端 dev 分支

## 背景

用户希望将 `https://github.com/Shallow-W/agent-hub/tree/dev` 的内容整合到当前本地项目，并强调“最好在兼容的情况下整合”。

当前本地分支为 `feat/multi-agent`，远端 `origin/dev` 比当前分支包含大量新增提交，涉及前端、后端、daemon、知识库、Orchestrator、Agent 工具配置等模块。

## 目标

- 将 `origin/dev` 中的新功能和修复合并进当前分支。
- 保留当前分支已有提交和本地已有工作，不覆盖未确认的用户改动。
- 对明显不兼容或不应提交的内容做兼容修正。
- 整合后尽可能通过前端构建、后端测试或至少核心编译检查。

## 非目标

- 不重写远端历史。
- 不推送到远端。
- 不删除用户未确认的本地改动。
- 不进行大规模重构，只处理整合冲突和必要兼容问题。

## 已知风险

- `origin/dev` 的 `.gitignore` 存在残留冲突标记，需要合并后修复。
- `origin/dev` 包含若干二进制构建产物，如 `src/backend/agenthub-server`、`src/backend/main`、`src/backend/tmp/main`，整合时需要判断是否应保留。
- 当前工作树已有 `src/daemon-npm/bin/agenthub-daemon.js` 脏状态，不能直接覆盖。
- 前后端和数据库 migration 都有变化，可能出现类型、路由、模型、迁移顺序冲突。

## 验收标准

- `git status` 中没有未解决冲突。
- `.gitignore` 不含冲突标记。
- 本地当前分支包含 `origin/dev` 的目标代码变更，同时保留当前分支改动。
- 前端依赖安装和构建检查通过，或明确记录失败原因。
- 后端 Go 测试或编译检查通过，或明确记录失败原因。
