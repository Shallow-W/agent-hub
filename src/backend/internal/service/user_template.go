package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

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

// UserTemplateCatalogItem is the catalog-package-neutral representation of one
// catalog.Item for the user_template domain.
type UserTemplateCatalogItem struct {
	ID        string
	UserID    string
	Type      string
	Name      string
	Content   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// UserTemplateCatalogStore is the subset of catalog.Service consumed by
// UserTemplateService.
type UserTemplateCatalogStore interface {
	ListUserTemplates(ctx context.Context, userID, tplType string) ([]UserTemplateCatalogItem, error)
	CreateUserTemplate(ctx context.Context, userID, tplType, name, content string) (*UserTemplateCatalogItem, error)
	UpdateUserTemplate(ctx context.Context, id, userID, name, content string) (*UserTemplateCatalogItem, error)
	DeleteUserTemplate(ctx context.Context, id, userID string) error
}

type UserTemplateService struct {
	repo    UserTemplateRepo
	catalog UserTemplateCatalogStore
}

func NewUserTemplateService(repo UserTemplateRepo) *UserTemplateService {
	return &UserTemplateService{repo: repo}
}

func (s *UserTemplateService) SetCatalogStore(store UserTemplateCatalogStore) {
	s.catalog = store
}

func (s *UserTemplateService) List(ctx context.Context, userID, tplType string) ([]model.UserTemplate, error) {
	if strings.TrimSpace(userID) == "" || !validTemplateTypes[tplType] {
		return nil, ErrUserTemplateInvalid
	}
	if s.catalog != nil {
		items, err := s.catalog.ListUserTemplates(ctx, userID, tplType)
		if err != nil {
			return nil, fmt.Errorf("list user templates via catalog: %w", err)
		}
		out := make([]model.UserTemplate, 0, len(items))
		for _, it := range items {
			out = append(out, catalogItemToUserTemplate(it))
		}
		return out, nil
	}
	return nil, fmt.Errorf("list user templates: catalog store not configured")
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
	if s.catalog != nil {
		it, cErr := s.catalog.CreateUserTemplate(ctx, userID, tplType, name, string(contentJSON))
		if cErr != nil {
			return nil, cErr
		}
		m := catalogItemToUserTemplate(*it)
		return &m, nil
	}
	return nil, fmt.Errorf("create user template: catalog store not configured")
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
	if s.catalog != nil {
		it, cErr := s.catalog.UpdateUserTemplate(ctx, id, userID, name, string(contentJSON))
		if cErr != nil {
			return nil, cErr
		}
		if it == nil {
			return nil, ErrUserTemplateNotFound
		}
		m := catalogItemToUserTemplate(*it)
		return &m, nil
	}
	return nil, fmt.Errorf("update user template: catalog store not configured")
}

func (s *UserTemplateService) Delete(ctx context.Context, id, userID string) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(userID) == "" {
		return ErrUserTemplateInvalid
	}
	if s.catalog != nil {
		if err := s.catalog.DeleteUserTemplate(ctx, id, userID); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("delete user template: catalog store not configured")
}

func catalogItemToUserTemplate(it UserTemplateCatalogItem) model.UserTemplate {
	return model.UserTemplate{
		ID:        it.ID,
		UserID:    it.UserID,
		Type:      it.Type,
		Name:      it.Name,
		Content:   []byte(it.Content),
		CreatedAt: it.CreatedAt,
		UpdatedAt: it.UpdatedAt,
	}
}
