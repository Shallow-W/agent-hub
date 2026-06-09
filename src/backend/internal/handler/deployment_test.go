package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/repository"
	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// --- fakes implementing the exported service interfaces ---

type fakeDeployRepo struct{ dep *model.Deployment }

func (f *fakeDeployRepo) Create(_ context.Context, d model.Deployment) (*model.Deployment, error) {
	cp := d
	return &cp, nil
}
func (f *fakeDeployRepo) UpdateStatus(_ context.Context, id, status, url, errMsg string) (*model.Deployment, error) {
	return &model.Deployment{ID: id, Status: status, URL: url, Error: errMsg}, nil
}
func (f *fakeDeployRepo) GetByID(_ context.Context, id string) (*model.Deployment, error) {
	if f.dep != nil && f.dep.ID == id {
		return f.dep, nil
	}
	return nil, repository.ErrDeploymentNotFound
}

type fakeDeployArtRepo struct{}

func (fakeDeployArtRepo) GetLatestByRoot(_ context.Context, _ string) (*model.Artifact, error) {
	return nil, repository.ErrArtifactRootNotFound
}
func (fakeDeployArtRepo) GetConversationIDByRoot(_ context.Context, _ string) (string, error) {
	return "", repository.ErrArtifactRootNotFound
}
func (fakeDeployArtRepo) GetLatestRootByConversation(_ context.Context, _ string) (string, error) {
	return "", repository.ErrArtifactRootNotFound
}

type fakeDeployConvRepo struct{}

func (fakeDeployConvRepo) GetByID(_ context.Context, _ string) (*model.Conversation, error) {
	return nil, nil
}
func (fakeDeployConvRepo) GetMember(_ context.Context, _, _ string) (*model.ConversationMember, error) {
	return nil, nil
}

// setup 构造一个 baseDir 已落盘好站点的部署 + gin 路由（与 main.go 注册方式一致）。
func setup(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	id := uuid.NewString()
	siteDir := filepath.Join(dir, id)
	if err := os.MkdirAll(siteDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(siteDir, "index.html"), []byte("<h1>deployed site</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(siteDir, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}

	dRepo := &fakeDeployRepo{dep: &model.Deployment{ID: id, Status: "success", URL: "/api/sites/" + id + "/index.html"}}
	svc := service.NewDeploymentService(dRepo, fakeDeployArtRepo{}, fakeDeployConvRepo{}, dir, "")
	h := NewDeploymentHandler(svc)

	r := gin.New()
	api := r.Group("/api")
	// 镜像 main.go：authed 状态查询 + 公开站点服务 + 公开下载，验证三条路由共存不冲突
	api.GET("/deployments/:id", middleware.ValidateUUIDParam("id"), h.Get)
	r.GET("/api/sites/:id/*filepath", middleware.ValidateUUIDParam("id"), h.ServeSite)
	r.GET("/api/deployments/:id/download", middleware.ValidateUUIDParam("id"), h.Download)

	return r, id
}

func TestServeSite_ServesIndex(t *testing.T) {
	r, id := setup(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+id+"/index.html", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("deployed site")) {
		t.Fatalf("body missing site content: %s", w.Body.String())
	}
}

func TestServeSite_MissingFile404(t *testing.T) {
	r, id := setup(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+id+"/nope.html", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", w.Code)
	}
}

func TestDownload_ReturnsZipWithFiles(t *testing.T) {
	r, id := setup(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/deployments/"+id+"/download", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("content-type = %q, want application/zip", ct)
	}

	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	names := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		names[f.Name] = string(b)
	}
	if _, ok := names["index.html"]; !ok {
		t.Fatalf("zip missing index.html, has: %v", names)
	}
	if names["main.go"] != "package main" {
		t.Fatalf("zip main.go = %q, want 'package main'", names["main.go"])
	}
}

func TestDeploymentGet_Status(t *testing.T) {
	r, id := setup(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/deployments/"+id, nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(id)) {
		t.Fatalf("status body missing id: %s", w.Body.String())
	}
}
