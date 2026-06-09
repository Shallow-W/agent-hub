package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/agent-hub/backend/internal/model"
)

const (
	// 离线消息队列 key: offline:{userID}:{conversationID}
	offlineKeyPrefix = "offline:"
	// 未读计数 key: unread:{userID}:{conversationID}
	unreadKeyPrefix = "unread:"
	// 离线投递状态过期时间
	offlineMaxTTL = 7 * 24 * time.Hour
)

// RedisMsgRepo stores transient message delivery state in Redis.
type RedisMsgRepo struct {
	rdb *goredis.Client
}

// NewRedisMsgRepo 创建 Redis 消息投递状态仓库
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

// dequeueOfflineScript 原子地读取并删除离线队列（Lua 脚本避免读删竞态）
var dequeueOfflineScript = goredis.NewScript(`
local results = redis.call('ZRANGEBYSCORE', KEYS[1], ARGV[1], ARGV[2])
if #results > 0 then
	redis.call('DEL', KEYS[1])
end
return results
`)

// DequeueOfflineAfter 拉取指定时间之后的离线消息并清空队列（原子操作）
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

	results, err := dequeueOfflineScript.Run(ctx, r.rdb, []string{key}, min, "+inf").StringSlice()
	if err != nil {
		return nil, fmt.Errorf("redis dequeue offline: %w", err)
	}

	if len(results) == 0 {
		return nil, nil
	}

	msgs := make([]model.Message, 0, len(results))
	for _, data := range results {
		var m model.Message
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			slog.Warn("unmarshal offline message failed", "conversation_id", conversationID, "error", err)
			continue
		}
		msgs = append(msgs, m)
	}

	return msgs, nil
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
