package service

import (
	"context"
	"errors"

	"github.com/agent-hub/backend/internal/model"
)

// UserProfileRepo 用户资料仓库接口（与 auth.UserRepo 分离，避免冲突）
type UserProfileRepo interface {
	GetUserByID(ctx context.Context, id string) (*model.User, error)
	UpdateUsername(ctx context.Context, id, username string) (*model.User, error)
}

var (
	ErrUsernameEmpty = errors.New("用户名不能为空")
)

// UserService 用户业务逻辑
type UserService struct {
	repo UserProfileRepo
}

// NewUserService 创建用户服务
func NewUserService(repo UserProfileRepo) *UserService {
	return &UserService{repo: repo}
}

// GetProfile 获取用户资料
func (s *UserService) GetProfile(ctx context.Context, userID string) (*model.User, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}
	return user, nil
}

// UpdateProfile 更新用户资料
func (s *UserService) UpdateProfile(ctx context.Context, userID, username string) (*model.User, error) {
	if username == "" {
		return nil, ErrUsernameEmpty
	}
	user, err := s.repo.UpdateUsername(ctx, userID, username)
	if err != nil {
		return nil, err
	}
	return user, nil
}
