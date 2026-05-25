# 文档编写规范

## 通用规则

- **语言**：中文
- **格式**：Markdown（`.md`）
- **编码**：UTF-8，LF 换行
- **文件命名**：小写 + 短横线分隔（`api-design.md`，非 `apiDesign.md` 或 `API_Design.md`）

---

## 文档分层

```
doc/
├── architecture/              # 稳定层（很少变）
│   ├── overview.md            # 系统架构图 + 一段话描述
│   ├── boundaries.md          # 模块边界和依赖规则
│   └── data-flow.md           # 数据流转图
│
├── conventions/               # 规范层（偶尔更新）
│   ├── README.md              # 规范总览（索引）
│   ├── frontend-conventions.md
│   ├── backend-conventions.md
│   ├── git-conventions.md
│   ├── doc-conventions.md
│   └── project-structure.md
│
├── design/                    # 设计层（按功能组织）
│   ├── feature-xxx.md         # Status: ✅ Implemented / 📋 Approved / 📝 Draft
│   ├── api-*.md               # API 设计
│   └── data-model.md          # 数据模型
│
├── plans/                     # 计划层（频繁变）
│   ├── current-sprint.md      # 当前迭代
│   └── backlog.md             # 待办
│
├── reference/                 # 参考层（自动生成）
│   ├── api-spec.yaml
│   └── error-codes.md
│
├── task/                      # 任务详情（与 TASKLIST.md 配合）
├── 需求文档.md
├── TASKLIST.md
└── AgentHub-_多Agent协作平台设计.pdf
```

---

## 文档触发时机

以下情况**必须**同步更新文档：

- 新增或修改 API 接口 → 更新 API 设计文档
- 新增或修改数据表 / 结构体 → 更新数据模型文档
- 架构调整（新增模块、变更依赖关系）→ 更新架构设计文档
- 新增配置项 → 更新配置说明

---

## 设计文档模板

### API 设计文档（`doc/design/api-*.md`）

```markdown
# {模块名} API 设计

## 概述
<!-- 简述模块用途 -->

## 接口列表

### {POST/GET/PUT/DELETE} {路径}

**描述**：{接口用途}

**请求参数**：
| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|

**请求示例**：
​```json
{ }
​```

**响应示例**：
​```json
{ }
​```

**错误码**：
| code | 说明 |
|------|------|
```

### 数据模型文档（`doc/design/data-model.md`）

```markdown
# 数据模型

## {表名/结构体名}

**用途**：{描述}

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|

### 关联关系
<!-- 与其他表的关联 -->
```

### 架构设计文档（`doc/design/architecture-*.md`）

```markdown
# {模块名} 架构设计

## 背景
<!-- 为什么需要这个模块 -->

## 架构图
<!-- 用文字或 Mermaid 描述 -->

## 模块职责
<!-- 各子模块/层的职责 -->

## 关键决策
| 决策 | 选项 | 最终选择 | 原因 |
|------|------|----------|------|

## 依赖关系
<!-- 模块间的依赖 -->
```

---

## 编写原则

1. **面向未来读者**：文档要让 3 个月后的自己或新加入的成员能看懂
2. **只记"为什么"**：代码和命名已经表达了"是什么"，文档侧重记录设计决策和约束的原因
3. **保持同步**：文档与代码不一致时，以代码为准，但应及时修正文档
4. **避免大段重复**：不在多处维护同一份信息，用引用或链接指向唯一来源
5. **增量更新**：每次变更只更新相关部分，不做无关的文档重构
