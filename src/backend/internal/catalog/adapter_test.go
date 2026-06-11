package catalog

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agent-hub/backend/internal/model"
	"github.com/jackc/pgx/v5/pgconn"
)

// ── fake repos for AdapterStore.List paths ───────────────────────────────────

type fakeToolDefRepo struct {
	items []model.ToolDefinition
	err   error
}

func (f *fakeToolDefRepo) List(_ context.Context) ([]model.ToolDefinition, error) {
	return f.items, f.err
}

type fakePlatformSkillRepo struct {
	items        []model.PlatformSkill
	createErr    error
	createCalls  []createCall
	updateCalls  []updateCall
	deleteCalls  []deleteCall
	deleteResult bool
	deleteErr    error
}

type createCall struct {
	userID, name, category, description, trigger, detail string
}

type updateCall struct {
	id, userID, name, category, description, trigger, detail string
}

type deleteCall struct {
	id, userID string
}

func (f *fakePlatformSkillRepo) ListByUser(_ context.Context, userID string) ([]model.PlatformSkill, error) {
	out := make([]model.PlatformSkill, 0, len(f.items))
	for _, m := range f.items {
		if m.UserID == userID {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f *fakePlatformSkillRepo) Create(_ context.Context, userID, name, category, description, trigger, detail string) (*model.PlatformSkill, error) {
	f.createCalls = append(f.createCalls, createCall{userID, name, category, description, trigger, detail})
	if f.createErr != nil {
		return nil, f.createErr
	}
	m := model.PlatformSkill{
		ID:          "p-" + name,
		UserID:      userID,
		Name:        name,
		Category:    category,
		Description: description,
		Trigger:     trigger,
		Detail:      detail,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	f.items = append(f.items, m)
	return &m, nil
}

func (f *fakePlatformSkillRepo) Update(_ context.Context, id, userID, name, category, description, trigger, detail string) (*model.PlatformSkill, error) {
	f.updateCalls = append(f.updateCalls, updateCall{id, userID, name, category, description, trigger, detail})
	for i := range f.items {
		if f.items[i].ID == id && f.items[i].UserID == userID {
			f.items[i].Name = name
			f.items[i].Category = category
			f.items[i].Description = description
			f.items[i].Trigger = trigger
			f.items[i].Detail = detail
			f.items[i].UpdatedAt = time.Now()
			return &f.items[i], nil
		}
	}
	return nil, nil
}

func (f *fakePlatformSkillRepo) Delete(_ context.Context, id, userID string) (bool, error) {
	f.deleteCalls = append(f.deleteCalls, deleteCall{id, userID})
	if f.deleteErr != nil {
		return false, f.deleteErr
	}
	for i := range f.items {
		if f.items[i].ID == id && f.items[i].UserID == userID {
			f.items = append(f.items[:i], f.items[i+1:]...)
			return true, nil
		}
	}
	return f.deleteResult, nil
}

type fakeAgentPromptRepo struct {
	items []model.AgentPromptTemplate
}

func (f *fakeAgentPromptRepo) ListByUser(_ context.Context, _ string) ([]model.AgentPromptTemplate, error) {
	return f.items, nil
}

type fakeUserTemplateRepo struct {
	items []model.UserTemplate
}

func (f *fakeUserTemplateRepo) ListByUserAndType(_ context.Context, _, _ string) ([]model.UserTemplate, error) {
	return f.items, nil
}

// ── converter round-trip tests ───────────────────────────────────────────────

func TestPlatformSkillToItem_MapsFields(t *testing.T) {
	m := &model.PlatformSkill{
		ID: "p1", UserID: "u1", Name: "Skill A", Category: "Cat",
		Description: "desc", Trigger: "t", Detail: "d",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	it := platformSkillToItem(m)
	if it.ID != m.ID || it.Key != m.Name || it.Label != m.Name {
		t.Errorf("ID/Key/Label mismatch: %+v", it)
	}
	if it.Category != m.Category {
		t.Errorf("Category: %s vs %s", it.Category, m.Category)
	}
	if it.UserID == nil || *it.UserID != "u1" {
		t.Errorf("UserID not set correctly: %v", it.UserID)
	}
	if it.PayloadJSON == "" {
		t.Errorf("PayloadJSON empty")
	}
}

func TestToolDefinitionToItem_NameIsKey(t *testing.T) {
	m := &model.ToolDefinition{
		Name: "tool-x", Label: "Tool X", Category: "cat", Description: "d",
		CreatedAt: time.Now(),
	}
	it := toolDefinitionToItem(m)
	if it.ID != "tool-x" {
		t.Errorf("ID (natural key) should be Name: %s", it.ID)
	}
	if it.Key != "tool-x" {
		t.Errorf("Key should be Name: %s", it.Key)
	}
	if it.Label != "Tool X" {
		t.Errorf("Label should preserve model.Label: %s", it.Label)
	}
	if it.UserID != nil {
		t.Errorf("system scope: UserID must be nil, got %v", it.UserID)
	}
}

func TestToolDefinitionToItem_LabelFallsBackToName(t *testing.T) {
	m := &model.ToolDefinition{Name: "n", CreatedAt: time.Now()}
	it := toolDefinitionToItem(m)
	if it.Label != "n" {
		t.Errorf("Label should fall back to Name when empty: %q", it.Label)
	}
}

func TestAgentPromptTemplateToItem_PayloadCarriesSystemPrompt(t *testing.T) {
	m := &model.AgentPromptTemplate{
		ID: "a1", UserID: "u1", Name: "tpl", Category: "c",
		Description: "d", SystemPrompt: "SYS",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	it := agentPromptTemplateToItem(m)
	if it.Key != "tpl" {
		t.Errorf("Key: %s", it.Key)
	}
	if it.PayloadJSON == "" || !contains(it.PayloadJSON, "system_prompt") {
		t.Errorf("PayloadJSON missing system_prompt: %s", it.PayloadJSON)
	}
	if !contains(it.PayloadJSON, "SYS") {
		t.Errorf("PayloadJSON missing value SYS: %s", it.PayloadJSON)
	}
}

func TestUserTemplateToItem_ContentIsPayload(t *testing.T) {
	m := &model.UserTemplate{
		ID: "ut1", UserID: "u1", Type: "tools", Name: "n",
		Content: []byte(`{"foo":"bar"}`),
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	it := userTemplateToItem(m)
	if it.Subtype != "tools" {
		t.Errorf("Subtype: %s", it.Subtype)
	}
	if it.PayloadJSON != `{"foo":"bar"}` {
		t.Errorf("PayloadJSON mismatch: %q", it.PayloadJSON)
	}
}

// ── AdapterStore end-to-end (read paths) ─────────────────────────────────────

func TestAdapterStore_List_ToolDefinition(t *testing.T) {
	reg := NewRegistry(DomainSpec{Name: DomainToolDefinition, Scope: ScopeSystem})
	store := NewAdapterStore(AdapterDeps{
		ToolDef:  &fakeToolDefRepo{items: []model.ToolDefinition{
			{Name: "a", Label: "A", Category: "x", Description: "d1", CreatedAt: time.Now()},
			{Name: "b", Label: "B", Category: "y", Description: "d2", CreatedAt: time.Now()},
		}},
		Registry: reg,
	})
	items, err := store.List(context.Background(), DomainToolDefinition, ListQuery{})
	if err != nil {
		t.Fatalf("List err: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Key != "a" {
		t.Errorf("first item Key: %s", items[0].Key)
	}
}

func TestAdapterStore_GetByID_ToolDefinition(t *testing.T) {
	reg := NewRegistry(DomainSpec{Name: DomainToolDefinition, Scope: ScopeSystem})
	store := NewAdapterStore(AdapterDeps{
		ToolDef: &fakeToolDefRepo{items: []model.ToolDefinition{
			{Name: "found", Label: "L", CreatedAt: time.Now()},
		}},
		Registry: reg,
	})
	it, err := store.GetByID(context.Background(), "found")
	if err != nil {
		t.Fatalf("GetByID err: %v", err)
	}
	if it.Key != "found" {
		t.Errorf("Key: %s", it.Key)
	}

	if _, err := store.GetByID(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestAdapterStore_List_UnknownDomain(t *testing.T) {
	store := NewAdapterStore(AdapterDeps{Registry: NewRegistry()})
	if _, err := store.List(context.Background(), Domain("nope"), ListQuery{}); !errors.Is(err, ErrUnknownDomain) {
		t.Fatalf("expected ErrUnknownDomain, got %v", err)
	}
}

func TestAdapterStore_List_PlatformSkillByUser(t *testing.T) {
	reg := NewRegistry(DomainSpec{Name: DomainPlatformSkill, Scope: ScopeUser})
	store := NewAdapterStore(AdapterDeps{
		PlatformSkill: &fakePlatformSkillRepo{items: []model.PlatformSkill{
			{ID: "p1", UserID: "u1", Name: "n", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		}},
		Registry: reg,
	})
	items, err := store.List(context.Background(), DomainPlatformSkill, ListQuery{UserID: "u1"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
}

func TestAdapterStore_Create_ReturnsReadOnly(t *testing.T) {
	store := NewAdapterStore(AdapterDeps{Registry: NewRegistry(
		DomainSpec{Name: DomainToolDefinition, Scope: ScopeSystem},
	)})
	if _, err := store.Create(context.Background(), CreateInput{Domain: DomainToolDefinition}); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("expected ErrReadOnly, got %v", err)
	}
}

// contains is a tiny test helper to avoid importing strings in this file.
func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// ── B2: platform_skill write paths ──────────────────────────────────────────

func TestAdapterStore_Create_PlatformSkill(t *testing.T) {
	reg := NewRegistry(DomainSpec{Name: DomainPlatformSkill, Scope: ScopeUser})
	repo := &fakePlatformSkillRepo{}
	store := NewAdapterStore(AdapterDeps{PlatformSkill: repo, Registry: reg})
	payload := `{"trigger":"tri","detail":"det"}`
	item, err := store.Create(context.Background(), CreateInput{
		Domain:      DomainPlatformSkill,
		UserID:      "u1",
		Key:         "新技能",
		Category:    "产品经理",
		Description: "d",
		PayloadJSON: payload,
	})
	if err != nil {
		t.Fatalf("Create err: %v", err)
	}
	if item.Key != "新技能" || item.Category != "产品经理" {
		t.Errorf("item fields wrong: %+v", item)
	}
	if !contains(item.PayloadJSON, "tri") || !contains(item.PayloadJSON, "det") {
		t.Errorf("payload not preserved: %s", item.PayloadJSON)
	}
	if len(repo.createCalls) != 1 {
		t.Fatalf("expected 1 repo Create call, got %d", len(repo.createCalls))
	}
	c := repo.createCalls[0]
	if c.trigger != "tri" || c.detail != "det" {
		t.Errorf("trigger/detail not decoded into repo args: %+v", c)
	}
}

func TestAdapterStore_Create_PlatformSkill_EmptyPayload(t *testing.T) {
	reg := NewRegistry(DomainSpec{Name: DomainPlatformSkill, Scope: ScopeUser})
	repo := &fakePlatformSkillRepo{}
	store := NewAdapterStore(AdapterDeps{PlatformSkill: repo, Registry: reg})
	item, err := store.Create(context.Background(), CreateInput{
		Domain: DomainPlatformSkill, UserID: "u1", Key: "k",
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if item == nil {
		t.Fatal("nil item")
	}
	if len(repo.createCalls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(repo.createCalls))
	}
	if repo.createCalls[0].trigger != "" || repo.createCalls[0].detail != "" {
		t.Errorf("empty payload should map to empty trigger/detail: %+v", repo.createCalls[0])
	}
}

func TestAdapterStore_Create_PlatformSkill_BadPayload(t *testing.T) {
	reg := NewRegistry(DomainSpec{Name: DomainPlatformSkill, Scope: ScopeUser})
	store := NewAdapterStore(AdapterDeps{
		PlatformSkill: &fakePlatformSkillRepo{},
		Registry:      reg,
	})
	if _, err := store.Create(context.Background(), CreateInput{
		Domain:      DomainPlatformSkill,
		UserID:      "u1",
		Key:         "k",
		PayloadJSON: `{not json`,
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid, got %v", err)
	}
}

func TestAdapterStore_Create_PlatformSkill_DuplicateMapsToErrDuplicate(t *testing.T) {
	reg := NewRegistry(DomainSpec{Name: DomainPlatformSkill, Scope: ScopeUser})
	repo := &fakePlatformSkillRepo{
		createErr: dupErr(),
	}
	store := NewAdapterStore(AdapterDeps{PlatformSkill: repo, Registry: reg})
	_, err := store.Create(context.Background(), CreateInput{
		Domain: DomainPlatformSkill, UserID: "u1", Key: "dup",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(mapStoreErr(err), ErrDuplicate) {
		t.Fatalf("expected mapStoreErr(err) to be ErrDuplicate, got %v", mapStoreErr(err))
	}
}

func TestAdapterStore_Update_PlatformSkill_PreservesUnsetFields(t *testing.T) {
	reg := NewRegistry(DomainSpec{Name: DomainPlatformSkill, Scope: ScopeUser})
	now := time.Now()
	repo := &fakePlatformSkillRepo{items: []model.PlatformSkill{{
		ID: "p1", UserID: "u1", Name: "原", Category: "c", Description: "d",
		Trigger: "t", Detail: "dt", CreatedAt: now, UpdatedAt: now,
	}}}
	store := NewAdapterStore(AdapterDeps{PlatformSkill: repo, Registry: reg})

	newLabel := "改名后"
	item, err := store.Update(context.Background(), "p1", UpdateInput{
		Domain: DomainPlatformSkill,
		UserID: "u1",
		Key:    &newLabel,
	})
	if err != nil {
		t.Fatalf("Update err: %v", err)
	}
	if item.Key != "改名后" {
		t.Errorf("Key not updated: %s", item.Key)
	}
	if item.Category != "c" || item.Description != "d" {
		t.Errorf("nil-pointer fields should preserve current row: %+v", item)
	}
	if len(repo.updateCalls) != 1 {
		t.Fatalf("expected 1 update call, got %d", len(repo.updateCalls))
	}
	uc := repo.updateCalls[0]
	if uc.category != "c" || uc.trigger != "t" || uc.detail != "dt" {
		t.Errorf("repo got merged values, got %+v", uc)
	}
}

func TestAdapterStore_Update_PlatformSkill_NotFound(t *testing.T) {
	reg := NewRegistry(DomainSpec{Name: DomainPlatformSkill, Scope: ScopeUser})
	repo := &fakePlatformSkillRepo{}
	store := NewAdapterStore(AdapterDeps{PlatformSkill: repo, Registry: reg})
	newLabel := "x"
	_, err := store.Update(context.Background(), "missing", UpdateInput{
		Domain: DomainPlatformSkill, UserID: "u1", Key: &newLabel,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// TestAdapterStore_Update_PlatformSkill_RejectsCrossUser proves the
// partial-merge source lookup is userID-scoped: an attacker cannot read
// another user's row to fill nil-pointer fields (which would also let
// them effectively overwrite that row by feeding its current values
// into repo.Update with their own userID). The lookup must NOT match.
func TestAdapterStore_Update_PlatformSkill_RejectsCrossUser(t *testing.T) {
	reg := NewRegistry(DomainSpec{Name: DomainPlatformSkill, Scope: ScopeUser})
	repo := &fakePlatformSkillRepo{items: []model.PlatformSkill{{
		ID: "victim", UserID: "victim-uid", Name: "原", Category: "c",
	}}}
	store := NewAdapterStore(AdapterDeps{PlatformSkill: repo, Registry: reg})
	newLabel := "hijacked"
	_, err := store.Update(context.Background(), "victim", UpdateInput{
		Domain: DomainPlatformSkill, UserID: "attacker", Key: &newLabel,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user update must be blocked with ErrNotFound, got %v", err)
	}
	// Repo must not have been called with the attacker's userID either.
	if len(repo.updateCalls) != 0 {
		t.Errorf("repo.Update should not have been invoked, got %+v", repo.updateCalls)
	}
}

// TestPlatformSkillToItem_EmptyPayloadDecodes test the read-side decode
// of an Item whose source model had empty Trigger/Detail. The bridge's
// decodePlatformSkillPayload (main.go) and the adapter's
// decodePlatformSkillPayload must both yield ("", "") for an explicit
// {"trigger":"","detail":""} payload — not error, not garbage.
func TestPlatformSkillToItem_EmptyPayloadDecodes(t *testing.T) {
	m := &model.PlatformSkill{
		ID: "p1", UserID: "u1", Name: "n",
		Trigger: "", Detail: "",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	it := platformSkillToItem(m)
	trigger, detail, err := decodePlatformSkillPayload(it.PayloadJSON)
	if err != nil {
		t.Fatalf("decode err: %v", err)
	}
	if trigger != "" || detail != "" {
		t.Errorf("empty model fields should decode to empty strings, got trigger=%q detail=%q",
			trigger, detail)
	}
}

func TestAdapterStore_Delete_PlatformSkill(t *testing.T) {
	reg := NewRegistry(DomainSpec{Name: DomainPlatformSkill, Scope: ScopeUser})
	repo := &fakePlatformSkillRepo{items: []model.PlatformSkill{
		{ID: "p1", UserID: "u1", Name: "n", CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}}
	store := NewAdapterStore(AdapterDeps{PlatformSkill: repo, Registry: reg})
	if err := store.Delete(context.Background(), DomainPlatformSkill, "u1", "p1"); err != nil {
		t.Fatalf("Delete err: %v", err)
	}
	if len(repo.deleteCalls) != 1 {
		t.Fatalf("expected 1 delete call, got %d", len(repo.deleteCalls))
	}
	if repo.deleteCalls[0].userID != "u1" {
		t.Errorf("userID not threaded: %+v", repo.deleteCalls[0])
	}
	if len(repo.items) != 0 {
		t.Errorf("item should be removed, got %d", len(repo.items))
	}
}

func TestAdapterStore_Delete_PlatformSkill_NotFound(t *testing.T) {
	reg := NewRegistry(DomainSpec{Name: DomainPlatformSkill, Scope: ScopeUser})
	store := NewAdapterStore(AdapterDeps{
		PlatformSkill: &fakePlatformSkillRepo{},
		Registry:      reg,
	})
	if err := store.Delete(context.Background(), DomainPlatformSkill, "u1", "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// dupErr returns a synthetic pgconn.PgError with SQLSTATE 23505 so we can
// verify mapStoreErr's unique-violation branch without a real DB.
func dupErr() error {
	return &pgconn.PgError{Code: "23505", Message: "unique constraint"}
}
