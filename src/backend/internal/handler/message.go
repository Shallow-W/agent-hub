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
	Content       string                    `json:"content"`
	ArtifactsJSON string                    `json:"artifacts_json"`
	Attachments   []model.MessageAttachment `json:"attachments"`
	ReplyTo       *string                   `json:"reply_to"`
	AgentID       string                    `json:"agent_id"`
	Mentions      []string                  `json:"mentions"`
}

// BlackboardRequest updates user-authored conversation blackboard context.
type BlackboardRequest struct {
	ManualContext string `json:"manual_context"`
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
	msg, err := h.svc.SendMessageWithReply(c.Request.Context(), convID, userID, req.Role, req.Content, req.ArtifactsJSON, req.Attachments, req.ReplyTo, req.AgentID, req.Mentions)
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
		if errors.Is(err, service.ErrMsgReplyNotFound) || errors.Is(err, service.ErrMsgReplyWrongConv) {
			middleware.ErrorResponse(c, http.StatusBadRequest, 40031, err.Error())
			return
		}
		if errors.Is(err, service.ErrMsgEmptyContent) {
			middleware.ErrorResponse(c, http.StatusBadRequest, 40042, err.Error())
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

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "25"))
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
		if errors.Is(err, service.ErrMsgConvNoPerm) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40324, err.Error())
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

	userID := middleware.GetUserID(c)
	msgs, err := h.svc.SearchMessages(c.Request.Context(), convID, userID, keyword)
	if err != nil {
		if errors.Is(err, service.ErrMsgConvNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40426, err.Error())
			return
		}
		if errors.Is(err, service.ErrMsgConvNoPerm) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40326, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50025, "搜索消息失败")
		return
	}
	if msgs == nil {
		msgs = []model.Message{}
	}
	middleware.SuccessResponse(c, msgs)
}

// Pin 将一条消息加入群聊共享上下文黑板。
func (h *MessageHandler) Pin(c *gin.Context) {
	convID := c.Param("id")
	messageID := c.Param("messageId")
	if convID == "" || messageID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40032, "缺少对话 ID 或消息 ID")
		return
	}

	userID := middleware.GetUserID(c)
	pin, err := h.svc.PinMessage(c.Request.Context(), convID, messageID, userID)
	if err != nil {
		if errors.Is(err, service.ErrMsgConvNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40427, err.Error())
			return
		}
		if errors.Is(err, service.ErrMsgConvNoPerm) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40327, err.Error())
			return
		}
		if errors.Is(err, service.ErrMsgNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40428, err.Error())
			return
		}
		if errors.Is(err, service.ErrMsgReplyWrongConv) {
			middleware.ErrorResponse(c, http.StatusBadRequest, 40033, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50026, "Pin 消息失败")
		return
	}

	middleware.SuccessResponse(c, pin)
}

// Unpin 将一条消息从群聊共享上下文黑板移除。
func (h *MessageHandler) Unpin(c *gin.Context) {
	convID := c.Param("id")
	messageID := c.Param("messageId")
	if convID == "" || messageID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40034, "缺少对话 ID 或消息 ID")
		return
	}

	userID := middleware.GetUserID(c)
	err := h.svc.UnpinMessage(c.Request.Context(), convID, messageID, userID)
	if err != nil {
		if errors.Is(err, service.ErrMsgConvNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40429, err.Error())
			return
		}
		if errors.Is(err, service.ErrMsgConvNoPerm) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40328, err.Error())
			return
		}
		if errors.Is(err, service.ErrMsgNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40430, err.Error())
			return
		}
		if errors.Is(err, service.ErrMsgReplyWrongConv) {
			middleware.ErrorResponse(c, http.StatusBadRequest, 40035, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50027, "取消 Pin 消息失败")
		return
	}

	middleware.SuccessResponse(c, nil)
}

// UpdateCard 更新消息中的交互式卡片状态（用户选择方案/确认操作/进度更新）。
func (h *MessageHandler) UpdateCard(c *gin.Context) {
	convID := c.Param("id")
	messageID := c.Param("messageId")
	if messageID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40052, "缺少消息 ID")
		return
	}
	var req struct {
		CardsJSON string `json:"cards_json"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40053, "参数错误")
		return
	}
	// 权限校验：消息必须属于路径中的对话，避免跨对话篡改卡片状态。
	if _, err := h.svc.GetMessageByID(c.Request.Context(), convID, messageID); err != nil {
		middleware.ErrorResponse(c, http.StatusForbidden, 40301, "消息不存在或不属于该对话")
		return
	}
	if err := h.svc.UpdateMessageCardsAndBroadcast(c.Request.Context(), messageID, req.CardsJSON); err != nil {
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50037, "更新卡片失败")
		return
	}
	middleware.SuccessResponse(c, nil)
}

// HideMessage 当前用户隐藏消息（仅对自己不可见，其他用户仍可见）。
func (h *MessageHandler) HideMessage(c *gin.Context) {
	convID := c.Param("id")
	messageID := c.Param("messageId")
	if convID == "" || messageID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40050, "缺少对话 ID 或消息 ID")
		return
	}
	userID := middleware.GetUserID(c)
	if err := h.svc.HideMessage(c.Request.Context(), userID, messageID); err != nil {
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50035, "隐藏消息失败")
		return
	}
	middleware.SuccessResponse(c, nil)
}

// UnhideMessage 取消隐藏消息。
func (h *MessageHandler) UnhideMessage(c *gin.Context) {
	convID := c.Param("id")
	messageID := c.Param("messageId")
	if convID == "" || messageID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40051, "缺少对话 ID 或消息 ID")
		return
	}
	userID := middleware.GetUserID(c)
	if err := h.svc.UnhideMessage(c.Request.Context(), userID, messageID); err != nil {
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50036, "取消隐藏失败")
		return
	}
	middleware.SuccessResponse(c, nil)
}

// PinnedContext 查询当前会话共享上下文黑板中的用户 Pin 上下文。
func (h *MessageHandler) PinnedContext(c *gin.Context) {
	convID := c.Param("id")
	if convID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40036, "缺少对话 ID")
		return
	}

	userID := middleware.GetUserID(c)
	items, err := h.svc.ListPinnedContext(c.Request.Context(), convID, userID)
	if err != nil {
		if errors.Is(err, service.ErrMsgConvNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40431, err.Error())
			return
		}
		if errors.Is(err, service.ErrMsgConvNoPerm) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40329, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50028, "查询 Pin 上下文失败")
		return
	}

	middleware.SuccessResponse(c, items)
}

// GetBlackboard 查询会话上下文黑板中的用户手写上下文。
func (h *MessageHandler) GetBlackboard(c *gin.Context) {
	convID := c.Param("id")
	if convID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40037, "缺少对话 ID")
		return
	}

	userID := middleware.GetUserID(c)
	blackboard, err := h.svc.GetConversationBlackboard(c.Request.Context(), convID, userID)
	if err != nil {
		if errors.Is(err, service.ErrMsgConvNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40432, err.Error())
			return
		}
		if errors.Is(err, service.ErrMsgConvNoPerm) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40330, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50029, "查询黑板失败")
		return
	}

	middleware.SuccessResponse(c, blackboard)
}

// UpdateBlackboard 保存会话上下文黑板中的用户手写上下文。
func (h *MessageHandler) UpdateBlackboard(c *gin.Context) {
	convID := c.Param("id")
	if convID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40038, "缺少对话 ID")
		return
	}

	var req BlackboardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40039, "参数错误: "+err.Error())
		return
	}

	userID := middleware.GetUserID(c)
	blackboard, err := h.svc.UpdateConversationBlackboard(c.Request.Context(), convID, userID, req.ManualContext)
	if err != nil {
		if errors.Is(err, service.ErrMsgConvNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40433, err.Error())
			return
		}
		if errors.Is(err, service.ErrMsgConvNoPerm) {
			middleware.ErrorResponse(c, http.StatusForbidden, 40331, err.Error())
			return
		}
		if errors.Is(err, service.ErrMsgBlackboardTooLong) {
			middleware.ErrorResponse(c, http.StatusRequestEntityTooLarge, 40040, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50030, "保存黑板失败")
		return
	}

	middleware.SuccessResponse(c, blackboard)
}

// Replies 获取某条消息的所有回复
func (h *MessageHandler) Replies(c *gin.Context) {
	convID := c.Param("id")
	messageID := c.Param("messageId")
	if convID == "" || messageID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40041, "缺少对话 ID 或消息 ID")
		return
	}

	replies, err := h.svc.GetReplies(c.Request.Context(), convID, messageID)
	if err != nil {
		if errors.Is(err, service.ErrMsgNotFound) {
			middleware.ErrorResponse(c, http.StatusNotFound, 40434, err.Error())
			return
		}
		if errors.Is(err, service.ErrMsgReplyWrongConv) {
			middleware.ErrorResponse(c, http.StatusBadRequest, 40043, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50031, "获取回复失败")
		return
	}

	if replies == nil {
		replies = []model.Message{}
	}
	middleware.SuccessResponse(c, replies)
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
