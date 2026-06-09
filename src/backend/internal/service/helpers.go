package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/agent-hub/backend/internal/model"
)

// daemonTaskGetter 获取 daemon 任务的最小接口，供 waitDaemonTask 使用
type daemonTaskGetter interface {
	GetDaemonTask(ctx context.Context, id string) (*model.DaemonTask, error)
}

// truncateString 截断字符串到 maxRunes 个 rune，超出追加 "..."
func truncateString(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

var promptWhitespaceRE = regexp.MustCompile(`\s+`)

func normalizePromptLine(s string) string {
	return strings.TrimSpace(promptWhitespaceRE.ReplaceAllString(s, " "))
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func optionalStringPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

// waitDaemonTask 轮询等待 daemon 任务完成（600ms 间隔，120s 超时）
// 供 OrchestratorService 和 MessageService 共享使用
func waitDaemonTask(ctx context.Context, repo daemonTaskGetter, taskID string) (*model.DaemonTask, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	ticker := time.NewTicker(600 * time.Millisecond)
	defer ticker.Stop()

	for {
		task, err := repo.GetDaemonTask(ctx, taskID)
		if err != nil {
			return nil, fmt.Errorf("get daemon task: %w", err)
		}
		if task != nil && (task.Status == "completed" || task.Status == "failed") {
			return task, nil
		}

		select {
		case <-ctx.Done():
			return nil, ErrMsgAgentTimeout
		case <-ticker.C:
		}
	}
}
