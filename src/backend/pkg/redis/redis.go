package redis

import (
	"context"

	goredis "github.com/redis/go-redis/v9"
)

// Config Redis 连接配置
type Config struct {
	Host     string `koanf:"host"`
	Port     int    `koanf:"port"`
	Password string `koanf:"password"`
	DB       int    `koanf:"db"`
}

// NewClient 创建 Redis 客户端
func NewClient(cfg Config) (*goredis.Client, error) {
	client := goredis.NewClient(&goredis.Options{
		Addr:         resolveAddr(cfg),
		Password:     cfg.Password,
		DB:           cfg.DB,
		MaxRetries:   3,
		DialTimeout:  defaultDialTimeout,
		ReadTimeout:  defaultReadTimeout,
		WriteTimeout: defaultWriteTimeout,
		PoolSize:     defaultPoolSize,
		MinIdleConns: defaultMinIdle,
	})

	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, err
	}
	return client, nil
}
