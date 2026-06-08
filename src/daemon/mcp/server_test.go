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

func TestNoAgentToolSet_IsEmpty(t *testing.T) {
	allowed := noAgentToolSet()
	if len(allowed) != 0 {
		t.Fatalf("expected no tools without agent identity, got %#v", allowed)
	}
}
