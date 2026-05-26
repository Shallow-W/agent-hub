package handler

import (
	"errors"
	"net/http"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// GroupHandler 群聊接口处理器
type GroupHandler struct {
	svc *service.GroupService
}

// NewGroupHandler 创建群聊处理器
func NewGroupHandler(svc *service.GroupService) *GroupHandler {
	return &GroupHandler{svc: svc}
}

// CreateGroupRequest 创建群聊请求体
type CreateGroupRequest struct {
	Name      string   `json:"name" binding:"required,min=1,max=50"`
	MemberIDs []string `json:"member_ids" binding:"max=100"`
}

// AddMemberRequest 添加成员请求体
type AddMemberRequest struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

// CreateGroup 创建群聊
func (h *GroupHandler) CreateGroup(c *gin.Context) {
	var req CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40300, "参数错误: "+err.Error())
		return
	}

	userID := middleware.GetUserID(c)
	conv, err := h.svc.CreateGroup(c.Request.Context(), userID, req.Name, req.MemberIDs)
	if err != nil {
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50300, "创建群聊失败")
		return
	}

	middleware.CreatedResponse(c, conv)
}

// AddMember 添加群成员
func (h *GroupHandler) AddMember(c *gin.Context) {
	conversationID := c.Param("id")
	if conversationID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40300, "缺少群聊 ID")
		return
	}

	var req AddMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40300, "参数错误: "+err.Error())
		return
	}
	if req.UserID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40300, "缺少用户 ID")
		return
	}
	if req.Role == "" {
		req.Role = "member"
	}
	if req.Role != "member" && req.Role != "admin" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40300, "无效的角色，只允许 member 或 admin")
		return
	}

	userID := middleware.GetUserID(c)
	err := h.svc.AddMember(c.Request.Context(), conversationID, userID, req.UserID, req.Role)
	if err != nil {
		if errors.Is(err, service.ErrNotAdmin) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40301, err.Error())
			return
		}
		if errors.Is(err, service.ErrNotMember) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40304, err.Error())
			return
		}
		if errors.Is(err, service.ErrUserNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40302, err.Error())
			return
		}
		if errors.Is(err, service.ErrAlreadyMember) {
			middleware.ErrorResponse(c, http.StatusConflict, 40303, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50301, "添加成员失败")
		return
	}

	middleware.SuccessResponse(c, nil)
}

// RemoveMember 移除群成员
func (h *GroupHandler) RemoveMember(c *gin.Context) {
	conversationID := c.Param("id")
	targetUserID := c.Param("userId")
	if conversationID == "" || targetUserID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40300, "缺少群聊 ID 或用户 ID")
		return
	}

	userID := middleware.GetUserID(c)
	err := h.svc.RemoveMember(c.Request.Context(), conversationID, userID, targetUserID)
	if err != nil {
		if errors.Is(err, service.ErrNotAdmin) || errors.Is(err, service.ErrNotOwner) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40301, err.Error())
			return
		}
		if errors.Is(err, service.ErrNotMember) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40304, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50302, "移除成员失败")
		return
	}

	middleware.SuccessResponse(c, nil)
}

// ListMembers 列出群成员
func (h *GroupHandler) ListMembers(c *gin.Context) {
	conversationID := c.Param("id")
	if conversationID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40300, "缺少群聊 ID")
		return
	}

	userID := middleware.GetUserID(c)
	list, err := h.svc.ListMembers(c.Request.Context(), conversationID, userID)
	if err != nil {
		if errors.Is(err, service.ErrNotMember) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40304, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50303, "查询成员列表失败")
		return
	}

	middleware.SuccessResponse(c, list)
}

// LeaveGroup 离开群聊
func (h *GroupHandler) LeaveGroup(c *gin.Context) {
	conversationID := c.Param("id")
	if conversationID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40300, "缺少群聊 ID")
		return
	}

	userID := middleware.GetUserID(c)
	err := h.svc.LeaveGroup(c.Request.Context(), conversationID, userID)
	if err != nil {
		if errors.Is(err, service.ErrOwnerLeave) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40301, err.Error())
			return
		}
		if errors.Is(err, service.ErrNotMember) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40304, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50304, "离开群聊失败")
		return
	}

	middleware.SuccessResponse(c, nil)
}

// GetGroupInfo 获取群聊详情
func (h *GroupHandler) GetGroupInfo(c *gin.Context) {
	conversationID := c.Param("id")
	if conversationID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40300, "缺少群聊 ID")
		return
	}

	userID := middleware.GetUserID(c)
	conv, members, err := h.svc.GetGroupInfo(c.Request.Context(), conversationID, userID)
	if err != nil {
		if errors.Is(err, service.ErrNotMember) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40304, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50305, "获取群聊详情失败")
		return
	}

	middleware.SuccessResponse(c, gin.H{
		"conversation": conv,
		"members":      members,
	})
}
