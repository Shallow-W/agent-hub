package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFileURLBuilderUploadURL(t *testing.T) {
	t.Run("relative api url", func(t *testing.T) {
		builder := NewFileURLBuilder("")
		got := builder.UploadURL("uploads/originals/file.png")
		if got != "/api/uploads/originals/file.png" {
			t.Fatalf("UploadURL() = %q", got)
		}
	})

	t.Run("absolute public url", func(t *testing.T) {
		builder := NewFileURLBuilder("http://111.228.35.61:8080/")
		got := builder.UploadURL("uploads/thumbnails/file.jpg")
		if got != "http://111.228.35.61:8080/api/uploads/thumbnails/file.jpg" {
			t.Fatalf("UploadURL() = %q", got)
		}
	})

	t.Run("path without uploads prefix", func(t *testing.T) {
		builder := NewFileURLBuilder("https://agenthub.example.com")
		got := builder.UploadURL("/knowledge/kb-1/file.pdf")
		if got != "https://agenthub.example.com/api/uploads/knowledge/kb-1/file.pdf" {
			t.Fatalf("UploadURL() = %q", got)
		}
	})

	t.Run("reject traversal", func(t *testing.T) {
		builder := NewFileURLBuilder("https://agenthub.example.com")
		if got := builder.UploadURL("uploads/originals/../secret.png"); got != "" {
			t.Fatalf("UploadURL() traversal = %q", got)
		}
	})
}

func TestFileURLBuilderKnowledgeFileURL(t *testing.T) {
	builder := NewFileURLBuilder("https://agenthub.example.com/")
	got := builder.KnowledgeFileURL("kb-1", "file-1")
	if got != "https://agenthub.example.com/api/knowledge-bases/kb-1/files/file-1/content" {
		t.Fatalf("KnowledgeFileURL() = %q", got)
	}
}

func TestSafeJoinUploadPath(t *testing.T) {
	dir := t.TempDir()
	got, err := SafeJoinUploadPath(dir, "uploads/knowledge/kb-1/file.pdf")
	if err != nil {
		t.Fatalf("SafeJoinUploadPath() failed: %v", err)
	}
	want := filepath.Join(dir, "knowledge", "kb-1", "file.pdf")
	if got != want {
		t.Fatalf("SafeJoinUploadPath() = %q, want %q", got, want)
	}

	got, err = SafeJoinUploadPath(dir, "knowledge/kb-1/file.pdf")
	if err != nil {
		t.Fatalf("SafeJoinUploadPath() without prefix failed: %v", err)
	}
	if got != want {
		t.Fatalf("SafeJoinUploadPath() without prefix = %q, want %q", got, want)
	}

	if _, err := SafeJoinUploadPath(dir, "knowledge/../secret.pdf"); !errors.Is(err, ErrInvalidStoragePath) {
		t.Fatalf("expected ErrInvalidStoragePath, got %v", err)
	}

	other := filepath.Dir(dir)
	if _, err := SafeJoinUploadPath(dir, filepath.Join(other, "secret.pdf")); !errors.Is(err, ErrInvalidStoragePath) {
		t.Fatalf("expected ErrInvalidStoragePath for absolute path, got %v", err)
	}

	if _, err := os.Stat(want); !os.IsNotExist(err) {
		t.Fatalf("SafeJoinUploadPath should not create files, stat err = %v", err)
	}
}
