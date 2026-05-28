package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// MessageHandler 消息接口处理器
type MessageHandler struct {
	svc *service.MessageService
}

// NewMessageHandler 创建消息处理器
func NewMessageHandler(svc *service.MessageService) *MessageHandler {
	return &MessageHandler{svc: svc}
}

// SendMessageRequest 发送消息请求体
type SendMessageRequest struct {
	Role          string `json:"role"`
	Content       string `json:"content" binding:"required"`
	ArtifactsJSON string `json:"artifacts_json"`
	AgentID       string `json:"agent_id"`
}

// Send 发送消息
func (h *MessageHandler) Send(c *gin.Context) {
	convID := c.Param("id")
	if convID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40020, "缺少对话 ID")
		return
	}

	var req SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40021, "参数错误: "+err.Error())
		return
	}

	userID := middleware.GetUserID(c)
	msg, err := h.svc.SendMessage(c.Request.Context(), convID, userID, req.Role, req.Content, req.ArtifactsJSON, req.AgentID)
	if err != nil {
		if errors.Is(err, service.ErrMsgConvNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40420, err.Error())
			return
		}
		if errors.Is(err, service.ErrMsgConvNoPerm) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40320, err.Error())
			return
		}
		if errors.Is(err, service.ErrAgentNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40422, err.Error())
			return
		}
		if errors.Is(err, service.ErrMsgAgentNoPerm) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40322, err.Error())
			return
		}
		if errors.Is(err, service.ErrMsgAgentOffline) {
			middleware.ErrorResponse(c, http.StatusBadRequest, 40024, err.Error())
			return
		}
		if errors.Is(err, service.ErrMsgAgentTimeout) {
			middleware.ErrorResponse(c, http.StatusGatewayTimeout, 50420, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50020, "发送消息失败")
		return
	}

	middleware.CreatedResponse(c, msg)
}

// History 获取消息历史
func (h *MessageHandler) History(c *gin.Context) {
	convID := c.Param("id")
	if convID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40022, "缺少对话 ID")
		return
	}

	userID := middleware.GetUserID(c)

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	var before time.Time
	if beforeStr := c.Query("before"); beforeStr != "" {
		t, err := time.Parse(time.RFC3339, beforeStr)
		if err != nil {
			middleware.ErrorResponse(c, http.StatusBadRequest, 40023, "before 格式错误，需为 RFC3339")
			return
		}
		before = t
	}

	messages, err := h.svc.GetHistory(c.Request.Context(), convID, userID, before, limit)
	if err != nil {
		if errors.Is(err, service.ErrMsgConvNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40421, err.Error())
			return
		}
		if errors.Is(err, service.ErrMsgConvNoPerm) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40321, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50021, "查询消息历史失败")
		return
	}

	middleware.SuccessResponse(c, messages)
}
