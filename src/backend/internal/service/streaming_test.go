package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/agent-hub/backend/internal/model"
)

// TestStreamingWatchdog_MarksStaleMessage 验证 watchdog 把超时的 streaming message 标 error。
func TestStreamingWatchdog_MarksStaleMessage(t *testing.T) {
	repo := &fakeMsgRepo{}
	// 预创建一条 streaming message，created_at 在 2 分钟前（超过 60s 阈值）
	old := model.Message{
		ID:        "msg-old",
		Status:    model.MessageStatusStreaming,
		CreatedAt: time.Now().Add(-2 * time.Minute),
	}
	repo.messages = append(repo.messages, old)

	wd := NewStreamingWatchdog(repo, slog.Default(), 60*time.Second, time.Hour)
	wd.scanOnce(context.Background())

	if repo.messages[0].Status != model.MessageStatusError {
		t.Errorf("expected stale message marked error, got %q", repo.messages[0].Status)
	}
}

// TestStreamingWatchdog_SkipsFreshMessage 验证新鲜的 streaming message 不被误标。
func TestStreamingWatchdog_SkipsFreshMessage(t *testing.T) {
	repo := &fakeMsgRepo{}
	fresh := model.Message{
		ID:        "msg-fresh",
		Status:    model.MessageStatusStreaming,
		CreatedAt: time.Now().Add(-5 * time.Second), // 5s 前，远低于 60s 阈值
	}
	repo.messages = append(repo.messages, fresh)

	wd := NewStreamingWatchdog(repo, slog.Default(), 60*time.Second, time.Hour)
	wd.scanOnce(context.Background())

	if repo.messages[0].Status != model.MessageStatusStreaming {
		t.Errorf("fresh streaming message should not be marked, got %q", repo.messages[0].Status)
	}
}

// TestStreamingWatchdog_SkipsCompleteMessage 验证 complete 状态的 message 不被误碰。
func TestStreamingWatchdog_SkipsCompleteMessage(t *testing.T) {
	repo := &fakeMsgRepo{}
	old := model.Message{
		ID:        "msg-complete",
		Status:    model.MessageStatusComplete,
		CreatedAt: time.Now().Add(-2 * time.Hour),
	}
	repo.messages = append(repo.messages, old)

	wd := NewStreamingWatchdog(repo, slog.Default(), 60*time.Second, time.Hour)
	wd.scanOnce(context.Background())

	if repo.messages[0].Status != model.MessageStatusComplete {
		t.Errorf("complete message status should not change, got %q", repo.messages[0].Status)
	}
}

// TestStreamingWatchdog_Defaults 验证 maxAge / interval 默认值。
func TestStreamingWatchdog_Defaults(t *testing.T) {
	wd := NewStreamingWatchdog(&fakeMsgRepo{}, slog.Default(), 0, 0)
	if wd.maxAge != 60*time.Second {
		t.Errorf("default maxAge should be 60s, got %v", wd.maxAge)
	}
	if wd.interval != 10*time.Second {
		t.Errorf("default interval should be 10s, got %v", wd.interval)
	}
}

// TestStreamingWatchdog_RepoError 验证 repo 错误被吞掉（不影响后续 ticker）。
func TestStreamingWatchdog_RepoError(t *testing.T) {
	repo := &failingStreamingRepo{}
	wd := NewStreamingWatchdog(repo, slog.Default(), 60*time.Second, time.Hour)
	// 应该不 panic
	wd.scanOnce(context.Background())
}

type failingStreamingRepo struct{}

func (f *failingStreamingRepo) MarkStaleStreaming(ctx context.Context, maxAge time.Duration) (int, error) {
	return 0, errors.New("simulated DB error")
}

// TestCreateStreaming_InitialStatus 验证预创建的 message 初始 status 是 streaming。
func TestCreateStreaming_InitialStatus(t *testing.T) {
	repo := &fakeMsgRepo{}
	msg, err := repo.CreateStreaming(context.Background(), "conv-1", "assistant", strPtr("agent-1"), nil)
	if err != nil {
		t.Fatalf("CreateStreaming failed: %v", err)
	}
	if msg.Status != model.MessageStatusStreaming {
		t.Errorf("expected initial status streaming, got %q", msg.Status)
	}
	if msg.ID != "msg-streaming" {
		t.Errorf("expected msg-streaming ID, got %q", msg.ID)
	}
	// messages slice 应该包含新创建的 streaming message
	found := false
	for _, m := range repo.messages {
		if m.ID == msg.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("CreateStreaming did not append to messages slice")
	}
}

// TestFinalizeStreaming_StatusTransition 验证 finalize 切换 status + 写入 content。
func TestFinalizeStreaming_StatusTransition(t *testing.T) {
	repo := &fakeMsgRepo{}
	msg, _ := repo.CreateStreaming(context.Background(), "conv-1", "assistant", strPtr("agent-1"), nil)

	err := repo.FinalizeStreaming(context.Background(), msg.ID, model.MessageStatusComplete, "final content", "", `{"agent_id":"a1"}`)
	if err != nil {
		t.Fatalf("FinalizeStreaming failed: %v", err)
	}
	// 从 repo.messages 取回验证
	got := repo.messages[0]
	if got.Status != model.MessageStatusComplete {
		t.Errorf("expected status complete, got %q", got.Status)
	}
	if got.Content != "final content" {
		t.Errorf("expected content 'final content', got %q", got.Content)
	}
	if !strings.Contains(got.ArtifactsJSON, "agent_id") {
		t.Errorf("expected artifacts_json with agent_id, got %q", got.ArtifactsJSON)
	}
}

// TestFinalizeStreaming_StatusError 验证 error 状态：保留空 content（前端显示 error block）。
func TestFinalizeStreaming_StatusError(t *testing.T) {
	repo := &fakeMsgRepo{}
	msg, _ := repo.CreateStreaming(context.Background(), "conv-1", "assistant", strPtr("agent-1"), nil)

	_ = repo.FinalizeStreaming(context.Background(), msg.ID, model.MessageStatusError, "", "", "")
	got := repo.messages[0]
	if got.Status != model.MessageStatusError {
		t.Errorf("expected status error, got %q", got.Status)
	}
	if got.Content != "" {
		t.Errorf("error path should keep content empty, got %q", got.Content)
	}
}

// TestListStreaming_FiltersByStatus 验证 ListStreaming 只返回 streaming 状态。
func TestListStreaming_FiltersByStatus(t *testing.T) {
	repo := &fakeMsgRepo{}
	repo.messages = []model.Message{
		{ID: "m1", Status: model.MessageStatusStreaming},
		{ID: "m2", Status: model.MessageStatusComplete},
		{ID: "m3", Status: model.MessageStatusStreaming},
		{ID: "m4", Status: model.MessageStatusError},
	}
	streaming, err := repo.ListStreaming(context.Background())
	if err != nil {
		t.Fatalf("ListStreaming failed: %v", err)
	}
	if len(streaming) != 2 {
		t.Errorf("expected 2 streaming messages, got %d", len(streaming))
	}
}

// 辅助：把 string 转 *string。
func strPtr(s string) *string { return &s }
