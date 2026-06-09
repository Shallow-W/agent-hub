package handler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agent-hub/backend/internal/service"
)

func TestSafeJoinUploadPathAllowsStoredRelativePath(t *testing.T) {
	uploadDir := t.TempDir()

	got, err := service.SafeJoinUploadPath(uploadDir, filepath.Join("knowledge", "kb-id", "file.txt"))
	if err != nil {
		t.Fatalf("SafeJoinUploadPath returned error: %v", err)
	}

	want := filepath.Join(uploadDir, "knowledge", "kb-id", "file.txt")
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestSafeJoinUploadPathRejectsTraversal(t *testing.T) {
	uploadDir := t.TempDir()

	if _, err := service.SafeJoinUploadPath(uploadDir, filepath.Join("..", "secret.txt")); err == nil {
		t.Fatal("expected traversal path to be rejected")
	}
}

func TestSafeJoinUploadPathRejectsAbsolutePath(t *testing.T) {
	uploadDir := t.TempDir()
	absPath := filepath.Join(string(os.PathSeparator), "tmp", "secret.txt")

	if _, err := service.SafeJoinUploadPath(uploadDir, absPath); err == nil {
		t.Fatal("expected absolute path to be rejected")
	}
}
