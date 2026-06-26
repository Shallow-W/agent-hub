package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agent-hub/backend/internal/model"
)

type fakeUserTemplateCatalogStore struct {
	listErr   error
	createErr error
	updateErr error
	deleteErr error
	listItems []UserTemplateCatalogItem
	updateNil bool

	createCalls []utCatalogCreateCall
	updateCalls []utCatalogUpdateCall
	deleteCalls []utCatalogDeleteCall
}

type utCatalogCreateCall struct {
	userID, tplType, name, content string
}

type utCatalogUpdateCall struct {
	id, userID, name, content string
}

type utCatalogDeleteCall struct {
	id, userID string
}

func (f *fakeUserTemplateCatalogStore) ListUserTemplates(_ context.Context, _, _ string) ([]UserTemplateCatalogItem, error) {
	return f.listItems, f.listErr
}

func (f *fakeUserTemplateCatalogStore) CreateUserTemplate(_ context.Context, userID, tplType, name, content string) (*UserTemplateCatalogItem, error) {
	f.createCalls = append(f.createCalls, utCatalogCreateCall{userID, tplType, name, content})
	if f.createErr != nil {
		return nil, f.createErr
	}
	now := time.Now()
	return &UserTemplateCatalogItem{
		ID: "cat-" + name, UserID: userID, Type: tplType, Name: name,
		Content: content, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (f *fakeUserTemplateCatalogStore) UpdateUserTemplate(_ context.Context, id, userID, name, content string) (*UserTemplateCatalogItem, error) {
	f.updateCalls = append(f.updateCalls, utCatalogUpdateCall{id, userID, name, content})
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	if f.updateNil {
		return nil, nil
	}
	now := time.Now()
	return &UserTemplateCatalogItem{
		ID: id, UserID: userID, Name: name, Content: content,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (f *fakeUserTemplateCatalogStore) DeleteUserTemplate(_ context.Context, id, userID string) error {
	f.deleteCalls = append(f.deleteCalls, utCatalogDeleteCall{id, userID})
	return f.deleteErr
}

// ── Routing tests ────────────────────────────────────────────────────────────

func TestUserTemplateService_ListRoutesViaCatalog(t *testing.T) {
	created := time.Date(2025, 6, 11, 10, 0, 0, 0, time.UTC)
	updated := time.Date(2025, 6, 11, 11, 0, 0, 0, time.UTC)
	svc := NewUserTemplateService(&fakeUserTemplateCatalogRepo{})
	svc.SetCatalogStore(&fakeUserTemplateCatalogStore{listItems: []UserTemplateCatalogItem{
		{
			ID: "ut1", UserID: "u1", Type: "tools", Name: "工具集1",
			Content: `{"files":["a","b"]}`, CreatedAt: created, UpdatedAt: updated,
		},
	}})

	out, err := svc.List(context.Background(), "u1", "tools")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 item, got %d", len(out))
	}
	m := out[0]
	if m.ID != "ut1" || m.UserID != "u1" || m.Name != "工具集1" {
		t.Errorf("ID/UserID/Name mismatch: %+v", m)
	}
	if m.Type != "tools" {
		t.Errorf("Type not preserved: %q", m.Type)
	}
	if string(m.Content) != `{"files":["a","b"]}` {
		t.Errorf("Content round-trip failed: %q", string(m.Content))
	}
	if !m.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt not preserved: got %v want %v", m.CreatedAt, created)
	}
	if !m.UpdatedAt.Equal(updated) {
		t.Errorf("UpdatedAt not preserved: got %v want %v", m.UpdatedAt, updated)
	}
}

func TestUserTemplateService_CreateRoutesViaCatalog(t *testing.T) {
	repo := &fakeUserTemplateCatalogRepo{}
	svc := NewUserTemplateService(repo)
	cat := &fakeUserTemplateCatalogStore{}
	svc.SetCatalogStore(cat)

	m, err := svc.Create(context.Background(), "u1", "tools", "新模板", map[string]interface{}{"files": []string{"a"}})
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
	if c.tplType != "tools" || c.name != "新模板" {
		t.Errorf("args not passed through: %+v", c)
	}
	if c.content != `{"files":["a"]}` {
		t.Errorf("content not JSON-marshaled: %q", c.content)
	}
	if m.Type != "tools" {
		t.Errorf("Type round-trip failed: %q", m.Type)
	}
}

func TestUserTemplateService_UpdateRoutesViaCatalog(t *testing.T) {
	svc := NewUserTemplateService(&fakeUserTemplateCatalogRepo{})
	svc.SetCatalogStore(&fakeUserTemplateCatalogStore{updateErr: ErrUserTemplateNotFound})

	_, err := svc.Update(context.Background(), "missing", "u1", "n", map[string]string{"a": "b"})
	if !errors.Is(err, ErrUserTemplateNotFound) {
		t.Fatalf("expected ErrUserTemplateNotFound, got %v", err)
	}
}

func TestUserTemplateService_DeleteRoutesViaCatalog(t *testing.T) {
	svc := NewUserTemplateService(&fakeUserTemplateCatalogRepo{})
	cat := &fakeUserTemplateCatalogStore{deleteErr: ErrUserTemplateNotFound}
	svc.SetCatalogStore(cat)

	err := svc.Delete(context.Background(), "ut1", "u1")
	if !errors.Is(err, ErrUserTemplateNotFound) {
		t.Fatalf("expected ErrUserTemplateNotFound, got %v", err)
	}
	if len(cat.deleteCalls) != 1 {
		t.Fatalf("expected 1 delete call, got %d", len(cat.deleteCalls))
	}
}

// ── Error propagation tests ──────────────────────────────────────────────────

func TestUserTemplateService_CatalogErrorPropagates(t *testing.T) {
	svc := NewUserTemplateService(&fakeUserTemplateCatalogRepo{})
	svc.SetCatalogStore(&fakeUserTemplateCatalogStore{listErr: errors.New("boom")})

	if _, err := svc.List(context.Background(), "u1", "tools"); err == nil {
		t.Fatal("expected error propagation")
	}
}

// ── fake repo (legacy path) ─────────────────────────────────────────────────

type fakeUserTemplateCatalogRepo struct {
	createdData []model.UserTemplate
	created     []model.UserTemplate
}

func (f *fakeUserTemplateCatalogRepo) ListByUserAndType(_ context.Context, _, _ string) ([]model.UserTemplate, error) {
	return f.createdData, nil
}

func (f *fakeUserTemplateCatalogRepo) Create(_ context.Context, userID, tplType, name, content string) (*model.UserTemplate, error) {
	m := model.UserTemplate{
		ID: "r-" + name, UserID: userID, Type: tplType, Name: name,
		Content: []byte(content),
	}
	f.created = append(f.created, m)
	return &m, nil
}

func (f *fakeUserTemplateCatalogRepo) Update(_ context.Context, _, _, _, _ string) (*model.UserTemplate, error) {
	return nil, nil
}

func (f *fakeUserTemplateCatalogRepo) Delete(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
