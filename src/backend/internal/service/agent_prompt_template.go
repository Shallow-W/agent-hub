package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

type AgentPromptTemplateService struct {
	repo AgentPromptTemplateRepo
}

func NewAgentPromptTemplateService(repo AgentPromptTemplateRepo) *AgentPromptTemplateService {
	return &AgentPromptTemplateService{repo: repo}
}

func (s *AgentPromptTemplateService) List(ctx context.Context, userID string) ([]model.AgentPromptTemplate, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, ErrAgentPromptTemplateInvalid
	}
	list, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list agent prompt templates: %w", err)
	}
	if list == nil {
		return []model.AgentPromptTemplate{}, nil
	}
	return list, nil
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
	tpl, err := s.repo.Create(ctx, userID, name, category, description, systemPrompt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrAgentPromptTemplateDuplicate
		}
		return nil, fmt.Errorf("create agent prompt template: %w", err)
	}
	return tpl, nil
}

func (s *AgentPromptTemplateService) Update(ctx context.Context, id, userID, name, category, description, systemPrompt string) (*model.AgentPromptTemplate, error) {
	if strings.TrimSpace(id) == "" {
		return nil, ErrAgentPromptTemplateInvalid
	}
	name, category, description, systemPrompt, err := normalizeAgentPromptTemplateFields(userID, name, category, description, systemPrompt)
	if err != nil {
		return nil, err
	}
	tpl, err := s.repo.Update(ctx, id, userID, name, category, description, systemPrompt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrAgentPromptTemplateDuplicate
		}
		return nil, fmt.Errorf("update agent prompt template: %w", err)
	}
	if tpl == nil {
		return nil, ErrAgentPromptTemplateNotFound
	}
	return tpl, nil
}

func (s *AgentPromptTemplateService) Delete(ctx context.Context, id, userID string) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(userID) == "" {
		return ErrAgentPromptTemplateInvalid
	}
	deleted, err := s.repo.Delete(ctx, id, userID)
	if err != nil {
		return fmt.Errorf("delete agent prompt template: %w", err)
	}
	if !deleted {
		return ErrAgentPromptTemplateNotFound
	}
	return nil
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
