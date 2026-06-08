package mcp

import (
	"encoding/json"
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

func TestNoAgentToolSet_IsEmpty(t *testing.T) {
	allowed := noAgentToolSet()
	if len(allowed) != 0 {
		t.Fatalf("expected no tools without agent identity, got %#v", allowed)
	}
}
