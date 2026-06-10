package repository

import (
	"context"
	"fmt"

	"github.com/agent-hub/backend/internal/model"
	"github.com/jmoiron/sqlx"
)

type ToolDefinitionRepo struct {
	db *sqlx.DB
}

func NewToolDefinitionRepo(db *sqlx.DB) *ToolDefinitionRepo {
	return &ToolDefinitionRepo{db: db}
}

func (r *ToolDefinitionRepo) List(ctx context.Context) ([]model.ToolDefinition, error) {
	var list []model.ToolDefinition
	err := r.db.SelectContext(ctx, &list,
		`SELECT name, label, category, description, created_at
		 FROM tool_definitions
		 ORDER BY name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list tool definitions: %w", err)
	}
	return list, nil
}

func (r *ToolDefinitionRepo) ListBuiltinTemplates(ctx context.Context) ([]model.BuiltinToolsetTemplate, error) {
	var list []model.BuiltinToolsetTemplate
	err := r.db.SelectContext(ctx, &list,
		`SELECT name, label, description, tool_names, created_at
		 FROM builtin_toolset_templates
		 ORDER BY name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list builtin toolset templates: %w", err)
	}
	return list, nil
}
