package service

import (
	"context"
	"errors"
	"testing"

	"github.com/agent-hub/backend/internal/model"
)

type fakePlatformSkillRepo struct {
	createdName string
	updateNil   bool
	deleted     bool
	createErr   error
}

func (r *fakePlatformSkillRepo) ListByUser(ctx context.Context, userID string) ([]model.PlatformSkill, error) {
	return []model.PlatformSkill{{ID: "skill-1", UserID: userID, Name: "审查"}}, nil
}

func (r *fakePlatformSkillRepo) Create(ctx context.Context, userID, name, description, trigger, detail string) (*model.PlatformSkill, error) {
	r.createdName = name
	if r.createErr != nil {
		return nil, r.createErr
	}
	return &model.PlatformSkill{ID: "skill-1", UserID: userID, Name: name, Description: description, Trigger: trigger, Detail: detail}, nil
}

func (r *fakePlatformSkillRepo) Update(ctx context.Context, id, userID, name, description, trigger, detail string) (*model.PlatformSkill, error) {
	if r.updateNil {
		return nil, nil
	}
	return &model.PlatformSkill{ID: id, UserID: userID, Name: name, Description: description, Trigger: trigger, Detail: detail}, nil
}

func (r *fakePlatformSkillRepo) Delete(ctx context.Context, id, userID string) (bool, error) {
	return r.deleted, nil
}

func TestPlatformSkillCreateNormalizesFields(t *testing.T) {
	repo := &fakePlatformSkillRepo{}
	svc := NewPlatformSkillService(repo)
	skill, err := svc.Create(context.Background(), "user-1", "  审查  ", " desc ", " trigger ", " detail ")
	if err != nil {
		t.Fatalf("create platform skill failed: %v", err)
	}
	if repo.createdName != "审查" || skill.Description != "desc" || skill.Trigger != "trigger" || skill.Detail != "detail" {
		t.Fatalf("unexpected normalized skill: %#v repoName=%q", skill, repo.createdName)
	}
}

func TestPlatformSkillCreateRejectsEmptyName(t *testing.T) {
	svc := NewPlatformSkillService(&fakePlatformSkillRepo{})
	_, err := svc.Create(context.Background(), "user-1", " ", "", "", "")
	if !errors.Is(err, ErrPlatformSkillInvalid) {
		t.Fatalf("expected ErrPlatformSkillInvalid, got %v", err)
	}
}

func TestPlatformSkillUpdateReturnsNotFound(t *testing.T) {
	svc := NewPlatformSkillService(&fakePlatformSkillRepo{updateNil: true})
	_, err := svc.Update(context.Background(), "skill-1", "user-1", "审查", "", "", "")
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
