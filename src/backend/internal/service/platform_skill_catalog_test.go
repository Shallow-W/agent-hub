package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agent-hub/backend/internal/model"
)

// fakePlatformSkillCatalogStore is a stand-in for the catalog-backed
// platformSkillCatalogBridge that main.go wires into PlatformSkillService.
// Returns canned data + captures the args of every method so tests can
// assert routing + arg-passthrough.
type fakePlatformSkillCatalogStore struct {
	listErr    error
	createErr  error
	updateErr  error
	deleteErr  error
	listItems  []PlatformSkillCatalogItem
	updateItem *PlatformSkillCatalogItem
	updateNil  bool
	deleted    bool
	duplicates map[string]bool

	createdName string
	created     []string
	createdData []PlatformSkillCatalogItem

	createCalls []createCallArgs
	updateCalls []updateCallArgs
	deleteCalls []deleteCallArgs
}

type createCallArgs struct {
	userID, name, category, description, trigger, detail string
}

type updateCallArgs struct {
	id, userID, name, category, description, trigger, detail string
}

type deleteCallArgs struct {
	id, userID string
}

func (f *fakePlatformSkillCatalogStore) ListPlatformSkills(_ context.Context, userID string) ([]PlatformSkillCatalogItem, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if len(f.listItems) > 0 {
		return f.listItems, nil
	}
	if len(f.createdData) > 0 {
		return f.createdData, nil
	}
	return []PlatformSkillCatalogItem{
		{ID: "skill-1", UserID: userID, Name: "审查"},
	}, nil
}

func (f *fakePlatformSkillCatalogStore) CreatePlatformSkill(_ context.Context, userID, name, category, description, trigger, detail string) (*PlatformSkillCatalogItem, error) {
	f.createCalls = append(f.createCalls, createCallArgs{userID, name, category, description, trigger, detail})
	f.createdName = name
	f.created = append(f.created, name)
	if f.duplicates != nil && f.duplicates[name] {
		return nil, ErrPlatformSkillDuplicate
	}
	if f.createErr != nil {
		return nil, f.createErr
	}
	now := time.Now()
	it := PlatformSkillCatalogItem{
		ID: "skill-" + name, UserID: userID, Name: name, Category: category,
		Description: description, Trigger: trigger, Detail: detail,
		CreatedAt: now, UpdatedAt: now,
	}
	f.createdData = append(f.createdData, it)
	return &it, nil
}

func (f *fakePlatformSkillCatalogStore) UpdatePlatformSkill(_ context.Context, id, userID, name, category, description, trigger, detail string) (*PlatformSkillCatalogItem, error) {
	f.updateCalls = append(f.updateCalls, updateCallArgs{id, userID, name, category, description, trigger, detail})
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	if f.updateNil {
		return nil, nil
	}
	if f.updateItem != nil {
		return f.updateItem, nil
	}
	now := time.Now()
	return &PlatformSkillCatalogItem{
		ID: id, UserID: userID, Name: name, Category: category,
		Description: description, Trigger: trigger, Detail: detail,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (f *fakePlatformSkillCatalogStore) DeletePlatformSkill(_ context.Context, id, userID string) error {
	f.deleteCalls = append(f.deleteCalls, deleteCallArgs{id, userID})
	if f.deleteErr != nil {
		return f.deleteErr
	}
	if !f.deleted {
		return ErrPlatformSkillNotFound
	}
	return nil
}

// ── Routing tests ────────────────────────────────────────────────────────────

// TestPlatformSkillService_ListRoutesViaCatalog proves that with a catalog
// store wired, List reads from catalog (not repo). Every field including
// CreatedAt/UpdatedAt must round-trip — that was the regression trellis-check
// caught in B1's tool_definition pilot.
func TestPlatformSkillService_ListRoutesViaCatalog(t *testing.T) {
	created := time.Date(2025, 6, 11, 10, 0, 0, 0, time.UTC)
	updated := time.Date(2025, 6, 11, 11, 0, 0, 0, time.UTC)
	svc := NewPlatformSkillService(&fakePlatformSkillRepo{})
	svc.SetCatalogStore(&fakePlatformSkillCatalogStore{listItems: []PlatformSkillCatalogItem{
		{
			ID: "p1", UserID: "u1", Name: "审查", Category: "产品经理",
			Description: "desc", Trigger: "tri", Detail: "det",
			CreatedAt: created, UpdatedAt: updated,
		},
	}})

	out, err := svc.List(context.Background(), "u1")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 item, got %d", len(out))
	}
	m := out[0]
	if m.ID != "p1" || m.UserID != "u1" || m.Name != "审查" {
		t.Errorf("ID/UserID/Name mismatch: %+v", m)
	}
	if m.Category != "产品经理" || m.Description != "desc" {
		t.Errorf("Category/Description mismatch: %+v", m)
	}
	if m.Trigger != "tri" || m.Detail != "det" {
		t.Errorf("Trigger/Detail round-trip failed: %+v", m)
	}
	if !m.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt not preserved: got %v want %v", m.CreatedAt, created)
	}
	if !m.UpdatedAt.Equal(updated) {
		t.Errorf("UpdatedAt not preserved: got %v want %v", m.UpdatedAt, updated)
	}
}

// TestPlatformSkillService_CreateRoutesViaCatalog proves Create routes
// through catalog and that payload fields (trigger, detail) survive the
// round-trip via the catalog's JSON payload.
func TestPlatformSkillService_CreateRoutesViaCatalog(t *testing.T) {
	repo := &fakePlatformSkillRepo{} // repo should NOT be called
	svc := NewPlatformSkillService(repo)
	cat := &fakePlatformSkillCatalogStore{}
	svc.SetCatalogStore(cat)

	m, err := svc.Create(context.Background(), "u1", "新技能", "产品经理", "d", "t", "dt")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(repo.created) != 0 {
		t.Errorf("repo.Create was called: %+v", repo.created)
	}
	if len(cat.createCalls) != 1 {
		t.Fatalf("expected 1 catalog Create call, got %d", len(cat.createCalls))
	}
	c := cat.createCalls[0]
	if c.trigger != "t" || c.detail != "dt" {
		t.Errorf("trigger/detail not passed through: %+v", c)
	}
	if m.Trigger != "t" || m.Detail != "dt" {
		t.Errorf("Trigger/Detail round-trip failed: %+v", m)
	}
}

// TestPlatformSkillService_UpdateRoutesViaCatalog proves Update routes
// through catalog and the not-found path still surfaces correctly.
func TestPlatformSkillService_UpdateRoutesViaCatalog(t *testing.T) {
	svc := NewPlatformSkillService(&fakePlatformSkillRepo{})
	svc.SetCatalogStore(&fakePlatformSkillCatalogStore{updateErr: ErrPlatformSkillNotFound})

	_, err := svc.Update(context.Background(), "missing", "u1", "n", "c", "d", "t", "dt")
	if !errors.Is(err, ErrPlatformSkillNotFound) {
		t.Fatalf("expected ErrPlatformSkillNotFound, got %v", err)
	}
}

// TestPlatformSkillService_DeleteRoutesViaCatalog proves Delete routes
// through catalog and propagates errors.
func TestPlatformSkillService_DeleteRoutesViaCatalog(t *testing.T) {
	svc := NewPlatformSkillService(&fakePlatformSkillRepo{})
	cat := &fakePlatformSkillCatalogStore{deleteErr: ErrPlatformSkillNotFound}
	svc.SetCatalogStore(cat)

	err := svc.Delete(context.Background(), "p1", "u1")
	if !errors.Is(err, ErrPlatformSkillNotFound) {
		t.Fatalf("expected ErrPlatformSkillNotFound, got %v", err)
	}
	if len(cat.deleteCalls) != 1 {
		t.Fatalf("expected 1 delete call, got %d", len(cat.deleteCalls))
	}
}

// TestPlatformSkillService_ImportDefaultsRoutesViaCatalog is the critical
// test: ImportDefaults must invoke the catalog Create path (not the repo
// Create path), since it internally calls s.Create which auto-routes.
// We assert call routing (not full filter semantics, which require a
// stateful fake — the routing proof is what matters for B2).
func TestPlatformSkillService_ImportDefaultsRoutesViaCatalog(t *testing.T) {
	repo := &fakePlatformSkillRepo{createdData: []model.PlatformSkill{}} // repo should NOT be called
	svc := NewPlatformSkillService(repo)
	cat := &fakePlatformSkillCatalogStore{}
	svc.SetCatalogStore(cat)

	// Make catalog List return the templates so ImportDefaults' filter
	// step finds them and returns the full count.
	allDefaults := DefaultPlatformSkillTemplates()
	cat.listItems = make([]PlatformSkillCatalogItem, 0, len(allDefaults))
	for _, tpl := range allDefaults {
		cat.listItems = append(cat.listItems, PlatformSkillCatalogItem{
			ID:       "cat-" + tpl.Name,
			UserID:   "u1",
			Name:     tpl.Name,
			Category: tpl.Category,
		})
	}

	skills, err := svc.ImportDefaults(context.Background(), "u1")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(skills) != len(allDefaults) {
		t.Fatalf("imported %d skills, want %d", len(skills), len(allDefaults))
	}
	if len(repo.created) != 0 {
		t.Errorf("repo.Create should NOT have been called, got %+v", repo.created)
	}
	if len(cat.createCalls) != len(allDefaults) {
		t.Errorf("catalog Create should have been called %d times, got %d",
			len(allDefaults), len(cat.createCalls))
	}
	if cat.createCalls[0].name != "产品需求澄清" {
		t.Errorf("first imported template name mismatch: %q", cat.createCalls[0].name)
	}
}

// ── Error-propagation tests ───────────────────────────────────────────────

// TestPlatformSkillService_CatalogErrorPropagates verifies that arbitrary
// catalog errors are wrapped (not silently swallowed).
func TestPlatformSkillService_CatalogErrorPropagates(t *testing.T) {
	svc := NewPlatformSkillService(&fakePlatformSkillRepo{})
	svc.SetCatalogStore(&fakePlatformSkillCatalogStore{listErr: errors.New("boom")})

	if _, err := svc.List(context.Background(), "u1"); err == nil {
		t.Fatal("expected error propagation")
	}
}

// ── Normalize behavior in catalog path ───────────────────────────────────────

// TestPlatformSkillService_CreateViaCatalogStillNormalizes proves the
// catalog path runs normalizePlatformSkillFields before forwarding. This
// protects the byte-equivalence contract — the legacy path normalizes,
// so the catalog path must too.
func TestPlatformSkillService_CreateViaCatalogStillNormalizes(t *testing.T) {
	svc := NewPlatformSkillService(&fakePlatformSkillRepo{})
	cat := &fakePlatformSkillCatalogStore{}
	svc.SetCatalogStore(cat)

	_, err := svc.Create(context.Background(), "u1", "  审查  ", "  产品经理  ", " desc ", " trigger ", " detail ")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(cat.createCalls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(cat.createCalls))
	}
	c := cat.createCalls[0]
	if c.name != "审查" {
		t.Errorf("name not trimmed: %q", c.name)
	}
	if c.category != "产品经理" {
		t.Errorf("category not trimmed: %q", c.category)
	}
	if c.description != "desc" || c.trigger != "trigger" || c.detail != "detail" {
		t.Errorf("desc/trigger/detail not trimmed: %+v", c)
	}
}

// TestPlatformSkillService_CreateViaCatalogDefaultsCategory proves the
// catalog path fills DefaultCategory when category is blank — same as
// the legacy repo path.
func TestPlatformSkillService_CreateViaCatalogDefaultsCategory(t *testing.T) {
	svc := NewPlatformSkillService(&fakePlatformSkillRepo{})
	cat := &fakePlatformSkillCatalogStore{}
	svc.SetCatalogStore(cat)

	m, err := svc.Create(context.Background(), "u1", "审查", " ", "", "", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if m.Category != "未分类" {
		t.Errorf("expected default category 未分类, got %q", m.Category)
	}
	if cat.createCalls[0].category != "未分类" {
		t.Errorf("category forwarded to catalog not 未分类: %q", cat.createCalls[0].category)
	}
}
