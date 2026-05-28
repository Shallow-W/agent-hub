package handler

import (
	"errors"
	"net/http"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// UserHandler 用户接口处理器
type UserHandler struct {
	friendSvc *service.FriendService
	userSvc   *service.UserService
}

// NewUserHandler 创建用户处理器
func NewUserHandler(friendSvc *service.FriendService, userSvc *service.UserService) *UserHandler {
	return &UserHandler{friendSvc: friendSvc, userSvc: userSvc}
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

// UpdateProfileRequest 更新资料请求体
type UpdateProfileRequest struct {
	Username string `json:"username" binding:"required"`
}

// GetProfile 获取当前用户资料
func (h *UserHandler) GetProfile(c *gin.Context) {
	userID := middleware.GetUserID(c)
	user, err := h.userSvc.GetProfile(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40601, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50601, "获取用户资料失败")
		return
	}

	middleware.SuccessResponse(c, user)
}

// UpdateProfile 更新当前用户资料
func (h *UserHandler) UpdateProfile(c *gin.Context) {
	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40602, "参数错误: "+err.Error())
		return
	}

	userID := middleware.GetUserID(c)
	user, err := h.userSvc.UpdateProfile(c.Request.Context(), userID, req.Username)
	if err != nil {
		if errors.Is(err, service.ErrUsernameEmpty) {
			middleware.ErrorResponse(c, http.StatusBadRequest, 40603, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50602, "更新用户资料失败")
		return
	}

	middleware.SuccessResponse(c, user)
}
