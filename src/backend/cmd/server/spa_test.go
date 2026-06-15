package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterSPARoutesServesAssetsAndHistoryFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	distDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(distDir, "index.html"), []byte("<!doctype html><div id=\"root\"></div>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "app.js"), []byte("console.log('agenthub')"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	router := gin.New()
	registerSPARoutes(router, distDir)

	asset := httptest.NewRecorder()
	router.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "/app.js", nil))
	if asset.Code != http.StatusOK {
		t.Fatalf("asset status = %d, want %d", asset.Code, http.StatusOK)
	}
	if asset.Body.String() != "console.log('agenthub')" {
		t.Fatalf("asset body = %q", asset.Body.String())
	}

	page := httptest.NewRecorder()
	router.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/settings", nil))
	if page.Code != http.StatusOK {
		t.Fatalf("page status = %d, want %d", page.Code, http.StatusOK)
	}
	if page.Body.String() != "<!doctype html><div id=\"root\"></div>" {
		t.Fatalf("page body = %q", page.Body.String())
	}
}

func TestSPAFallbackHandlerServesBrowserHistoryRoute(t *testing.T) {
	distDir := t.TempDir()
	indexPath := filepath.Join(distDir, "index.html")
	if err := os.WriteFile(indexPath, []byte("<!doctype html><div id=\"root\"></div>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/settings", nil)

	spaFallbackHandler(distDir, indexPath)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), `id="root"`) {
		t.Fatalf("body missing root: %q", w.Body.String())
	}
}

func TestSPAFallbackHandlerNormalizesURLPathsWithinDist(t *testing.T) {
	distDir := t.TempDir()
	indexPath := filepath.Join(distDir, "index.html")
	assetsDir := filepath.Join(distDir, "assets")
	if err := os.Mkdir(assetsDir, 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	if err := os.WriteFile(indexPath, []byte("index"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "app.js"), []byte("app"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/assets/%2e%2e/index.html", nil)

	spaFallbackHandler(distDir, indexPath)(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if w.Body.String() == "app" {
		t.Fatal("traversal path should not serve an asset from dist")
	}
}

func TestLoadConfigUsesEnvironmentPath(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "desktop.yaml")
	content := []byte(`server:
  port: 18080
database:
  host: localhost
  port: 5432
  user: agenthub
  password: agenthub
  dbname: agenthub
  sslmode: disable
jwt:
  secret: test-secret
  expiry_hours: 48
`)
	if err := os.WriteFile(configPath, content, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("AGENTHUB_CONFIG", configPath)
	cfg, err := loadConfig("missing.yaml")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Server.Port != 18080 {
		t.Fatalf("port = %d, want 18080", cfg.Server.Port)
	}
}

func TestRegisterSPARoutesDoesNotHandleAPINotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	distDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(distDir, "index.html"), []byte("index"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	router := gin.New()
	registerSPARoutes(router, distDir)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/missing", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("api status = %d, want %d", w.Code, http.StatusNotFound)
	}
	if w.Body.String() == "index" {
		t.Fatal("api route should not be served by SPA fallback")
	}
}

func TestCleanUploadRoutePathRejectsWindowsRootedPaths(t *testing.T) {
	for _, input := range []string{
		"/C:/Windows/win.ini",
		"C:/Windows/win.ini",
		`C:\Windows\win.ini`,
		"//server/share/file.txt",
		`\\server\share\file.txt`,
		"uploads/C:/Windows/win.ini",
	} {
		if got := cleanUploadRoutePath(input); got != "" {
			t.Fatalf("cleanUploadRoutePath(%q) = %q, want empty", input, got)
		}
	}
}

func TestFrontendDistDirUsesEnvironmentPath(t *testing.T) {
	distDir := t.TempDir()
	t.Setenv("AGENTHUB_FRONTEND_DIST", distDir)

	if got := frontendDistDir(); got != distDir {
		t.Fatalf("frontendDistDir() = %q, want %q", got, distDir)
	}
}

func TestFrontendDistCandidatesCoverBuildScriptWorkingDirectories(t *testing.T) {
	candidates := frontendDistCandidates()
	want := map[string]bool{
		filepath.Clean("src/frontend/dist"):   true,
		filepath.Clean("../../frontend/dist"): true,
	}

	for _, candidate := range candidates {
		delete(want, filepath.Clean(candidate))
	}
	if len(want) > 0 {
		t.Fatalf("missing frontend dist candidates: %v", want)
	}
}
