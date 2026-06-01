package handler

import (
	"errors"
	"net/http"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

type OpenSkillLocationRequest struct {
	SourcePath string `json:"source_path" binding:"required"`
}

// OpenSkillLocation 打开 daemon 电脑上的真实 SKILL.md 位置。
func (h *AgentHandler) OpenSkillLocation(c *gin.Context) {
	var req OpenSkillLocationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40040, "参数错误: "+err.Error())
		return
	}
	userID := middleware.GetUserID(c)
	err := h.svc.OpenDaemonSkillLocation(c.Request.Context(), userID, c.Param("id"), req.SourcePath)
	if err != nil {
		if errors.Is(err, service.ErrAgentInvalidInput) {
			middleware.ErrorResponse(c, http.StatusBadRequest, 40041, err.Error())
			return
		}
		if errors.Is(err, service.ErrAgentNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40434, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50039, "打开 skill 位置失败")
		return
	}
	middleware.SuccessResponse(c, nil)
}
