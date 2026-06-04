# General Conventions

> 适用于前后端的通用硬性规则。

---

## 编码规范

1. **注释语言：中文**（说明"为什么"），**命名语言：英文**
2. **换行：LF**，编码：**UTF-8**
3. **单文件不超过 300 行**（前端组件 / Go 文件），超过则拆分
4. **禁止提交敏感信息**（API Key、密码、`.env`）
5. **新增代码必须有对应测试**

## Codegraph 使用

缺少项目认识时，优先使用 Codegraph（`codegraph_context` / `codegraph_trace`）了解模块全貌和调用关系，再动手改代码。用法见 `doc/reference/codegraph.md`。
