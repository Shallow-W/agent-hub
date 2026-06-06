package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/agent-hub/backend/internal/model"
)

// UserProfileRepo 用户资料仓库接口（与 auth.UserRepo 分离，避免冲突）
type UserProfileRepo interface {
	GetUserByID(ctx context.Context, id string) (*model.User, error)
	UpdateUsername(ctx context.Context, id, username string) (*model.User, error)
	UpdateAvatar(ctx context.Context, id, avatar string) (*model.User, error)
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

// UpdateProfile 更新用户资料。username/avatar 均为可选（nil 表示不更新该字段），
// 仅更新当前登录用户自己。
func (s *UserService) UpdateProfile(ctx context.Context, userID string, username, avatar *string) (*model.User, error) {
	if username == nil && avatar == nil {
		return nil, fmt.Errorf("%w: 没有要更新的字段", ErrInvalidInput)
	}

	var user *model.User
	if username != nil {
		if *username == "" {
			return nil, ErrUsernameEmpty
		}
		if !usernamePattern.MatchString(*username) {
			return nil, fmt.Errorf("%w: 用户名只能包含字母、数字、下划线或中文", ErrInvalidInput)
		}
		updated, err := s.repo.UpdateUsername(ctx, userID, *username)
		if err != nil {
			return nil, err
		}
		if updated == nil {
			return nil, ErrUserNotFound
		}
		user = updated
	}

	if avatar != nil {
		updated, err := s.repo.UpdateAvatar(ctx, userID, *avatar)
		if err != nil {
			return nil, err
		}
		if updated == nil {
			return nil, ErrUserNotFound
		}
		user = updated
	}

	return user, nil
}

// UpdateAvatar 仅更新当前登录用户头像
func (s *UserService) UpdateAvatar(ctx context.Context, userID, avatar string) (*model.User, error) {
	user, err := s.repo.UpdateAvatar(ctx, userID, avatar)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}
	return user, nil
}
