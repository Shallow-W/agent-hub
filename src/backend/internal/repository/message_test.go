package repository

import (
	"strings"
	"testing"
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
