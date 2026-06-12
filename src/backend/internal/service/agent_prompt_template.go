package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agent-hub/backend/internal/model"
)

var (
	ErrAgentPromptTemplateNotFound  = errors.New("Agent Prompt 模板不存在")
	ErrAgentPromptTemplateInvalid   = errors.New("Agent Prompt 模板参数无效")
	ErrAgentPromptTemplateDuplicate = errors.New("Agent Prompt 模板名称已存在")
)

// AgentPromptTemplateRepo 是 AgentPromptTemplateService 依赖的仓库接口。
type AgentPromptTemplateRepo interface {
	ListByUser(ctx context.Context, userID string) ([]model.AgentPromptTemplate, error)
	Create(ctx context.Context, userID, name, category, description, systemPrompt string) (*model.AgentPromptTemplate, error)
	Update(ctx context.Context, id, userID, name, category, description, systemPrompt string) (*model.AgentPromptTemplate, error)
	Delete(ctx context.Context, id, userID string) (bool, error)
}

// AgentPromptTemplateCatalogItem is the catalog-package-neutral representation
// of one catalog.Item for the agent_prompt_template domain. Declared locally so
// the service package doesn't need to import internal/catalog.
type AgentPromptTemplateCatalogItem struct {
	ID          string
	UserID      string
	Name        string
	Category    string
	Description string
	SystemPrompt string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// AgentPromptTemplateCatalogStore is the subset of catalog.Service consumed by
// AgentPromptTemplateService. Wire an implementation at composition time; when
// nil, the service falls back to repo.
type AgentPromptTemplateCatalogStore interface {
	ListAgentPromptTemplates(ctx context.Context, userID string) ([]AgentPromptTemplateCatalogItem, error)
	CreateAgentPromptTemplate(ctx context.Context, userID, name, category, description, systemPrompt string) (*AgentPromptTemplateCatalogItem, error)
	UpdateAgentPromptTemplate(ctx context.Context, id, userID, name, category, description, systemPrompt string) (*AgentPromptTemplateCatalogItem, error)
	DeleteAgentPromptTemplate(ctx context.Context, id, userID string) error
}

type AgentPromptTemplateService struct {
	repo    AgentPromptTemplateRepo
	catalog AgentPromptTemplateCatalogStore
}

func NewAgentPromptTemplateService(repo AgentPromptTemplateRepo) *AgentPromptTemplateService {
	return &AgentPromptTemplateService{repo: repo}
}

func (s *AgentPromptTemplateService) SetCatalogStore(store AgentPromptTemplateCatalogStore) {
	s.catalog = store
}

func (s *AgentPromptTemplateService) List(ctx context.Context, userID string) ([]model.AgentPromptTemplate, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, ErrAgentPromptTemplateInvalid
	}
	if s.catalog != nil {
		items, err := s.catalog.ListAgentPromptTemplates(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("list agent prompt templates via catalog: %w", err)
		}
		out := make([]model.AgentPromptTemplate, 0, len(items))
		for _, it := range items {
			out = append(out, catalogItemToAgentPromptTemplate(it))
		}
		return out, nil
	}
	return nil, fmt.Errorf("list agent prompt templates: catalog store not configured")
}

func (s *AgentPromptTemplateService) ImportDefaults(ctx context.Context, userID string) ([]model.AgentPromptTemplate, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, ErrAgentPromptTemplateInvalid
	}
	imported := make([]model.AgentPromptTemplate, 0, len(DefaultAgentPromptTemplates()))
	for _, tpl := range DefaultAgentPromptTemplates() {
		saved, err := s.Create(ctx, userID, tpl.Name, tpl.Category, tpl.Description, tpl.SystemPrompt)
		if err != nil {
			if errors.Is(err, ErrAgentPromptTemplateDuplicate) {
				continue
			}
			return nil, err
		}
		imported = append(imported, *saved)
	}
	return imported, nil
}

func (s *AgentPromptTemplateService) Create(ctx context.Context, userID, name, category, description, systemPrompt string) (*model.AgentPromptTemplate, error) {
	name, category, description, systemPrompt, err := normalizeAgentPromptTemplateFields(userID, name, category, description, systemPrompt)
	if err != nil {
		return nil, err
	}
	if s.catalog != nil {
		it, cErr := s.catalog.CreateAgentPromptTemplate(ctx, userID, name, category, description, systemPrompt)
		if cErr != nil {
			return nil, cErr
		}
		m := catalogItemToAgentPromptTemplate(*it)
		return &m, nil
	}
	return nil, fmt.Errorf("create agent prompt template: catalog store not configured")
}

func (s *AgentPromptTemplateService) Update(ctx context.Context, id, userID, name, category, description, systemPrompt string) (*model.AgentPromptTemplate, error) {
	if strings.TrimSpace(id) == "" {
		return nil, ErrAgentPromptTemplateInvalid
	}
	name, category, description, systemPrompt, err := normalizeAgentPromptTemplateFields(userID, name, category, description, systemPrompt)
	if err != nil {
		return nil, err
	}
	if s.catalog != nil {
		it, cErr := s.catalog.UpdateAgentPromptTemplate(ctx, id, userID, name, category, description, systemPrompt)
		if cErr != nil {
			return nil, cErr
		}
		if it == nil {
			return nil, ErrAgentPromptTemplateNotFound
		}
		m := catalogItemToAgentPromptTemplate(*it)
		return &m, nil
	}
	return nil, fmt.Errorf("update agent prompt template: catalog store not configured")
}

func (s *AgentPromptTemplateService) Delete(ctx context.Context, id, userID string) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(userID) == "" {
		return ErrAgentPromptTemplateInvalid
	}
	if s.catalog != nil {
		if err := s.catalog.DeleteAgentPromptTemplate(ctx, id, userID); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("delete agent prompt template: catalog store not configured")
}

func normalizeAgentPromptTemplateFields(userID, name, category, description, systemPrompt string) (string, string, string, string, error) {
	name = strings.TrimSpace(name)
	if strings.TrimSpace(userID) == "" || name == "" {
		return "", "", "", "", ErrAgentPromptTemplateInvalid
	}
	category = strings.TrimSpace(category)
	if category == "" {
		category = "通用"
	}
	return truncateString(name, 80),
		truncateString(category, 60),
		truncateString(strings.TrimSpace(description), 200),
		truncateString(strings.TrimSpace(systemPrompt), 8000),
		nil
}

func catalogItemToAgentPromptTemplate(it AgentPromptTemplateCatalogItem) model.AgentPromptTemplate {
	return model.AgentPromptTemplate{
		ID:           it.ID,
		UserID:       it.UserID,
		Name:         it.Name,
		Category:     it.Category,
		Description:  it.Description,
		SystemPrompt: it.SystemPrompt,
		CreatedAt:    it.CreatedAt,
		UpdatedAt:    it.UpdatedAt,
	}
}
