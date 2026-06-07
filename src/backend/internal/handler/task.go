package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// taskConvAccess 验证用户是否有权访问会话资源。
type taskConvAccess interface {
	GetMember(ctx context.Context, conversationID, userID string) (*model.ConversationMember, error)
	GetByID(ctx context.Context, id string) (*model.Conversation, error)
}

// TaskHandler 处理任务看板接口。
type TaskHandler struct {
	svc      *service.TaskService
	convRepo taskConvAccess
}

// NewTaskHandler 创建任务处理器。
func NewTaskHandler(svc *service.TaskService, convRepo taskConvAccess) *TaskHandler {
	return &TaskHandler{svc: svc, convRepo: convRepo}
}

// requireMember 校验当前用户是否有权访问指定会话的任务（成员或创建者）。
func (h *TaskHandler) requireMember(c *gin.Context, conversationID string) bool {
	userID := middleware.GetUserID(c)
	// 检查是否为会话成员
	member, _ := h.convRepo.GetMember(c.Request.Context(), conversationID, userID)
	if member != nil {
		return true
	}
	// 检查是否为会话创建者（owner 不在 conversation_members 表中）
	conv, err := h.convRepo.GetByID(c.Request.Context(), conversationID)
	if err != nil || conv == nil || conv.UserID != userID {
		middleware.ErrorResponse(c, http.StatusForbidden, 40301, "无权访问该对话的任务")
		return false
	}
	return true
}

// CreateTaskRequest 创建任务请求。
type CreateTaskRequest struct {
	ConversationID *string `json:"conversation_id" binding:"required"`
	AssigneeID     *string `json:"assignee_id"`
	AgentID        *string `json:"agent_id"`
	Title          string  `json:"title" binding:"required,min=1,max=120"`
	Description    string  `json:"description"`
	Status         string  `json:"status"`
	Priority       string  `json:"priority"`
}

// UpdateTaskRequest 更新任务请求。
type UpdateTaskRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Priority    *string `json:"priority"`
	AssigneeID  *string `json:"assignee_id"`
	AgentID     *string `json:"agent_id"`
}

// MoveTaskStatusRequest 状态流转请求。
type MoveTaskStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

// List 查询任务列表。conversation_id 为必填 query 参数。
func (h *TaskHandler) List(c *gin.Context) {
	convID := c.Query("conversation_id")
	if convID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40400, "conversation_id 必填")
		return
	}
	if !h.requireMember(c, convID) {
		return
	}
	tasks, err := h.svc.List(c.Request.Context(), "", model.TaskFilter{
		ConversationID: convID,
		Status:         c.Query("status"),
	})
	if err != nil {
		if errors.Is(err, service.ErrTaskInvalid) {
			middleware.ErrorResponse(c, http.StatusBadRequest, 40400, "任务筛选参数错误")
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50400, "查询任务失败")
		return
	}
	middleware.SuccessResponse(c, tasks)
}

// Create 新建任务。conversation_id 为必填字段。
func (h *TaskHandler) Create(c *gin.Context) {
	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40401, "任务参数错误: "+err.Error())
		return
	}
	if req.ConversationID != nil && *req.ConversationID != "" {
		if !h.requireMember(c, *req.ConversationID) {
			return
		}
	}
	userID := middleware.GetUserID(c)
	task, err := h.svc.Create(c.Request.Context(), userID, model.TaskCreateInput{
		ConversationID: req.ConversationID,
		AssigneeID:     req.AssigneeID,
		AgentID:        req.AgentID,
		Title:          req.Title,
		Description:    req.Description,
		Status:         req.Status,
		Priority:       req.Priority,
	})
	if err != nil {
		if errors.Is(err, service.ErrTaskInvalid) {
			middleware.ErrorResponse(c, http.StatusBadRequest, 40401, "任务参数错误")
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50401, "创建任务失败")
		return
	}
	middleware.CreatedResponse(c, task)
}

// Update 更新任务内容。
func (h *TaskHandler) Update(c *gin.Context) {
	// 先获取任务以校验会话成员权限
	existing, err := h.svc.GetByID(c.Request.Context(), "", c.Param("id"))
	if err != nil {
		writeTaskError(c, err, 40402, "更新任务失败")
		return
	}
	if existing.ConversationID != nil && *existing.ConversationID != "" {
		if !h.requireMember(c, *existing.ConversationID) {
			return
		}
	}
	var req UpdateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40402, "任务参数错误: "+err.Error())
		return
	}
	task, err := h.svc.Update(c.Request.Context(), "", c.Param("id"), model.TaskUpdateInput{
		Title:       req.Title,
		Description: req.Description,
		Priority:    req.Priority,
		AssigneeID:  req.AssigneeID,
		AgentID:     req.AgentID,
	})
	if err != nil {
		writeTaskError(c, err, 40402, "更新任务失败")
		return
	}
	middleware.SuccessResponse(c, task)
}

// MoveStatus 更新任务状态。
func (h *TaskHandler) MoveStatus(c *gin.Context) {
	// 先获取任务以校验会话成员权限
	existing, err := h.svc.GetByID(c.Request.Context(), "", c.Param("id"))
	if err != nil {
		writeTaskError(c, err, 40403, "任务状态流转失败")
		return
	}
	if existing.ConversationID != nil && *existing.ConversationID != "" {
		if !h.requireMember(c, *existing.ConversationID) {
			return
		}
	}
	var req MoveTaskStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40403, "状态参数错误: "+err.Error())
		return
	}
	task, err := h.svc.MoveStatus(c.Request.Context(), "", c.Param("id"), req.Status)
	if err != nil {
		writeTaskError(c, err, 40403, "任务状态流转失败")
		return
	}
	middleware.SuccessResponse(c, task)
}

// Delete 删除任务。
func (h *TaskHandler) Delete(c *gin.Context) {
	taskID := c.Param("id")
	// 先获取任务以校验会话成员权限
	existing, err := h.svc.GetByID(c.Request.Context(), "", taskID)
	if err != nil {
		writeTaskError(c, err, 40404, "删除任务失败")
		return
	}
	if existing.ConversationID != nil && *existing.ConversationID != "" {
		if !h.requireMember(c, *existing.ConversationID) {
			return
		}
	}
	if err := h.svc.Delete(c.Request.Context(), "", taskID); err != nil {
		writeTaskError(c, err, 40404, "删除任务失败")
		return
	}
	middleware.SuccessResponse(c, gin.H{"deleted": true, "id": taskID})
}

func writeTaskError(c *gin.Context, err error, badRequestCode int, fallback string) {
	if errors.Is(err, service.ErrTaskInvalid) {
		middleware.ErrorResponse(c, http.StatusBadRequest, badRequestCode, "任务参数错误")
		return
	}
	if errors.Is(err, service.ErrTaskNotFound) {
		middleware.ErrorResponse(c, http.StatusNotFound, 40410, "任务不存在")
		return
	}
	middleware.ErrorResponse(c, http.StatusInternalServerError, 50410, fallback)
}
