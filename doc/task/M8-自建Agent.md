# M8 自建 Agent

## 目标

用户可在 WebUI 上创建自定义 Agent，选择底层 CLI 工具、编写 System Prompt，并分配该 Agent 可用的平台工具集与平台 Skills。

## 子任务

### M8-1 自建 Agent 创建页面（部分完成）

- 表单字段：名称、头像、描述、底层 CLI 工具（下拉选择已安装的）、System Prompt（多行文本）
- System Prompt 提供示例模板（如"你是一个前端专家，擅长 React 和 TypeScript..."）
- 保存后出现在 Agent 列表中
- 已支持从已连接电脑的候选底座创建 Agent 时选择工具集模板、勾选具体平台 MCP 工具，并填写初始平台 Skills。
- 已支持在 Agent 详情页继续编辑工具集和平台 Skills。

### M8-2 自建 Agent 后端（部分完成）

- 存储到 `agents` 表（type=custom, cli_tool=用户选择的, system_prompt=用户编写的）
- 与系统 Agent 共用同一个 Agent 列表接口
- 调度时：用自建 Agent 的 system_prompt 替换默认值，通过对应 CLI 工具的适配器执行
- `agents.tools_config` 使用 `{"toolset": string, "allowed_tools": string[]}` 作为 per-Agent MCP 工具授权配置；后端保存前过滤未知工具名。
- `agents.custom_skills` 存储用户为该 Agent 分配的平台 Skills，不由 daemon 底座扫描覆盖。

### M8-3 自建 Agent 对话验证

- 用自建 Agent 发起对话
- 验证自定义 System Prompt 生效（如 Agent 自我介绍符合设定的角色）
- 验证使用用户选择的 CLI 工具执行

## 验收标准

- [x] 可在 WebUI 上创建自建 Agent
- [ ] 自建 Agent 出现在 Agent 列表中（有自定义名称和头像）
- [ ] 用自建 Agent 对话时，使用自定义 System Prompt
- [ ] 自建 Agent 使用用户选择的底层 CLI 工具
- [ ] 可修改和删除自建 Agent
- [x] 创建和编辑时可分配工具集与平台 Skills
- [x] 工具集配置持久化到后端并被 daemon MCP runtime 按 Agent 限制

## 验证记录

- 2026-06-09：使用账号 `wjc` 在 WebUI 创建测试 Agent，确认 `tools_config` 和 `custom_skills` 落库；随后在详情页将工具集改为 `none`，API 返回 `{"toolset":"none","allowed_tools":[]}`。

## 依赖

- M4-6（Agent 配置 CRUD 接口 + 列表 UI）
- M6（单聊跑通，确保基础对话链路可用）
