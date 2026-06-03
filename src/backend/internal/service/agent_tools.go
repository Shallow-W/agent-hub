package service

import "fmt"

// GenerateManagementTools generates markdown tool definitions for agent management.
// serverURL is the base URL of the AgentHub server (e.g., "http://127.0.0.1:8080").
// token is a scoped JWT token for the agent to authenticate.
func GenerateManagementTools(serverURL, token string) string {
	return fmt.Sprintf(`# AgentHub 平台管理工具

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
