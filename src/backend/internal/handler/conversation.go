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

// PinRequest 置顶请求体
type PinRequest struct {
	Pinned bool `json:"pinned"`
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

// Delete 删除对话
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

	var req PinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40013, "参数错误: "+err.Error())
		return
	}

	userID := middleware.GetUserID(c)
	err := h.svc.TogglePin(c.Request.Context(), userID, convID, req.Pinned)
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
