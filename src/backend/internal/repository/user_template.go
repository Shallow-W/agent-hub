package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/agent-hub/backend/internal/model"
	"github.com/jmoiron/sqlx"
)

type UserTemplateRepo struct {
	db *sqlx.DB
}

func NewUserTemplateRepo(db *sqlx.DB) *UserTemplateRepo {
	return &UserTemplateRepo{db: db}
}

func (r *UserTemplateRepo) ListByUserAndType(ctx context.Context, userID, tplType string) ([]model.UserTemplate, error) {
	var list []model.UserTemplate
	err := r.db.SelectContext(ctx, &list,
		`SELECT id, user_id, type, name, content, created_at, updated_at
		 FROM user_templates
		 WHERE user_id = $1 AND type = $2
		 ORDER BY updated_at DESC, name ASC`,
		userID, tplType,
	)
	if err != nil {
		return nil, fmt.Errorf("list user templates: %w", err)
	}
	return list, nil
}

func (r *UserTemplateRepo) Create(ctx context.Context, userID, tplType, name, content string) (*model.UserTemplate, error) {
	var tpl model.UserTemplate
	err := r.db.QueryRowxContext(ctx,
		`INSERT INTO user_templates (user_id, type, name, content)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, user_id, type, name, content, created_at, updated_at`,
		userID, tplType, name, content,
	).StructScan(&tpl)
	if err != nil {
		return nil, fmt.Errorf("create user template: %w", err)
	}
	return &tpl, nil
}

func (r *UserTemplateRepo) Update(ctx context.Context, id, userID, name, content string) (*model.UserTemplate, error) {
	var tpl model.UserTemplate
	err := r.db.QueryRowxContext(ctx,
		`UPDATE user_templates
		 SET name = $3, content = $4, updated_at = NOW()
		 WHERE id = $1 AND user_id = $2
		 RETURNING id, user_id, type, name, content, created_at, updated_at`,
		id, userID, name, content,
	).StructScan(&tpl)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update user template: %w", err)
	}
	return &tpl, nil
}

func (r *UserTemplateRepo) Delete(ctx context.Context, id, userID string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM user_templates WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
	if err != nil {
		return false, fmt.Errorf("delete user template: %w", err)
	}
	count, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	return count > 0, nil
}
