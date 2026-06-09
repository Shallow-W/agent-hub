package handler

import (
	"errors"
	"net/http"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

type PlatformSkillHandler struct {
	svc *service.PlatformSkillService
}

func NewPlatformSkillHandler(svc *service.PlatformSkillService) *PlatformSkillHandler {
	return &PlatformSkillHandler{svc: svc}
}

type PlatformSkillRequest struct {
	Name        string `json:"name" binding:"required,max=100"`
	Description string `json:"description"`
	Trigger     string `json:"trigger"`
	Detail      string `json:"detail"`
}

func (h *PlatformSkillHandler) List(c *gin.Context) {
	userID := middleware.GetUserID(c)
	list, err := h.svc.List(c.Request.Context(), userID)
	if err != nil {
		middleware.HandleServiceError(c, err, "查询平台 Skills 失败")
		return
	}
	middleware.SuccessResponse(c, list)
}

func (h *PlatformSkillHandler) Create(c *gin.Context) {
	var req PlatformSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40080, "参数错误: "+err.Error())
		return
	}
	userID := middleware.GetUserID(c)
	skill, err := h.svc.Create(c.Request.Context(), userID, req.Name, req.Description, req.Trigger, req.Detail)
	if err != nil {
		handlePlatformSkillError(c, err, "创建平台 Skill 失败")
		return
	}
	middleware.CreatedResponse(c, skill)
}

func (h *PlatformSkillHandler) Update(c *gin.Context) {
	var req PlatformSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40081, "参数错误: "+err.Error())
		return
	}
	userID := middleware.GetUserID(c)
	skill, err := h.svc.Update(c.Request.Context(), c.Param("id"), userID, req.Name, req.Description, req.Trigger, req.Detail)
	if err != nil {
		handlePlatformSkillError(c, err, "更新平台 Skill 失败")
		return
	}
	middleware.SuccessResponse(c, skill)
}

func (h *PlatformSkillHandler) Delete(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if err := h.svc.Delete(c.Request.Context(), c.Param("id"), userID); err != nil {
		handlePlatformSkillError(c, err, "删除平台 Skill 失败")
		return
	}
	middleware.SuccessResponse(c, nil)
}

func handlePlatformSkillError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, service.ErrPlatformSkillInvalid):
		middleware.ErrorResponse(c, http.StatusBadRequest, 40082, err.Error())
	case errors.Is(err, service.ErrPlatformSkillDuplicate):
		middleware.ErrorResponse(c, http.StatusConflict, 40980, err.Error())
	case errors.Is(err, service.ErrPlatformSkillNotFound):
		middleware.ErrorResponse(c, http.StatusNotFound, 40480, err.Error())
	default:
		middleware.HandleServiceError(c, err, fallback)
	}
}
