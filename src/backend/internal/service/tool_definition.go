package service

import (
	"context"
	"fmt"

	"github.com/agent-hub/backend/internal/model"
)

// ToolDefinitionRepo 定义 ToolDefinitionService 需要的仓库接口。
type ToolDefinitionRepo interface {
	List(ctx context.Context) ([]model.ToolDefinition, error)
	ListBuiltinTemplates(ctx context.Context) ([]model.BuiltinToolsetTemplate, error)
}

type ToolDefinitionService struct {
	repo ToolDefinitionRepo
}

func NewToolDefinitionService(repo ToolDefinitionRepo) *ToolDefinitionService {
	return &ToolDefinitionService{repo: repo}
}

func (s *ToolDefinitionService) ListDefinitions(ctx context.Context) ([]model.ToolDefinition, error) {
	list, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list definitions: %w", err)
	}
	if list == nil {
		return []model.ToolDefinition{}, nil
	}
	return list, nil
}

func (s *ToolDefinitionService) ListBuiltinTemplates(ctx context.Context) ([]model.BuiltinToolsetTemplate, error) {
	list, err := s.repo.ListBuiltinTemplates(ctx)
	if err != nil {
		return nil, fmt.Errorf("list builtin templates: %w", err)
	}
	if list == nil {
		return []model.BuiltinToolsetTemplate{}, nil
	}
	return list, nil
}
