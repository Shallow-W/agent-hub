package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPptPreview_PathTraversalForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewPptPreviewHandler(t.TempDir())
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "filepath", Value: "/../secret.pptx"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/ppt-preview/../secret.pptx", nil)

	h.Preview(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestPptPreview_RejectsNonPowerPointFile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "originals"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "originals", "note.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewPptPreviewHandler(dir)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "filepath", Value: "/originals/note.txt"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/ppt-preview/originals/note.txt", nil)

	h.Preview(c)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", w.Code)
	}
}
