package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// APIClient 调用 AgentHub 后端 REST API
type APIClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewAPIClient 创建后端 API 客户端
func NewAPIClient(baseURL, token string) *APIClient {
	return &APIClient{
		baseURL:    baseURL,
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *APIClient) doGet(path string, query map[string]string) (interface{}, error) {
	u, _ := url.Parse(c.baseURL + path)
	if len(query) > 0 {
		q := u.Query()
		for k, v := range query {
			if v != "" {
				q.Set(k, v)
			}
		}
		u.RawQuery = q.Encode()
	}
	return c.doRequest("GET", u.String(), nil)
}

func (c *APIClient) doPost(path string, body interface{}) (interface{}, error) {
	return c.doRequest("POST", c.baseURL+path, body)
}

func (c *APIClient) doPut(path string, body interface{}) (interface{}, error) {
	return c.doRequest("PUT", c.baseURL+path, body)
}

func (c *APIClient) doDelete(path string) (interface{}, error) {
	return c.doRequest("DELETE", c.baseURL+path, nil)
}

func (c *APIClient) doRequest(method, reqURL string, body interface{}) (interface{}, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Message != "" {
			return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, errResp.Message)
		}
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	// 解析统一响应格式 {"code":200,"data":...}
	var wrapper struct {
		Code int         `json:"code"`
		Data interface{} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &wrapper); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return wrapper.Data, nil
}

// HandleAllTools 处理所有 tool 调用
func HandleAllTools(api *APIClient) ToolHandlerFunc {
	return func(toolName string, args map[string]interface{}) (interface{}, error) {
		switch toolName {
		// 会话
		case "list_conversations":
			return api.doGet("/mcp/conversations", nil)
		case "list_conversation_agents":
			id, _ := args["conversation_id"].(string)
			if id == "" {
				return nil, fmt.Errorf("conversation_id is required")
			}
			return api.doGet("/mcp/conversations/"+id+"/agents", nil)
		case "list_group_agents":
			id, _ := args["conversation_id"].(string)
			if id == "" {
				return nil, fmt.Errorf("conversation_id is required")
			}
			return api.doGet("/mcp/conversations/"+id+"/agents", nil)
		case "get_messages":
			return handleGetMessages(api, args)
		// 任务看板
		case "list_tasks":
			return handleListTasks(api, args)
		case "create_task":
			return handleCreateTask(api, args)
		case "update_task":
			return handleUpdateTask(api, args)
		case "move_task_status":
			return handleMoveTaskStatus(api, args)
		case "delete_task":
			return handleDeleteTask(api, args)
		// 智能体
		case "list_agents":
			return api.doGet("/mcp/agents", nil)
		case "list_agent_candidates":
			return api.doGet("/mcp/daemon/agent-candidates", nil)
		// 机器
		case "list_machines":
			return api.doGet("/mcp/daemon/machines", nil)
		// 群聊
		case "get_group_info":
			groupID, _ := args["group_id"].(string)
			if groupID == "" {
				return nil, fmt.Errorf("group_id is required")
			}
			return api.doGet("/mcp/groups/"+groupID, nil)
		case "list_group_members":
			groupID, _ := args["group_id"].(string)
			if groupID == "" {
				return nil, fmt.Errorf("group_id is required")
			}
			return api.doGet("/mcp/groups/"+groupID+"/members", nil)
		default:
			return nil, fmt.Errorf("unknown tool: %s", toolName)
		}
	}
}

func handleGetMessages(api *APIClient, args map[string]interface{}) (interface{}, error) {
	id, _ := args["conversation_id"].(string)
	if id == "" {
		return nil, fmt.Errorf("conversation_id is required")
	}
	query := map[string]string{}
	if v, ok := args["limit"].(float64); ok && v > 0 {
		query["limit"] = fmt.Sprintf("%.0f", v)
	}
	return api.doGet("/mcp/conversations/"+id+"/messages", query)
}

// AllowedToolsForAgent resolves the MCP tool allowlist for one Agent.
func (c *APIClient) AllowedToolsForAgent(agentID string) map[string]bool {
	if agentID == "" {
		return noAgentToolSet()
	}
	data, err := c.doGet("/mcp/agents", nil)
	if err != nil {
		return noAgentToolSet()
	}
	agents, ok := data.([]interface{})
	if !ok {
		return noAgentToolSet()
	}
	for _, item := range agents {
		agent, ok := item.(map[string]interface{})
		if !ok || agent["id"] != agentID {
			continue
		}
		raw, _ := agent["tools_config"].(string)
		return allowedToolsFromConfig(raw)
	}
	return noAgentToolSet()
}

func handleListTasks(api *APIClient, args map[string]interface{}) (interface{}, error) {
	query := map[string]string{}
	if v, ok := args["conversation_id"].(string); ok {
		query["conversation_id"] = v
	}
	if v, ok := args["status"].(string); ok {
		query["status"] = v
	}
	return api.doGet("/mcp/tasks", query)
}

func handleCreateTask(api *APIClient, args map[string]interface{}) (interface{}, error) {
	title, _ := args["title"].(string)
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}
	body := map[string]interface{}{
		"title": title,
	}
	optionalString(args, "description", body)
	optionalString(args, "status", body)
	optionalString(args, "priority", body)
	optionalString(args, "conversation_id", body)
	optionalString(args, "assignee_id", body)
	optionalString(args, "agent_id", body)
	return api.doPost("/mcp/tasks", body)
}

func handleUpdateTask(api *APIClient, args map[string]interface{}) (interface{}, error) {
	id, _ := args["id"].(string)
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	body := map[string]interface{}{}
	optionalString(args, "title", body)
	optionalString(args, "description", body)
	optionalString(args, "priority", body)
	optionalString(args, "assignee_id", body)
	optionalString(args, "agent_id", body)
	return api.doPut("/mcp/tasks/"+id, body)
}

func handleMoveTaskStatus(api *APIClient, args map[string]interface{}) (interface{}, error) {
	id, _ := args["id"].(string)
	status, _ := args["status"].(string)
	if id == "" || status == "" {
		return nil, fmt.Errorf("id and status are required")
	}
	return api.doPost("/mcp/tasks/"+id+"/status", map[string]interface{}{"status": status})
}

func handleDeleteTask(api *APIClient, args map[string]interface{}) (interface{}, error) {
	id, _ := args["id"].(string)
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	return api.doDelete("/mcp/tasks/" + id)
}

// optionalString 如果 args 中存在非空字符串 key，则写入 body
func optionalString(args map[string]interface{}, key string, body map[string]interface{}) {
	if v, ok := args[key].(string); ok && v != "" {
		body[key] = v
	}
}
