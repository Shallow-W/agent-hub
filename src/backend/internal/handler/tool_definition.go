package handler

import (
	"net/http"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

type ToolDefinitionHandler struct {
	svc *service.ToolDefinitionService
}

func NewToolDefinitionHandler(svc *service.ToolDefinitionService) *ToolDefinitionHandler {
	return &ToolDefinitionHandler{svc: svc}
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
