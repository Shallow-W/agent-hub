package handler

import (
	"errors"
	"net/http"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// FriendHandler 好友接口处理器
type FriendHandler struct {
	svc *service.FriendService
}

// NewFriendHandler 创建好友处理器
func NewFriendHandler(svc *service.FriendService) *FriendHandler {
	return &FriendHandler{svc: svc}
}

// SendFriendRequest 发送好友申请请求体
type SendFriendRequestBody struct {
	FriendID string `json:"friend_id" binding:"omitempty,uuid"`
	Username string `json:"username"`
}

// SendRequest 发送好友申请
func (h *FriendHandler) SendRequest(c *gin.Context) {
	var req SendFriendRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40200, "参数错误: "+err.Error())
		return
	}

	userID := middleware.GetUserID(c)

	// 解析目标用户 ID（支持 friend_id 或 username）
	friendID, err := h.svc.ResolveFriendID(c.Request.Context(), req.FriendID, req.Username)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40430, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusBadRequest, 40201, err.Error())
		return
	}

	friend, err := h.svc.SendFriendRequest(c.Request.Context(), userID, friendID)
	if err != nil {
		if errors.Is(err, service.ErrFriendSelf) {
			middleware.ErrorResponse(c, http.StatusBadRequest, 40202, err.Error())
			return
		}
		if errors.Is(err, service.ErrFriendExists) {
			middleware.ErrorResponse(c, http.StatusConflict, 40203, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50200, "发送好友申请失败")
		return
	}

	middleware.CreatedResponse(c, friend)
}

// AcceptRequest 接受好友申请
func (h *FriendHandler) AcceptRequest(c *gin.Context) {
	requestID := c.Param("id")
	if requestID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40204, "缺少好友申请 ID")
		return
	}

	userID := middleware.GetUserID(c)
	err := h.svc.AcceptFriendRequest(c.Request.Context(), userID, requestID)
	if err != nil {
		if errors.Is(err, service.ErrFriendNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40431, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50201, "接受好友申请失败")
		return
	}

	middleware.SuccessResponse(c, nil)
}

// RejectRequest 拒绝好友申请
func (h *FriendHandler) RejectRequest(c *gin.Context) {
	requestID := c.Param("id")
	if requestID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40205, "缺少好友申请 ID")
		return
	}

	userID := middleware.GetUserID(c)
	err := h.svc.RejectFriendRequest(c.Request.Context(), userID, requestID)
	if err != nil {
		if errors.Is(err, service.ErrFriendNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40432, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50202, "拒绝好友申请失败")
		return
	}

	middleware.SuccessResponse(c, nil)
}

// ListFriends 查询好友列表
func (h *FriendHandler) ListFriends(c *gin.Context) {
	userID := middleware.GetUserID(c)
	list, err := h.svc.ListFriends(c.Request.Context(), userID)
	if err != nil {
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50203, "查询好友列表失败")
		return
	}

	middleware.SuccessResponse(c, list)
}

// ListPending 查询收到的好友申请
func (h *FriendHandler) ListPending(c *gin.Context) {
	userID := middleware.GetUserID(c)
	list, err := h.svc.ListPending(c.Request.Context(), userID)
	if err != nil {
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50204, "查询好友申请失败")
		return
	}

	middleware.SuccessResponse(c, list)
}

// SearchUsers 搜索用户
func (h *FriendHandler) SearchUsers(c *gin.Context) {
	username := c.Query("username")
	if username == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40210, "缺少搜索关键词")
		return
	}

	userID := middleware.GetUserID(c)
	list, err := h.svc.SearchUsers(c.Request.Context(), userID, username, 20)
	if err != nil {
		middleware.ErrorResponse(c, http.StatusInternalServerError, 40211, "搜索用户失败")
		return
	}

	middleware.SuccessResponse(c, list)
}
