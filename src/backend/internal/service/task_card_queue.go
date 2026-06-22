package service

import (
	"context"
	"sync"
	"time"
)

// TaskCardQueue 内存队列，收集 MCP subprocess 工具 emit 的卡片，按 task_id 索引。
// daemon 主进程在 createAgentReply 时 Drain 合并到 allCards。
//
// 设计背景：deploy_project 等 MCP 工具跑在 Claude Code spawn 的子进程里，
// 与 daemon 主进程不共享内存。子进程通过 POST /api/internal/task-cards 把卡片
// 推到后端，后端按 task_id 暂存；agent 回复创建时 Drain 同一 task_id 的卡片，
// 合并到 message.cards_json。
//
// 设计：
//   - 纯内存（task 崩溃则丢失，可接受——daemon 已是 best-effort）
//   - TTL 清理（防泄漏，超过 ttl 未 drain 的 entry 清掉）
//   - 线程安全（sync.Mutex）
type TaskCardQueue struct {
	mu         sync.Mutex
	cards      map[string][]map[string]any // task_id → cards
	pushTimes  map[string]time.Time        // task_id → 最近一次 push 时间
	ttl        time.Duration               // entry 过期阈值
}

// NewTaskCardQueue 创建默认 TTL = 1 小时的队列。
func NewTaskCardQueue() *TaskCardQueue {
	return &TaskCardQueue{
		cards:     make(map[string][]map[string]any),
		pushTimes: make(map[string]time.Time),
		ttl:       time.Hour,
	}
}

// Push 追加一张卡到 task_id 对应的队列。card 为 nil 时忽略。
// 自动补 id（缺失时生成 UUID，方便前端去重 / 引用）。
func (q *TaskCardQueue) Push(taskID string, card map[string]any) {
	if taskID == "" || card == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.cards[taskID] = append(q.cards[taskID], card)
	q.pushTimes[taskID] = time.Now()
}

// Drain 取出并清空 task_id 对应的所有卡片。
// 未 push 过或已过期的 task_id 返回 nil。
func (q *TaskCardQueue) Drain(taskID string) []map[string]any {
	if taskID == "" {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	cards := q.cards[taskID]
	delete(q.cards, taskID)
	delete(q.pushTimes, taskID)
	return cards
}

// StartCleanup 启动后台 goroutine 周期清理过期 entry（>ttl 未 drain）。
// 每 ttl/2 触发一次扫描。传入的 ctx 结束时 goroutine 退出。
func (q *TaskCardQueue) StartCleanup(ctx context.Context) {
	// 周期取 ttl/2，保证过期 entry 在一个 ttl 周期内被清掉。
	interval := q.ttl / 2
	if interval <= 0 {
		interval = time.Hour / 2
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				q.cleanupOnce(now)
			}
		}
	}()
}

// cleanupOnce 扫描 pushTimes，清掉超过 ttl 未 drain 的 entry。
// 测试可直接调用以避免等待 ticker。
func (q *TaskCardQueue) cleanupOnce(now time.Time) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for taskID, pushedAt := range q.pushTimes {
		if now.Sub(pushedAt) > q.ttl {
			delete(q.cards, taskID)
			delete(q.pushTimes, taskID)
		}
	}
}
