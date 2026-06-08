package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/agent-hub/backend/internal/model"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// ErrArtifactRootNotFound 血缘根不存在（rootId 无对应产物）
var ErrArtifactRootNotFound = errors.New("artifact root not found")

// 产物查询列（统一 COALESCE 处理可空列，含 root_id）
const artifactCols = `id, message_id, root_id, version, type,
	COALESCE(language, '') AS language,
	COALESCE(filename, '') AS filename,
	COALESCE(title, '') AS title,
	COALESCE(url, '') AS url,
	COALESCE(content, '') AS content,
	sort_order, created_at`

// ArtifactRepo 产物数据访问
type ArtifactRepo struct {
	db *sqlx.DB
}

// NewArtifactRepo 创建产物仓库
func NewArtifactRepo(db *sqlx.DB) *ArtifactRepo {
	return &ArtifactRepo{db: db}
}

// CreateArtifacts 批量创建消息产物（v1）。
// 产物来源于 daemon 解析的 Agent 回复，在 assistant 消息持久化后写入。
// id 在 Go 侧生成，root_id = id（首版自成血缘根），不依赖 DB 默认 gen_random_uuid，
// 否则无法在写入时拿到 id 去回填 root_id。
func (r *ArtifactRepo) CreateArtifacts(ctx context.Context, messageID string, artifacts []model.Artifact) error {
	if len(artifacts) == 0 {
		return nil
	}
	for i := range artifacts {
		version := artifacts[i].Version
		if version <= 0 {
			version = 1
		}
		id := uuid.NewString()
		_, err := r.db.ExecContext(ctx,
			`INSERT INTO artifacts (id, message_id, root_id, version, type, language, filename, title, url, content, sort_order)
			 VALUES ($1, $2, $1, $3, $4, $5, $6, $7, $8, $9, $10)`,
			id,
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
		// 回填生成的身份字段，使调用方内存对象（用于 WS 广播）带完整 root_id 等信息
		artifacts[i].ID = id
		artifacts[i].RootID = id
		artifacts[i].MessageID = messageID
		artifacts[i].Version = version
	}
	return nil
}

// CreateVersion 为指定血缘根创建新版本：version = 当前 root_id 下 max(version)+1。
// 新行有新 id、相同 root_id、相同 message_id（沿用根的归属消息），返回完整新行。
// 用事务 + FOR UPDATE 锁住该血缘的现有行，避免并发版本号竞争。
func (r *ArtifactRepo) CreateVersion(ctx context.Context, rootID string, in model.Artifact) (*model.Artifact, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// 先锁住该血缘 v1（根行）防止并发版本号竞争，并拿到归属消息。
	// 注意：FOR UPDATE 不能与聚合函数同查询，需分两步。
	var messageID string
	err = tx.QueryRowxContext(ctx,
		`SELECT message_id FROM artifacts WHERE root_id = $1 ORDER BY version ASC LIMIT 1 FOR UPDATE`,
		rootID,
	).Scan(&messageID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrArtifactRootNotFound
		}
		return nil, fmt.Errorf("lock artifact lineage: %w", err)
	}

	// 取当前最大版本号（根行已加锁，串行化并发创建）
	var maxVersion int
	if err := tx.QueryRowxContext(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM artifacts WHERE root_id = $1`,
		rootID,
	).Scan(&maxVersion); err != nil {
		return nil, fmt.Errorf("max artifact version: %w", err)
	}
	if maxVersion == 0 {
		return nil, ErrArtifactRootNotFound
	}

	id := uuid.NewString()
	newVersion := maxVersion + 1
	var out model.Artifact
	err = tx.QueryRowxContext(ctx,
		`INSERT INTO artifacts (id, message_id, root_id, version, type, language, filename, title, url, content, sort_order)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 0)
		 RETURNING `+artifactCols,
		id,
		messageID,
		rootID,
		newVersion,
		in.Type,
		nullIfEmpty(in.Language),
		nullIfEmpty(in.Filename),
		nullIfEmpty(in.Title),
		nullIfEmpty(in.URL),
		in.Content,
	).StructScan(&out)
	if err != nil {
		return nil, fmt.Errorf("insert artifact version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return &out, nil
}

// ListVersions 返回某血缘根的全部版本，按 version 升序。
func (r *ArtifactRepo) ListVersions(ctx context.Context, rootID string) ([]model.Artifact, error) {
	var list []model.Artifact
	err := r.db.SelectContext(ctx, &list,
		`SELECT `+artifactCols+` FROM artifacts WHERE root_id = $1 ORDER BY version ASC`,
		rootID,
	)
	if err != nil {
		return nil, fmt.Errorf("list artifact versions: %w", err)
	}
	return list, nil
}

// GetLatestByRoot 取某血缘根的最新版本（version 最大）。
func (r *ArtifactRepo) GetLatestByRoot(ctx context.Context, rootID string) (*model.Artifact, error) {
	var a model.Artifact
	err := r.db.QueryRowxContext(ctx,
		`SELECT `+artifactCols+` FROM artifacts WHERE root_id = $1 ORDER BY version DESC LIMIT 1`,
		rootID,
	).StructScan(&a)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrArtifactRootNotFound
		}
		return nil, fmt.Errorf("get latest artifact: %w", err)
	}
	return &a, nil
}

// ListByMessageIDs 批量查询多条消息的产物，每个血缘只返回最新版本。
// 直接对表做 DISTINCT ON (root_id)，复用 artifactCols（含 COALESCE + uuid/timestamptz 正确扫描），
// 避免外层子查询导致 sqlx 无法将 uuid/timestamptz 映射到 Go string/time.Time 的 bug。
// DISTINCT ON 要求 ORDER BY 首列与 DISTINCT ON 列一致，故 SQL 按 root_id, version DESC 取最新版；
// 最终按 sort_order/id 的排序在 Go 侧完成。
func (r *ArtifactRepo) ListByMessageIDs(ctx context.Context, messageIDs []string) (map[string][]model.Artifact, error) {
	if len(messageIDs) == 0 {
		return nil, nil
	}

	query, args, err := sqlx.In(
		`SELECT DISTINCT ON (root_id) `+artifactCols+`
		 FROM artifacts
		 WHERE message_id IN (?)
		 ORDER BY root_id, version DESC`,
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

	// DISTINCT ON 要求首列排序为 root_id，最终分组和 sort_order 排序在 Go 侧完成，
	// 保证对外每条消息内产物顺序稳定（与原行为一致）。
	result := make(map[string][]model.Artifact, len(messageIDs))
	for _, a := range list {
		result[a.MessageID] = append(result[a.MessageID], a)
	}
	for k := range result {
		sort.SliceStable(result[k], func(i, j int) bool {
			if result[k][i].SortOrder != result[k][j].SortOrder {
				return result[k][i].SortOrder < result[k][j].SortOrder
			}
			return result[k][i].ID < result[k][j].ID
		})
	}
	return result, nil
}

// GetLatestRootByConversation 返回某对话中最近一次产生的产物的血缘根（按产物 created_at 取最新）。
// 用于聊天「部署」指令：部署对话里最新的那个产物。无产物时返回 ErrArtifactRootNotFound。
func (r *ArtifactRepo) GetLatestRootByConversation(ctx context.Context, convID string) (string, error) {
	var rootID string
	err := r.db.QueryRowxContext(ctx,
		`SELECT a.root_id
		 FROM artifacts a JOIN messages m ON m.id = a.message_id
		 WHERE m.conversation_id = $1
		 ORDER BY a.created_at DESC, a.version DESC
		 LIMIT 1`,
		convID,
	).Scan(&rootID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrArtifactRootNotFound
		}
		return "", fmt.Errorf("get latest artifact root by conversation: %w", err)
	}
	return rootID, nil
}

// GetConversationIDByRoot 通过血缘根回溯所属对话（用于鉴权）。
func (r *ArtifactRepo) GetConversationIDByRoot(ctx context.Context, rootID string) (string, error) {
	var convID string
	err := r.db.QueryRowxContext(ctx,
		`SELECT m.conversation_id
		 FROM artifacts a JOIN messages m ON m.id = a.message_id
		 WHERE a.root_id = $1
		 LIMIT 1`,
		rootID,
	).Scan(&convID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrArtifactRootNotFound
		}
		return "", fmt.Errorf("get conversation by artifact root: %w", err)
	}
	return convID, nil
}

// nullIfEmpty 空字符串转 NULL，避免可空列存空串
func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
