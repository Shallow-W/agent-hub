package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/agent-hub/backend/internal/model"
)

var (
	ErrUserTemplateNotFound  = errors.New("模板不存在")
	ErrUserTemplateInvalid   = errors.New("模板参数无效")
	ErrUserTemplateDuplicate = errors.New("模板名称已存在")
)

var validTemplateTypes = map[string]bool{"tools": true, "skills": true}

// UserTemplateRepo 是 UserTemplateService 依赖的仓库接口。
type UserTemplateRepo interface {
	ListByUserAndType(ctx context.Context, userID, tplType string) ([]model.UserTemplate, error)
	Create(ctx context.Context, userID, tplType, name, content string) (*model.UserTemplate, error)
	Update(ctx context.Context, id, userID, name, content string) (*model.UserTemplate, error)
	Delete(ctx context.Context, id, userID string) (bool, error)
}

type UserTemplateService struct {
	repo UserTemplateRepo
}

func NewUserTemplateService(repo UserTemplateRepo) *UserTemplateService {
	return &UserTemplateService{repo: repo}
}

func (s *UserTemplateService) List(ctx context.Context, userID, tplType string) ([]model.UserTemplate, error) {
	if strings.TrimSpace(userID) == "" || !validTemplateTypes[tplType] {
		return nil, ErrUserTemplateInvalid
	}
	list, err := s.repo.ListByUserAndType(ctx, userID, tplType)
	if err != nil {
		return nil, fmt.Errorf("list user templates: %w", err)
	}
	if list == nil {
		return []model.UserTemplate{}, nil
	}
	return list, nil
}

func (s *UserTemplateService) Create(ctx context.Context, userID, tplType, name string, content interface{}) (*model.UserTemplate, error) {
	name = strings.TrimSpace(name)
	if strings.TrimSpace(userID) == "" || name == "" || !validTemplateTypes[tplType] {
		return nil, ErrUserTemplateInvalid
	}
	name = truncateString(name, 100)
	contentJSON, err := json.Marshal(content)
	if err != nil {
		return nil, ErrUserTemplateInvalid
	}
	tpl, err := s.repo.Create(ctx, userID, tplType, name, string(contentJSON))
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrUserTemplateDuplicate
		}
		return nil, fmt.Errorf("create user template: %w", err)
	}
	return tpl, nil
}

func (s *UserTemplateService) Update(ctx context.Context, id, userID, name string, content interface{}) (*model.UserTemplate, error) {
	name = strings.TrimSpace(name)
	if strings.TrimSpace(id) == "" || strings.TrimSpace(userID) == "" || name == "" {
		return nil, ErrUserTemplateInvalid
	}
	name = truncateString(name, 100)
	contentJSON, err := json.Marshal(content)
	if err != nil {
		return nil, ErrUserTemplateInvalid
	}
	tpl, err := s.repo.Update(ctx, id, userID, name, string(contentJSON))
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrUserTemplateDuplicate
		}
		return nil, fmt.Errorf("update user template: %w", err)
	}
	if tpl == nil {
		return nil, ErrUserTemplateNotFound
	}
	return tpl, nil
}

func (s *UserTemplateService) Delete(ctx context.Context, id, userID string) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(userID) == "" {
		return ErrUserTemplateInvalid
	}
	deleted, err := s.repo.Delete(ctx, id, userID)
	if err != nil {
		return fmt.Errorf("delete user template: %w", err)
	}
	if !deleted {
		return ErrUserTemplateNotFound
	}
	return nil
}
