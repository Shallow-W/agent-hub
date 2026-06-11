package catalog

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agent-hub/backend/internal/model"
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
	items []model.PlatformSkill
}

func (f *fakePlatformSkillRepo) ListByUser(_ context.Context, _ string) ([]model.PlatformSkill, error) {
	return f.items, nil
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
