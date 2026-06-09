package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServerFiltersToolsAndRejectsUnauthorizedCalls(t *testing.T) {
	tools := []Tool{
		{Name: "list_tasks", Description: "list", InputSchema: map[string]interface{}{"type": "object"}},
		{Name: "list_agents", Description: "agents", InputSchema: map[string]interface{}{"type": "object"}},
	}
	server := NewServer("test", "0", tools, func(toolName string, arguments map[string]interface{}) (interface{}, error) {
		return map[string]string{"tool": toolName}, nil
	}, nil).WithAllowedTools(toolSet([]string{"list_tasks"}))

	listReq := &jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/list",
	}
	listResp := server.handleRequest(listReq)
	result, ok := listResp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected tools/list result: %#v", listResp.Result)
	}
	listedTools, ok := result["tools"].([]map[string]interface{})
	if !ok {
		t.Fatalf("unexpected tools payload: %#v", result["tools"])
	}
	if len(listedTools) != 1 || listedTools[0]["name"] != "list_tasks" {
		t.Fatalf("unexpected listed tools: %#v", listedTools)
	}

	params := json.RawMessage(`{"name":"list_agents","arguments":{}}`)
	callReq := &jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`2`),
		Method:  "tools/call",
		Params:  params,
	}
	callResp := server.handleRequest(callReq)
	serialized, err := json.Marshal(callResp.Result)
	if err != nil {
		t.Fatalf("marshal call response: %v", err)
	}
	if !strings.Contains(string(serialized), "tool not authorized: list_agents") {
		t.Fatalf("expected unauthorized response, got %s", serialized)
	}
}

func TestAllowedToolsFromConfig_EmptyAllowedToolsMeansNoTools(t *testing.T) {
	allowed := allowedToolsFromConfig(`{"toolset":"","allowed_tools":[]}`)
	if len(allowed) != 0 {
		t.Fatalf("expected no allowed tools, got %#v", allowed)
	}
}

func TestAllowedToolsFromConfig_EmptyConfigMeansNoTools(t *testing.T) {
	allowed := allowedToolsFromConfig("")
	if len(allowed) != 0 {
		t.Fatalf("expected no allowed tools for empty config, got %#v", allowed)
	}
}

func TestAllowedToolsFromConfig_LegacyTextMeansNoTools(t *testing.T) {
	allowed := allowedToolsFromConfig("## legacy docs")
	if len(allowed) != 0 {
		t.Fatalf("expected no allowed tools for legacy text, got %#v", allowed)
	}
}

func TestAllowedToolsForAgent_UsesBackendAgentConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp/agents" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer token-1" {
			t.Fatalf("unexpected authorization header: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"data":[{"id":"agent-1","tools_config":"{\"allowed_tools\":[\"list_tasks\"]}"},{"id":"agent-2","tools_config":"{\"allowed_tools\":[\"list_agents\"]}"}]}`))
	}))
	defer server.Close()

	client := NewAPIClient(server.URL, "token-1")
	allowed := client.AllowedToolsForAgent("agent-1")
	if !allowed["list_tasks"] {
		t.Fatalf("expected list_tasks to be allowed, got %#v", allowed)
	}
	if allowed["list_agents"] {
		t.Fatalf("expected list_agents to be denied for agent-1, got %#v", allowed)
	}
}

func TestHandleGetAgentSkillScopesToCurrentAgent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp/agents" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"data":[{"id":"agent-1","custom_skills":"[{\"name\":\"代码审查\",\"description\":\"找 bug\",\"trigger\":\"review\",\"detail\":\"逐项检查权限和测试\"}]"},{"id":"agent-2","custom_skills":"[{\"name\":\"代码审查\",\"detail\":\"不应返回\"}]"}]}`))
	}))
	defer server.Close()

	client := NewAPIClient(server.URL, "token-1")
	handler := HandleAllTools(client, "agent-1")
	got, err := handler("get_agent_skill", map[string]interface{}{"name": "代码审查"})
	if err != nil {
		t.Fatalf("get_agent_skill failed: %v", err)
	}
	skill, ok := got.(platformSkill)
	if !ok {
		t.Fatalf("unexpected skill type: %#v", got)
	}
	if skill.Detail != "逐项检查权限和测试" {
		t.Fatalf("expected current agent skill detail, got %#v", skill)
	}
}

func TestAllowedToolsFromConfig_TasksIncludesSkillLookup(t *testing.T) {
	allowed := allowedToolsFromConfig(`{"toolset":"tasks"}`)
	if !allowed["get_agent_skill"] {
		t.Fatalf("expected tasks toolset to include get_agent_skill, got %#v", allowed)
	}
}

func TestAllowedToolsFromConfig_KnowledgeToolsetsIncludeReadAndSearch(t *testing.T) {
	for _, toolset := range []string{"orchestrator", "knowledge"} {
		allowed := allowedToolsFromConfig(`{"toolset":"` + toolset + `"}`)
		for _, tool := range []string{"list_knowledge_bases", "list_knowledge_files", "search_knowledge", "read_knowledge_file"} {
			if !allowed[tool] {
				t.Fatalf("expected %s toolset to include %s, got %#v", toolset, tool, allowed)
			}
		}
	}
}

func TestKnowledgeToolHandlersUseBackendSearchAndTextEndpoints(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.String())
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/mcp/knowledge-bases/kb-1/search":
			if r.URL.Query().Get("keyword") != "Agent" {
				t.Fatalf("unexpected keyword query: %s", r.URL.RawQuery)
			}
			if r.URL.Query().Get("limit") != "5" {
				t.Fatalf("unexpected limit query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":[{"id":"file-1","filename":"guide.md"}]}`))
		case "/mcp/knowledge-bases/kb-1/files/file-1/text":
			_, _ = w.Write([]byte(`{"code":0,"data":{"file_id":"file-1","text":"Agent knowledge"}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.String())
		}
	}))
	defer server.Close()

	handler := HandleAllTools(NewAPIClient(server.URL, "token-1"), "agent-1")
	if _, err := handler("search_knowledge", map[string]interface{}{"knowledge_base_id": "kb-1", "keyword": "Agent", "limit": float64(5)}); err != nil {
		t.Fatalf("search_knowledge failed: %v", err)
	}
	if _, err := handler("read_knowledge_file", map[string]interface{}{"knowledge_base_id": "kb-1", "file_id": "file-1"}); err != nil {
		t.Fatalf("read_knowledge_file failed: %v", err)
	}
	if len(seen) != 2 {
		t.Fatalf("expected 2 backend calls, got %#v", seen)
	}
}

func TestNoAgentToolSet_IsEmpty(t *testing.T) {
	allowed := noAgentToolSet()
	if len(allowed) != 0 {
		t.Fatalf("expected no tools without agent identity, got %#v", allowed)
	}
}
