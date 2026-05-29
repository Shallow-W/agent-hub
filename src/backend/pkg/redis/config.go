package redis

import (
	"fmt"
	"time"
)

const (
	defaultDialTimeout  = 5 * time.Second
	defaultReadTimeout  = 3 * time.Second
	defaultWriteTimeout = 3 * time.Second
	defaultPoolSize     = 20
	defaultMinIdle      = 5
)

func resolveAddr(cfg Config) string {
	return fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
}
