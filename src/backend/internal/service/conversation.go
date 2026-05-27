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
	UpdateTitle(ctx context.Context, id, title string) error
	Archive(ctx context.Context, id string) error
	GetMember(ctx context.Context, conversationID, userID string) (*model.ConversationMember, error)
	DeleteMember(ctx context.Context, conversationID, userID string) error
	FindPrivateChat(ctx context.Context, userID, friendID string) (*model.Conversation, error)
	CreatePrivateChat(ctx context.Context, userID, friendID, title string) (*model.Conversation, error)
}

var (
	ErrConvNotFound   = errors.New("对话不存在")
	ErrConvNoPerm     = errors.New("无权操作此对话")
	ErrConvNotGroup   = errors.New("私聊会话不支持此操作")
	ErrConvNotMember  = errors.New("不是该会话成员")
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

// GetOrCreatePrivateChat 查找或创建两个用户之间的私聊会话
func (s *ConversationService) GetOrCreatePrivateChat(ctx context.Context, userID, friendID string) (*model.Conversation, error) {
	// 先尝试查找已有的私聊
	conv, err := s.repo.FindPrivateChat(ctx, userID, friendID)
	if err != nil {
		return nil, fmt.Errorf("find private chat: %w", err)
	}
	if conv != nil {
		return conv, nil
	}
	// 不存在则创建
	return s.repo.CreatePrivateChat(ctx, userID, friendID, "私聊")
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

// DeleteConversation 删除当前用户在私聊会话中的成员记录
func (s *ConversationService) DeleteConversation(ctx context.Context, userID, convID string) error {
	conv, err := s.repo.GetByID(ctx, convID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return ErrConvNotFound
	}
	if conv.Type != "single" {
		return ErrConvNotGroup
	}
	return s.repo.DeleteMember(ctx, convID, userID)
}

// TogglePin 切换对话置顶状态（需要 owner/admin 权限）
func (s *ConversationService) TogglePin(ctx context.Context, userID, convID string) error {
	conv, err := s.repo.GetByID(ctx, convID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return ErrConvNotFound
	}
	// 私聊会话 owner 可操作
	if conv.Type == "single" && conv.UserID == userID {
		return s.repo.UpdatePinned(ctx, convID, !conv.Pinned)
	}
	// 群聊需要 owner/admin 权限
	member, err := s.repo.GetMember(ctx, convID, userID)
	if err != nil {
		return fmt.Errorf("get member: %w", err)
	}
	if member == nil || (member.Role != "owner" && member.Role != "admin") {
		return ErrConvNoPerm
	}
	return s.repo.UpdatePinned(ctx, convID, !conv.Pinned)
}

// RenameConversation 重命名会话（仅 group 类型，操作者需为 owner/admin）
func (s *ConversationService) RenameConversation(ctx context.Context, userID, conversationID, title string) error {
	conv, err := s.repo.GetByID(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return ErrConvNotFound
	}
	if conv.Type != "group" {
		return ErrConvNotGroup
	}
	member, err := s.repo.GetMember(ctx, conversationID, userID)
	if err != nil {
		return fmt.Errorf("get member: %w", err)
	}
	if member == nil {
		return ErrConvNotMember
	}
	if member.Role != "owner" && member.Role != "admin" {
		return ErrConvNoPerm
	}
	return s.repo.UpdateTitle(ctx, conversationID, title)
}

// ArchiveConversation 归档会话（软删除，需要 owner/admin 权限）
func (s *ConversationService) ArchiveConversation(ctx context.Context, userID, conversationID string) error {
	conv, err := s.repo.GetByID(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return ErrConvNotFound
	}
	// 私聊会话 owner 可操作
	if conv.Type == "single" && conv.UserID == userID {
		return s.repo.Archive(ctx, conversationID)
	}
	// 群聊需要 owner/admin 权限
	member, err := s.repo.GetMember(ctx, conversationID, userID)
	if err != nil {
		return fmt.Errorf("check member: %w", err)
	}
	if member == nil || (member.Role != "owner" && member.Role != "admin") {
		return ErrConvNotMember
	}
	return s.repo.Archive(ctx, conversationID)
}
