package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/model"
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
	Role          string                    `json:"role"`
	Content       string                    `json:"content" binding:"required"`
	ArtifactsJSON string                    `json:"artifacts_json"`
	Attachments   []model.MessageAttachment `json:"attachments"`
	ReplyTo       *string                   `json:"reply_to"`
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
	msg, err := h.svc.SendMessageWithReply(c.Request.Context(), convID, userID, req.Role, req.Content, req.ArtifactsJSON, req.Attachments, req.ReplyTo)
	if err != nil {
		slog.Error("send message failed", "error", err, "convID", convID, "userID", userID)
		if errors.Is(err, service.ErrMsgConvNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40420, err.Error())
			return
		}
		if errors.Is(err, service.ErrMsgConvNoPerm) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40320, err.Error())
			return
		}
		if errors.Is(err, service.ErrMsgTooLong) {
			middleware.ErrorResponse(c, http.StatusRequestEntityTooLarge, 40026, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50020, "发送消息失败")
		return
	}

	middleware.CreatedResponse(c, msg)
}

// MarkAsRead 标记会话消息已读
func (h *MessageHandler) MarkAsRead(c *gin.Context) {
	convID := c.Param("id")
	if convID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40024, "缺少对话 ID")
		return
	}

	userID := middleware.GetUserID(c)
	if err := h.svc.MarkAsRead(c.Request.Context(), userID, convID); err != nil {
		if errors.Is(err, service.ErrMsgConvNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40422, err.Error())
			return
		}
		if errors.Is(err, service.ErrMsgConvNoPerm) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40322, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50022, "标记已读失败")
		return
	}

	middleware.SuccessResponse(c, nil)
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

	var beforeArg interface{}
	if !before.IsZero() {
		beforeArg = before
	}

	messages, err := h.svc.GetHistory(c.Request.Context(), convID, userID, beforeArg, limit)
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

// Unread 获取离线/未读消息
func (h *MessageHandler) Unread(c *gin.Context) {
	convID := c.Param("id")
	if convID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40025, "缺少对话 ID")
		return
	}

	userID := middleware.GetUserID(c)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))

	messages, err := h.svc.GetUnreadMessages(c.Request.Context(), convID, userID, limit)
	if err != nil {
		if errors.Is(err, service.ErrMsgConvNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40423, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50023, "获取未读消息失败")
		return
	}

	middleware.SuccessResponse(c, messages)
}

// Search 搜索对话消息
func (h *MessageHandler) Search(c *gin.Context) {
	convID := c.Param("id")
	if convID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40029, "缺少对话 ID")
		return
	}
	keyword := c.Query("keyword")
	if keyword == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40030, "keyword required")
		return
	}

	msgs, err := h.svc.SearchMessages(c.Request.Context(), convID, keyword)
	if err != nil {
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50025, "搜索消息失败")
		return
	}
	if msgs == nil {
		msgs = []model.Message{}
	}
	middleware.SuccessResponse(c, msgs)
}

// Recall 撤回消息
func (h *MessageHandler) Recall(c *gin.Context) {
	convID := c.Param("id")
	messageID := c.Param("messageId")
	if convID == "" || messageID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40027, "缺少对话 ID 或消息 ID")
		return
	}

	userID := middleware.GetUserID(c)
	err := h.svc.RecallMessage(c.Request.Context(), convID, messageID, userID)
	if err != nil {
		if errors.Is(err, service.ErrMsgConvNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40424, err.Error())
			return
		}
		if errors.Is(err, service.ErrMsgConvNoPerm) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40323, err.Error())
			return
		}
		if errors.Is(err, service.ErrMsgNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40425, err.Error())
			return
		}
		if errors.Is(err, service.ErrMsgNotSender) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40324, err.Error())
			return
		}
		if errors.Is(err, service.ErrMsgRecallExpired) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40325, err.Error())
			return
		}
		if errors.Is(err, service.ErrMsgAlreadyDeleted) {
			middleware.ErrorResponse(c, http.StatusBadRequest, 40028, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50024, "撤回消息失败")
		return
	}

	middleware.SuccessResponse(c, nil)
}
