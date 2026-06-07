package service

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const (
	machineOfflineThreshold = 75 * time.Second
	machineSweepInterval    = 30 * time.Second
)

// MachineStatusRepo 机器状态持久化接口（仅在线/离线变更时调用）
type MachineStatusRepo interface {
	SetMachineAndAgentsOnline(ctx context.Context, machineID string) error
	SetMachineAndAgentsOffline(ctx context.Context, machineID string) error
}

type machineEntry struct {
	lastHB   time.Time
	dbOnline bool
}

// MachineTracker 基于内存心跳的机器在线状态追踪器。
// 仅在状态变更（online↔offline）时写 DB，中间过程零 DB 开销。
type MachineTracker struct {
	mu       sync.Mutex
	machines map[string]*machineEntry // machineID -> entry
	repo     MachineStatusRepo
	logger   *slog.Logger
}

func NewMachineTracker(repo MachineStatusRepo, logger *slog.Logger) *MachineTracker {
	return &MachineTracker{
		machines: make(map[string]*machineEntry),
		repo:     repo,
		logger:   logger,
	}
}

// Touch 更新心跳（ClaimTask 每 1.5s 调用一次，纯内存操作）。
// 若机器之前不在线（dbOnline=false），同步写 DB 保证状态一致。
func (t *MachineTracker) Touch(machineID string) {
	now := time.Now()
	needMarkOnline := false

	t.mu.Lock()
	entry, exists := t.machines[machineID]
	if !exists {
		entry = &machineEntry{lastHB: now, dbOnline: false}
		t.machines[machineID] = entry
		needMarkOnline = true
	} else {
		entry.lastHB = now
		if !entry.dbOnline {
			needMarkOnline = true
		}
	}
	t.mu.Unlock()

	if needMarkOnline {
		if err := t.markOnlineSync(machineID); err == nil {
			t.mu.Lock()
			if e, ok := t.machines[machineID]; ok {
				e.dbOnline = true
			}
			t.mu.Unlock()
		}
	}
}

// MarkOnline 标记上线（daemon register 时调用，内存 + 同步 DB）。
func (t *MachineTracker) MarkOnline(machineID string) {
	t.mu.Lock()
	entry, exists := t.machines[machineID]
	if !exists {
		entry = &machineEntry{lastHB: time.Now(), dbOnline: false}
		t.machines[machineID] = entry
	} else {
		entry.lastHB = time.Now()
	}
	needDB := !entry.dbOnline
	t.mu.Unlock()

	if needDB {
		if err := t.markOnlineSync(machineID); err == nil {
			t.mu.Lock()
			if e, ok := t.machines[machineID]; ok {
				e.dbOnline = true
			}
			t.mu.Unlock()
		}
	}
}

// IsOnline 检查机器是否在线（纯内存读取）。
func (t *MachineTracker) IsOnline(machineID string) bool {
	t.mu.Lock()
	entry, ok := t.machines[machineID]
	t.mu.Unlock()
	return ok && time.Since(entry.lastHB) < machineOfflineThreshold
}

// Run 启动后台清扫 goroutine，应在独立 goroutine 中调用。
func (t *MachineTracker) Run(ctx context.Context) {
	// 启动时将所有机器标记为离线，清除上次运行残留的 stale 状态
	t.resetAll()

	ticker := time.NewTicker(machineSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.sweep()
		}
	}
}

// resetAll 将 DB 中所有机器和 Agent 标记为离线（服务启动时调用）。
func (t *MachineTracker) resetAll() {
	dbCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := t.repo.SetMachineAndAgentsOffline(dbCtx, ""); err != nil {
		t.logger.Warn("reset machines on startup failed", "error", err)
	}
}

// sweep 检测超时机器，仅当 dbOnline=true 时才写 DB（避免与 markOnline 竞争）。
func (t *MachineTracker) sweep() {
	now := time.Now()
	var stale []string

	t.mu.Lock()
	for id, entry := range t.machines {
		if now.Sub(entry.lastHB) > machineOfflineThreshold {
			if entry.dbOnline {
				stale = append(stale, id)
			}
			delete(t.machines, id)
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

// MarkOffline 标记单个机器离线（WS 断开时调用，内存 + 同步 DB）。
func (t *MachineTracker) MarkOffline(machineID string) {
	t.mu.Lock()
	entry, exists := t.machines[machineID]
	if exists {
		entry.dbOnline = false
		delete(t.machines, machineID)
	}
	t.mu.Unlock()

	dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := t.repo.SetMachineAndAgentsOffline(dbCtx, machineID); err != nil {
		t.logger.Error("mark machine offline failed", "machine_id", machineID, "error", err)
	}
	cancel()
	t.logger.Info("machine went offline (ws disconnect)", "machine_id", machineID)
}

func (t *MachineTracker) markOnlineSync(machineID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := t.repo.SetMachineAndAgentsOnline(ctx, machineID); err != nil {
		t.logger.Error("mark machine online failed", "machine_id", machineID, "error", err)
		return err
	}
	return nil
}
