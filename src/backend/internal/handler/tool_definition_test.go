package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/port"
	"github.com/agent-hub/backend/internal/service"
	"github.com/agent-hub/backend/internal/service/tool_specs"
	"github.com/gin-gonic/gin"
)

type fakeToolDefinitionRepo struct {
	definitions []model.ToolDefinition
	templates   []model.BuiltinToolsetTemplate
}

func (r *fakeToolDefinitionRepo) List(_ context.Context) ([]model.ToolDefinition, error) {
	return r.definitions, nil
}

func (r *fakeToolDefinitionRepo) ListBuiltinTemplates(_ context.Context) ([]model.BuiltinToolsetTemplate, error) {
	return r.templates, nil
}

func (r *fakeToolDefinitionRepo) ListBuiltinSkillTemplates(_ context.Context) ([]model.BuiltinSkillTemplate, error) {
	return nil, nil
}

func setupToolDefinitionHandlerTest(t *testing.T, repo *fakeToolDefinitionRepo) (*gin.Engine, *service.ToolRegistry) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	registry := service.NewToolRegistry(nil)
	for _, spec := range []port.MCPToolSpec{
		tool_specs.ListConversationAgents(),
		tool_specs.GetMessages(),
		tool_specs.CreateAgent(),
	} {
		if err := registry.Register(context.Background(), spec); err != nil {
			t.Fatalf("register spec %s: %v", spec.Name(), err)
		}
	}

	h := NewToolDefinitionHandler(service.NewToolDefinitionService(repo))
	h.SetToolRegistry(registry)

	r := gin.New()
	r.GET("/definitions", h.ListDefinitions)
	r.GET("/templates", h.ListBuiltinTemplates)
	return r, registry
}

func TestToolDefinitionHandlerDefinitionsUseRegistry(t *testing.T) {
	r, _ := setupToolDefinitionHandlerTest(t, &fakeToolDefinitionRepo{
		definitions: []model.ToolDefinition{
			{Name: "list_group_agents"},
			{Name: "deploy_artifact"},
		},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/definitions", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Code int                    `json:"code"`
		Data []model.ToolDefinition `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	names := map[string]bool{}
	for _, item := range body.Data {
		names[item.Name] = true
	}
	if !names["list_conversation_agents"] {
		t.Fatalf("expected registry tool list_conversation_agents, got %+v", body.Data)
	}
	if names["list_group_agents"] || names["deploy_artifact"] {
		t.Fatalf("stale DB-only tools leaked into definitions: %+v", body.Data)
	}
}

func TestToolDefinitionHandlerTemplatesNormalizeAgainstRegistry(t *testing.T) {
	r, _ := setupToolDefinitionHandlerTest(t, &fakeToolDefinitionRepo{
		templates: []model.BuiltinToolsetTemplate{
			{
				Name:      "basic",
				ToolNames: json.RawMessage(`["list_group_agents","unknown","get_messages","list_conversation_agents"]`),
			},
		},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/templates", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Code int `json:"code"`
		Data []struct {
			Name      string   `json:"name"`
			ToolNames []string `json:"tool_names"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data) != 1 {
		t.Fatalf("expected one template, got %+v", body.Data)
	}
	got := body.Data[0].ToolNames
	want := []string{"list_conversation_agents", "get_messages"}
	if len(got) != len(want) {
		t.Fatalf("tool_names=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tool_names=%v, want %v", got, want)
		}
	}
}
