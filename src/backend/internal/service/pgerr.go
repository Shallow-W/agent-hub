package service

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// isUniqueViolation 判断 error 是否为 PostgreSQL unique constraint violation (SQLSTATE 23505)。
// 多个 service 共享此 helper 处理并发写冲突。
// 注意：依赖 errors.As 透传能力，要求 repository 层用 %w 包裹原始 pg 错误，
// 不能用 %v 或 fmt.Println 之类的吞错手段。
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
