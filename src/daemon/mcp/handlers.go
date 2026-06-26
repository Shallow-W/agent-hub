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

// APIClient calls the AgentHub backend REST API.
type APIClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
	Toolsets   *ToolsetStore
}

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

	var wrapper struct {
		Code int         `json:"code"`
		Data interface{} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &wrapper); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return wrapper.Data, nil
}

// AllowedToolsForAgent resolves the MCP tool allowlist for one Agent.
func (c *APIClient) AllowedToolsForAgent(agentID string) map[string]bool {
	if agentID == "" {
		return map[string]bool{}
	}
	data, err := c.doGet("/mcp/agents", nil)
	if err != nil {
		return map[string]bool{}
	}
	agents, ok := data.([]interface{})
	if !ok {
		return map[string]bool{}
	}
	for _, item := range agents {
		agent, ok := item.(map[string]interface{})
		if !ok || agent["id"] != agentID {
			continue
		}
		raw, _ := agent["tools_config"].(string)
		ts := c.Toolsets
		if ts == nil {
			ts = NewToolsetStore()
		}
		return ts.AllowedToolsFromConfig(raw)
	}
	return map[string]bool{}
}

// ---------------------------------------------------------------------------
// BuildRegistry — single entry point that wires all tool categories.
// Adding a new category = one RegisterXxx function + one line here.
// ---------------------------------------------------------------------------

func BuildRegistry(api *APIClient, agentID string) *Registry {
	if api.Toolsets == nil {
		api.Toolsets = NewToolsetStore()
	}
	r := NewRegistry()
	ts := api.Toolsets

	RegisterConversationTools(r, api)
	RegisterTaskTools(r, api)
	RegisterAgentTools(r, api, agentID)
	RegisterMachineTools(r, api)
	RegisterGroupTools(r, api)
	RegisterAgentManagementTools(r, api)
	RegisterKnowledgeTools(r, api)
	RegisterAgentCreationTools(r, api, ts)
	RegisterSkillTools(r, api, agentID)

	return r
}

// ---------------------------------------------------------------------------
// Complex handler factories (tools that need multi-step logic)
// ---------------------------------------------------------------------------

type platformSkill struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Trigger     string `json:"trigger,omitempty"`
	Detail      string `json:"detail,omitempty"`
}

func makeGetAgentSkillHandler(api *APIClient, agentID string) ToolHandlerFunc {
	return func(_ string, args map[string]interface{}) (interface{}, error) {
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

func makeUpdateAgentPromptHandler(api *APIClient) ToolHandlerFunc {
	return func(_ string, args map[string]interface{}) (interface{}, error) {
		agentID, _ := args["agent_id"].(string)
		systemPrompt, _ := args["system_prompt"].(string)
		if agentID == "" {
			return nil, fmt.Errorf("agent_id is required")
		}
		if systemPrompt == "" {
			return nil, fmt.Errorf("system_prompt is required")
		}
		data, err := api.doGet("/mcp/agents/"+agentID, nil)
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
			"system_prompt":           systemPrompt,
			"tools_config":            agent["tools_config"],
			"capabilities_json":       agent["capabilities_json"],
			"enable_management_tools": agent["enable_management_tools"],
		}
		return api.doPut("/mcp/agents/"+agentID, body)
	}
}

func makeCreateAgentHandler(api *APIClient, ts *ToolsetStore) ToolHandlerFunc {
	return func(_ string, args map[string]interface{}) (interface{}, error) {
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
		if tpl, ok := ts.Lookup(toolset); ok {
			allowedTools = tpl
		} else {
			toolset = "none"
			allowedTools = []string{}
		}

		body := map[string]interface{}{
			"name":          name,
			"cli_tool":      cliTool,
			"system_prompt": systemPrompt,
			"tools_config":  ts.ToolsConfigJSON(toolset, allowedTools),
		}
		if tags, ok := args["tags"].(string); ok && tags != "" {
			body["tags"] = tags
		}
		return api.doPost("/mcp/agents", body)
	}
}

func makeUpdateAgentHandler(api *APIClient, ts *ToolsetStore) ToolHandlerFunc {
	return func(_ string, args map[string]interface{}) (interface{}, error) {
		agentID, _ := args["agent_id"].(string)
		if agentID == "" {
			return nil, fmt.Errorf("agent_id is required")
		}

		data, err := api.doGet("/mcp/agents/"+agentID, nil)
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

		optionalString(args, "name", body)
		optionalString(args, "system_prompt", body)
		optionalString(args, "tags", body)

		if toolset, ok := args["toolset"].(string); ok && toolset != "" {
			if tpl, found := ts.Lookup(toolset); found {
				body["tools_config"] = ts.ToolsConfigJSON(toolset, tpl)
			}
		}

		if rawTools, ok := args["allowed_tools"].([]interface{}); ok && len(rawTools) > 0 {
			tools := make([]string, 0, len(rawTools))
			for _, t := range rawTools {
				if s, ok := t.(string); ok && s != "" {
					tools = append(tools, s)
				}
			}
			if len(tools) > 0 {
				body["tools_config"] = ts.ToolsConfigJSON("", tools)
			}
		}

		return api.doPut("/mcp/agents/"+agentID, body)
	}
}

func makeListToolsetsHandler(ts *ToolsetStore) ToolHandlerFunc {
	return func(_ string, _ map[string]interface{}) (interface{}, error) {
		return ts.List(), nil
	}
}

func optionalString(args map[string]interface{}, key string, body map[string]interface{}) {
	if v, ok := args[key].(string); ok && v != "" {
		body[key] = v
	}
}
