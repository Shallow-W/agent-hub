package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/agent-hub/backend/internal/domain"
	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// ConversationHandler 对话接口处理器
type ConversationHandler struct {
	svc     *service.ConversationService
	roleSvc *service.RoleService
}

// NewConversationHandler 创建对话处理器。
// roleSvc 用于 PUT /conversations/:id/agents/:agentID/role，
// 角色变更与广播交给 RoleService 独立承担。
func NewConversationHandler(svc *service.ConversationService, roleSvc *service.RoleService) *ConversationHandler {
	return &ConversationHandler{svc: svc, roleSvc: roleSvc}
}

// CreateRequest 创建对话请求体
type CreateRequest struct {
	Type  string `json:"type" binding:"required,oneof=single group"`
	Title string `json:"title" binding:"max=100"`
}

// RenameRequest 重命名请求体
type RenameRequest struct {
	Title string `json:"title" binding:"required"`
}

// PrivateChatRequest 私聊请求体
type PrivateChatRequest struct {
	FriendID string `json:"friend_id" binding:"required,uuid"`
}

// AgentChatRequest 智能体私聊请求体。
type AgentChatRequest struct {
	AgentID string `json:"agent_id" binding:"required,uuid"`
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

// GetOrCreateAgentPrivate 查找或创建与指定智能体的一对一会话。
func (h *ConversationHandler) GetOrCreateAgentPrivate(c *gin.Context) {
	var req AgentChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40046, "参数错误: "+err.Error())
		return
	}

	userID := middleware.GetUserID(c)
	conv, err := h.svc.GetOrCreateAgentChat(c.Request.Context(), userID, req.AgentID)
	if err != nil {
		if errors.Is(err, service.ErrConvNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40415, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50019, "创建智能体私聊失败")
		return
	}

	middleware.SuccessResponse(c, conv)
}

// AgentMemberRequest 添加 Robot 成员请求体。
type AgentMemberRequest struct {
	AgentID string `json:"agent_id" binding:"required"`
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

// Delete 删除当前用户可见的对话。
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
		if errors.Is(err, service.ErrConvNoPerm) || errors.Is(err, service.ErrConvNotMember) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40310, err.Error())
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

// ListAgents 查询当前对话中的 Robot 成员。
func (h *ConversationHandler) ListAgents(c *gin.Context) {
	userID := middleware.GetUserID(c)
	list, err := h.svc.ListConversationAgents(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		if errors.Is(err, service.ErrConvNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40412, err.Error())
			return
		}
		if errors.Is(err, service.ErrConvNoPerm) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40312, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50014, "查询对话 Robot 失败")
		return
	}
	middleware.SuccessResponse(c, list)
}

// AddAgent 把一个已创建 Agent 作为 Robot 加入当前对话。
func (h *ConversationHandler) AddAgent(c *gin.Context) {
	var req AgentMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40014, "参数错误: "+err.Error())
		return
	}

	userID := middleware.GetUserID(c)
	item, err := h.svc.AddConversationAgent(c.Request.Context(), userID, c.Param("id"), req.AgentID)
	if err != nil {
		if errors.Is(err, service.ErrConvNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40413, err.Error())
			return
		}
		if errors.Is(err, service.ErrConvNoPerm) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40313, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50015, "添加对话 Robot 失败")
		return
	}
	middleware.CreatedResponse(c, item)
}

// RemoveAgent 从当前对话移除 Robot。
func (h *ConversationHandler) RemoveAgent(c *gin.Context) {
	userID := middleware.GetUserID(c)
	err := h.svc.RemoveConversationAgent(c.Request.Context(), userID, c.Param("id"), c.Param("agentID"))
	if err != nil {
		if errors.Is(err, service.ErrConvNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40414, err.Error())
			return
		}
		if errors.Is(err, service.ErrConvNoPerm) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40314, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50016, "移除对话 Robot 失败")
		return
	}
	middleware.SuccessResponse(c, nil)
}

// SetAgentRoleRequest 设置 Agent 角色请求体
type SetAgentRoleRequest struct {
	Role string `json:"role" binding:"required"`
}

// SetAgentRole 设置会话中 Agent 的角色（Orchestrator/Worker）
func (h *ConversationHandler) SetAgentRole(c *gin.Context) {
	var req SetAgentRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40015, "参数错误: "+err.Error())
		return
	}

	userID := middleware.GetUserID(c)
	err := h.roleSvc.Set(c.Request.Context(), userID, c.Param("id"), c.Param("agentID"), domain.Role(req.Role))
	if err != nil {
		if errors.Is(err, service.ErrConvInvalidRole) {
			middleware.ErrorResponse(c, http.StatusBadRequest, 40016, err.Error())
			return
		}
		if errors.Is(err, service.ErrConvNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40415, err.Error())
			return
		}
		if errors.Is(err, service.ErrConvNoPerm) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40315, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50017, "设置 Agent 角色失败")
		return
	}
	middleware.SuccessResponse(c, nil)
}
