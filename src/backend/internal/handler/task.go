package handler

import (
	"errors"
	"net/http"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// TaskHandler 处理任务看板接口。
type TaskHandler struct {
	svc *service.TaskService
}

// NewTaskHandler 创建任务处理器。
func NewTaskHandler(svc *service.TaskService) *TaskHandler {
	return &TaskHandler{svc: svc}
}

// CreateTaskRequest 创建任务请求。
type CreateTaskRequest struct {
	ConversationID *string `json:"conversation_id"`
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

// List 查询任务列表。
func (h *TaskHandler) List(c *gin.Context) {
	userID := middleware.GetUserID(c)
	tasks, err := h.svc.List(c.Request.Context(), userID, model.TaskFilter{
		ConversationID: c.Query("conversation_id"),
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

// Create 新建任务。
func (h *TaskHandler) Create(c *gin.Context) {
	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40401, "任务参数错误: "+err.Error())
		return
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
	var req UpdateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40402, "任务参数错误: "+err.Error())
		return
	}
	task, err := h.svc.Update(c.Request.Context(), middleware.GetUserID(c), c.Param("id"), model.TaskUpdateInput{
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
	var req MoveTaskStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40403, "状态参数错误: "+err.Error())
		return
	}
	task, err := h.svc.MoveStatus(c.Request.Context(), middleware.GetUserID(c), c.Param("id"), req.Status)
	if err != nil {
		writeTaskError(c, err, 40403, "任务状态流转失败")
		return
	}
	middleware.SuccessResponse(c, task)
}

// Delete 删除任务。
func (h *TaskHandler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), middleware.GetUserID(c), c.Param("id")); err != nil {
		writeTaskError(c, err, 40404, "删除任务失败")
		return
	}
	middleware.SuccessResponse(c, nil)
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
