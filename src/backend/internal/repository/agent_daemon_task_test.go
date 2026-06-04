package repository

import (
	"context"
	"sync"
	"testing"
)

// 内存版 daemon 任务队列：不依赖 DB，db 字段保持 nil。
func newTaskRepo() *AgentRepo { return NewAgentRepo(nil) }

func TestDaemonTask_Lifecycle(t *testing.T) {
	r := newTaskRepo()
	ctx := context.Background()

	created, err := r.CreateDaemonTask(ctx, "u1", "c1", "a1", "m1", "openclaw", "hi", "ctx")
	if err != nil || created == nil {
		t.Fatalf("create: %v", err)
	}
	if created.Status != "pending" || created.ID == "" {
		t.Fatalf("unexpected created task: %+v", created)
	}
	if created.ContextMessages != "ctx" || created.Prompt != "hi" {
		t.Fatalf("context/prompt not preserved: %+v", created)
	}

	got, _ := r.GetDaemonTask(ctx, created.ID)
	if got == nil || got.Status != "pending" {
		t.Fatalf("get pending: %+v", got)
	}

	claimed, _ := r.ClaimDaemonTask(ctx, "m1")
	if claimed == nil || claimed.ID != created.ID || claimed.Status != "running" {
		t.Fatalf("claim: %+v", claimed)
	}
	if claimed.ClaimedAt == nil {
		t.Fatalf("claimed_at not set")
	}

	ok, _ := r.CompleteDaemonTask(ctx, created.ID, "m1", "done!", "")
	if !ok {
		t.Fatalf("complete should succeed")
	}
	done, _ := r.GetDaemonTask(ctx, created.ID)
	if done.Status != "completed" || done.Result != "done!" || done.CompletedAt == nil {
		t.Fatalf("completed task: %+v", done)
	}
}

func TestDaemonTask_FIFOPerMachine(t *testing.T) {
	r := newTaskRepo()
	ctx := context.Background()
	t1, _ := r.CreateDaemonTask(ctx, "u", "", "a", "m1", "claude", "first", "")
	t2, _ := r.CreateDaemonTask(ctx, "u", "", "a", "m1", "claude", "second", "")
	// 另一台机器的任务不应被 m1 领走
	_, _ = r.CreateDaemonTask(ctx, "u", "", "a", "m2", "claude", "other", "")

	c1, _ := r.ClaimDaemonTask(ctx, "m1")
	c2, _ := r.ClaimDaemonTask(ctx, "m1")
	if c1.ID != t1.ID || c2.ID != t2.ID {
		t.Fatalf("FIFO order broken: got %s,%s want %s,%s", c1.ID, c2.ID, t1.ID, t2.ID)
	}
	if empty, _ := r.ClaimDaemonTask(ctx, "m1"); empty != nil {
		t.Fatalf("expected no more tasks for m1, got %+v", empty)
	}
}

func TestDaemonTask_ClaimEmptyAndGuards(t *testing.T) {
	r := newTaskRepo()
	ctx := context.Background()
	if c, _ := r.ClaimDaemonTask(ctx, "none"); c != nil {
		t.Fatalf("claim empty should be nil")
	}
	created, _ := r.CreateDaemonTask(ctx, "u", "", "a", "m1", "claude", "x", "")
	// 未领取(pending)时 complete 应失败
	if ok, _ := r.CompleteDaemonTask(ctx, created.ID, "m1", "r", ""); ok {
		t.Fatalf("complete on pending should fail")
	}
	_, _ = r.ClaimDaemonTask(ctx, "m1")
	// 机器不匹配应失败
	if ok, _ := r.CompleteDaemonTask(ctx, created.ID, "wrong", "r", ""); ok {
		t.Fatalf("complete with wrong machine should fail")
	}
	// 正确完成
	if ok, _ := r.CompleteDaemonTask(ctx, created.ID, "m1", "r", ""); !ok {
		t.Fatalf("correct complete should succeed")
	}
	// 重复完成(已非 running)应失败
	if ok, _ := r.CompleteDaemonTask(ctx, created.ID, "m1", "r", ""); ok {
		t.Fatalf("double complete should fail")
	}
}

func TestDaemonTask_FailedStatus(t *testing.T) {
	r := newTaskRepo()
	ctx := context.Background()
	created, _ := r.CreateDaemonTask(ctx, "u", "", "a", "m1", "claude", "x", "")
	_, _ = r.ClaimDaemonTask(ctx, "m1")
	_, _ = r.CompleteDaemonTask(ctx, created.ID, "m1", "", "boom")
	got, _ := r.GetDaemonTask(ctx, created.ID)
	if got.Status != "failed" || got.Error != "boom" {
		t.Fatalf("failed task: %+v", got)
	}
}

// 并发领取同一机器的任务不应重复领取（每条任务至多被领一次）。
func TestDaemonTask_ConcurrentClaimNoDuplicate(t *testing.T) {
	r := newTaskRepo()
	ctx := context.Background()
	const n = 50
	ids := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		task, _ := r.CreateDaemonTask(ctx, "u", "", "a", "m1", "claude", "x", "")
		ids[task.ID] = true
	}

	var mu sync.Mutex
	claimed := make(map[string]int)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				task, _ := r.ClaimDaemonTask(ctx, "m1")
				if task == nil {
					return
				}
				mu.Lock()
				claimed[task.ID]++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(claimed) != n {
		t.Fatalf("claimed %d distinct tasks, want %d", len(claimed), n)
	}
	for id, count := range claimed {
		if count != 1 {
			t.Fatalf("task %s claimed %d times (want 1)", id, count)
		}
	}
}
