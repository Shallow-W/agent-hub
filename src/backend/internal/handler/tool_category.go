package handler

import (
	"net/http"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/repository"
	"github.com/gin-gonic/gin"
)

type ToolCategoryHandler struct {
	repo *repository.ToolCategoryRepo
}

func NewToolCategoryHandler(repo *repository.ToolCategoryRepo) *ToolCategoryHandler {
	return &ToolCategoryHandler{repo: repo}
}

func (h *ToolCategoryHandler) List(c *gin.Context) {
	list, err := h.repo.List(c.Request.Context())
	if err != nil {
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50093, "查询工具类别失败")
		return
	}
	middleware.SuccessResponse(c, list)
}
