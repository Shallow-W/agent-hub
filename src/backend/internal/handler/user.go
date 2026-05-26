package handler

import (
	"net/http"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// UserHandler 用户接口处理器
type UserHandler struct {
	friendSvc *service.FriendService
}

// NewUserHandler 创建用户处理器
func NewUserHandler(friendSvc *service.FriendService) *UserHandler {
	return &UserHandler{friendSvc: friendSvc}
}

// Search 搜索用户
func (h *UserHandler) Search(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40600, "缺少搜索关键词")
		return
	}

	userID := middleware.GetUserID(c)
	list, err := h.friendSvc.SearchUsers(c.Request.Context(), userID, q, 20)
	if err != nil {
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50600, "搜索用户失败")
		return
	}

	middleware.SuccessResponse(c, list)
}
