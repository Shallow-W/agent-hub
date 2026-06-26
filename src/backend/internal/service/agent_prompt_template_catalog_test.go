package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agent-hub/backend/internal/model"
)

type fakeAgentPromptCatalogStore struct {
	listErr    error
	createErr  error
	updateErr  error
	deleteErr  error
	listItems  []AgentPromptTemplateCatalogItem
	updateItem *AgentPromptTemplateCatalogItem
	updateNil  bool
	duplicates map[string]bool

	createCalls []agentPromptCreateCallArgs
	updateCalls []agentPromptUpdateCallArgs
	deleteCalls []agentPromptDeleteCallArgs
}

type agentPromptCreateCallArgs struct {
	userID, name, category, description, systemPrompt string
}

type agentPromptUpdateCallArgs struct {
	id, userID, name, category, description, systemPrompt string
}

type agentPromptDeleteCallArgs struct {
	id, userID string
}

func (f *fakeAgentPromptCatalogStore) ListAgentPromptTemplates(_ context.Context, _ string) ([]AgentPromptTemplateCatalogItem, error) {
	return f.listItems, f.listErr
}

func (f *fakeAgentPromptCatalogStore) CreateAgentPromptTemplate(_ context.Context, userID, name, category, description, systemPrompt string) (*AgentPromptTemplateCatalogItem, error) {
	f.createCalls = append(f.createCalls, agentPromptCreateCallArgs{userID, name, category, description, systemPrompt})
	if f.duplicates != nil && f.duplicates[name] {
		return nil, ErrAgentPromptTemplateDuplicate
	}
	if f.createErr != nil {
		return nil, f.createErr
	}
	now := time.Now()
	return &AgentPromptTemplateCatalogItem{
		ID: "cat-" + name, UserID: userID, Name: name, Category: category,
		Description: description, SystemPrompt: systemPrompt,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (f *fakeAgentPromptCatalogStore) UpdateAgentPromptTemplate(_ context.Context, id, userID, name, category, description, systemPrompt string) (*AgentPromptTemplateCatalogItem, error) {
	f.updateCalls = append(f.updateCalls, agentPromptUpdateCallArgs{id, userID, name, category, description, systemPrompt})
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
	return &AgentPromptTemplateCatalogItem{
		ID: id, UserID: userID, Name: name, Category: category,
		Description: description, SystemPrompt: systemPrompt,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (f *fakeAgentPromptCatalogStore) DeleteAgentPromptTemplate(_ context.Context, id, userID string) error {
	f.deleteCalls = append(f.deleteCalls, agentPromptDeleteCallArgs{id, userID})
	return f.deleteErr
}

// ── Routing tests ────────────────────────────────────────────────────────────

func TestAgentPromptTemplateService_ListRoutesViaCatalog(t *testing.T) {
	created := time.Date(2025, 6, 11, 10, 0, 0, 0, time.UTC)
	updated := time.Date(2025, 6, 11, 11, 0, 0, 0, time.UTC)
	svc := NewAgentPromptTemplateService(&fakeAgentPromptCatalogRepo{})
	svc.SetCatalogStore(&fakeAgentPromptCatalogStore{listItems: []AgentPromptTemplateCatalogItem{
		{
			ID: "a1", UserID: "u1", Name: "助手", Category: "通用",
			Description: "desc", SystemPrompt: "SP",
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
	if m.ID != "a1" || m.UserID != "u1" || m.Name != "助手" {
		t.Errorf("ID/UserID/Name mismatch: %+v", m)
	}
	if m.Category != "通用" || m.Description != "desc" {
		t.Errorf("Category/Description mismatch: %+v", m)
	}
	if m.SystemPrompt != "SP" {
		t.Errorf("SystemPrompt round-trip failed: %q", m.SystemPrompt)
	}
	if !m.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt not preserved: got %v want %v", m.CreatedAt, created)
	}
	if !m.UpdatedAt.Equal(updated) {
		t.Errorf("UpdatedAt not preserved: got %v want %v", m.UpdatedAt, updated)
	}
}

func TestAgentPromptTemplateService_CreateRoutesViaCatalog(t *testing.T) {
	repo := &fakeAgentPromptCatalogRepo{}
	svc := NewAgentPromptTemplateService(repo)
	cat := &fakeAgentPromptCatalogStore{}
	svc.SetCatalogStore(cat)

	m, err := svc.Create(context.Background(), "u1", "新模板", "通用", "d", "SP")
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
	if c.systemPrompt != "SP" {
		t.Errorf("systemPrompt not passed through: %+v", c)
	}
	if m.SystemPrompt != "SP" {
		t.Errorf("SystemPrompt round-trip failed: %q", m.SystemPrompt)
	}
}

func TestAgentPromptTemplateService_UpdateRoutesViaCatalog(t *testing.T) {
	svc := NewAgentPromptTemplateService(&fakeAgentPromptCatalogRepo{})
	svc.SetCatalogStore(&fakeAgentPromptCatalogStore{updateErr: ErrAgentPromptTemplateNotFound})

	_, err := svc.Update(context.Background(), "missing", "u1", "n", "c", "d", "SP")
	if !errors.Is(err, ErrAgentPromptTemplateNotFound) {
		t.Fatalf("expected ErrAgentPromptTemplateNotFound, got %v", err)
	}
}

func TestAgentPromptTemplateService_DeleteRoutesViaCatalog(t *testing.T) {
	svc := NewAgentPromptTemplateService(&fakeAgentPromptCatalogRepo{})
	cat := &fakeAgentPromptCatalogStore{deleteErr: ErrAgentPromptTemplateNotFound}
	svc.SetCatalogStore(cat)

	err := svc.Delete(context.Background(), "a1", "u1")
	if !errors.Is(err, ErrAgentPromptTemplateNotFound) {
		t.Fatalf("expected ErrAgentPromptTemplateNotFound, got %v", err)
	}
	if len(cat.deleteCalls) != 1 {
		t.Fatalf("expected 1 delete call, got %d", len(cat.deleteCalls))
	}
}

func TestAgentPromptTemplateService_ImportDefaultsRoutesViaCatalog(t *testing.T) {
	repo := &fakeAgentPromptCatalogRepo{createdData: []model.AgentPromptTemplate{}}
	svc := NewAgentPromptTemplateService(repo)
	cat := &fakeAgentPromptCatalogStore{}
	svc.SetCatalogStore(cat)

	allDefaults := DefaultAgentPromptTemplates()
	cat.listItems = make([]AgentPromptTemplateCatalogItem, 0, len(allDefaults))
	for _, tpl := range allDefaults {
		cat.listItems = append(cat.listItems, AgentPromptTemplateCatalogItem{
			ID:       "cat-" + tpl.Name,
			UserID:   "u1",
			Name:     tpl.Name,
			Category: tpl.Category,
		})
	}

	imported, err := svc.ImportDefaults(context.Background(), "u1")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(imported) != len(allDefaults) {
		t.Fatalf("imported %d templates, want %d", len(imported), len(allDefaults))
	}
	if len(repo.created) != 0 {
		t.Errorf("repo.Create should NOT have been called, got %+v", repo.created)
	}
	if len(cat.createCalls) != len(allDefaults) {
		t.Errorf("catalog Create should have been called %d times, got %d",
			len(allDefaults), len(cat.createCalls))
	}
}

// ── Error propagation tests ──────────────────────────────────────────────────

func TestAgentPromptTemplateService_CatalogErrorPropagates(t *testing.T) {
	svc := NewAgentPromptTemplateService(&fakeAgentPromptCatalogRepo{})
	svc.SetCatalogStore(&fakeAgentPromptCatalogStore{listErr: errors.New("boom")})

	if _, err := svc.List(context.Background(), "u1"); err == nil {
		t.Fatal("expected error propagation")
	}
}

func TestAgentPromptTemplateService_CreateViaCatalogStillNormalizes(t *testing.T) {
	svc := NewAgentPromptTemplateService(&fakeAgentPromptCatalogRepo{})
	cat := &fakeAgentPromptCatalogStore{}
	svc.SetCatalogStore(cat)

	_, err := svc.Create(context.Background(), "u1", "  助手  ", "  通用  ", " desc ", " prompt ")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(cat.createCalls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(cat.createCalls))
	}
	c := cat.createCalls[0]
	if c.name != "助手" {
		t.Errorf("name not trimmed: %q", c.name)
	}
	if c.category != "通用" {
		t.Errorf("category not trimmed: %q", c.category)
	}
	if c.systemPrompt != "prompt" {
		t.Errorf("systemPrompt not trimmed: %q", c.systemPrompt)
	}
}

func TestAgentPromptTemplateService_CreateViaCatalogDefaultsCategory(t *testing.T) {
	svc := NewAgentPromptTemplateService(&fakeAgentPromptCatalogRepo{})
	cat := &fakeAgentPromptCatalogStore{}
	svc.SetCatalogStore(cat)

	m, err := svc.Create(context.Background(), "u1", "助手", " ", "", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if m.Category != "通用" {
		t.Errorf("expected default category 通用, got %q", m.Category)
	}
	if cat.createCalls[0].category != "通用" {
		t.Errorf("category forwarded to catalog not 通用: %q", cat.createCalls[0].category)
	}
}

type fakeAgentPromptCatalogRepo struct {
	createdData []model.AgentPromptTemplate
	created     []model.AgentPromptTemplate
}

func (f *fakeAgentPromptCatalogRepo) ListByUser(_ context.Context, _ string) ([]model.AgentPromptTemplate, error) {
	return f.createdData, nil
}

func (f *fakeAgentPromptCatalogRepo) Create(_ context.Context, userID, name, category, description, systemPrompt string) (*model.AgentPromptTemplate, error) {
	m := model.AgentPromptTemplate{
		ID: "r-" + name, UserID: userID, Name: name, Category: category,
		Description: description, SystemPrompt: systemPrompt,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	f.created = append(f.created, m)
	return &m, nil
}

func (f *fakeAgentPromptCatalogRepo) Update(_ context.Context, _, _, _, _, _, _ string) (*model.AgentPromptTemplate, error) {
	return nil, nil
}

func (f *fakeAgentPromptCatalogRepo) Delete(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
