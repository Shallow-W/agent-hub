package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// ConversationHandler 对话接口处理器
type ConversationHandler struct {
	svc *service.ConversationService
}

// NewConversationHandler 创建对话处理器
func NewConversationHandler(svc *service.ConversationService) *ConversationHandler {
	return &ConversationHandler{svc: svc}
}

// CreateRequest 创建对话请求体
type CreateRequest struct {
	Type  string `json:"type"`
	Title string `json:"title"`
}

// RenameRequest 重命名请求体
type RenameRequest struct {
	Title string `json:"title" binding:"required"`
}

// PrivateChatRequest 私聊请求体
type PrivateChatRequest struct {
	FriendID string `json:"friend_id" binding:"required,uuid"`
}

// GetOrCreatePrivate 查找或创建与指定好友的私聊会话
func (h *ConversationHandler) GetOrCreatePrivate(c *gin.Context) {
	var req PrivateChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40019, "参数错误: "+err.Error())
		return
	}

	userID := middleware.GetUserID(c)
	conv, err := h.svc.GetOrCreatePrivateChat(c.Request.Context(), userID, req.FriendID)
	if err != nil {
		if errors.Is(err, service.ErrSelfChat) || errors.Is(err, service.ErrNotFriends) {
			middleware.ErrorResponse(c, http.StatusBadRequest, 40041, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50016, "创建私聊失败")
		return
	}

	middleware.SuccessResponse(c, conv)
}

// Create 创建新对话
func (h *ConversationHandler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40010, "参数错误: "+err.Error())
		return
	}

	userID := middleware.GetUserID(c)
	conv, err := h.svc.CreateConversation(c.Request.Context(), userID, req.Type, req.Title)
	if err != nil {
		if errors.Is(err, service.ErrConvInvalidTitle) {
			middleware.ErrorResponse(c, http.StatusBadRequest, 40044, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50010, "创建对话失败")
		return
	}

	middleware.CreatedResponse(c, conv)
}

// List 查询对话列表
func (h *ConversationHandler) List(c *gin.Context) {
	userID := middleware.GetUserID(c)

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	list, err := h.svc.ListConversations(c.Request.Context(), userID, limit, offset)
	if err != nil {
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50011, "查询对话列表失败")
		return
	}

	middleware.SuccessResponse(c, list)
}

// Delete 删除对话（仅私聊，移除当前用户的成员记录）
func (h *ConversationHandler) Delete(c *gin.Context) {
	convID := c.Param("id")
	if convID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40011, "缺少对话 ID")
		return
	}

	userID := middleware.GetUserID(c)
	err := h.svc.DeleteConversation(c.Request.Context(), userID, convID)
	if err != nil {
		if errors.Is(err, service.ErrConvNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40410, err.Error())
			return
		}
		if errors.Is(err, service.ErrConvNoPerm) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40310, err.Error())
			return
		}
		if errors.Is(err, service.ErrConvNotGroup) {
			middleware.ErrorResponse(c, http.StatusBadRequest, 40014, "仅私聊会话可删除")
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50012, "删除对话失败")
		return
	}

	middleware.SuccessResponse(c, nil)
}

// TogglePin 切换对话置顶状态
func (h *ConversationHandler) TogglePin(c *gin.Context) {
	convID := c.Param("id")
	if convID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40012, "缺少对话 ID")
		return
	}

	userID := middleware.GetUserID(c)
	err := h.svc.TogglePin(c.Request.Context(), userID, convID)
	if err != nil {
		if errors.Is(err, service.ErrConvNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40411, err.Error())
			return
		}
		if errors.Is(err, service.ErrConvNoPerm) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40311, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50013, "更新置顶状态失败")
		return
	}

	middleware.SuccessResponse(c, nil)
}

// RenameConversation 重命名会话（仅群聊，需 owner/admin 权限）
func (h *ConversationHandler) RenameConversation(c *gin.Context) {
	convID := c.Param("id")
	if convID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40015, "缺少对话 ID")
		return
	}

	var req RenameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40016, "参数错误: "+err.Error())
		return
	}

	userID := middleware.GetUserID(c)
	err := h.svc.RenameConversation(c.Request.Context(), userID, convID, req.Title)
	if err != nil {
		if errors.Is(err, service.ErrConvNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40412, err.Error())
			return
		}
		if errors.Is(err, service.ErrConvNoPerm) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40312, err.Error())
			return
		}
		if errors.Is(err, service.ErrConvNotGroup) {
			middleware.ErrorResponse(c, http.StatusBadRequest, 40017, "私聊会话不能重命名")
			return
		}
		if errors.Is(err, service.ErrConvNotMember) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40313, err.Error())
			return
		}
		if errors.Is(err, service.ErrConvInvalidTitle) {
			middleware.ErrorResponse(c, http.StatusBadRequest, 40045, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50014, "重命名对话失败")
		return
	}

	middleware.SuccessResponse(c, nil)
}

// ListArchived 查询已归档的对话列表
func (h *ConversationHandler) ListArchived(c *gin.Context) {
	userID := middleware.GetUserID(c)

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	list, err := h.svc.ListArchivedConversations(c.Request.Context(), userID, limit, offset)
	if err != nil {
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50017, "查询归档对话失败")
		return
	}

	middleware.SuccessResponse(c, list)
}

// ArchiveConversation 归档会话（软删除）
func (h *ConversationHandler) ArchiveConversation(c *gin.Context) {
	convID := c.Param("id")
	if convID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40018, "缺少对话 ID")
		return
	}

	userID := middleware.GetUserID(c)
	err := h.svc.ArchiveConversation(c.Request.Context(), userID, convID)
	if err != nil {
		if errors.Is(err, service.ErrConvNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40413, err.Error())
			return
		}
		if errors.Is(err, service.ErrConvNoPerm) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40314, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50015, "归档对话失败")
		return
	}

	middleware.SuccessResponse(c, nil)
}

// UnarchiveConversation 取消归档会话
func (h *ConversationHandler) UnarchiveConversation(c *gin.Context) {
	convID := c.Param("id")
	if convID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40042, "缺少对话 ID")
		return
	}

	userID := middleware.GetUserID(c)
	err := h.svc.UnarchiveConversation(c.Request.Context(), userID, convID)
	if err != nil {
		if errors.Is(err, service.ErrConvNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40414, err.Error())
			return
		}
		if errors.Is(err, service.ErrConvNotMember) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40315, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50018, "取消归档对话失败")
		return
	}

	middleware.SuccessResponse(c, nil)
}
