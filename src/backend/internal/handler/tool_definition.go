package handler

import (
	"net/http"
	"regexp"

	"github.com/agent-hub/backend/internal/middleware"
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
