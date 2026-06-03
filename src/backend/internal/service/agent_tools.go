package service

import "fmt"

// GenerateManagementTools generates markdown tool definitions for agent management.
// serverURL is the base URL of the AgentHub server (e.g., "http://127.0.0.1:8080").
// token is a scoped JWT token for the agent to authenticate.
func GenerateManagementTools(serverURL, token string) string {
	return fmt.Sprintf(`# AgentHub 管理工具

以下工具允许你管理 AgentHub 平台上的 Agent 和电脑资源。使用前请确保 token 有效。
所有 API 调用需要 Authorization header。

## agent_list
- 描述：获取当前用户的所有 Agent 列表
- 参数：无
- 调用方式：
  `+"`"+`curl -s %s/api/agents -H "Authorization: Bearer %s"`+"`"+`

## agent_create
- 描述：创建一个新的 Agent
- 参数：name (string), cli_tool (string, e.g. "claude"/"codex"), system_prompt (string, 可选)
- 调用方式：
  `+"`"+`curl -s -X POST %s/api/agents -H "Authorization: Bearer %s" -H "Content-Type: application/json" -d '{"name":"xxx","cli_tool":"claude"}'`+"`"+`

## agent_update
- 描述：更新 Agent 配置（系统提示词、工具配置等）
- 参数：agent_id (string), system_prompt (string, 可选), tools_config (string, 可选), name (string, 可选)
- 调用方式：
  `+"`"+`curl -s -X PUT %s/api/agents/{agent_id} -H "Authorization: Bearer %s" -H "Content-Type: application/json" -d '{"name":"xxx","system_prompt":"xxx"}'`+"`"+`

## agent_restart
- 描述：重启指定 Agent
- 参数：agent_id (string)
- 调用方式：
  `+"`"+`curl -s -X POST %s/api/agents/{agent_id}/restart -H "Authorization: Bearer %s"`+"`"+`

## agent_stop
- 描述：停止指定 Agent
- 参数：agent_id (string)
- 调用方式：
  `+"`"+`curl -s -X POST %s/api/agents/{agent_id}/stop -H "Authorization: Bearer %s"`+"`"+`

## agent_delete
- 描述：删除指定 Agent（不可恢复）
- 参数：agent_id (string)
- 调用方式：
  `+"`"+`curl -s -X DELETE %s/api/agents/{agent_id} -H "Authorization: Bearer %s"`+"`"+`

## machine_list
- 描述：获取当前用户的所有电脑列表
- 参数：无
- 调用方式：
  `+"`"+`curl -s %s/api/daemon/machines -H "Authorization: Bearer %s"`+"`"+`

## machine_create
- 描述：创建一台新的电脑（返回 API Key 和连接命令）
- 参数：name (string)
- 调用方式：
  `+"`"+`curl -s -X POST %s/api/daemon/machines -H "Authorization: Bearer %s" -H "Content-Type: application/json" -d '{"name":"xxx"}'`+"`"+`

## machine_connect
- 描述：获取电脑的连接命令
- 参数：machine_id (string)
- 调用方式：
  `+"`"+`curl -s %s/api/daemon/machines/{machine_id}/connect -H "Authorization: Bearer %s"`+"`"+`
`, serverURL, token, serverURL, token, serverURL, token, serverURL, token, serverURL, token, serverURL, token, serverURL, token, serverURL, token, serverURL, token)
}
