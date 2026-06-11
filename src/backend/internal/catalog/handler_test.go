package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agent-hub/backend/internal/model"
	"github.com/gin-gonic/gin"
)

// init ensures gin is in test mode before any test runs.
func init() { gin.SetMode(gin.TestMode) }

// newTestRouter builds a gin engine with the catalog routes mounted at
// /api/catalog, plus an in-memory tool-definition repo for the read test.
func newTestRouter(t *testing.T, store Store, reg *Registry) *gin.Engine {
	t.Helper()
	r := gin.New()
	svc := NewService(store, reg)
	h := NewHandler(svc)
	g := r.Group("/api/catalog")
	g.GET("", h.DomainsHandler)
	h.Register(g)
	return r
}

// do issues a request against the test router and returns the response.
func do(t *testing.T, r *gin.Engine, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		req = httptest.NewRequest(method, path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// catalogEnvelope mirrors middleware.Response shape.
type catalogEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func decodeEnvelope(t *testing.T, w *httptest.ResponseRecorder) catalogEnvelope {
	t.Helper()
	var env catalogEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v, body: %s", err, w.Body.String())
	}
	return env
}

// ── Handler tests ────────────────────────────────────────────────────────────

func TestHandler_List_ToolDefinition_FromAdapter(t *testing.T) {
	reg := DefaultRegistry()
	store := NewAdapterStore(AdapterDeps{
		ToolDef: &fakeToolDefRepo{items: []model.ToolDefinition{
			{Name: "search_web", Label: "Search Web", Category: "MCP", Description: "Search the web", CreatedAt: time.Now()},
		}},
		Registry: reg,
	})
	r := newTestRouter(t, store, reg)

	w := do(t, r, http.MethodGet, "/api/catalog/tool_definition", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status: %d, body: %s", w.Code, w.Body.String())
	}
	env := decodeEnvelope(t, w)
	var items []Item
	if err := json.Unmarshal(env.Data, &items); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if len(items) != 1 || items[0].Key != "search_web" {
		t.Fatalf("unexpected items: %+v", items)
	}
	if items[0].Label != "Search Web" {
		t.Errorf("Label: %s", items[0].Label)
	}
	if items[0].Domain != DomainToolDefinition {
		t.Errorf("Domain: %s", items[0].Domain)
	}
}

func TestHandler_Get_ToolDefinition(t *testing.T) {
	reg := DefaultRegistry()
	store := NewAdapterStore(AdapterDeps{
		ToolDef: &fakeToolDefRepo{items: []model.ToolDefinition{
			{Name: "tool-x", Label: "Tool X", CreatedAt: time.Now()},
		}},
		Registry: reg,
	})
	r := newTestRouter(t, store, reg)

	w := do(t, r, http.MethodGet, "/api/catalog/tool_definition/tool-x", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status: %d, body: %s", w.Code, w.Body.String())
	}
	env := decodeEnvelope(t, w)
	var it Item
	if err := json.Unmarshal(env.Data, &it); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if it.Key != "tool-x" {
		t.Errorf("Key: %s", it.Key)
	}
}

func TestHandler_Get_NotFound(t *testing.T) {
	reg := DefaultRegistry()
	store := NewAdapterStore(AdapterDeps{
		ToolDef:  &fakeToolDefRepo{},
		Registry: reg,
	})
	r := newTestRouter(t, store, reg)

	w := do(t, r, http.MethodGet, "/api/catalog/tool_definition/nope", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status: %d, body: %s", w.Code, w.Body.String())
	}
}

func TestHandler_Create_ReadOnlyDomain_405(t *testing.T) {
	reg := DefaultRegistry()
	store := NewAdapterStore(AdapterDeps{Registry: reg})
	r := newTestRouter(t, store, reg)

	w := do(t, r, http.MethodPost, "/api/catalog/tool_definition", map[string]string{
		"key": "x",
	})
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status: %d, body: %s", w.Code, w.Body.String())
	}
}

func TestHandler_Create_UserScope(t *testing.T) {
	reg := DefaultRegistry()
	fake := newFakeStore()
	store := composingStore{fake: fake, adapter: storeOrNil()}

	// Replace the read-only AdapterStore with a hybrid: writes go to fake,
	// reads also go to fake. This keeps the test focused on the handler.
	_ = store
	svc := NewService(fake, reg)
	r := gin.New()
	h := NewHandler(svc)
	g := r.Group("/api/catalog")
	h.Register(g)

	w := do(t, r, http.MethodPost, "/api/catalog/user_template", map[string]string{
		"subtype": "tools",
		"key":     "my-tool",
		"label":   "My Tool",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("status: %d, body: %s", w.Code, w.Body.String())
	}
}

// composingStore is a placeholder for future compositions; currently unused
// (kept so the test compiles cleanly without dead-code warnings).
type composingStore struct {
	fake    *fakeStore
	adapter *AdapterStore
}

func (c composingStore) List(ctx context.Context, d Domain, q ListQuery) ([]Item, error) {
	return c.fake.List(ctx, d, q)
}
func (c composingStore) GetByID(ctx context.Context, id string) (*Item, error) {
	return c.fake.GetByID(ctx, id)
}
func (c composingStore) Create(ctx context.Context, in CreateInput) (*Item, error) {
	return c.fake.Create(ctx, in)
}
func (c composingStore) Update(ctx context.Context, id string, in UpdateInput) (*Item, error) {
	return c.fake.Update(ctx, id, in)
}
func (c composingStore) Delete(ctx context.Context, id string) error {
	return c.fake.Delete(ctx, id)
}

func storeOrNil() *AdapterStore { return nil }

func TestHandler_DomainsHandler_ListsAllRegistered(t *testing.T) {
	reg := DefaultRegistry()
	store := NewAdapterStore(AdapterDeps{Registry: reg})
	r := newTestRouter(t, store, reg)

	w := do(t, r, http.MethodGet, "/api/catalog", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status: %d, body: %s", w.Code, w.Body.String())
	}
	env := decodeEnvelope(t, w)
	var domains []string
	if err := json.Unmarshal(env.Data, &domains); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(domains) != 4 {
		t.Errorf("expected 4 domains, got %d: %v", len(domains), domains)
	}
}

func TestHandler_List_UnknownDomain_404(t *testing.T) {
	reg := DefaultRegistry()
	store := NewAdapterStore(AdapterDeps{Registry: reg})
	r := newTestRouter(t, store, reg)

	w := do(t, r, http.MethodGet, "/api/catalog/does_not_exist", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status: %d, body: %s", w.Code, w.Body.String())
	}
}
