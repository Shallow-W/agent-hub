package repository

import (
	"context"
	"fmt"

	"github.com/agent-hub/backend/internal/model"
	"github.com/jmoiron/sqlx"
)

type ToolCategoryRepo struct {
	db *sqlx.DB
}

func NewToolCategoryRepo(db *sqlx.DB) *ToolCategoryRepo {
	return &ToolCategoryRepo{db: db}
}

func (r *ToolCategoryRepo) List(ctx context.Context) ([]model.ToolCategory, error) {
	var list []model.ToolCategory
	err := r.db.SelectContext(ctx, &list,
		`SELECT name, label, color, sort_order
		 FROM tool_categories
		 ORDER BY sort_order ASC, name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list tool categories: %w", err)
	}
	return list, nil
}

// Upsert supports future auto-sync from a tool category registry.
// Currently the table is seeded only by migration 050.
func (r *ToolCategoryRepo) Upsert(ctx context.Context, c model.ToolCategory) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO tool_categories (name, label, color, sort_order)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (name) DO UPDATE SET
		   label = EXCLUDED.label,
		   color = EXCLUDED.color,
		   sort_order = EXCLUDED.sort_order`,
		c.Name, c.Label, c.Color, c.SortOrder,
	)
	return err
}
