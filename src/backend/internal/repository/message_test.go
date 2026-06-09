package repository

import (
	"strings"
	"testing"

	"github.com/agent-hub/backend/internal/model"
)

func TestTruncateRunesDoesNotSplitMultibyteCharacters(t *testing.T) {
	input := strings.Repeat("测", 60)
	got := truncateRunes(input, 50)
	if strings.ContainsRune(got, '\uFFFD') {
		t.Fatalf("expected no replacement character, got %q", got)
	}
	if got != strings.Repeat("测", 50)+"..." {
		t.Fatalf("unexpected truncation result: %q", got)
	}
}

func TestTruncateRunesKeepsShortText(t *testing.T) {
	input := "我目前正在测试你作为orch的功能"
	if got := truncateRunes(input, 50); got != input {
		t.Fatalf("expected short text unchanged, got %q", got)
	}
}

func TestReplyPreviewUsernameUsesAssistantAgentName(t *testing.T) {
	got := replyPreviewUsername("assistant", "wjc", `{"agent_name":"员工2"}`)
	if got != "员工2" {
		t.Fatalf("expected assistant agent name, got %q", got)
	}
}

func TestReplyPreviewUsernameKeepsUserName(t *testing.T) {
	got := replyPreviewUsername("user", "wjc", `{"agent_name":"员工2"}`)
	if got != "wjc" {
		t.Fatalf("expected user name, got %q", got)
	}
}

func TestCollectReplyIDsSkipsEmptyAndDuplicates(t *testing.T) {
	empty := ""
	spaces := "   "
	id := "15b80856-efd4-431e-8468-a95dd0b79ce6"
	other := "72248706-553d-4a23-a82c-a7410f59b2ef"

	got := collectReplyIDs([]model.Message{
		{ReplyTo: nil},
		{ReplyTo: &empty},
		{ReplyTo: &spaces},
		{ReplyTo: &id},
		{ReplyTo: &id},
		{ReplyTo: &other},
	})

	if len(got) != 2 {
		t.Fatalf("expected 2 reply IDs, got %#v", got)
	}
	if got[0] != id || got[1] != other {
		t.Fatalf("unexpected reply IDs: %#v", got)
	}
}
