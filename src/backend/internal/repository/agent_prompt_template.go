package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/agent-hub/backend/internal/model"
	"github.com/jmoiron/sqlx"
)

// AgentPromptTemplateRepo 管理用户级 Agent system prompt 模板。
type AgentPromptTemplateRepo struct {
	db *sqlx.DB
}

func NewAgentPromptTemplateRepo(db *sqlx.DB) *AgentPromptTemplateRepo {
	return &AgentPromptTemplateRepo{db: db}
}

func (r *AgentPromptTemplateRepo) ListByUser(ctx context.Context, userID string) ([]model.AgentPromptTemplate, error) {
	var list []model.AgentPromptTemplate
	err := r.db.SelectContext(ctx, &list,
		`SELECT id, user_id, name, category, description, system_prompt, created_at, updated_at
		 FROM agent_prompt_templates
		 WHERE user_id = $1
		 ORDER BY category ASC, updated_at DESC, name ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list agent prompt templates: %w", err)
	}
	return list, nil
}

func (r *AgentPromptTemplateRepo) Create(ctx context.Context, userID, name, category, description, systemPrompt string) (*model.AgentPromptTemplate, error) {
	var tpl model.AgentPromptTemplate
	err := r.db.QueryRowxContext(ctx,
		`INSERT INTO agent_prompt_templates (user_id, name, category, description, system_prompt)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, user_id, name, category, description, system_prompt, created_at, updated_at`,
		userID, name, category, description, systemPrompt,
	).StructScan(&tpl)
	if err != nil {
		return nil, fmt.Errorf("create agent prompt template: %w", err)
	}
	return &tpl, nil
}

func (r *AgentPromptTemplateRepo) Update(ctx context.Context, id, userID, name, category, description, systemPrompt string) (*model.AgentPromptTemplate, error) {
	var tpl model.AgentPromptTemplate
	err := r.db.QueryRowxContext(ctx,
		`UPDATE agent_prompt_templates
		 SET name = $3, category = $4, description = $5, system_prompt = $6, updated_at = NOW()
		 WHERE id = $1 AND user_id = $2
		 RETURNING id, user_id, name, category, description, system_prompt, created_at, updated_at`,
		id, userID, name, category, description, systemPrompt,
	).StructScan(&tpl)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update agent prompt template: %w", err)
	}
	return &tpl, nil
}

func (r *AgentPromptTemplateRepo) Delete(ctx context.Context, id, userID string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM agent_prompt_templates WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
	if err != nil {
		return false, fmt.Errorf("delete agent prompt template: %w", err)
	}
	count, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	return count > 0, nil
}
