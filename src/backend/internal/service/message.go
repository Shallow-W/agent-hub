package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/agent-hub/backend/internal/model"
)

// MessageService 消息服务所需的仓库接口
type MsgRepo interface {
	Create(ctx context.Context, conversationID, role, content, artifactsJSON string) (*model.Message, error)
	ListByConversation(ctx context.Context, conversationID string, before time.Time, limit int) ([]model.Message, error)
	MarkConversationRead(ctx context.Context, conversationID, userID string) error
}

// ConvRepoForMsg 消息服务需要的对话仓库接口（用于权限校验和更新时间戳）
type ConvRepoForMsg interface {
	GetByID(ctx context.Context, id string) (*model.Conversation, error)
	UpdateTimestamp(ctx context.Context, id string) error
		GetMember(ctx context.Context, conversationID, userID string) (*model.ConversationMember, error)
}

var (
	ErrMsgConvNotFound = errors.New("对话不存在")
	ErrMsgConvNoPerm   = errors.New("无权操作此对话")
)

// MessageService 消息业务逻辑
type MessageService struct {
	msgRepo  MsgRepo
	convRepo ConvRepoForMsg
}

// NewMessageService 创建消息服务
func NewMessageService(msgRepo MsgRepo, convRepo ConvRepoForMsg) *MessageService {
	return &MessageService{msgRepo: msgRepo, convRepo: convRepo}
}

// SendMessage 发送消息并刷新对话时间戳
func (s *MessageService) SendMessage(ctx context.Context, convID, userID, role, content, artifactsJSON string) (*model.Message, error) {
	// 校验对话存在且属于当前用户
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return nil, ErrMsgConvNotFound
	}
	if conv.UserID != userID {
		return nil, ErrMsgConvNoPerm
	}

	if role == "" {
		role = "user"
	}

	msg, err := s.msgRepo.Create(ctx, convID, role, content, artifactsJSON)
	if err != nil {
		return nil, fmt.Errorf("create message: %w", err)
	}

	return msg, nil
}

// MarkAsRead 标记会话消息已读
func (s *MessageService) MarkAsRead(ctx context.Context, userID, convID string) error {
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return ErrMsgConvNotFound
	}
	// 验证是会话成员（不限制为创建者）
	member, err := s.convRepo.GetMember(ctx, convID, userID)
	if err != nil {
		return fmt.Errorf("check member: %w", err)
	}
	if member == nil {
		return ErrMsgConvNoPerm
	}
	return s.msgRepo.MarkConversationRead(ctx, convID, userID)
}

// GetHistory 获取对话消息历史，支持 before 游标分页
func (s *MessageService) GetHistory(ctx context.Context, convID, userID string, before time.Time, limit int) ([]model.Message, error) {
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return nil, ErrMsgConvNotFound
	}
	if conv.UserID != userID {
		return nil, ErrMsgConvNoPerm
	}

	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	messages, err := s.msgRepo.ListByConversation(ctx, convID, before, limit)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	return messages, nil
}
