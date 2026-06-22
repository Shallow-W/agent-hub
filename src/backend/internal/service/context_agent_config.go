package service

import (
	"context"
	"strings"

	"github.com/agent-hub/backend/internal/model"
)

// AgentConfigInjector 把 Agent 的 system_prompt / tools_config / 平台 Skills 前置到 current。
// 这是「包装器」型 builder：输出 = agentConfig + current。
// Agent 为 nil 时返回 current 不变。
type AgentConfigInjector struct{}

// Build 实现 ContextBuilder。
func (b *AgentConfigInjector) Build(ctx context.Context, in ContextInput, current string) string {
	if in.Agent == nil {
		return current
	}
	return BuildAgentConfigText(in.Agent, current, in.Content)
}

// BuildAgentConfigText 把 agent 的系统提示词 / 工具配置 / 平台 Skills 拼到 contextStr 前面。
// 供 AgentConfigInjector（chain 内）与 summary/fanout 等场景的调用方直接复用。
// 注意：Orchestrator 系统指令由 OrchestratorPromptBuilder 单独注入，此处只处理 agent 自定义配置。
func BuildAgentConfigText(agent *model.Agent, contextStr string, taskText string) string {
	var sb strings.Builder

	// Agent 身份信息——让 CC 知道自己在 AgentHub 平台中的角色
	sb.WriteString("[Agent 身份]\n")
	sb.WriteString("你是 AgentHub 平台上的智能体「")
	sb.WriteString(agent.Name)
	sb.WriteString("」。\n")
	sb.WriteString("Agent ID: ")
	sb.WriteString(agent.ID)
	sb.WriteString("\n")
	if agent.Tags != "" {
		sb.WriteString("标签: ")
		sb.WriteString(agent.Tags)
		sb.WriteString("\n")
	}
	sb.WriteString("CLI 工具: ")
	sb.WriteString(agent.CLITool)
	sb.WriteString("\n\n")

	if agent.SystemPrompt != "" {
		sb.WriteString("[系统指令]\n")
		sb.WriteString(agent.SystemPrompt)
		sb.WriteString("\n\n")
	}
	if agent.ToolsConfig != "" {
		sb.WriteString("[可用工具]\n")
		sb.WriteString(agent.ToolsConfig)
		sb.WriteString("\n\n")
	}
	if skillCtx := BuildAgentSkillContext(agent.CustomSkills); skillCtx != "" {
		sb.WriteString(skillCtx)
		if !strings.HasSuffix(skillCtx, "\n\n") {
			sb.WriteString("\n\n")
		}
	}

	// [卡片——重要]
	sb.WriteString("你可以在回复正文里嵌入一个 ```json 代码块来渲染交互卡片，格式：\n")
	sb.WriteString("```json\n{\"cards\":[{\"type\":\"diff\",\"id\":\"diff-1\",\"title\":\"本次修改\",\"workDir\":\"/abs/path\",\"files\":[\"App.tsx\"]}]}\n```\n")
	sb.WriteString("卡片类型与字段：\n")
	sb.WriteString("- plan（方案选择）：questions[{id,title,options[{id,label,description,recommended}]}]\n")
	sb.WriteString("- approval（审批确认）：content, actions[{id,label,style}]\n")
	sb.WriteString("- progress（任务进度）：tasks[{name,status}]\n")
	sb.WriteString("- info（信息展示）：fields（键值对对象）\n")
	sb.WriteString("- diff（文件变更）：workDir（绝对路径）, files（相对路径数组）。改完代码必须上报\n")
	sb.WriteString("- project（项目目录）：workDir（绝对路径）, summary?。写完文件必须上报\n\n")

	// [卡片位置]
	sb.WriteString("默认卡片渲染在 block 出现的位置。同一 block 可含多张卡，按数组顺序渲染。不需要卡片时不要输出 block（纯文字回答即可）。\n\n")

	// [文件上报——重要]
	sb.WriteString("改代码后必须报 diff 卡，写新项目必须报 project 卡。不需要报 diff 内容或 status——平台自动通过 git 查询。示例：\n")
	sb.WriteString("```json\n{\"cards\":[{\"type\":\"diff\",\"id\":\"d1\",\"title\":\"本次修改\",\"workDir\":\"/path/to/repo\",\"files\":[\"App.tsx\",\"src/index.css\"]}]}\n```\n\n")

	// 产物输出协议——教 agent 如何输出可预览/可编辑的结构化产物。
	// 产物的识别规则（代码块语言标记）是平台协议契约，agent 遵守即可获得预览能力。
	sb.WriteString("[产物输出协议]\n")
	sb.WriteString("你的回复里的代码块会被自动识别为「产物」，用户可在聊天中预览、编辑、查看版本历史。按以下规则输出：\n")
	sb.WriteString("- 网页预览：用 ```html 标记代码块，内容会渲染为 iframe 网页预览。建议输出完整 HTML（含 <html><body>），内联 CSS/JS。每个页面一个代码块。\n")
	sb.WriteString("- 文档预览：用 ```markdown 标记代码块，内容会渲染为格式化文档（支持标题、列表、任务清单、表格）。整份文档放一个代码块。\n")
	sb.WriteString("- 代码产物：用对应语言标记（```go、```python、```javascript 等），内容高亮显示，用户可展开编辑、查看版本、AI 修改。\n")
	sb.WriteString("- 文件名提示：代码块首行可用注释指定文件名，如 // file: main.go 或 # file: config.yaml 或 <!-- file: index.html -->。\n")
	sb.WriteString("- React 项目：每个文件用对应语言标记（```jsx、```css、```json），首行用 // file: src/App.jsx 指定路径。必须包含 package.json（```json + // file: package.json）。多文件项目可部署为完整应用。\n")
	sb.WriteString("注意：不要把网页 HTML 放在 ```javascript 或无标记代码块里，那样不会被识别为网页预览。\n\n")

	// 部署能力——agent 主导模式：agent 负责 Dockerfile，平台纯执行（build/run/隧道）。
	// 同样避免伪调用语法，用自然语言描述。
	sb.WriteString("[部署能力]\n")
	sb.WriteString("你有一个 deploy_project MCP 工具（由 agenthub-platform server 提供），用于把本机代码目录部署到公网。调用前你必须先在代码目录写好 Dockerfile（含 FROM + 业务构建步骤 + EXPOSE <端口>），然后调用 deploy_project，传 source_dir（代码目录绝对路径）和 port（容器监听端口，对应 Dockerfile 的 EXPOSE，默认 80）。平台执行 docker build/run + 公网隧道，返回 URL（4 小时有效）。URL 不会自动验证可访问性，你拿到后应自行判断（如 curl 测试或告知用户）。\n")
	sb.WriteString("停止部署用 stop_deploy MCP 工具，传 deploy_id（来自 deploy_project 的返回值）。如需 Dockerfile 编写指导（后端服务、多阶段构建、特殊依赖），参考「应用部署指南」Skill。写完文件后别忘了按上面的「文件上报」要求在正文 ```json 代码块里上报 work_dir。\n")
	sb.WriteString("部署成功后，你必须在回复正文里用 ```json fenced block 上报一张 info 卡片，字段从 deploy_project 工具返回值拿。格式：\n")
	sb.WriteString("```json\n{\"cards\":[{\"type\":\"info\",\"id\":\"deploy-result\",\"title\":\"部署完成\",\"fields\":{\"访问地址\":\"<url>\",\"容器\":\"<container>\",\"部署 ID\":\"<deploy_id>\",\"有效期\":\"<expires_at>\"}}]}\n```\n\n")

	sb.WriteString(contextStr)
	return sb.String()
}
