package service

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const (
	machineOfflineThreshold = 15 * time.Second
	machineSweepInterval    = 30 * time.Second
)

// MachineStatusRepo 机器状态持久化接口（仅在线/离线变更时调用）
type MachineStatusRepo interface {
	SetMachineAndAgentsOnline(ctx context.Context, machineID string) error
	SetMachineAndAgentsOffline(ctx context.Context, machineID string) error
}

// MachineTracker 基于内存心跳的机器在线状态追踪器。
// 仅在状态变更（online↔offline）时写 DB，中间过程零 DB 开销。
type MachineTracker struct {
	mu         sync.Mutex
	heartbeats map[string]time.Time // machineID -> last heartbeat time
	repo       MachineStatusRepo
	logger     *slog.Logger
}

func NewMachineTracker(repo MachineStatusRepo, logger *slog.Logger) *MachineTracker {
	return &MachineTracker{
		heartbeats: make(map[string]time.Time),
		repo:       repo,
		logger:     logger,
	}
}

// Touch 更新心跳（ClaimTask 每 1.5s 调用一次，纯内存操作）。
// 若机器之前不在 map 中（离线→上线），异步写 DB。
func (t *MachineTracker) Touch(machineID string) {
	now := time.Now()
	t.mu.Lock()
	_, existed := t.heartbeats[machineID]
	t.heartbeats[machineID] = now
	t.mu.Unlock()

	if !existed {
		go t.markOnline(machineID)
	}
}

// MarkOnline 标记上线（daemon register 时调用，内存 + DB）。
func (t *MachineTracker) MarkOnline(machineID string) {
	t.mu.Lock()
	t.heartbeats[machineID] = time.Now()
	t.mu.Unlock()
	go t.markOnline(machineID)
}

// IsOnline 检查机器是否在线（纯内存读取）。
func (t *MachineTracker) IsOnline(machineID string) bool {
	t.mu.Lock()
	last, ok := t.heartbeats[machineID]
	t.mu.Unlock()
	return ok && time.Since(last) < machineOfflineThreshold
}

// Run 启动后台清扫 goroutine，应在独立 goroutine 中调用。
func (t *MachineTracker) Run(ctx context.Context) {
	// 启动时将所有机器标记为离线，清除上次运行残留的 stale 状态
	t.resetAll(ctx)

	ticker := time.NewTicker(machineSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.sweep(ctx)
		}
	}
}

// resetAll 将 DB 中所有机器和 Agent 标记为离线（服务启动时调用）。
func (t *MachineTracker) resetAll(ctx context.Context) {
	dbCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := t.repo.SetMachineAndAgentsOffline(dbCtx, ""); err != nil {
		t.logger.Warn("reset machines on startup failed", "error", err)
	}
}

// sweep 检测超时机器，删除内存记录 + 写 DB（仅离线时）。
func (t *MachineTracker) sweep(ctx context.Context) {
	now := time.Now()
	var stale []string

	t.mu.Lock()
	for id, last := range t.heartbeats {
		if now.Sub(last) > machineOfflineThreshold {
			delete(t.heartbeats, id)
			stale = append(stale, id)
		}
	}
	t.mu.Unlock()

	for _, id := range stale {
		dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := t.repo.SetMachineAndAgentsOffline(dbCtx, id); err != nil {
			t.logger.Error("mark machine offline failed", "machine_id", id, "error", err)
		}
		cancel()
		t.logger.Info("machine went offline", "machine_id", id)
	}
}

func (t *MachineTracker) markOnline(machineID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := t.repo.SetMachineAndAgentsOnline(ctx, machineID); err != nil {
		t.logger.Error("mark machine online failed", "machine_id", machineID, "error", err)
	}
}
