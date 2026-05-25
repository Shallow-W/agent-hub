# CLAUDE.md
规则 1——先想后写。 杜绝隐蔽假设。明确声明你的前提假设，阐明方案权衡。有疑问先确认，切忌盲目猜测。若存在更简单的实现路径，应果断提出异议。
规则 2——简单至上。 仅以最少代码解决问题。不添加"以防万一"的冗余功能，不为仅用一次的代码强行设计抽象层。若资深工程师会认为其过度复杂，则立即简化。
规则 3——外科手术式修改。仅改动绝对必要的部分。切勿"顺手优化"相邻代码、注释或格式排版。未出问题的代码绝不重构。严格贴合项目现有代码风格。
规则 4——目标驱动。 明确定义成功标准（验收条件）。持续迭代直至验证通过。不要规定具体操作步骤，只需清晰描述"成功的最终形态"，交由模型自主探索与迭代。
This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

项目信息与快速导航见 `AGENTS.md`。

## 工作流

1. **无明确指令时**，按 `doc/TASKLIST.md` 顺序解决未完成任务（按依赖关系，优先最短跑通路径）
2. **提出新任务时**，在 `doc/TASKLIST.md` 添加索引行，在 `doc/task/` 下创建详情文件，遵循 `doc/conventions/` 中的文件命名和格式规范
3. **完成任务时**，将 TASKLIST.md 中对应状态改为 `[x]`
4. **每次任务完成后，自行判断是否需要提交代码。有实质性改动则直接 commit，严格遵守 `doc/conventions/git-conventions.md` 中的 commit 格式：`type(scope): 中文描述`。不要主动询问用户是否提交**
5. **提交后自动 Review**：commit 后启动 2 个并行 sub-agent 对该 commit 进行 code review（一个侧重代码质量/逻辑缺陷，一个侧重安全性/规范合规）。最少迭代 3 轮，最多 8 轮，收敛（两个 reviewer 均无 critical 问题）即提前停止。review 修复产生的 commit 不再触发 Rule 5（避免反馈循环）
6. **Review 通过后更新文档**：检查本次任务是否涉及需要文档化的内容（新增结构体/接口、API 变更、架构调整等），如有则同步更新 `doc/` 下的设计文档、API 文档或数据模型文档，遵循 `doc/conventions/doc-conventions.md` 中的文档编写规范。文档更新产生的 commit 不再触发步骤 5
