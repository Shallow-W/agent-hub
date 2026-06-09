package service

import "fmt"

// GenerateManagementTools generates markdown tool definitions for agent management.
// serverURL is the base URL of the AgentHub server (e.g., "http://127.0.0.1:8080").
// token is a scoped JWT token for the agent to authenticate.
func GenerateManagementTools(serverURL, token string) string {
	return fmt.Sprintf(`# AgentHub 平台管理工具

**重要：这些工具仅用于管理 Agent 和 Machine。禁止使用这些 API 发送消息到会话或与用户交互。当用户给你发消息时，直接回复即可，不要尝试通过 API 转发消息或调用消息相关接口。**

这些工具通过 Bash 执行 curl 命令调用 REST API，**不要尝试使用 MCP 或其他方式调用**。
所有 API 调用需要在 HTTP header 中携带 Authorization: Bearer token。token 已嵌入下方命令中，直接复制执行即可。

## agent_list
获取当前用户的所有 Agent 列表。
`+"```bash"+`
curl -s %s/api/agents -H "Authorization: Bearer %s"
`+"```"+`

## agent_create
创建一个新的 Agent。参数: name (string), cli_tool (string, "claude"/"codex"), system_prompt (string, 可选)。
`+"```bash"+`
curl -s -X POST %s/api/agents \
  -H "Authorization: Bearer %s" \
  -H "Content-Type: application/json" \
  -d '{"name":"新Agent名称","cli_tool":"claude"}'
`+"```"+`

## agent_update
更新 Agent 配置（名称、系统提示词、工具配置等）。参数: agent_id (必填), name/system_prompt/tools_config (可选)。
`+"```bash"+`
curl -s -X PUT %s/api/agents/{agent_id} \
  -H "Authorization: Bearer %s" \
  -H "Content-Type: application/json" \
  -d '{"name":"新名称","system_prompt":"新提示词"}'
`+"```"+`

## agent_restart
重启指定 Agent。参数: agent_id (string)。
`+"```bash"+`
curl -s -X POST %s/api/agents/{agent_id}/restart -H "Authorization: Bearer %s"
`+"```"+`

## agent_stop
停止指定 Agent。参数: agent_id (string)。
`+"```bash"+`
curl -s -X POST %s/api/agents/{agent_id}/stop -H "Authorization: Bearer %s"
`+"```"+`

## agent_delete
删除指定 Agent（不可恢复）。参数: agent_id (string)。
`+"```bash"+`
curl -s -X DELETE %s/api/agents/{agent_id} -H "Authorization: Bearer %s"
`+"```"+`

## machine_list
获取当前用户的所有电脑列表。
`+"```bash"+`
curl -s %s/api/daemon/machines -H "Authorization: Bearer %s"
`+"```"+`

## machine_create
创建一台新电脑（返回 API Key 和连接命令）。参数: name (string)。
`+"```bash"+`
curl -s -X POST %s/api/daemon/machines \
  -H "Authorization: Bearer %s" \
  -H "Content-Type: application/json" \
  -d '{"name":"电脑名称"}'
`+"```"+`

## machine_connect
获取电脑的连接命令。参数: machine_id (string)。
`+"```bash"+`
curl -s %s/api/daemon/machines/{machine_id}/connect -H "Authorization: Bearer %s"
`+"```"+`
`, serverURL, token, serverURL, token, serverURL, token, serverURL, token, serverURL, token, serverURL, token, serverURL, token, serverURL, token, serverURL, token)
}

// GenerateKBReadTool 生成知识库文件读取工具的 markdown 定义。
// 当消息引用了知识库且其中包含非文本或大文件时自动注入。
// 使用与 management tools 相同的 scope=agent_management JWT 进行鉴权。
func GenerateKBReadTool(serverURL, token string) string {
	return fmt.Sprintf(`# 知识库文件读取工具

用户消息引用了知识库，部分文件无法直接内联到上下文中（非文本格式或文件过大）。你可以使用以下工具按需读取。

## read_knowledge_file
读取知识库中指定文件的内容。返回文件的原始内容（文本文件返回文本，二进制文件返回 Base64 编码）。
参数:
- kb_id (string, 必填): 知识库 ID
- file_id (string, 必填): 文件 ID

以上方引用的知识库为例，文件 ID 在上面文件列表中（需要从 API 获取）。

### 获取知识库文件列表
`+"```bash"+`
curl -s %s/api/knowledge-bases/{kb_id}/files -H "Authorization: Bearer %s"
`+"```"+`

### 读取文件内容（文本文件）
`+"```bash"+`
curl -s %s/api/knowledge-bases/{kb_id}/files/{file_id}/text -H "Authorization: Bearer %s"
`+"```"+`

注意：token 已嵌入上方命令中，直接复制替换 {kb_id} 和 {file_id} 即可执行。
`, serverURL, token, serverURL, token)
}
