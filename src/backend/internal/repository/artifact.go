package repository

import (
	"context"
	"fmt"

	"github.com/agent-hub/backend/internal/model"
	"github.com/jmoiron/sqlx"
)

// ArtifactRepo 产物数据访问
type ArtifactRepo struct {
	db *sqlx.DB
}

// NewArtifactRepo 创建产物仓库
func NewArtifactRepo(db *sqlx.DB) *ArtifactRepo {
	return &ArtifactRepo{db: db}
}

// CreateArtifacts 批量创建消息产物。
// 产物来源于 daemon 解析的 Agent 回复，在 assistant 消息持久化后写入。
func (r *ArtifactRepo) CreateArtifacts(ctx context.Context, messageID string, artifacts []model.Artifact) error {
	if len(artifacts) == 0 {
		return nil
	}
	for i := range artifacts {
		version := artifacts[i].Version
		if version <= 0 {
			version = 1
		}
		_, err := r.db.ExecContext(ctx,
			`INSERT INTO artifacts (id, message_id, version, type, language, filename, title, url, content, sort_order)
			 VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			messageID,
			version,
			artifacts[i].Type,
			nullIfEmpty(artifacts[i].Language),
			nullIfEmpty(artifacts[i].Filename),
			nullIfEmpty(artifacts[i].Title),
			nullIfEmpty(artifacts[i].URL),
			artifacts[i].Content,
			i,
		)
		if err != nil {
			return fmt.Errorf("insert artifact %d: %w", i, err)
		}
	}
	return nil
}

// ListByMessageIDs 批量查询多条消息的产物
func (r *ArtifactRepo) ListByMessageIDs(ctx context.Context, messageIDs []string) (map[string][]model.Artifact, error) {
	if len(messageIDs) == 0 {
		return nil, nil
	}

	query, args, err := sqlx.In(
		`SELECT id, message_id, version, type,
		        COALESCE(language, '') AS language,
		        COALESCE(filename, '') AS filename,
		        COALESCE(title, '') AS title,
		        COALESCE(url, '') AS url,
		        COALESCE(content, '') AS content,
		        sort_order, created_at
		 FROM artifacts WHERE message_id IN (?)
		 ORDER BY message_id, version, sort_order, id`,
		messageIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("build in query: %w", err)
	}
	query = r.db.Rebind(query)

	var list []model.Artifact
	if err := r.db.SelectContext(ctx, &list, query, args...); err != nil {
		return nil, fmt.Errorf("list artifacts: %w", err)
	}

	result := make(map[string][]model.Artifact, len(messageIDs))
	for _, a := range list {
		result[a.MessageID] = append(result[a.MessageID], a)
	}
	return result, nil
}

// nullIfEmpty 空字符串转 NULL，避免可空列存空串
func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
