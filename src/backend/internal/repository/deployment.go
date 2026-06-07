package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/agent-hub/backend/internal/model"
	"github.com/jmoiron/sqlx"
)

// ErrDeploymentNotFound 部署记录不存在
var ErrDeploymentNotFound = errors.New("deployment not found")

// 部署查询列（统一 COALESCE 处理可空列）
const deploymentCols = `id, artifact_root_id, conversation_id, mode, status,
	COALESCE(url, '') AS url,
	COALESCE(error, '') AS error,
	created_at`

// DeploymentRepo 部署数据访问
type DeploymentRepo struct {
	db *sqlx.DB
}

// NewDeploymentRepo 创建部署仓库
func NewDeploymentRepo(db *sqlx.DB) *DeploymentRepo {
	return &DeploymentRepo{db: db}
}

// Create 插入一条部署记录。id 由调用方生成，以便先据 id 建盘上目录再落库。
func (r *DeploymentRepo) Create(ctx context.Context, d model.Deployment) (*model.Deployment, error) {
	var out model.Deployment
	err := r.db.QueryRowxContext(ctx,
		`INSERT INTO deployments (id, artifact_root_id, conversation_id, mode, status, url, error)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING `+deploymentCols,
		d.ID, d.ArtifactRootID, d.ConversationID, d.Mode, d.Status,
		nullIfEmpty(d.URL), nullIfEmpty(d.Error),
	).StructScan(&out)
	if err != nil {
		return nil, fmt.Errorf("insert deployment: %w", err)
	}
	return &out, nil
}

// UpdateStatus 更新部署状态及 url/error，返回更新后的完整行。
func (r *DeploymentRepo) UpdateStatus(ctx context.Context, id, status, url, errMsg string) (*model.Deployment, error) {
	var out model.Deployment
	err := r.db.QueryRowxContext(ctx,
		`UPDATE deployments SET status = $2, url = $3, error = $4 WHERE id = $1
		 RETURNING `+deploymentCols,
		id, status, nullIfEmpty(url), nullIfEmpty(errMsg),
	).StructScan(&out)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDeploymentNotFound
		}
		return nil, fmt.Errorf("update deployment: %w", err)
	}
	return &out, nil
}

// GetByID 按 id 查询部署记录。
func (r *DeploymentRepo) GetByID(ctx context.Context, id string) (*model.Deployment, error) {
	var d model.Deployment
	err := r.db.QueryRowxContext(ctx,
		`SELECT `+deploymentCols+` FROM deployments WHERE id = $1`, id,
	).StructScan(&d)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDeploymentNotFound
		}
		return nil, fmt.Errorf("get deployment: %w", err)
	}
	return &d, nil
}
