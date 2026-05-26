package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/agent-hub/backend/internal/model"
)

const (
	// 离线消息队列 key: offline:{conversationID}
	offlineKeyPrefix = "offline:"
	// 热消息缓存 key: msgs:{conversationID}
	msgCacheKeyPrefix = "msgs:"
	// 未读计数 key: unread:{userID}:{conversationID}
	unreadKeyPrefix = "unread:"
	// 缓存过期时间
	msgCacheTTL   = 30 * time.Minute
	offlineMaxTTL = 7 * 24 * time.Hour
	// 默认缓存条数
	msgCacheSize = 50
)

// RedisMsgRepo Redis 消息缓存层
type RedisMsgRepo struct {
	rdb *goredis.Client
}

// NewRedisMsgRepo 创建 Redis 消息仓库
func NewRedisMsgRepo(rdb *goredis.Client) *RedisMsgRepo {
	return &RedisMsgRepo{rdb: rdb}
}

// --- 离线消息队列（Sorted Set，score=timestamp） ---

func offlineKey(userID, conversationID string) string {
	return offlineKeyPrefix + userID + ":" + conversationID
}

// EnqueueOffline 将消息加入用户在该会话的离线队列
func (r *RedisMsgRepo) EnqueueOffline(ctx context.Context, userID, conversationID string, msg *model.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	key := offlineKey(userID, conversationID)
	member := goredis.Z{
		Score:  float64(msg.CreatedAt.UnixNano()),
		Member: data,
	}
	pipe := r.rdb.Pipeline()
	pipe.ZAdd(ctx, key, member)
	pipe.Expire(ctx, key, offlineMaxTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis zadd offline: %w", err)
	}
	return nil
}

// DequeueOfflineAfter 拉取指定时间之后的离线消息并清空队列
func (r *RedisMsgRepo) DequeueOfflineAfter(ctx context.Context, userID, conversationID string, after interface{}) ([]model.Message, error) {
	key := offlineKey(userID, conversationID)
	var min string
	switch v := after.(type) {
	case time.Time:
		min = fmt.Sprintf("(%d", v.UnixNano())
	case string:
		min = fmt.Sprintf("(%s", v)
	default:
		min = "-inf"
	}

	results, err := r.rdb.ZRangeByScore(ctx, key, &goredis.ZRangeBy{
		Min: min,
		Max: "+inf",
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("redis zrange offline: %w", err)
	}

	if len(results) == 0 {
		return nil, nil
	}

	msgs := make([]model.Message, 0, len(results))
	for _, data := range results {
		var m model.Message
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			continue
		}
		msgs = append(msgs, m)
	}

	// 清空已读的离线消息
	r.rdb.Del(ctx, key)

	return msgs, nil
}

// --- 热消息缓存（List，最近 N 条） ---

func msgCacheKey(conversationID string) string {
	return msgCacheKeyPrefix + conversationID
}

// CacheMessage 将消息追加到会话热缓存
func (r *RedisMsgRepo) CacheMessage(ctx context.Context, conversationID string, msg *model.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	key := msgCacheKey(conversationID)
	pipe := r.rdb.Pipeline()
	pipe.LPush(ctx, key, data)
	pipe.LTrim(ctx, key, 0, int64(msgCacheSize-1))
	pipe.Expire(ctx, key, msgCacheTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis cache message: %w", err)
	}
	return nil
}

// GetCachedMessages 从缓存获取最近消息（返回时间升序）
func (r *RedisMsgRepo) GetCachedMessages(ctx context.Context, conversationID string, limit int) ([]model.Message, error) {
	key := msgCacheKey(conversationID)
	results, err := r.rdb.LRange(ctx, key, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, fmt.Errorf("redis lrange cached: %w", err)
	}
	if len(results) == 0 {
		return nil, nil
	}

	msgs := make([]model.Message, 0, len(results))
	for i := len(results) - 1; i >= 0; i-- {
		var m model.Message
		if err := json.Unmarshal([]byte(results[i]), &m); err != nil {
			continue
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}

// InvalidateCache 清除会话消息缓存
func (r *RedisMsgRepo) InvalidateCache(ctx context.Context, conversationID string) error {
	return r.rdb.Del(ctx, msgCacheKey(conversationID)).Err()
}

// --- 未读计数 ---

func unreadKey(userID, conversationID string) string {
	return unreadKeyPrefix + userID + ":" + conversationID
}

// IncrementUnread 递增用户在某会话的未读计数
func (r *RedisMsgRepo) IncrementUnread(ctx context.Context, userID, conversationID string) error {
	key := unreadKey(userID, conversationID)
	pipe := r.rdb.Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, 7*24*time.Hour)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis incr unread: %w", err)
	}
	return nil
}

// GetUnreadCount 获取用户在某会话的未读计数
func (r *RedisMsgRepo) GetUnreadCount(ctx context.Context, userID, conversationID string) (int64, error) {
	val, err := r.rdb.Get(ctx, unreadKey(userID, conversationID)).Int64()
	if err == goredis.Nil {
		return 0, nil
	}
	return val, err
}

// ClearUnread 清零未读计数
func (r *RedisMsgRepo) ClearUnread(ctx context.Context, userID, conversationID string) error {
	return r.rdb.Del(ctx, unreadKey(userID, conversationID)).Err()
}
