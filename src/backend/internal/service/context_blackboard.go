package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/agent-hub/backend/internal/model"
)

// blackboardPinLimit 等常量已定义在 orchestrator.go，这里直接复用。

// BlackboardBuilder 构建「会话上下文黑板」段，前置到 current。
// 黑板为空时返回 current 不变。
type BlackboardBuilder struct {
	MsgRepo MsgRepo
}

// Build 实现 ContextBuilder。
func (b *BlackboardBuilder) Build(ctx context.Context, in ContextInput, current string) string {
	if b.MsgRepo == nil || in.ConvID == "" {
		return current
	}
	text := BuildBlackboardText(ctx, b.MsgRepo, in.ConvID)
	if text == "" {
		return current
	}
	return text + current
}

// BuildBlackboardText 从 MsgRepo 加载 Pin 消息 + 手写上下文，
// 生成 `{会话上下文黑板 ...}` 段。出错时记 warn 并返回空串。
// 抽出来的逻辑供 BlackboardBuilder 与 OrchestratorService façade 方法复用。
func BuildBlackboardText(ctx context.Context, repo MsgRepo, convID string) string {
	if repo == nil {
		return ""
	}
	items, err := repo.ListPinnedMessages(ctx, convID, blackboardPinLimit)
	if err != nil {
		slog.Warn("build blackboard context failed", "conversation_id", convID, "error", err)
		return ""
	}
	blackboard, err := repo.GetConversationBlackboard(ctx, convID)
	if err != nil {
		slog.Warn("load manual blackboard context failed", "conversation_id", convID, "error", err)
		blackboard = &model.ConversationBlackboard{ConversationID: convID, ManualContext: ""}
	}
	var sb strings.Builder
	sb.WriteString("{会话上下文黑板\n")
	sb.WriteString("{用户 Pin 上下文\n")
	if len(items) == 0 {
		sb.WriteString("无\n")
	} else {
		for _, item := range items {
			author := fallbackText(item.Username)
			content := normalizePromptLine(truncateString(item.Content, blackboardMaxEntryRunes))
			fmt.Fprintf(&sb, "- %s: %s\n", author, content)
		}
	}
	sb.WriteString("}\n")
	sb.WriteString("{用户手写上下文\n")
	manualContext := ""
	if blackboard != nil {
		manualContext = strings.TrimSpace(blackboard.ManualContext)
	}
	if manualContext == "" {
		sb.WriteString("无\n")
	} else {
		truncatedManual := truncateString(manualContext, blackboardMaxManualRunes)
		sb.WriteString(truncatedManual)
		if !strings.HasSuffix(truncatedManual, "\n") {
			sb.WriteString("\n")
		}
	}
	sb.WriteString("}\n")
	sb.WriteString("}\n\n")

	result := sb.String()
	if len([]rune(result)) > blackboardMaxContextRunes {
		return truncateString(result, blackboardMaxContextRunes)
	}
	return result
}
