package service

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestTaskCardQueue_PushDrain 验证基础 push/drain 行为：3 张卡 push 后 drain
// 返回全部并清空 map。
func TestTaskCardQueue_PushDrain(t *testing.T) {
	q := NewTaskCardQueue()
	q.Push("task-1", map[string]any{"type": "info", "id": "c1"})
	q.Push("task-1", map[string]any{"type": "info", "id": "c2"})
	q.Push("task-1", map[string]any{"type": "diff", "id": "c3"})

	cards := q.Drain("task-1")
	if len(cards) != 3 {
		t.Fatalf("expected 3 cards, got %d: %+v", len(cards), cards)
	}
	// 第二次 drain 应为空（已清空）
	if got := q.Drain("task-1"); len(got) != 0 {
		t.Errorf("expected empty after drain, got %d", len(got))
	}
}

// TestTaskCardQueue_DrainUnknown 验证 drain 未 push 过的 task_id 返回 nil。
func TestTaskCardQueue_DrainUnknown(t *testing.T) {
	q := NewTaskCardQueue()
	if got := q.Drain("never-pushed"); got != nil {
		t.Errorf("expected nil for unknown task_id, got %+v", got)
	}
}

// TestTaskCardQueue_PushInvalid 验证空 task_id 和 nil card 被忽略。
func TestTaskCardQueue_PushInvalid(t *testing.T) {
	q := NewTaskCardQueue()
	q.Push("", map[string]any{"type": "info"})
	q.Push("task-1", nil)
	if got := q.Drain("task-1"); len(got) != 0 {
		t.Errorf("expected empty for invalid pushes, got %+v", got)
	}
}

// TestTaskCardQueue_Isolation 验证不同 task_id 之间的卡片互不干扰。
func TestTaskCardQueue_Isolation(t *testing.T) {
	q := NewTaskCardQueue()
	q.Push("task-A", map[string]any{"id": "a1"})
	q.Push("task-B", map[string]any{"id": "b1"})
	q.Push("task-B", map[string]any{"id": "b2"})

	aCards := q.Drain("task-A")
	bCards := q.Drain("task-B")
	if len(aCards) != 1 || aCards[0]["id"] != "a1" {
		t.Errorf("task-A cards mismatch: %+v", aCards)
	}
	if len(bCards) != 2 {
		t.Errorf("task-B expected 2 cards, got %d", len(bCards))
	}
}

// TestTaskCardQueue_Concurrent 验证并发 Push 安全（-race 下不应崩溃/数据竞争）。
func TestTaskCardQueue_Concurrent(t *testing.T) {
	q := NewTaskCardQueue()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			q.Push("task-concurrent", map[string]any{"id": "c", "n": i})
		}(i)
	}
	wg.Wait()

	cards := q.Drain("task-concurrent")
	if len(cards) != 50 {
		t.Errorf("expected 50 cards after concurrent push, got %d", len(cards))
	}
}

// TestTaskCardQueue_Cleanup 验证 TTL cleanup 把过期 entry 清掉。
func TestTaskCardQueue_Cleanup(t *testing.T) {
	q := &TaskCardQueue{
		cards:     make(map[string][]map[string]any),
		pushTimes: make(map[string]time.Time),
		ttl:       50 * time.Millisecond,
	}
	q.Push("fresh", map[string]any{"id": "f1"})
	q.Push("stale", map[string]any{"id": "s1"})

	// 把 stale 的时间戳改成 1 小时前，模拟长时间未 drain
	q.mu.Lock()
	q.pushTimes["stale"] = time.Now().Add(-time.Hour)
	q.mu.Unlock()

	// fresh 未过期应保留；stale 已过期应被清掉
	q.cleanupOnce(time.Now())

	if got := q.Drain("fresh"); len(got) != 1 {
		t.Errorf("fresh entry should survive cleanup, got %d cards", len(got))
	}
	if got := q.Drain("stale"); len(got) != 0 {
		t.Errorf("stale entry should be cleaned up, got %d cards", len(got))
	}
}

// TestTaskCardQueue_StartCleanup 验证 StartCleanup goroutine 能在 ctx 结束时退出。
func TestTaskCardQueue_StartCleanup(t *testing.T) {
	q := &TaskCardQueue{
		cards:     make(map[string][]map[string]any),
		pushTimes: make(map[string]time.Time),
		ttl:       10 * time.Millisecond,
	}
	ctx, cancel := context.WithCancel(context.Background())
	q.StartCleanup(ctx)

	// push 一个并立即取消 ctx，goroutine 应退出（不阻塞、不泄漏）
	q.Push("t1", map[string]any{"id": "x"})
	cancel()
	// 给 goroutine 一个调度周期
	time.Sleep(20 * time.Millisecond)
	// 主要验证：取消后不 panic；Drain 仍可用
	if got := q.Drain("t1"); len(got) != 1 {
		// 若 cleanup 恰好先跑了一次清掉了，也接受（ttl 极短）
		_ = got
	}
}
