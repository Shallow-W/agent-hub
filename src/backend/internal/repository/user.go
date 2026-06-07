package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/agent-hub/backend/internal/model"
	"github.com/jmoiron/sqlx"
)

// UserRepo 用户数据访问
type UserRepo struct {
	db *sqlx.DB
}

// NewUserRepo 创建用户仓库
func NewUserRepo(db *sqlx.DB) *UserRepo {
	return &UserRepo{db: db}
}

// CreateUser 创建新用户，返回含 ID 的用户对象
func (r *UserRepo) CreateUser(ctx context.Context, username, passwordHash string) (*model.User, error) {
	var u model.User
	err := r.db.QueryRowxContext(ctx,
		`INSERT INTO users (username, password_hash) VALUES ($1, $2) RETURNING id, username, password_hash, avatar, created_at`,
		username, passwordHash,
	).StructScan(&u)
	if err != nil {
		return nil, fmt.Errorf("insert user: %w", err)
	}
	return &u, nil
}

// GetUserByUsername 按用户名查找
func (r *UserRepo) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	var u model.User
	err := r.db.QueryRowxContext(ctx,
		`SELECT id, username, password_hash, avatar, created_at FROM users WHERE username = $1`,
		username,
	).StructScan(&u)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by username: %w", err)
	}
	return &u, nil
}

// GetUserByID 按 ID 查找
func (r *UserRepo) GetUserByID(ctx context.Context, id string) (*model.User, error) {
	var u model.User
	err := r.db.QueryRowxContext(ctx,
		`SELECT id, username, password_hash, avatar, created_at FROM users WHERE id = $1`,
		id,
	).StructScan(&u)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &u, nil
}

// UpdateUsername 更新用户名
func (r *UserRepo) UpdateUsername(ctx context.Context, id, username string) (*model.User, error) {
	var u model.User
	err := r.db.QueryRowxContext(ctx,
		`UPDATE users SET username = $1 WHERE id = $2
		 RETURNING id, username, password_hash, avatar, created_at`,
		username, id,
	).StructScan(&u)
	if err != nil {
		return nil, fmt.Errorf("update username: %w", err)
	}
	return &u, nil
}

// UpdateAvatar 更新用户头像（key 或 URL）
func (r *UserRepo) UpdateAvatar(ctx context.Context, id, avatar string) (*model.User, error) {
	var u model.User
	err := r.db.QueryRowxContext(ctx,
		`UPDATE users SET avatar = $1 WHERE id = $2
		 RETURNING id, username, password_hash, avatar, created_at`,
		avatar, id,
	).StructScan(&u)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update avatar: %w", err)
	}
	return &u, nil
}
