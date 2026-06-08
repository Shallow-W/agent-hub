package handler

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeKnowledgeFilePathAllowsStoredRelativePath(t *testing.T) {
	uploadDir := t.TempDir()

	got, err := safeKnowledgeFilePath(uploadDir, filepath.Join("knowledge", "kb-id", "file.txt"))
	if err != nil {
		t.Fatalf("safeKnowledgeFilePath returned error: %v", err)
	}

	want := filepath.Join(uploadDir, "knowledge", "kb-id", "file.txt")
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestSafeKnowledgeFilePathRejectsTraversal(t *testing.T) {
	uploadDir := t.TempDir()

	if _, err := safeKnowledgeFilePath(uploadDir, filepath.Join("..", "secret.txt")); err == nil {
		t.Fatal("expected traversal path to be rejected")
	}
}

func TestSafeKnowledgeFilePathRejectsAbsolutePath(t *testing.T) {
	uploadDir := t.TempDir()
	absPath := filepath.Join(string(os.PathSeparator), "tmp", "secret.txt")

	if _, err := safeKnowledgeFilePath(uploadDir, absPath); err == nil {
		t.Fatal("expected absolute path to be rejected")
	}
}
