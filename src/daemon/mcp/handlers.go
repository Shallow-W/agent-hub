package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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
func HandleAllTools(api *APIClient, agentID string) ToolHandlerFunc {
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
		case "create_group":
			return handleCreateGroup(api, args)
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
		case "get_agent_skill":
			return handleGetAgentSkill(api, agentID, args)
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
		// Agent 管理
		case "get_agent_detail":
			return handleGetAgentDetail(api, args)
		case "update_agent_prompt":
			return handleUpdateAgentPrompt(api, args)
		case "start_agent":
			return handleStartAgent(api, args)
		case "stop_agent":
			return handleStopAgent(api, args)
		// 知识库
		case "list_knowledge_bases":
			return api.doGet("/mcp/knowledge-bases", nil)
		case "list_knowledge_files":
			return handleListKnowledgeFiles(api, args)
		case "search_knowledge":
			return handleSearchKnowledge(api, args)
		case "read_knowledge_file":
			return handleReadKnowledgeFile(api, args)
		// Agent 自建
		case "create_agent":
			return handleCreateAgent(api, args)
		case "update_agent":
			return handleUpdateAgent(api, args)
		case "delete_agent":
			return handleDeleteAgent(api, args)
		case "list_toolsets":
			return handleListToolsets()
		default:
			return nil, fmt.Errorf("unknown tool: %s", toolName)
		}
	}
}

type platformSkill struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Trigger     string `json:"trigger,omitempty"`
	Detail      string `json:"detail,omitempty"`
}

func handleGetAgentSkill(api *APIClient, agentID string, args map[string]interface{}) (interface{}, error) {
	if strings.TrimSpace(agentID) == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	name, _ := args["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	skill, ok, err := api.AgentSkill(agentID, name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("skill not found for current agent: %s", name)
	}
	return skill, nil
}

func (c *APIClient) AgentSkill(agentID, skillName string) (platformSkill, bool, error) {
	data, err := c.doGet("/mcp/agents", nil)
	if err != nil {
		return platformSkill{}, false, err
	}
	agents, ok := data.([]interface{})
	if !ok {
		return platformSkill{}, false, nil
	}
	for _, item := range agents {
		agent, ok := item.(map[string]interface{})
		if !ok || agent["id"] != agentID {
			continue
		}
		raw, _ := agent["custom_skills"].(string)
		if strings.TrimSpace(raw) == "" {
			return platformSkill{}, false, nil
		}
		var skills []platformSkill
		if err := json.Unmarshal([]byte(raw), &skills); err != nil {
			return platformSkill{}, false, fmt.Errorf("parse custom skills: %w", err)
		}
		for _, skill := range skills {
			if strings.EqualFold(strings.TrimSpace(skill.Name), strings.TrimSpace(skillName)) {
				return skill, true, nil
			}
		}
		return platformSkill{}, false, nil
	}
	return platformSkill{}, false, nil
}

func handleCreateGroup(api *APIClient, args map[string]interface{}) (interface{}, error) {
	name, _ := args["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	body := map[string]interface{}{"name": name}
	if ids, ok := args["member_ids"].([]interface{}); ok {
		members := make([]string, 0, len(ids))
		for _, id := range ids {
			if value, ok := id.(string); ok && value != "" {
				members = append(members, value)
			}
		}
		body["member_ids"] = members
	}
	return api.doPost("/mcp/groups", body)
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

func handleGetAgentDetail(api *APIClient, args map[string]interface{}) (interface{}, error) {
	agentID, _ := args["agent_id"].(string)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	return api.doGet("/mcp/agents/"+agentID, nil)
}

func handleUpdateAgentPrompt(api *APIClient, args map[string]interface{}) (interface{}, error) {
	agentID, _ := args["agent_id"].(string)
	systemPrompt, _ := args["system_prompt"].(string)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	if systemPrompt == "" {
		return nil, fmt.Errorf("system_prompt is required")
	}
	// 先获取当前完整信息
	data, err := api.doGet("/mcp/agents/"+agentID, nil)
	if err != nil {
		return nil, fmt.Errorf("get agent detail: %w", err)
	}
	agent, ok := data.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected agent response format")
	}
	// 构造更新请求，只改 system_prompt，其他字段原样传回
	body := map[string]interface{}{
		"name":                    agent["name"],
		"cli_tool":                agent["cli_tool"],
		"system_prompt":           systemPrompt,
		"tools_config":            agent["tools_config"],
		"capabilities_json":       agent["capabilities_json"],
		"enable_management_tools": agent["enable_management_tools"],
	}
	return api.doPut("/mcp/agents/"+agentID, body)
}

func handleStartAgent(api *APIClient, args map[string]interface{}) (interface{}, error) {
	agentID, _ := args["agent_id"].(string)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	return api.doPost("/mcp/agents/"+agentID+"/start", nil)
}

func handleStopAgent(api *APIClient, args map[string]interface{}) (interface{}, error) {
	agentID, _ := args["agent_id"].(string)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	return api.doPost("/mcp/agents/"+agentID+"/stop", nil)
}

func handleListKnowledgeFiles(api *APIClient, args map[string]interface{}) (interface{}, error) {
	kbID, _ := args["knowledge_base_id"].(string)
	if kbID == "" {
		return nil, fmt.Errorf("knowledge_base_id is required")
	}
	return api.doGet("/mcp/knowledge-bases/"+kbID+"/files", nil)
}

func handleSearchKnowledge(api *APIClient, args map[string]interface{}) (interface{}, error) {
	kbID, _ := args["knowledge_base_id"].(string)
	keyword, _ := args["keyword"].(string)
	if kbID == "" {
		return nil, fmt.Errorf("knowledge_base_id is required")
	}
	if keyword == "" {
		return nil, fmt.Errorf("keyword is required")
	}
	query := map[string]string{"keyword": keyword}
	switch v := args["limit"].(type) {
	case float64:
		if v > 0 {
			query["limit"] = fmt.Sprintf("%d", int(v))
		}
	case int:
		if v > 0 {
			query["limit"] = fmt.Sprintf("%d", v)
		}
	}
	return api.doGet("/mcp/knowledge-bases/"+kbID+"/search", query)
}

func handleReadKnowledgeFile(api *APIClient, args map[string]interface{}) (interface{}, error) {
	kbID, _ := args["knowledge_base_id"].(string)
	fileID, _ := args["file_id"].(string)
	if kbID == "" {
		return nil, fmt.Errorf("knowledge_base_id is required")
	}
	if fileID == "" {
		return nil, fmt.Errorf("file_id is required")
	}
	return api.doGet("/mcp/knowledge-bases/"+kbID+"/files/"+fileID+"/text", nil)
}

// agentCreationToolsets mirrors the backend platformToolsets for computing tools_config.
var agentCreationToolsets = map[string][]string{
	"none":          {},
	"basic":         {"list_group_agents", "get_messages", "get_agent_skill"},
	"tasks":         {"list_group_agents", "get_messages", "get_agent_skill", "list_tasks", "create_task", "update_task", "move_task_status"},
	"orchestrator":  {"list_group_agents", "get_messages", "get_agent_skill", "list_tasks", "create_task", "update_task", "move_task_status", "list_conversation_agents", "list_conversations", "get_group_info", "list_group_members", "list_knowledge_bases", "list_knowledge_files", "search_knowledge", "read_knowledge_file", "create_agent", "update_agent", "delete_agent", "list_toolsets"},
	"agent_builder": {"list_agents", "list_group_agents", "get_agent_skill", "list_agent_candidates", "list_machines", "get_agent_detail", "create_agent", "update_agent", "delete_agent", "list_toolsets"},
	"agent_manager": {"list_agents", "get_agent_detail", "update_agent_prompt", "start_agent", "stop_agent", "get_agent_skill"},
	"knowledge":     {"list_knowledge_bases", "list_knowledge_files", "search_knowledge", "read_knowledge_file"},
}

func toolsConfigJSON(toolset string, allowedTools []string) string {
	cfg := map[string]interface{}{
		"toolset":       toolset,
		"allowed_tools": allowedTools,
	}
	data, _ := json.Marshal(cfg)
	return string(data)
}

func handleCreateAgent(api *APIClient, args map[string]interface{}) (interface{}, error) {
	name, _ := args["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	systemPrompt, _ := args["system_prompt"].(string)
	if systemPrompt == "" {
		return nil, fmt.Errorf("system_prompt is required")
	}
	cliTool, _ := args["cli_tool"].(string)
	if cliTool == "" {
		cliTool = "claude"
	}
	toolset, _ := args["toolset"].(string)
	if toolset == "" {
		toolset = "none"
	}

	var allowedTools []string
	if tpl, ok := agentCreationToolsets[toolset]; ok {
		allowedTools = tpl
	} else {
		toolset = "none"
		allowedTools = []string{}
	}

	body := map[string]interface{}{
		"name":          name,
		"cli_tool":      cliTool,
		"system_prompt": systemPrompt,
		"tools_config":  toolsConfigJSON(toolset, allowedTools),
	}
	if tags, ok := args["tags"].(string); ok && tags != "" {
		body["tags"] = tags
	}
	return api.doPost("/api/agents", body)
}

func handleUpdateAgent(api *APIClient, args map[string]interface{}) (interface{}, error) {
	agentID, _ := args["agent_id"].(string)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}

	// Fetch current agent to preserve unchanged fields
	data, err := api.doGet("/api/agents/"+agentID, nil)
	if err != nil {
		return nil, fmt.Errorf("get agent detail: %w", err)
	}
	agent, ok := data.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected agent response format")
	}

	body := map[string]interface{}{
		"name":                    agent["name"],
		"cli_tool":                agent["cli_tool"],
		"system_prompt":           agent["system_prompt"],
		"tools_config":            agent["tools_config"],
		"capabilities_json":       agent["capabilities_json"],
		"enable_management_tools": agent["enable_management_tools"],
	}

	// Override only provided fields
	optionalString(args, "name", body)
	optionalString(args, "system_prompt", body)
	optionalString(args, "tags", body)

	// Handle toolset change
	if toolset, ok := args["toolset"].(string); ok && toolset != "" {
		if tpl, found := agentCreationToolsets[toolset]; found {
			body["tools_config"] = toolsConfigJSON(toolset, tpl)
		}
	}

	// Handle allowed_tools override (custom toolset)
	if rawTools, ok := args["allowed_tools"].([]interface{}); ok && len(rawTools) > 0 {
		tools := make([]string, 0, len(rawTools))
		for _, t := range rawTools {
			if s, ok := t.(string); ok && s != "" {
				tools = append(tools, s)
			}
		}
		if len(tools) > 0 {
			body["tools_config"] = toolsConfigJSON("", tools)
		}
	}

	return api.doPut("/api/agents/"+agentID, body)
}

func handleDeleteAgent(api *APIClient, args map[string]interface{}) (interface{}, error) {
	agentID, _ := args["agent_id"].(string)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	return api.doDelete("/api/agents/" + agentID)
}

var toolsetDescriptions = []map[string]interface{}{
	{"name": "none", "label": "无工具", "description": "不分配任何平台工具"},
	{"name": "basic", "label": "基础群聊", "description": "包含群 Agent 列表、消息读取、Skill 查看等基础工具"},
	{"name": "tasks", "label": "任务协作", "description": "包含任务看板的完整增删改查能力"},
	{"name": "orchestrator", "label": "Orchestrator", "description": "编排器模板，包含会话、任务、群组管理和知识库搜索"},
	{"name": "agent_builder", "label": "Agent 创建", "description": "Agent 发现和详情查询工具"},
	{"name": "agent_manager", "label": "Agent 管理", "description": "Agent 详情、提示词更新、启停控制"},
	{"name": "knowledge", "label": "知识库", "description": "知识库列表、文件列表和关键词搜索"},
}

func handleListToolsets() (interface{}, error) {
	return toolsetDescriptions, nil
}
