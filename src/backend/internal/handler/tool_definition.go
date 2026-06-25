package handler

import (
	"encoding/json"
	"net/http"
	"regexp"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// paramPlaceholderRe matches `{name}` style path placeholders used internally
// by RouteInfo.Path. Output to clients (gin-compatible) requires `:name`.
var paramPlaceholderRe = regexp.MustCompile(`\{(\w+)\}`)

// ginizePath converts internal `{param}` path placeholders to gin-compatible
// `:param` form for emission to daemon/clients.
func ginizePath(p string) string {
	return paramPlaceholderRe.ReplaceAllString(p, ":$1")
}

type ToolDefinitionHandler struct {
	svc          *service.ToolDefinitionService
	toolRegistry *service.ToolRegistry
}

func NewToolDefinitionHandler(svc *service.ToolDefinitionService) *ToolDefinitionHandler {
	return &ToolDefinitionHandler{svc: svc}
}

// SetToolRegistry wires the ToolRegistry for the daemon tool-registry endpoint.
func (h *ToolDefinitionHandler) SetToolRegistry(tr *service.ToolRegistry) {
	h.toolRegistry = tr
}

func (h *ToolDefinitionHandler) ListDefinitions(c *gin.Context) {
	if h.toolRegistry != nil {
		middleware.SuccessResponse(c, h.definitionsFromRegistry())
		return
	}
	list, err := h.svc.ListDefinitions(c.Request.Context())
	if err != nil {
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50090, "查询工具定义失败")
		return
	}
	middleware.SuccessResponse(c, list)
}

func (h *ToolDefinitionHandler) ListBuiltinTemplates(c *gin.Context) {
	list, err := h.svc.ListBuiltinTemplates(c.Request.Context())
	if err != nil {
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50091, "查询内置模板失败")
		return
	}
	if h.toolRegistry != nil {
		list = h.normalizeBuiltinTemplates(list)
	}
	middleware.SuccessResponse(c, list)
}

func (h *ToolDefinitionHandler) BuiltinSkillTemplates(c *gin.Context) {
	list, err := h.svc.ListBuiltinSkillTemplates(c.Request.Context())
	if err != nil {
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50094, "查询内置技能模板失败")
		return
	}
	middleware.SuccessResponse(c, list)
}

var managementToolNames = map[string]bool{
	"create_agent": true,
	"update_agent": true,
	"delete_agent": true,
}

var legacyToolAliases = map[string]string{
	"list_group_agents": "list_conversation_agents",
}

func (h *ToolDefinitionHandler) definitionsFromRegistry() []model.ToolDefinition {
	specs := h.toolRegistry.List()
	out := make([]model.ToolDefinition, 0, len(specs))
	for _, spec := range specs {
		out = append(out, model.ToolDefinition{
			Name:         spec.Name(),
			Label:        spec.Label(),
			Category:     spec.Category(),
			Description:  spec.Description(),
			IsManagement: managementToolNames[spec.Name()],
		})
	}
	return out
}

func (h *ToolDefinitionHandler) normalizeBuiltinTemplates(list []model.BuiltinToolsetTemplate) []model.BuiltinToolsetTemplate {
	allowed := h.registryToolNames()
	out := make([]model.BuiltinToolsetTemplate, len(list))
	for i, tpl := range list {
		out[i] = tpl
		var names []string
		if err := json.Unmarshal(tpl.ToolNames, &names); err != nil {
			out[i].ToolNames = json.RawMessage("[]")
			continue
		}
		out[i].ToolNames = marshalToolNames(normalizeToolNamesForRegistry(names, allowed))
	}
	return out
}

func (h *ToolDefinitionHandler) registryToolNames() map[string]bool {
	allowed := map[string]bool{}
	for _, spec := range h.toolRegistry.List() {
		allowed[spec.Name()] = true
	}
	return allowed
}

func normalizeToolNamesForRegistry(names []string, allowed map[string]bool) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(names))
	for _, name := range names {
		if alias, ok := legacyToolAliases[name]; ok {
			name = alias
		}
		if name == "" || !allowed[name] || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

func marshalToolNames(names []string) json.RawMessage {
	data, err := json.Marshal(names)
	if err != nil {
		return json.RawMessage("[]")
	}
	return data
}

// ToolRegistryItem is the DTO for an individual tool in the tool-registry response.
type ToolRegistryItem struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Category    string                 `json:"category"`
	InputSchema map[string]interface{} `json:"inputSchema"`
	Route       *RouteInfoDTO          `json:"route"`
}

// RouteInfoDTO is the DTO for route metadata in the tool-registry response.
type RouteInfoDTO struct {
	Method   string   `json:"method"`
	Path     string   `json:"path"`
	Required []string `json:"required"`
	Optional []string `json:"optional"`
}

// ToolRegistry returns the full tool catalog with route metadata.
// This is called by daemons at startup to build their tool registry.
func (h *ToolDefinitionHandler) ToolRegistry(c *gin.Context) {
	if h.toolRegistry == nil {
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50092, "工具注册表未初始化")
		return
	}
	specs := h.toolRegistry.List()
	out := make([]ToolRegistryItem, 0, len(specs))
	for _, spec := range specs {
		item := ToolRegistryItem{
			Name:        spec.Name(),
			Description: spec.Description(),
			Category:    spec.Category(),
			InputSchema: spec.InputSchema(),
		}
		if ri := spec.RouteInfo(); ri != nil {
			item.Route = &RouteInfoDTO{
				Method:   ri.Method,
				Path:     ginizePath(ri.Path),
				Required: ri.Required,
				Optional: ri.Optional,
			}
		}
		out = append(out, item)
	}
	middleware.SuccessResponse(c, out)
}
