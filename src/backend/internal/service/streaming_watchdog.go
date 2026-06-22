package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/agent-hub/backend/internal/model"
)

// StreamingWatchdog 定期扫描 streaming 状态的 message，把超过 maxIdle 没有进展的
// 自动标记为 error（D6 ADR + R8）。
//
// 使用场景：daemon 崩溃 / WS 断开 / agent 进程异常时，预创建的 streaming message
// 会卡在 streaming 状态。watchdog 兜底，避免前端永远显示"生成中"。
//
// 用法：
//   wd := NewStreamingWatchdog(msgRepo, logger, 60*time.Second, 10*time.Second)
//   go wd.Run(ctx)
//
// 实现策略：直接复用 MessageRepo.MarkStaleStreaming（单条 UPDATE ... WHERE
// status='streaming' AND created_at + $interval < NOW()），避免把所有 streaming
// 拉到应用层。扫描间隔 10s，超时阈值 60s。
type StreamingWatchdog struct {
	msgRepo  MessageRepoStreaming
	logger   *slog.Logger
	maxAge   time.Duration
	interval time.Duration
}

// MessageRepoStreaming watchdog 依赖的最小接口，便于测试用 mock 替换。
type MessageRepoStreaming interface {
	MarkStaleStreaming(ctx context.Context, maxAge time.Duration) (int, error)
}

// NewStreamingWatchdog 构造 watchdog。maxAge 通常 60s，interval 通常 10s。
func NewStreamingWatchdog(msgRepo MessageRepoStreaming, logger *slog.Logger, maxAge, interval time.Duration) *StreamingWatchdog {
	if maxAge <= 0 {
		maxAge = 60 * time.Second
	}
	if interval <= 0 {
		interval = 10 * time.Second
	}
	return &StreamingWatchdog{
		msgRepo:  msgRepo,
		logger:   logger,
		maxAge:   maxAge,
		interval: interval,
	}
}

// Run 启动后台扫描循环。ctx 取消时退出。
func (w *StreamingWatchdog) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	w.logger.Info("streaming_watchdog_start", "max_age", w.maxAge, "interval", w.interval)
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("streaming_watchdog_stop")
			return
		case <-ticker.C:
			w.scanOnce(ctx)
		}
	}
}

// scanOnce 执行一次扫描。导出是为了测试可以直接调用而不用等 ticker。
func (w *StreamingWatchdog) scanOnce(ctx context.Context) {
	n, err := w.msgRepo.MarkStaleStreaming(ctx, w.maxAge)
	if err != nil {
		w.logger.Warn("streaming_watchdog_scan_failed", "error", err)
		return
	}
	if n > 0 {
		// 使用 model 常量避免拼写错误，同时让静态分析能追踪状态值变更。
		w.logger.Warn("streaming_watchdog_marked_stale",
			"count", n, "marked_status", model.MessageStatusError, "max_age", w.maxAge)
	}
}
