# PRD: 将 AGENTS.md/CLAUDE.md 编码规范迁移至 Trellis spec

## 目标
将分散在 `CLAUDE.md` 和 `AGENTS.md` 中的编码规范迁移到 `.trellis/spec/` 下，使 Trellis SessionStart hook 成为规范的唯一注入源，消除重复。

## 迁移映射

| 源内容 | 目标文件 |
|--------|---------|
| CLAUDE.md 4 条核心规则（先想后写/简单至上/外科手术式/目标驱动） | `.trellis/spec/guides/core-principles.md` |
| CLAUDE.md + AGENTS.md 工作流规则（commit/review/test/docs） | `.trellis/spec/guides/workflow-rules.md` |
| AGENTS.md 通用硬性规则（注释语言/换行/文件长度/敏感信息/测试） | `.trellis/spec/guides/general-conventions.md` |
| AGENTS.md 前端规则（no any/api模块/WS hook/CSS Modules） | `.trellis/spec/frontend/quality-guidelines.md` |
| AGENTS.md 后端规则（context.Context/%w/init禁用/DI/接口定义） | `.trellis/spec/backend/quality-guidelines.md` |

## 迁移后目标状态

### CLAUDE.md
精简为：4 条核心规则引用 Trellis spec + 指向 AGENTS.md 的指针。不再包含具体编码规范。

### AGENTS.md
保留项目简介、核心概念、技术栈、快速导航、优先级定义。删除硬性规则章节（已迁入 spec）。

### .trellis/spec/guides/index.md
更新索引表，添加新文件链接。

## 验收标准
- [ ] 5 个 spec 文件已创建/填充实际项目规范
- [ ] `guides/index.md` 已更新索引
- [ ] CLAUDE.md 和 AGENTS.md 中已删除迁移内容，无重复
- [ ] 无信息遗漏（对比迁移前后内容完整性）
