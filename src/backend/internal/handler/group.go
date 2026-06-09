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
	MemberIDs []string `json:"member_ids" binding:"max=100,dive,uuid"`
}

// AddMemberRequest 添加成员请求体
type AddMemberRequest struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

// UpdateGroupInfoRequest 更新群聊信息请求体
type UpdateGroupInfoRequest struct {
	Title        *string `json:"title"`
	Avatar       *string `json:"avatar"`
	Description  *string `json:"description"`
	Announcement *string `json:"announcement"`
	Tags         *string `json:"tags"`
}

// ChangeRoleRequest 修改成员角色请求体
type ChangeRoleRequest struct {
	Role string `json:"role" binding:"required,oneof=admin member"`
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
		if errors.Is(err, service.ErrGroupNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40415, err.Error())
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

// DissolveGroup 解散群聊
func (h *GroupHandler) DissolveGroup(c *gin.Context) {
	conversationID := c.Param("id")
	if conversationID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40300, "缺少群聊 ID")
		return
	}

	userID := middleware.GetUserID(c)
	err := h.svc.DissolveGroup(c.Request.Context(), conversationID, userID)
	if err != nil {
		if errors.Is(err, service.ErrNotOwner) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40301, err.Error())
			return
		}
		if errors.Is(err, service.ErrNotMember) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40304, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50307, "解散群聊失败")
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
		if errors.Is(err, service.ErrGroupNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40414, err.Error())
			return
		}
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

// UpdateGroupInfo 更新群聊基本信息
func (h *GroupHandler) UpdateGroupInfo(c *gin.Context) {
	conversationID := c.Param("id")
	if conversationID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40300, "缺少群聊 ID")
		return
	}

	var req UpdateGroupInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40300, "参数错误: "+err.Error())
		return
	}

	// 至少需要一个字段
	if req.Title == nil && req.Avatar == nil && req.Description == nil && req.Announcement == nil && req.Tags == nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40300, "至少需要更新一个字段")
		return
	}

	// 先获取当前值，以便只更新传入的字段
	userID := middleware.GetUserID(c)
	conv, _, err := h.svc.GetGroupInfo(c.Request.Context(), conversationID, userID)
	if err != nil {
		if errors.Is(err, service.ErrGroupNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40414, err.Error())
			return
		}
		if errors.Is(err, service.ErrNotMember) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40304, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50305, "获取群聊信息失败")
		return
	}

	// 合并：用请求值覆盖当前值
	title := conv.Title
	avatar := conv.Avatar
	description := conv.Description
	announcement := conv.Announcement
	tags := conv.Tags
	if req.Title != nil {
		title = *req.Title
	}
	if req.Avatar != nil {
		avatar = *req.Avatar
	}
	if req.Description != nil {
		description = *req.Description
	}
	if req.Announcement != nil {
		announcement = *req.Announcement
	}
	if req.Tags != nil {
		tags = *req.Tags
	}

	updated, err := h.svc.UpdateGroupInfo(c.Request.Context(), conversationID, userID, title, avatar, description, announcement, tags)
	if err != nil {
		if errors.Is(err, service.ErrNotAdmin) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40301, err.Error())
			return
		}
		if errors.Is(err, service.ErrNotMember) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40304, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50308, "更新群聊信息失败")
		return
	}

	middleware.SuccessResponse(c, updated)
}

// ChangeMemberRole 修改群成员角色
func (h *GroupHandler) ChangeMemberRole(c *gin.Context) {
	convID := c.Param("id")
	memberID := c.Param("memberId")
	if convID == "" || memberID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40300, "缺少群聊 ID 或成员 ID")
		return
	}

	var req ChangeRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40300, "参数错误: "+err.Error())
		return
	}

	userID := middleware.GetUserID(c)
	if err := h.svc.ChangeMemberRole(c.Request.Context(), convID, userID, memberID, req.Role); err != nil {
		if errors.Is(err, service.ErrNotOwner) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40316, err.Error())
			return
		}
		if errors.Is(err, service.ErrNotMember) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40416, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50306, "修改角色失败")
		return
	}

	middleware.SuccessResponse(c, nil)
}
