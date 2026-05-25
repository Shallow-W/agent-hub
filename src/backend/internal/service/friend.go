package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/agent-hub/backend/internal/model"
)

// FriendRepo 好友服务所需的仓库接口
type FriendRepo interface {
	SendRequest(ctx context.Context, userID, friendID string) (*model.Friend, error)
	AcceptRequest(ctx context.Context, userID, friendID string) error
	CreateReverseFriendship(ctx context.Context, userID, friendID string) error
	RejectRequest(ctx context.Context, userID, friendID string) error
	ListFriends(ctx context.Context, userID string) ([]*model.Friend, error)
	ListPendingRequests(ctx context.Context, userID string) ([]*model.Friend, error)
	GetFriendship(ctx context.Context, userID, friendID string) (*model.Friend, error)
	GetFriendshipByID(ctx context.Context, id string) (*model.Friend, error)
	GetUserByUsername(ctx context.Context, username string) (*model.User, error)
	GetUserByID(ctx context.Context, id string) (*model.User, error)
}

var (
	ErrFriendSelf     = errors.New("不能添加自己为好友")
	ErrFriendExists   = errors.New("好友关系已存在")
	ErrFriendNotFound = errors.New("好友申请不存在")
	ErrUserNotFound   = errors.New("用户不存在")
)

// FriendService 好友业务逻辑
type FriendService struct {
	repo FriendRepo
}

// NewFriendService 创建好友服务
func NewFriendService(repo FriendRepo) *FriendService {
	return &FriendService{repo: repo}
}

// SendFriendRequest 发送好友申请
func (s *FriendService) SendFriendRequest(ctx context.Context, userID, friendID string) (*model.Friend, error) {
	if userID == friendID {
		return nil, ErrFriendSelf
	}

	// 检查对方用户是否存在
	target, err := s.repo.GetUserByID(ctx, friendID)
	if err != nil {
		return nil, fmt.Errorf("check target user: %w", err)
	}
	if target == nil {
		return nil, ErrUserNotFound
	}

	// 检查是否已有好友关系（双向检查）
	existing, err := s.repo.GetFriendship(ctx, userID, friendID)
	if err != nil {
		return nil, fmt.Errorf("check existing friendship: %w", err)
	}
	if existing != nil {
		return nil, ErrFriendExists
	}

	friend, err := s.repo.SendRequest(ctx, userID, friendID)
	if err != nil {
		return nil, fmt.Errorf("send friend request: %w", err)
	}
	return friend, nil
}

// ResolveFriendID 根据好友 ID 或用户名查找目标用户 ID
func (s *FriendService) ResolveFriendID(ctx context.Context, friendID, username string) (string, error) {
	if friendID != "" {
		return friendID, nil
	}
	if username == "" {
		return "", errors.New("必须提供 friend_id 或 username")
	}
	user, err := s.repo.GetUserByUsername(ctx, username)
	if err != nil {
		return "", fmt.Errorf("lookup user by username: %w", err)
	}
	if user == nil {
		return "", ErrUserNotFound
	}
	return user.ID, nil
}

// AcceptFriendRequest 接受好友申请
func (s *FriendService) AcceptFriendRequest(ctx context.Context, userID, requestID string) error {
	// requestID 是好友记录的 ID，需要找到对应记录
	// 通过 friend_id = userID 且 id = requestID 来定位
	friend, err := s.repo.GetFriendshipByID(ctx, requestID)
	if err != nil {
		return fmt.Errorf("get friend request: %w", err)
	}
	if friend == nil || friend.FriendID != userID || friend.Status != "pending" {
		return ErrFriendNotFound
	}

	// 接受申请
	if err := s.repo.AcceptRequest(ctx, friend.UserID, userID); err != nil {
		return fmt.Errorf("accept request: %w", err)
	}

	// 创建反向好友关系
	if err := s.repo.CreateReverseFriendship(ctx, userID, friend.UserID); err != nil {
		return fmt.Errorf("create reverse friendship: %w", err)
	}
	return nil
}

// RejectFriendRequest 拒绝好友申请
func (s *FriendService) RejectFriendRequest(ctx context.Context, userID, requestID string) error {
	friend, err := s.repo.GetFriendshipByID(ctx, requestID)
	if err != nil {
		return fmt.Errorf("get friend request: %w", err)
	}
	if friend == nil || friend.FriendID != userID || friend.Status != "pending" {
		return ErrFriendNotFound
	}

	if err := s.repo.RejectRequest(ctx, friend.UserID, userID); err != nil {
		return fmt.Errorf("reject request: %w", err)
	}
	return nil
}

// ListFriends 查询好友列表
func (s *FriendService) ListFriends(ctx context.Context, userID string) ([]*model.Friend, error) {
	list, err := s.repo.ListFriends(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list friends: %w", err)
	}
	return list, nil
}

// ListPending 查询收到的好友申请
func (s *FriendService) ListPending(ctx context.Context, userID string) ([]*model.Friend, error) {
	list, err := s.repo.ListPendingRequests(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list pending requests: %w", err)
	}
	return list, nil
}
