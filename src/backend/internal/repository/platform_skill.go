package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/agent-hub/backend/internal/model"
	"github.com/jmoiron/sqlx"
)

// PlatformSkillRepo 管理用户级平台 Skill 库。
type PlatformSkillRepo struct {
	db *sqlx.DB
}

func NewPlatformSkillRepo(db *sqlx.DB) *PlatformSkillRepo {
	return &PlatformSkillRepo{db: db}
}

func (r *PlatformSkillRepo) ListByUser(ctx context.Context, userID string) ([]model.PlatformSkill, error) {
	var list []model.PlatformSkill
	err := r.db.SelectContext(ctx, &list,
		`SELECT id, user_id, name, category, description, trigger, detail, created_at, updated_at
		 FROM platform_skills
		 WHERE user_id = $1
		 ORDER BY category ASC, updated_at DESC, name ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list platform skills: %w", err)
	}
	return list, nil
}

func (r *PlatformSkillRepo) Create(ctx context.Context, userID, name, category, description, trigger, detail string) (*model.PlatformSkill, error) {
	var skill model.PlatformSkill
	err := r.db.QueryRowxContext(ctx,
		`INSERT INTO platform_skills (user_id, name, category, description, trigger, detail)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, user_id, name, category, description, trigger, detail, created_at, updated_at`,
		userID, name, category, description, trigger, detail,
	).StructScan(&skill)
	if err != nil {
		return nil, fmt.Errorf("create platform skill: %w", err)
	}
	return &skill, nil
}

func (r *PlatformSkillRepo) Update(ctx context.Context, id, userID, name, category, description, trigger, detail string) (*model.PlatformSkill, error) {
	var skill model.PlatformSkill
	err := r.db.QueryRowxContext(ctx,
		`UPDATE platform_skills
		 SET name = $3, category = $4, description = $5, trigger = $6, detail = $7, updated_at = NOW()
		 WHERE id = $1 AND user_id = $2
		 RETURNING id, user_id, name, category, description, trigger, detail, created_at, updated_at`,
		id, userID, name, category, description, trigger, detail,
	).StructScan(&skill)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update platform skill: %w", err)
	}
	return &skill, nil
}

func (r *PlatformSkillRepo) Delete(ctx context.Context, id, userID string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM platform_skills WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
	if err != nil {
		return false, fmt.Errorf("delete platform skill: %w", err)
	}
	count, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	return count > 0, nil
}
