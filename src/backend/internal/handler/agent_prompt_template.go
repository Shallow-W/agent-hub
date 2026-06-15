package handler

import (
	"errors"
	"net/http"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

type AgentPromptTemplateHandler struct {
	svc *service.AgentPromptTemplateService
}

func NewAgentPromptTemplateHandler(svc *service.AgentPromptTemplateService) *AgentPromptTemplateHandler {
	return &AgentPromptTemplateHandler{svc: svc}
}

type AgentPromptTemplateRequest struct {
	Name         string `json:"name" binding:"required,max=100"`
	Category     string `json:"category"`
	Description  string `json:"description"`
	SystemPrompt string `json:"system_prompt"`
}

func (h *AgentPromptTemplateHandler) List(c *gin.Context) {
	userID := middleware.GetUserID(c)
	list, err := h.svc.List(c.Request.Context(), userID)
	if err != nil {
		middleware.HandleServiceError(c, err, "查询 Agent Prompt 模板失败")
		return
	}
	middleware.SuccessResponse(c, list)
}

func (h *AgentPromptTemplateHandler) Create(c *gin.Context) {
	var req AgentPromptTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40090, "参数错误: "+err.Error())
		return
	}
	userID := middleware.GetUserID(c)
	tpl, err := h.svc.Create(c.Request.Context(), userID, req.Name, req.Category, req.Description, req.SystemPrompt)
	if err != nil {
		handleAgentPromptTemplateError(c, err, "创建 Agent Prompt 模板失败")
		return
	}
	middleware.CreatedResponse(c, tpl)
}

func (h *AgentPromptTemplateHandler) ImportDefaults(c *gin.Context) {
	userID := middleware.GetUserID(c)
	templates, err := h.svc.ImportDefaults(c.Request.Context(), userID)
	if err != nil {
		handleAgentPromptTemplateError(c, err, "导入默认 Agent Prompt 模板失败")
		return
	}
	middleware.SuccessResponse(c, templates)
}

func (h *AgentPromptTemplateHandler) Update(c *gin.Context) {
	var req AgentPromptTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40091, "参数错误: "+err.Error())
		return
	}
	userID := middleware.GetUserID(c)
	tpl, err := h.svc.Update(c.Request.Context(), c.Param("id"), userID, req.Name, req.Category, req.Description, req.SystemPrompt)
	if err != nil {
		handleAgentPromptTemplateError(c, err, "更新 Agent Prompt 模板失败")
		return
	}
	middleware.SuccessResponse(c, tpl)
}

func (h *AgentPromptTemplateHandler) Delete(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if err := h.svc.Delete(c.Request.Context(), c.Param("id"), userID); err != nil {
		handleAgentPromptTemplateError(c, err, "删除 Agent Prompt 模板失败")
		return
	}
	middleware.SuccessResponse(c, nil)
}

func handleAgentPromptTemplateError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, service.ErrAgentPromptTemplateInvalid):
		middleware.ErrorResponse(c, http.StatusBadRequest, 40092, err.Error())
	case errors.Is(err, service.ErrAgentPromptTemplateDuplicate):
		middleware.ErrorResponse(c, http.StatusConflict, 40990, err.Error())
	case errors.Is(err, service.ErrAgentPromptTemplateNotFound):
		middleware.ErrorResponse(c, http.StatusNotFound, 40490, err.Error())
	default:
		middleware.HandleServiceError(c, err, fallback)
	}
}
