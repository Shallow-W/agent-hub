package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agent-hub/backend/internal/model"
)

// fakeCatalogLister is a stand-in for the catalog-backed bridge that main.go
// wires into ToolDefinitionService. It returns whatever items we inject, so
// we can prove ListDefinitions routes through catalog when one is set.
type fakeCatalogLister struct {
	items []ToolDefinitionCatalogItem
	err   error
}

func (f *fakeCatalogLister) ListToolDefinitions(_ context.Context) ([]ToolDefinitionCatalogItem, error) {
	return f.items, f.err
}

// fakeToolDefRepo satisfies ToolDefinitionRepo for the fallback path.
type fakeToolDefRepo struct {
	items []model.ToolDefinition
	err   error
}

func (r *fakeToolDefRepo) List(_ context.Context) ([]model.ToolDefinition, error) {
	return r.items, r.err
}

func (r *fakeToolDefRepo) ListBuiltinTemplates(_ context.Context) ([]model.BuiltinToolsetTemplate, error) {
	return nil, nil
}

func (r *fakeToolDefRepo) ListBuiltinSkillTemplates(_ context.Context) ([]model.BuiltinSkillTemplate, error) {
	return nil, nil
}

// TestToolDefinitionService_RoutesViaCatalog verifies the B1 pilot
// migration: when SetCatalogLister has been called, ListDefinitions returns
// data sourced from the catalog lister (not the repo). The full field set
// — including CreatedAt — must be preserved so the /api/tools/definitions
// response stays byte-equivalent to the legacy direct-repo path.
func TestToolDefinitionService_RoutesViaCatalog(t *testing.T) {
	repo := &fakeToolDefRepo{items: []model.ToolDefinition{
		{Name: "from-repo"},
	}}
	ts := time.Date(2025, 6, 11, 10, 0, 0, 0, time.UTC)
	svc := NewToolDefinitionService(repo)
	svc.SetCatalogLister(&fakeCatalogLister{items: []ToolDefinitionCatalogItem{
		{Name: "from-catalog", Label: "Catalog", Category: "cat", Description: "desc", CreatedAt: ts},
	}})

	out, err := svc.ListDefinitions(context.Background())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(out) != 1 || out[0].Name != "from-catalog" {
		t.Fatalf("expected catalog-sourced item, got %+v", out)
	}
	if out[0].Label != "Catalog" {
		t.Errorf("Label not preserved: %q", out[0].Label)
	}
	if !out[0].CreatedAt.Equal(ts) {
		t.Errorf("CreatedAt not preserved: got %v, want %v", out[0].CreatedAt, ts)
	}
	if out[0].Category != "cat" || out[0].Description != "desc" {
		t.Errorf("Category/Description not preserved: %+v", out[0])
	}
}

// TestToolDefinitionService_CatalogErrorPropagates verifies that catalog
// errors are wrapped (not silently swallowed) so handler-level error
// mapping still works.
func TestToolDefinitionService_CatalogErrorPropagates(t *testing.T) {
	repo := &fakeToolDefRepo{}
	svc := NewToolDefinitionService(repo)
	svc.SetCatalogLister(&fakeCatalogLister{err: errors.New("boom")})

	if _, err := svc.ListDefinitions(context.Background()); err == nil {
		t.Fatalf("expected error propagation")
	}
}
