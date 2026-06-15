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
		`SELECT name, label, category, description, is_management, created_at
		 FROM tool_definitions
		 ORDER BY name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list tool definitions: %w", err)
	}
	return list, nil
}

func (r *ToolDefinitionRepo) Upsert(ctx context.Context, td model.ToolDefinition) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO tool_definitions (name, label, category, description)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (name) DO UPDATE SET
		   label = EXCLUDED.label,
		   category = EXCLUDED.category,
		   description = EXCLUDED.description`,
		td.Name, td.Label, td.Category, td.Description,
	)
	return err
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

// IsValidToolset 返回给定 toolset 名是否在 builtin_toolset_templates 中存在。
// "none" 是内置合法值，无需查 DB。
func (r *ToolDefinitionRepo) IsValidToolset(ctx context.Context, name string) (bool, error) {
	if name == "" || name == "none" {
		return name == "none", nil
	}
	var exists bool
	err := r.db.GetContext(ctx, &exists,
		`SELECT EXISTS(SELECT 1 FROM builtin_toolset_templates WHERE name = $1)`,
		name,
	)
	if err != nil {
		return false, fmt.Errorf("check toolset validity: %w", err)
	}
	return exists, nil
}
