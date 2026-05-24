# M8 自建 Agent

## 目标

用户可在 WebUI 上创建自定义 Agent，选择底层 CLI 工具并编写 System Prompt。

## 子任务

### M8-1 自建 Agent 创建页面

- 表单字段：名称、头像、描述、底层 CLI 工具（下拉选择已安装的）、System Prompt（多行文本）
- System Prompt 提供示例模板（如"你是一个前端专家，擅长 React 和 TypeScript..."）
- 保存后出现在 Agent 列表中

### M8-2 自建 Agent 后端

- 存储到 `agents` 表（type=custom, cli_tool=用户选择的, system_prompt=用户编写的）
- 与系统 Agent 共用同一个 Agent 列表接口
- 调度时：用自建 Agent 的 system_prompt 替换默认值，通过对应 CLI 工具的适配器执行

### M8-3 自建 Agent 对话验证

- 用自建 Agent 发起对话
- 验证自定义 System Prompt 生效（如 Agent 自我介绍符合设定的角色）
- 验证使用用户选择的 CLI 工具执行

## 验收标准

- [ ] 可在 WebUI 上创建自建 Agent
- [ ] 自建 Agent 出现在 Agent 列表中（有自定义名称和头像）
- [ ] 用自建 Agent 对话时，使用自定义 System Prompt
- [ ] 自建 Agent 使用用户选择的底层 CLI 工具
- [ ] 可修改和删除自建 Agent

## 依赖

- M4-6（Agent 配置 CRUD 接口 + 列表 UI）
- M6（单聊跑通，确保基础对话链路可用）
