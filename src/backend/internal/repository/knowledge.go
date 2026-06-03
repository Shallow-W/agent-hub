package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/agent-hub/backend/internal/model"
	"github.com/jmoiron/sqlx"
)

// KnowledgeRepo 知识库数据访问
type KnowledgeRepo struct {
	db *sqlx.DB
}

// NewKnowledgeRepo 创建知识库仓库
func NewKnowledgeRepo(db *sqlx.DB) *KnowledgeRepo {
	return &KnowledgeRepo{db: db}
}

// Create 创建知识库
func (r *KnowledgeRepo) Create(ctx context.Context, userID, name, description string) (*model.KnowledgeBase, error) {
	var kb model.KnowledgeBase
	err := r.db.QueryRowxContext(ctx,
		`INSERT INTO knowledge_bases (user_id, name, description) VALUES ($1, $2, $3)
		 RETURNING id, user_id, name, description, visibility, created_at, updated_at`,
		userID, name, description,
	).StructScan(&kb)
	if err != nil {
		return nil, fmt.Errorf("insert knowledge base: %w", err)
	}
	kb.Files = []model.KnowledgeFile{}
	kb.FileCount = 0
	return &kb, nil
}

// ListByUser 获取用户的所有知识库（包含文件数量）
func (r *KnowledgeRepo) ListByUser(ctx context.Context, userID string) ([]model.KnowledgeBase, error) {
	var kbs []model.KnowledgeBase
	err := r.db.SelectContext(ctx, &kbs,
		`SELECT kb.id, kb.user_id, kb.name, kb.description, kb.visibility, kb.created_at, kb.updated_at,
		        (SELECT COUNT(*) FROM knowledge_files kf WHERE kf.knowledge_base_id = kb.id) AS file_count
		 FROM knowledge_bases kb
		 WHERE kb.user_id = $1
		 ORDER BY kb.updated_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list knowledge bases: %w", err)
	}
	if kbs == nil {
		kbs = []model.KnowledgeBase{}
	}
	return kbs, nil
}

// GetByID 按ID获取知识库
func (r *KnowledgeRepo) GetByID(ctx context.Context, id string) (*model.KnowledgeBase, error) {
	var kb model.KnowledgeBase
	err := r.db.QueryRowxContext(ctx,
		`SELECT kb.id, kb.user_id, kb.name, kb.description, kb.visibility, kb.created_at, kb.updated_at,
		        (SELECT COUNT(*) FROM knowledge_files kf WHERE kf.knowledge_base_id = kb.id) AS file_count
		 FROM knowledge_bases kb
		 WHERE kb.id = $1`,
		id,
	).StructScan(&kb)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get knowledge base: %w", err)
	}
	return &kb, nil
}

// UpdateVisibility 更新知识库可见性
func (r *KnowledgeRepo) UpdateVisibility(ctx context.Context, id, visibility string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE knowledge_bases SET visibility = $1, updated_at = now() WHERE id = $2`,
		visibility, id,
	)
	if err != nil {
		return fmt.Errorf("update knowledge base visibility: %w", err)
	}
	return nil
}

// Delete 删除知识库
func (r *KnowledgeRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM knowledge_bases WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("delete knowledge base: %w", err)
	}
	return nil
}

// ListFiles 获取知识库中的文件列表
func (r *KnowledgeRepo) ListFiles(ctx context.Context, kbID string) ([]model.KnowledgeFile, error) {
	var files []model.KnowledgeFile
	err := r.db.SelectContext(ctx, &files,
		`SELECT id, knowledge_base_id, filename, file_path, file_size, mime_type, created_at
		 FROM knowledge_files
		 WHERE knowledge_base_id = $1
		 ORDER BY created_at DESC`,
		kbID,
	)
	if err != nil {
		return nil, fmt.Errorf("list knowledge files: %w", err)
	}
	if files == nil {
		files = []model.KnowledgeFile{}
	}
	return files, nil
}

// AddFile 添加文件到知识库
func (r *KnowledgeRepo) AddFile(ctx context.Context, kbID, filename, filePath string, fileSize int64, mimeType, sha256 string) (*model.KnowledgeFile, error) {
	var f model.KnowledgeFile
	err := r.db.QueryRowxContext(ctx,
		`INSERT INTO knowledge_files (knowledge_base_id, filename, file_path, file_size, mime_type, sha256)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, knowledge_base_id, filename, file_path, file_size, mime_type, created_at`,
		kbID, filename, filePath, fileSize, mimeType, sha256,
	).StructScan(&f)
	if err != nil {
		return nil, fmt.Errorf("insert knowledge file: %w", err)
	}
	// 更新知识库的 updated_at
	_, _ = r.db.ExecContext(ctx,
		`UPDATE knowledge_bases SET updated_at = now() WHERE id = $1`,
		kbID,
	)
	return &f, nil
}

// DeleteFile 删除知识库文件，返回文件路径用于删除物理文件
func (r *KnowledgeRepo) DeleteFile(ctx context.Context, kbID, fileID string) (string, error) {
	// 先获取文件路径以便删除物理文件
	var filePath string
	err := r.db.QueryRowxContext(ctx,
		`SELECT file_path FROM knowledge_files WHERE id = $1 AND knowledge_base_id = $2`,
		fileID, kbID,
	).Scan(&filePath)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil // 文件不存在，返回空路径
	}
	if err != nil {
		return "", fmt.Errorf("get knowledge file path: %w", err)
	}

	_, err = r.db.ExecContext(ctx,
		`DELETE FROM knowledge_files WHERE id = $1 AND knowledge_base_id = $2`,
		fileID, kbID,
	)
	if err != nil {
		return "", fmt.Errorf("delete knowledge file: %w", err)
	}
	return filePath, nil
}

// FindPublicByName 按名称查找公开知识库（用于群聊引用）
func (r *KnowledgeRepo) FindPublicByName(ctx context.Context, username, kbName string) (*model.KnowledgeBase, error) {
	var kb model.KnowledgeBase
	err := r.db.QueryRowxContext(ctx,
		`SELECT kb.id, kb.user_id, kb.name, kb.description, kb.visibility, kb.created_at, kb.updated_at,
		        u.username,
		        (SELECT COUNT(*) FROM knowledge_files kf WHERE kf.knowledge_base_id = kb.id) AS file_count
		 FROM knowledge_bases kb
		 JOIN users u ON u.id = kb.user_id
		 WHERE u.username = $1 AND kb.name = $2 AND kb.visibility = 'public'`,
		username, kbName,
	).StructScan(&kb)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find public knowledge base: %w", err)
	}
	return &kb, nil
}

// FindByUserAndName 按用户和名称查找知识库（自己的，无论公开私有）
func (r *KnowledgeRepo) FindByUserAndName(ctx context.Context, userID, kbName string) (*model.KnowledgeBase, error) {
	var kb model.KnowledgeBase
	err := r.db.QueryRowxContext(ctx,
		`SELECT kb.id, kb.user_id, kb.name, kb.description, kb.visibility, kb.created_at, kb.updated_at,
		        (SELECT COUNT(*) FROM knowledge_files kf WHERE kf.knowledge_base_id = kb.id) AS file_count
		 FROM knowledge_bases kb
		 WHERE kb.user_id = $1 AND kb.name = $2`,
		userID, kbName,
	).StructScan(&kb)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find knowledge base by user and name: %w", err)
	}
	return &kb, nil
}

// GetFileByID 按文件ID获取单个文件记录
func (r *KnowledgeRepo) GetFileByID(ctx context.Context, kbID, fileID string) (*model.KnowledgeFile, error) {
	var f model.KnowledgeFile
	err := r.db.QueryRowxContext(ctx,
		`SELECT id, knowledge_base_id, filename, file_path, file_size, mime_type, created_at
		 FROM knowledge_files
		 WHERE id = $1 AND knowledge_base_id = $2`,
		fileID, kbID,
	).StructScan(&f)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get knowledge file: %w", err)
	}
	return &f, nil
}

// GetFileContent 获取知识库文件路径列表（用于Agent引用）
func (r *KnowledgeRepo) GetFileContent(ctx context.Context, kbID string) ([]model.KnowledgeFile, error) {
	return r.ListFiles(ctx, kbID)
}
