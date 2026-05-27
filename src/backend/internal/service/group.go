package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/agent-hub/backend/internal/model"
)

// GroupRepo 群聊服务所需的仓库接口
type GroupRepo interface {
	CreateGroup(ctx context.Context, ownerID, name string, memberIDs []string) (*model.Conversation, error)
	AddMember(ctx context.Context, conversationID, userID, role string) error
	RemoveMember(ctx context.Context, conversationID, userID string) error
	ListMembers(ctx context.Context, conversationID string) ([]*model.ConversationMember, error)
	GetMember(ctx context.Context, conversationID, userID string) (*model.ConversationMember, error)
	IsMember(ctx context.Context, conversationID, userID string) (bool, error)
	GetConversationByID(ctx context.Context, id string) (*model.Conversation, error)
	GetUserByID(ctx context.Context, id string) (*model.User, error)
}

var (
	ErrNotOwner      = errors.New("只有群主才能执行此操作")
	ErrNotAdmin      = errors.New("需要管理员权限")
	ErrAlreadyMember = errors.New("用户已是群成员")
	ErrGroupNotFound = errors.New("群不存在")
	ErrNotMember     = errors.New("用户不是群成员")
	ErrOwnerLeave    = errors.New("群主不能离开群聊，请先转让群主")
)

// GroupService 群聊业务逻辑
type GroupService struct {
	repo GroupRepo
}

// NewGroupService 创建群聊服务
func NewGroupService(repo GroupRepo) *GroupService {
	return &GroupService{repo: repo}
}

// IsConversationMember 校验用户是否为会话成员（实现 MemberChecker 接口）
func (s *GroupService) IsConversationMember(ctx context.Context, conversationID, userID string) (bool, error) {
	return s.repo.IsMember(ctx, conversationID, userID)
}

// CreateGroup 创建群聊
func (s *GroupService) CreateGroup(ctx context.Context, ownerID, name string, memberIDs []string) (*model.Conversation, error) {
	if name == "" {
		return nil, errors.New("群名不能为空")
	}

	// memberIDs 去重且排除 owner
	deduped := dedupMembers(memberIDs, ownerID)

	conv, err := s.repo.CreateGroup(ctx, ownerID, name, deduped)
	if err != nil {
		return nil, fmt.Errorf("create group: %w", err)
	}
	return conv, nil
}

// AddMember 添加群成员
func (s *GroupService) AddMember(ctx context.Context, conversationID, operatorID, targetUserID, role string) error {
	// 验证请求者是 owner/admin
	member, err := s.repo.GetMember(ctx, conversationID, operatorID)
	if err != nil {
		return fmt.Errorf("check operator: %w", err)
	}
	if member == nil {
		return ErrNotMember
	}
	if member.Role != "owner" && member.Role != "admin" {
		return ErrNotAdmin
	}

	// 验证目标用户存在
	user, err := s.repo.GetUserByID(ctx, targetUserID)
	if err != nil {
		return fmt.Errorf("check target user: %w", err)
	}
	if user == nil {
		return ErrUserNotFound
	}

	// 验证未被添加
	isMember, err := s.repo.IsMember(ctx, conversationID, targetUserID)
	if err != nil {
		return fmt.Errorf("check existing member: %w", err)
	}
	if isMember {
		return ErrAlreadyMember
	}

	if err := s.repo.AddMember(ctx, conversationID, targetUserID, role); err != nil {
		return fmt.Errorf("add member: %w", err)
	}
	return nil
}

// RemoveMember 移除群成员
func (s *GroupService) RemoveMember(ctx context.Context, conversationID, operatorID, targetUserID string) error {
	// 验证请求者是 owner/admin
	member, err := s.repo.GetMember(ctx, conversationID, operatorID)
	if err != nil {
		return fmt.Errorf("check operator: %w", err)
	}
	if member == nil {
		return ErrNotMember
	}
	if member.Role != "owner" && member.Role != "admin" {
		return ErrNotAdmin
	}

	// 不能移除 owner
	target, err := s.repo.GetMember(ctx, conversationID, targetUserID)
	if err != nil {
		return fmt.Errorf("check target member: %w", err)
	}
	if target == nil {
		return ErrNotMember
	}
	if target.Role == "owner" {
		return ErrNotOwner
	}

	if err := s.repo.RemoveMember(ctx, conversationID, targetUserID); err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	return nil
}

// ListMembers 列出群成员（验证调用者是否为成员）
func (s *GroupService) ListMembers(ctx context.Context, conversationID, userID string) ([]*model.ConversationMember, error) {
	isMember, err := s.repo.IsMember(ctx, conversationID, userID)
	if err != nil {
		return nil, fmt.Errorf("check membership: %w", err)
	}
	if !isMember {
		return nil, ErrNotMember
	}

	list, err := s.repo.ListMembers(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	return list, nil
}

// LeaveGroup 离开群聊
func (s *GroupService) LeaveGroup(ctx context.Context, conversationID, userID string) error {
	member, err := s.repo.GetMember(ctx, conversationID, userID)
	if err != nil {
		return fmt.Errorf("check member: %w", err)
	}
	if member == nil {
		return ErrNotMember
	}
	if member.Role == "owner" {
		return ErrOwnerLeave
	}

	if err := s.repo.RemoveMember(ctx, conversationID, userID); err != nil {
		return fmt.Errorf("leave group: %w", err)
	}
	return nil
}

// GetGroupInfo 获取群信息+成员列表（验证调用者是否为成员）
func (s *GroupService) GetGroupInfo(ctx context.Context, conversationID, userID string) (*model.Conversation, []*model.ConversationMember, error) {
	conv, err := s.repo.GetConversationByID(ctx, conversationID)
	if err != nil {
		return nil, nil, fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return nil, nil, ErrGroupNotFound
	}

	isMember, err := s.repo.IsMember(ctx, conversationID, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("check membership: %w", err)
	}
	if !isMember {
		return nil, nil, ErrNotMember
	}

	members, err := s.repo.ListMembers(ctx, conversationID)
	if err != nil {
		return nil, nil, fmt.Errorf("list members: %w", err)
	}
	return conv, members, nil
}

// dedupMembers 去重并排除 ownerID
func dedupMembers(ids []string, exclude string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == exclude {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}
