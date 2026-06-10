package handler

import (
	"errors"
	"net/http"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

type UserTemplateHandler struct {
	svc *service.UserTemplateService
}

func NewUserTemplateHandler(svc *service.UserTemplateService) *UserTemplateHandler {
	return &UserTemplateHandler{svc: svc}
}

type UserTemplateRequest struct {
	Type    string      `json:"type" binding:"required,oneof=tools skills"`
	Name    string      `json:"name" binding:"required,max=100"`
	Content interface{} `json:"content" binding:"required"`
}

func (h *UserTemplateHandler) List(c *gin.Context) {
	userID := middleware.GetUserID(c)
	tplType := c.Query("type")
	if tplType == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40080, "缺少 type 参数")
		return
	}
	list, err := h.svc.List(c.Request.Context(), userID, tplType)
	if err != nil {
		handleUserTemplateError(c, err, "查询模板失败")
		return
	}
	middleware.SuccessResponse(c, list)
}

func (h *UserTemplateHandler) Create(c *gin.Context) {
	var req UserTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40081, "参数错误: "+err.Error())
		return
	}
	userID := middleware.GetUserID(c)
	tpl, err := h.svc.Create(c.Request.Context(), userID, req.Type, req.Name, req.Content)
	if err != nil {
		handleUserTemplateError(c, err, "创建模板失败")
		return
	}
	middleware.CreatedResponse(c, tpl)
}

func (h *UserTemplateHandler) Update(c *gin.Context) {
	var req UserTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40082, "参数错误: "+err.Error())
		return
	}
	userID := middleware.GetUserID(c)
	tpl, err := h.svc.Update(c.Request.Context(), c.Param("id"), userID, req.Name, req.Content)
	if err != nil {
		handleUserTemplateError(c, err, "更新模板失败")
		return
	}
	middleware.SuccessResponse(c, tpl)
}

func (h *UserTemplateHandler) Delete(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if err := h.svc.Delete(c.Request.Context(), c.Param("id"), userID); err != nil {
		handleUserTemplateError(c, err, "删除模板失败")
		return
	}
	middleware.SuccessResponse(c, nil)
}

func handleUserTemplateError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, service.ErrUserTemplateInvalid):
		middleware.ErrorResponse(c, http.StatusBadRequest, 40083, err.Error())
	case errors.Is(err, service.ErrUserTemplateDuplicate):
		middleware.ErrorResponse(c, http.StatusConflict, 40980, err.Error())
	case errors.Is(err, service.ErrUserTemplateNotFound):
		middleware.ErrorResponse(c, http.StatusNotFound, 40480, err.Error())
	default:
		middleware.HandleServiceError(c, err, fallback)
	}
}
