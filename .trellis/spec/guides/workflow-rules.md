# Workflow Rules

> 任务全生命周期的工作流规则——commit、review、测试、文档更新。

---

## 1. 无明确指令时
按 `doc/TASKLIST.md` 顺序解决未完成任务（按依赖关系，优先最短跑通路径）。

## 2. 提出新任务时
在 `doc/TASKLIST.md` 添加索引行，在 `doc/task/` 下创建详情文件，遵循 `doc/conventions/` 中的文件命名和格式规范。

## 3. 完成任务时
将 TASKLIST.md 中对应状态改为 `[x]`。

## 4. 自动提交
每次任务完成后，自行判断是否需要提交代码。有实质性改动则直接 commit，严格遵守 `doc/conventions/git-conventions.md` 中的 commit 格式：`type(scope): 中文描述`。不要主动询问用户是否提交。

## 5. 提交后自动 Review + 测试
commit 后启动 3 个并行 sub-agent：

- **(a) 代码质量/逻辑缺陷审查**——侧重代码可读性、潜在 bug、边界处理、SOLID 原则
- **(b) 功能验证**——扮演产品经理角色，验证功能是否完整实现、交互流程是否合理、用户体验是否达标、是否有遗漏场景
- **(c) 端到端测试**——根据变更内容编写并执行实际测试（API 调用、UI 操作），验证核心流程可跑通，发现的 bug 记录到 `doc/TASKLIST.md`

### 约束
- 最少迭代 3 轮，最多 8 轮
- 收敛（三个 agent 均无 critical 问题 + 测试通过）即提前停止
- review/测试修复产生的 commit 不再触发本条规则（避免反馈循环）
- **测试环节必须覆盖正常路径和边界情况（如 API 返回 null/空数组、并发请求等），不得仅依赖编译通过作为验证手段**

## 6. Review 通过后更新文档
检查本次任务是否涉及需要文档化的内容（新增结构体/接口、API 变更、架构调整等），如有则同步更新 `doc/` 下的设计文档、API 文档或数据模型文档，遵循 `doc/conventions/doc-conventions.md` 中的文档编写规范。文档更新产生的 commit 不再触发规则 5。
