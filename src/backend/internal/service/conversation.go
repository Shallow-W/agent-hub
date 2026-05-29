package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"log/slog"

	"github.com/agent-hub/backend/internal/model"
)

// ConvFriendRepo 好友关系查询接口（仅好友校验所需方法）
type ConvFriendRepo interface {
	GetFriendship(ctx context.Context, userID, friendID string) (*model.Friend, error)
}

var (
	ErrSelfChat   = errors.New("不能与自己创建私聊")
	ErrNotFriends = errors.New("双方不是好友，无法创建私聊")
)

// ConversationService 对话服务所需的仓库接口
type ConvRepo interface {
	Create(ctx context.Context, userID, convType, title string) (*model.Conversation, error)
	ListByUserID(ctx context.Context, userID string, limit, offset int) ([]model.Conversation, error)
	ListArchivedByUserID(ctx context.Context, userID string, limit, offset int) ([]model.Conversation, error)
	GetByID(ctx context.Context, id string) (*model.Conversation, error)
	Delete(ctx context.Context, id string) error
	UpdatePinned(ctx context.Context, id string, pinned bool) error
	UpdateTimestamp(ctx context.Context, id string) error
	UpdateTitle(ctx context.Context, id, title string) error
	Archive(ctx context.Context, id string) error
	Unarchive(ctx context.Context, id string) error
	GetMember(ctx context.Context, conversationID, userID string) (*model.ConversationMember, error)
	DeleteMember(ctx context.Context, conversationID, userID string) error
	AddMember(ctx context.Context, conversationID, userID, role string) error
	FindPrivateChat(ctx context.Context, userID, friendID string) (*model.Conversation, error)
	CreatePrivateChat(ctx context.Context, userID, friendID, title string) (*model.Conversation, error)
	FindAgentChat(ctx context.Context, userID, agentID string) (*model.Conversation, error)
	CreateAgentChat(ctx context.Context, userID, agentID string) (*model.Conversation, error)
	ListAgents(ctx context.Context, conversationID, userID string) ([]model.ConversationAgent, error)
	AddAgent(ctx context.Context, conversationID, agentID, userID string) (*model.ConversationAgent, error)
	RemoveAgent(ctx context.Context, conversationID, agentID, userID string) (bool, error)
}

var (
	ErrConvNotFound     = errors.New("对话不存在")
	ErrConvNoPerm       = errors.New("无权操作此对话")
	ErrConvNotGroup     = errors.New("私聊会话不支持此操作")
	ErrConvNotMember    = errors.New("不是该会话成员")
	ErrConvInvalidTitle = errors.New("标题无效")
)

// ConversationService 对话业务逻辑
type ConversationService struct {
	repo       ConvRepo
	friendRepo ConvFriendRepo
}

// NewConversationService 创建对话服务
func NewConversationService(repo ConvRepo, friendRepo ConvFriendRepo) *ConversationService {
	return &ConversationService{repo: repo, friendRepo: friendRepo}
}

// CreateConversation 创建对话
func (s *ConversationService) CreateConversation(ctx context.Context, userID, convType, title string) (*model.Conversation, error) {
	if convType == "" {
		convType = "single"
	}
	if convType == "group" && strings.TrimSpace(title) == "" {
		return nil, fmt.Errorf("%w: 群聊标题不能为纯空格", ErrConvInvalidTitle)
	}
	conv, err := s.repo.Create(ctx, userID, convType, title)
	if err != nil {
		return nil, fmt.Errorf("create conversation: %w", err)
	}
	// 群聊需将创建者加入成员表
	if convType == "group" {
		if err := s.repo.AddMember(ctx, conv.ID, userID, "owner"); err != nil {
			if delErr := s.repo.Delete(ctx, conv.ID); delErr != nil {
				slog.Warn("compensating delete failed", "conversation_id", conv.ID, "error", delErr)
			}
			return nil, fmt.Errorf("add creator as member: %w", err)
		}
	}
	return conv, nil
}

// GetOrCreatePrivateChat 查找或创建两个用户之间的私聊会话
func (s *ConversationService) GetOrCreatePrivateChat(ctx context.Context, userID, friendID string) (*model.Conversation, error) {
	if userID == friendID {
		return nil, ErrSelfChat
	}

	// 校验好友关系（双向检查）
	friendship, err := s.friendRepo.GetFriendship(ctx, userID, friendID)
	if err != nil {
		return nil, fmt.Errorf("check friendship: %w", err)
	}
	if friendship == nil || friendship.Status != "accepted" {
		friendship2, err2 := s.friendRepo.GetFriendship(ctx, friendID, userID)
		if err2 != nil {
			return nil, fmt.Errorf("check friendship reverse: %w", err2)
		}
		if friendship2 == nil || friendship2.Status != "accepted" {
			return nil, ErrNotFriends
		}
	}

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

// GetOrCreateAgentChat 查找或创建当前用户和指定 Agent 的一对一会话。
func (s *ConversationService) GetOrCreateAgentChat(ctx context.Context, userID, agentID string) (*model.Conversation, error) {
	if strings.TrimSpace(agentID) == "" {
		return nil, ErrConvNotFound
	}
	conv, err := s.repo.FindAgentChat(ctx, userID, agentID)
	if err != nil {
		return nil, fmt.Errorf("find agent chat: %w", err)
	}
	if conv != nil {
		return conv, nil
	}
	conv, err = s.repo.CreateAgentChat(ctx, userID, agentID)
	if err != nil {
		return nil, fmt.Errorf("create agent chat: %w", err)
	}
	if conv == nil {
		return nil, ErrConvNotFound
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
	if offset < 0 {
		offset = 0
	}
	list, err := s.repo.ListByUserID(ctx, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	if list == nil {
		list = []model.Conversation{}
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

	// 创建者直接删除整条会话，覆盖空白新对话和 Agent 单聊没有成员记录的场景。
	if conv.UserID == userID {
		return s.repo.Delete(ctx, convID)
	}

	member, err := s.repo.GetMember(ctx, convID, userID)
	if err != nil {
		return fmt.Errorf("get member: %w", err)
	}
	if member == nil {
		return ErrConvNotMember
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
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("%w: 标题不能为纯空格", ErrConvInvalidTitle)
	}
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
	// 私聊会话：任何成员都可归档
	if conv.Type == "single" {
		if conv.UserID == userID {
			return s.repo.Archive(ctx, conversationID)
		}
		member, err := s.repo.GetMember(ctx, conversationID, userID)
		if err != nil {
			return fmt.Errorf("check member: %w", err)
		}
		if member == nil {
			return ErrConvNotMember
		}
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

// ListArchivedConversations 查询用户已归档的对话列表
func (s *ConversationService) ListArchivedConversations(ctx context.Context, userID string, limit, offset int) ([]model.Conversation, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	list, err := s.repo.ListArchivedByUserID(ctx, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list archived conversations: %w", err)
	}
	if list == nil {
		list = []model.Conversation{}
	}
	return list, nil
}

// UnarchiveConversation 取消归档会话
func (s *ConversationService) UnarchiveConversation(ctx context.Context, userID, conversationID string) error {
	conv, err := s.repo.GetByID(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return ErrConvNotFound
	}
	// 私聊会话：任何成员都可取消归档
	if conv.Type == "single" {
		if conv.UserID == userID {
			return s.repo.Unarchive(ctx, conversationID)
		}
		member, err := s.repo.GetMember(ctx, conversationID, userID)
		if err != nil {
			return fmt.Errorf("check member: %w", err)
		}
		if member == nil {
			return ErrConvNotMember
		}
		return s.repo.Unarchive(ctx, conversationID)
	}
	// 群聊需要 owner/admin 权限
	member, err := s.repo.GetMember(ctx, conversationID, userID)
	if err != nil {
		return fmt.Errorf("check member: %w", err)
	}
	if member == nil || (member.Role != "owner" && member.Role != "admin") {
		return ErrConvNotMember
	}
	return s.repo.Unarchive(ctx, conversationID)
}

// ListConversationAgents 查询当前对话里已加入的 Robot。
func (s *ConversationService) ListConversationAgents(ctx context.Context, userID, convID string) ([]model.ConversationAgent, error) {
	conv, err := s.repo.GetByID(ctx, convID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return nil, ErrConvNotFound
	}
	if conv.UserID != userID {
		member, err := s.repo.GetMember(ctx, convID, userID)
		if err != nil {
			return nil, fmt.Errorf("get member: %w", err)
		}
		if member == nil {
			return nil, ErrConvNoPerm
		}
	}
	list, err := s.repo.ListAgents(ctx, convID, userID)
	if err != nil {
		return nil, fmt.Errorf("list conversation agents: %w", err)
	}
	return list, nil
}

func (s *ConversationService) canManageConversationAgents(ctx context.Context, userID string, conv *model.Conversation) error {
	if conv.Type != "group" {
		return ErrConvNotGroup
	}
	if conv.UserID == userID {
		return nil
	}
	member, err := s.repo.GetMember(ctx, conv.ID, userID)
	if err != nil {
		return fmt.Errorf("get member: %w", err)
	}
	if member == nil {
		return ErrConvNoPerm
	}
	if member.Role == "owner" || member.Role == "admin" {
		return nil
	}
	return ErrConvNoPerm
}

// AddConversationAgent 把 Robot 加入当前对话。
func (s *ConversationService) AddConversationAgent(ctx context.Context, userID, convID, agentID string) (*model.ConversationAgent, error) {
	if convID == "" || agentID == "" {
		return nil, ErrConvNotFound
	}
	conv, err := s.repo.GetByID(ctx, convID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return nil, ErrConvNotFound
	}
	if err := s.canManageConversationAgents(ctx, userID, conv); err != nil {
		return nil, err
	}
	item, err := s.repo.AddAgent(ctx, convID, agentID, userID)
	if err != nil {
		return nil, fmt.Errorf("add conversation agent: %w", err)
	}
	if item == nil {
		return nil, ErrConvNotFound
	}
	return item, nil
}

// RemoveConversationAgent 从当前对话移除 Robot。
func (s *ConversationService) RemoveConversationAgent(ctx context.Context, userID, convID, agentID string) error {
	conv, err := s.repo.GetByID(ctx, convID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return ErrConvNotFound
	}
	if err := s.canManageConversationAgents(ctx, userID, conv); err != nil {
		return err
	}
	ok, err := s.repo.RemoveAgent(ctx, convID, agentID, userID)
	if err != nil {
		return fmt.Errorf("remove conversation agent: %w", err)
	}
	if !ok {
		return ErrConvNotFound
	}
	return nil
}
