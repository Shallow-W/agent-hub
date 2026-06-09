package service

import (
	"context"
	"errors"
	"testing"

	"github.com/agent-hub/backend/internal/model"
)

type fakePlatformSkillRepo struct {
	createdName string
	created     []string
	createdData []model.PlatformSkill
	updateNil   bool
	deleted     bool
	createErr   error
	duplicates  map[string]bool
}

func (r *fakePlatformSkillRepo) ListByUser(ctx context.Context, userID string) ([]model.PlatformSkill, error) {
	list := make([]model.PlatformSkill, 0, len(r.createdData)+len(r.duplicates)+1)
	for name := range r.duplicates {
		list = append(list, model.PlatformSkill{
			ID:       "duplicate-" + name,
			UserID:   userID,
			Name:     name,
			Category: "产品经理",
		})
	}
	list = append(list, r.createdData...)
	if len(list) == 0 {
		return []model.PlatformSkill{{ID: "skill-1", UserID: userID, Name: "审查"}}, nil
	}
	return list, nil
}

func (r *fakePlatformSkillRepo) Create(ctx context.Context, userID, name, category, description, trigger, detail string) (*model.PlatformSkill, error) {
	r.createdName = name
	r.created = append(r.created, name)
	if r.duplicates != nil && r.duplicates[name] {
		return nil, ErrPlatformSkillDuplicate
	}
	if r.createErr != nil {
		return nil, r.createErr
	}
	skill := model.PlatformSkill{ID: "skill-" + name, UserID: userID, Name: name, Category: category, Description: description, Trigger: trigger, Detail: detail}
	r.createdData = append(r.createdData, skill)
	return &skill, nil
}

func (r *fakePlatformSkillRepo) Update(ctx context.Context, id, userID, name, category, description, trigger, detail string) (*model.PlatformSkill, error) {
	if r.updateNil {
		return nil, nil
	}
	return &model.PlatformSkill{ID: id, UserID: userID, Name: name, Category: category, Description: description, Trigger: trigger, Detail: detail}, nil
}

func (r *fakePlatformSkillRepo) Delete(ctx context.Context, id, userID string) (bool, error) {
	return r.deleted, nil
}

func TestPlatformSkillCreateNormalizesFields(t *testing.T) {
	repo := &fakePlatformSkillRepo{}
	svc := NewPlatformSkillService(repo)
	skill, err := svc.Create(context.Background(), "user-1", "  审查  ", " 开发人员 ", " desc ", " trigger ", " detail ")
	if err != nil {
		t.Fatalf("create platform skill failed: %v", err)
	}
	if repo.createdName != "审查" || skill.Category != "开发人员" || skill.Description != "desc" || skill.Trigger != "trigger" || skill.Detail != "detail" {
		t.Fatalf("unexpected normalized skill: %#v repoName=%q", skill, repo.createdName)
	}
}

func TestPlatformSkillCreateRejectsEmptyName(t *testing.T) {
	svc := NewPlatformSkillService(&fakePlatformSkillRepo{})
	_, err := svc.Create(context.Background(), "user-1", " ", "", "", "", "")
	if !errors.Is(err, ErrPlatformSkillInvalid) {
		t.Fatalf("expected ErrPlatformSkillInvalid, got %v", err)
	}
}

func TestPlatformSkillUpdateReturnsNotFound(t *testing.T) {
	svc := NewPlatformSkillService(&fakePlatformSkillRepo{updateNil: true})
	_, err := svc.Update(context.Background(), "skill-1", "user-1", "审查", "开发人员", "", "", "")
	if !errors.Is(err, ErrPlatformSkillNotFound) {
		t.Fatalf("expected ErrPlatformSkillNotFound, got %v", err)
	}
}

func TestPlatformSkillDeleteReturnsNotFound(t *testing.T) {
	svc := NewPlatformSkillService(&fakePlatformSkillRepo{deleted: false})
	err := svc.Delete(context.Background(), "skill-1", "user-1")
	if !errors.Is(err, ErrPlatformSkillNotFound) {
		t.Fatalf("expected ErrPlatformSkillNotFound, got %v", err)
	}
}

func TestPlatformSkillImportDefaultsCreatesTemplates(t *testing.T) {
	repo := &fakePlatformSkillRepo{}
	svc := NewPlatformSkillService(repo)
	skills, err := svc.ImportDefaults(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("import defaults failed: %v", err)
	}
	if len(skills) != len(DefaultPlatformSkillTemplates()) {
		t.Fatalf("imported %d skills, want %d", len(skills), len(DefaultPlatformSkillTemplates()))
	}
	if repo.created[0] != "产品需求澄清" {
		t.Fatalf("first default skill = %q", repo.created[0])
	}
	if skills[0].Category != "产品经理" {
		t.Fatalf("first default category = %q", skills[0].Category)
	}
}

func TestPlatformSkillImportDefaultsSkipsDuplicates(t *testing.T) {
	repo := &fakePlatformSkillRepo{duplicates: map[string]bool{"产品需求澄清": true}}
	svc := NewPlatformSkillService(repo)
	skills, err := svc.ImportDefaults(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("import defaults failed: %v", err)
	}
	if len(skills) != len(DefaultPlatformSkillTemplates()) {
		t.Fatalf("imported %d skills, want %d", len(skills), len(DefaultPlatformSkillTemplates()))
	}
	if len(repo.createdData) != len(DefaultPlatformSkillTemplates())-1 {
		t.Fatalf("created %d skills, want %d", len(repo.createdData), len(DefaultPlatformSkillTemplates())-1)
	}
}

func TestPlatformSkillCreateDefaultsEmptyCategory(t *testing.T) {
	repo := &fakePlatformSkillRepo{}
	svc := NewPlatformSkillService(repo)
	skill, err := svc.Create(context.Background(), "user-1", "审查", " ", "", "", "")
	if err != nil {
		t.Fatalf("create platform skill failed: %v", err)
	}
	if skill.Category != "未分类" {
		t.Fatalf("category = %q, want 未分类", skill.Category)
	}
}
