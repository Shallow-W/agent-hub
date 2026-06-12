package service

import (
	"context"
	"errors"
	"testing"

	"github.com/agent-hub/backend/internal/model"
)

type fakeAgentPromptTemplateRepo struct {
	createdName string
	created     []string
	updateNil   bool
	deleted     bool
	duplicates  map[string]bool
}

func (r *fakeAgentPromptTemplateRepo) ListByUser(ctx context.Context, userID string) ([]model.AgentPromptTemplate, error) {
	return []model.AgentPromptTemplate{{ID: "tpl-1", UserID: userID, Name: "代码实现"}}, nil
}

func (r *fakeAgentPromptTemplateRepo) Create(ctx context.Context, userID, name, category, description, systemPrompt string) (*model.AgentPromptTemplate, error) {
	r.createdName = name
	r.created = append(r.created, name)
	if r.duplicates != nil && r.duplicates[name] {
		return nil, ErrAgentPromptTemplateDuplicate
	}
	return &model.AgentPromptTemplate{
		ID:           "tpl-1",
		UserID:       userID,
		Name:         name,
		Category:     category,
		Description:  description,
		SystemPrompt: systemPrompt,
	}, nil
}

func (r *fakeAgentPromptTemplateRepo) Update(ctx context.Context, id, userID, name, category, description, systemPrompt string) (*model.AgentPromptTemplate, error) {
	if r.updateNil {
		return nil, nil
	}
	return &model.AgentPromptTemplate{
		ID:           id,
		UserID:       userID,
		Name:         name,
		Category:     category,
		Description:  description,
		SystemPrompt: systemPrompt,
	}, nil
}

func (r *fakeAgentPromptTemplateRepo) Delete(ctx context.Context, id, userID string) (bool, error) {
	return r.deleted, nil
}

func TestAgentPromptTemplateCreateNormalizesFields(t *testing.T) {
	cat := &fakeAgentPromptCatalogStore{}
	svc := NewAgentPromptTemplateService(nil)
	svc.SetCatalogStore(cat)
	tpl, err := svc.Create(context.Background(), "user-1", "  代码实现  ", "", " desc ", " prompt ")
	if err != nil {
		t.Fatalf("create agent prompt template failed: %v", err)
	}
	if cat.createCalls[0].name != "代码实现" || tpl.Category != "通用" || tpl.Description != "desc" || tpl.SystemPrompt != "prompt" {
		t.Fatalf("unexpected normalized template: %#v catName=%q", tpl, cat.createCalls[0].name)
	}
}

func TestAgentPromptTemplateCreateRejectsEmptyName(t *testing.T) {
	svc := NewAgentPromptTemplateService(nil)
	svc.SetCatalogStore(&fakeAgentPromptCatalogStore{})
	_, err := svc.Create(context.Background(), "user-1", " ", "", "", "")
	if !errors.Is(err, ErrAgentPromptTemplateInvalid) {
		t.Fatalf("expected ErrAgentPromptTemplateInvalid, got %v", err)
	}
}

func TestAgentPromptTemplateUpdateReturnsNotFound(t *testing.T) {
	svc := NewAgentPromptTemplateService(nil)
	svc.SetCatalogStore(&fakeAgentPromptCatalogStore{updateNil: true})
	_, err := svc.Update(context.Background(), "tpl-1", "user-1", "代码实现", "开发", "", "")
	if !errors.Is(err, ErrAgentPromptTemplateNotFound) {
		t.Fatalf("expected ErrAgentPromptTemplateNotFound, got %v", err)
	}
}

func TestAgentPromptTemplateDeleteReturnsNotFound(t *testing.T) {
	svc := NewAgentPromptTemplateService(nil)
	svc.SetCatalogStore(&fakeAgentPromptCatalogStore{deleteErr: ErrAgentPromptTemplateNotFound})
	err := svc.Delete(context.Background(), "tpl-1", "user-1")
	if !errors.Is(err, ErrAgentPromptTemplateNotFound) {
		t.Fatalf("expected ErrAgentPromptTemplateNotFound, got %v", err)
	}
}

func TestAgentPromptTemplateImportDefaultsCreatesTemplates(t *testing.T) {
	cat := &fakeAgentPromptCatalogStore{}
	svc := NewAgentPromptTemplateService(nil)
	svc.SetCatalogStore(cat)
	templates, err := svc.ImportDefaults(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("import defaults failed: %v", err)
	}
	if len(templates) != len(DefaultAgentPromptTemplates()) {
		t.Fatalf("imported %d templates, want %d", len(templates), len(DefaultAgentPromptTemplates()))
	}
	if cat.createCalls[0].name != "通用执行型 Agent" {
		t.Fatalf("first default template = %q", cat.createCalls[0].name)
	}
	if templates[0].Category != "通用" {
		t.Fatalf("first default category = %q", templates[0].Category)
	}
}

func TestAgentPromptTemplateImportDefaultsSkipsDuplicates(t *testing.T) {
	cat := &fakeAgentPromptCatalogStore{duplicates: map[string]bool{"通用执行型 Agent": true}}
	svc := NewAgentPromptTemplateService(nil)
	svc.SetCatalogStore(cat)
	templates, err := svc.ImportDefaults(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("import defaults failed: %v", err)
	}
	if len(templates) != len(DefaultAgentPromptTemplates())-1 {
		t.Fatalf("imported %d templates, want %d", len(templates), len(DefaultAgentPromptTemplates())-1)
	}
}
