package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/pkg/ws"
)

// StreamingWatchdog 定期扫描 streaming 状态的 message，把超过 maxAge 没有进展的
// 自动标记为 error（D6 ADR + R8）。
//
// 使用场景：daemon 崩溃 / WS 断开 / agent 进程异常时，预创建的 streaming message
// 会卡在 streaming 状态。watchdog 兜底，避免前端永远显示"生成中"。
//
// PR5：标记 stale 后同时广播 message.complete（status=error）给对应 conversation
// 成员，让前端立即感知 stale，不再永远 loading。广播是 best-effort：失败只 warn，
// 不阻塞 watchdog 循环。
//
// 用法：
//   wd := NewStreamingWatchdog(msgRepo, logger, 60*time.Second, 10*time.Second)
//   wd.SetBroadcaster(userHub, convRepo)
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
	// 广播相关（可选，未注入则只标记不广播，用于测试或最小化依赖）
	userHub  *ws.Hub
	convRepo ConvRepoForWatchdog
}

// MessageRepoStreaming watchdog 依赖的最小接口，便于测试用 mock 替换。
type MessageRepoStreaming interface {
	MarkStaleStreaming(ctx context.Context, maxAge time.Duration) (int, error)
	// ListStaleStreaming 在 MarkStaleStreaming 之前调用，拿到即将被标记的 messages
	// 的元数据（id / conversation_id / sender_id 等），用于广播 message.complete。
	ListStaleStreaming(ctx context.Context, before time.Time) ([]model.Message, error)
}

// ConvRepoForWatchdog 是 watchdog 广播所需的最小 conversation repo 接口。
// 与 service.ConvRepoForMsg 形状一致但独立定义，避免跨 service 耦合。
type ConvRepoForWatchdog interface {
	ListMemberIDs(ctx context.Context, conversationID string) ([]string, error)
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

// SetBroadcaster 注入 WS 广播依赖。
//
// main.go 在构造 watchdog 后启动 Run 之前调用。未注入时 watchdog 只执行
// 标记逻辑（向后兼容场景，如单元测试）。注入后，标记 stale 的同时广播
// message.complete（status=error）给对应 conversation 成员。
func (w *StreamingWatchdog) SetBroadcaster(hub *ws.Hub, convRepo ConvRepoForWatchdog) {
	w.userHub = hub
	w.convRepo = convRepo
}

// Run 启动后台扫描循环。ctx 取消时退出。
func (w *StreamingWatchdog) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	w.logger.Info("streaming_watchdog_start", "max_age", w.maxAge, "interval", w.interval, "broadcast_enabled", w.userHub != nil)
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
//
// 流程：
//   1. ListStaleStreaming 拿到即将被标记的 messages 元数据
//   2. MarkStaleStreaming 批量 UPDATE 为 status=error
//   3. 对每个 stale message 广播 message.complete（status=error）给 conversation 成员
//
// 广播失败（如 userHub 为 nil / ListMemberIDs 失败）只 warn，不影响标记成功。
func (w *StreamingWatchdog) scanOnce(ctx context.Context) {
	// before 取扫描时刻 - maxAge，与 MarkStaleStreaming 的 created_at + maxAge < NOW() 对齐。
	before := time.Now().Add(-w.maxAge)
	staleMsgs, err := w.msgRepo.ListStaleStreaming(ctx, before)
	if err != nil {
		w.logger.Warn("streaming_watchdog_list_stale_failed", "error", err)
		// 继续走 mark，即便 list 失败也不影响 stale 标记本身（广播只是附加功能）
		staleMsgs = nil
	}

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

	// 广播 message.complete（status=error）给受影响的 conversation 成员。
	// 即便 userHub 未注入，staleMsgs 为空也无需继续。
	if w.userHub == nil || len(staleMsgs) == 0 {
		return
	}
	for _, msg := range staleMsgs {
		w.broadcastStaleError(ctx, msg)
	}
}

// broadcastStaleError 向 msg.ConversationID 的成员广播一条 message.complete
// （status=error），让前端把 streaming 占位符切到 error block。
//
// payload 与 hub.handlePersistedMsg 对齐：前端用 message_id 路由到已存在的
// streaming message，用 status 决定是否切到 error block。
// 广播是 best-effort，任何失败只 warn 不 panic。
func (w *StreamingWatchdog) broadcastStaleError(ctx context.Context, msg model.Message) {
	if w.convRepo == nil {
		return
	}
	listCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	memberIDs, err := w.convRepo.ListMemberIDs(listCtx, msg.ConversationID)
	if err != nil {
		w.logger.Warn("streaming_watchdog_list_members_failed",
			"message_id", msg.ID, "conversation_id", msg.ConversationID, "error", err)
		return
	}
	if len(memberIDs) == 0 {
		return
	}
	// 构造最小 message payload：足够前端识别 + 渲染 error block 即可。
	// content / blocks_json 已被 MarkStaleStreaming UPDATE，前端刷新页面会看到完整结构；
	// 这里只广播 status 变更，前端 streaming reducer 据此切到 error 终态。
	payload := map[string]interface{}{
		"id":              msg.ID,
		"conversation_id": msg.ConversationID,
		"role":            msg.Role,
		"status":          model.MessageStatusError,
		"content":         msg.Content,
		"blocks_json":     msg.BlocksJSON,
		"artifacts_json":  msg.ArtifactsJSON,
		"sender_id":       nil,
		"watchdog":        true, // 标记来源，前端可用于打点（不做语义区分）
	}
	// 附加 sender_id（pointer → 字符串），JSON null 是合法值
	if msg.SenderID != nil {
		payload["sender_id"] = *msg.SenderID
	}
	// 尝试附加 username（agent 自报或 user.username），为空则不附加
	if msg.Username != "" {
		payload["username"] = msg.Username
	}
	// cards_json 默认空数组，便于前端 reducer 走统一路径
	if msg.CardsJSON != "" {
		payload["cards_json"] = msg.CardsJSON
	} else {
		payload["cards_json"] = "[]"
	}
	wsMsg := ws.WSMessage{
		Type: ws.TypeMessageComplete,
		Data: payload,
	}
	for _, uid := range memberIDs {
		w.userHub.SendToUser(uid, wsMsg)
	}
	// payload 用于 debug 日志（不包含敏感字段）
	if w.logger.Enabled(ctx, slog.LevelDebug) {
		payloadBytes, _ := json.Marshal(map[string]interface{}{
			"message_id":       msg.ID,
			"conversation_id":  msg.ConversationID,
			"member_count":     len(memberIDs),
			"marked_status":    model.MessageStatusError,
		})
		w.logger.Debug("streaming_watchdog_broadcast",
			"payload", string(payloadBytes))
	}
}
