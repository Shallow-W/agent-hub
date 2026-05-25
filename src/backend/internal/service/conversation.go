package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/agent-hub/backend/internal/model"
)

// ConversationService 对话服务所需的仓库接口
type ConvRepo interface {
	Create(ctx context.Context, userID, convType, title string) (*model.Conversation, error)
	ListByUserID(ctx context.Context, userID string, limit, offset int) ([]model.Conversation, error)
	GetByID(ctx context.Context, id string) (*model.Conversation, error)
	Delete(ctx context.Context, id string) error
	UpdatePinned(ctx context.Context, id string, pinned bool) error
	UpdateTimestamp(ctx context.Context, id string) error
}

var (
	ErrConvNotFound = errors.New("对话不存在")
	ErrConvNoPerm   = errors.New("无权操作此对话")
)

// ConversationService 对话业务逻辑
type ConversationService struct {
	repo ConvRepo
}

// NewConversationService 创建对话服务
func NewConversationService(repo ConvRepo) *ConversationService {
	return &ConversationService{repo: repo}
}

// CreateConversation 创建对话
func (s *ConversationService) CreateConversation(ctx context.Context, userID, convType, title string) (*model.Conversation, error) {
	if convType == "" {
		convType = "single"
	}
	conv, err := s.repo.Create(ctx, userID, convType, title)
	if err != nil {
		return nil, fmt.Errorf("create conversation: %w", err)
	}
	return conv, nil
}

// ListConversations 查询用户对话列表
func (s *ConversationService) ListConversations(ctx context.Context, userID string, limit, offset int) ([]model.Conversation, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	list, err := s.repo.ListByUserID(ctx, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	return list, nil
}

// DeleteConversation 删除对话（需验证所属权）
func (s *ConversationService) DeleteConversation(ctx context.Context, userID, convID string) error {
	conv, err := s.repo.GetByID(ctx, convID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return ErrConvNotFound
	}
	if conv.UserID != userID {
		return ErrConvNoPerm
	}
	return s.repo.Delete(ctx, convID)
}

// TogglePin 切换对话置顶状态
func (s *ConversationService) TogglePin(ctx context.Context, userID, convID string, pinned bool) error {
	conv, err := s.repo.GetByID(ctx, convID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return ErrConvNotFound
	}
	if conv.UserID != userID {
		return ErrConvNoPerm
	}
	return s.repo.UpdatePinned(ctx, convID, pinned)
}
