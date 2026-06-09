package adapter

import (
	"strings"
	"testing"
)

func TestFormatContextAsSystemPromptKeepsPlainTextContext(t *testing.T) {
	ctx := "[平台 Skills]\n{Skill 索引\n- 代码审查：检查 bug\n}\n"
	got := FormatContextAsSystemPrompt(ctx)
	if !strings.Contains(got, "[平台 Skills]") {
		t.Fatalf("expected plain text context to be preserved, got %q", got)
	}
}
