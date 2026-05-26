package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/agent-hub/backend/internal/model"
	"github.com/jmoiron/sqlx"
)

// FriendRepo 好友关系数据访问
type FriendRepo struct {
	db *sqlx.DB
}

// NewFriendRepo 创建好友仓库
func NewFriendRepo(db *sqlx.DB) *FriendRepo {
	return &FriendRepo{db: db}
}

// SendRequest 创建好友申请（pending 状态）
func (r *FriendRepo) SendRequest(ctx context.Context, userID, friendID string) (*model.Friend, error) {
	var f model.Friend
	err := r.db.QueryRowxContext(ctx,
		`INSERT INTO friends (user_id, friend_id, status) VALUES ($1, $2, 'pending')
		 RETURNING id, user_id, friend_id, status, created_at, updated_at`,
		userID, friendID,
	).StructScan(&f)
	if err != nil {
		return nil, fmt.Errorf("insert friend request: %w", err)
	}
	return &f, nil
}

// AcceptRequest 接受好友申请并创建反向关系（事务保证原子性）
func (r *FriendRepo) AcceptRequest(ctx context.Context, userID, friendID string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE friends SET status = 'accepted', updated_at = NOW()
		 WHERE user_id = $1 AND friend_id = $2 AND status = 'pending'`,
		userID, friendID,
	)
	if err != nil {
		return fmt.Errorf("accept friend request: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO friends (user_id, friend_id, status) VALUES ($1, $2, 'accepted')
		 ON CONFLICT (user_id, friend_id) DO NOTHING`,
		friendID, userID,
	)
	if err != nil {
		return fmt.Errorf("create reverse friendship: %w", err)
	}

	return tx.Commit()
}

// RejectRequest 拒绝好友申请
func (r *FriendRepo) RejectRequest(ctx context.Context, userID, friendID string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE friends SET status = 'rejected', updated_at = NOW()
		 WHERE user_id = $1 AND friend_id = $2 AND status = 'pending'`,
		userID, friendID,
	)
	if err != nil {
		return fmt.Errorf("reject friend request: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ListFriends 查询已接受的好友列表，附带好友用户名
func (r *FriendRepo) ListFriends(ctx context.Context, userID string) ([]*model.Friend, error) {
	var list []*model.Friend
	err := r.db.SelectContext(ctx, &list,
		`SELECT f.id, f.user_id, f.friend_id, f.status, f.created_at, f.updated_at,
		        u.username AS friend_name
		 FROM friends f JOIN users u ON u.id = f.friend_id
		 WHERE f.user_id = $1 AND f.status = 'accepted'
		 ORDER BY f.updated_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list friends: %w", err)
	}
	return list, nil
}

// ListPendingRequests 查询收到的好友申请（pending 状态）
func (r *FriendRepo) ListPendingRequests(ctx context.Context, userID string) ([]*model.Friend, error) {
	var list []*model.Friend
	err := r.db.SelectContext(ctx, &list,
		`SELECT f.id, f.user_id, f.friend_id, f.status, f.created_at, f.updated_at,
		        u.username AS friend_name
		 FROM friends f JOIN users u ON u.id = f.user_id
		 WHERE f.friend_id = $1 AND f.status = 'pending'
		 ORDER BY f.created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list pending requests: %w", err)
	}
	return list, nil
}

// GetFriendship 查询两人之间的好友关系
func (r *FriendRepo) GetFriendship(ctx context.Context, userID, friendID string) (*model.Friend, error) {
	var f model.Friend
	err := r.db.QueryRowxContext(ctx,
		`SELECT id, user_id, friend_id, status, created_at, updated_at
		 FROM friends WHERE user_id = $1 AND friend_id = $2`,
		userID, friendID,
	).StructScan(&f)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get friendship: %w", err)
	}
	return &f, nil
}

// GetFriendshipByID 按 ID 查询好友关系记录
func (r *FriendRepo) GetFriendshipByID(ctx context.Context, id string) (*model.Friend, error) {
	var f model.Friend
	err := r.db.QueryRowxContext(ctx,
		`SELECT id, user_id, friend_id, status, created_at, updated_at
		 FROM friends WHERE id = $1`,
		id,
	).StructScan(&f)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get friendship by id: %w", err)
	}
	return &f, nil
}

// GetUserByUsername 按用户名查找用户（好友模块复用，不查询密码）
func (r *FriendRepo) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	var u model.User
	err := r.db.QueryRowxContext(ctx,
		`SELECT id, username, created_at FROM users WHERE username = $1`,
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

// SearchUsers 按用户名前缀搜索用户
func (r *FriendRepo) SearchUsers(ctx context.Context, query string, limit int) ([]*model.User, error) {
	var list []*model.User
	escaped := escapeLike(query) + "%"
	err := r.db.SelectContext(ctx, &list,
		`SELECT id, username, created_at FROM users WHERE username LIKE $1 ESCAPE '\' LIMIT $2`,
		escaped, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search users: %w", err)
	}
	return list, nil
}

// escapeLike 转义 SQL LIKE 通配符
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// GetUserByID 按 ID 查找用户（好友模块复用，不查询密码）
func (r *FriendRepo) GetUserByID(ctx context.Context, id string) (*model.User, error) {
	var u model.User
	err := r.db.QueryRowxContext(ctx,
		`SELECT id, username, created_at FROM users WHERE id = $1`,
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
