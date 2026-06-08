package service

import (
	"context"
	"errors"
	"testing"

	"github.com/agent-hub/backend/internal/model"
)

type fakeUserProfileRepo struct {
	user          *model.User
	updatedName   string
	updatedAvatar string
	avatarUpdated bool
	missingUser   bool
}

func (r *fakeUserProfileRepo) GetUserByID(_ context.Context, id string) (*model.User, error) {
	if r.missingUser {
		return nil, nil
	}
	return r.user, nil
}

func (r *fakeUserProfileRepo) UpdateUsername(_ context.Context, id, username string) (*model.User, error) {
	if r.missingUser {
		return nil, nil
	}
	r.updatedName = username
	r.user.Username = username
	u := *r.user
	return &u, nil
}

func (r *fakeUserProfileRepo) UpdateAvatar(_ context.Context, id, avatar string) (*model.User, error) {
	if r.missingUser {
		return nil, nil
	}
	r.updatedAvatar = avatar
	r.avatarUpdated = true
	r.user.Avatar = avatar
	u := *r.user
	return &u, nil
}

func ptr(s string) *string { return &s }

// 仅更新 avatar：username 为 nil 时不应触发用户名校验/更新。
func TestUpdateProfileAvatarOnly(t *testing.T) {
	repo := &fakeUserProfileRepo{user: &model.User{ID: "u1", Username: "alice"}}
	svc := NewUserService(repo)

	user, err := svc.UpdateProfile(context.Background(), "u1", nil, ptr("user5"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updatedName != "" {
		t.Fatalf("username should not be updated, got %q", repo.updatedName)
	}
	if !repo.avatarUpdated || repo.updatedAvatar != "user5" {
		t.Fatalf("avatar not updated, got %q (updated=%v)", repo.updatedAvatar, repo.avatarUpdated)
	}
	if user.Avatar != "user5" {
		t.Fatalf("returned avatar mismatch: %q", user.Avatar)
	}
}

// 同时更新 username + avatar。
func TestUpdateProfileBoth(t *testing.T) {
	repo := &fakeUserProfileRepo{user: &model.User{ID: "u1", Username: "alice"}}
	svc := NewUserService(repo)

	user, err := svc.UpdateProfile(context.Background(), "u1", ptr("bob"), ptr("user9"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updatedName != "bob" {
		t.Fatalf("username mismatch: %q", repo.updatedName)
	}
	if user.Username != "bob" || user.Avatar != "user9" {
		t.Fatalf("returned user mismatch: %+v", user)
	}
}

// 两字段都为 nil 时报参数错误。
func TestUpdateProfileNoFields(t *testing.T) {
	repo := &fakeUserProfileRepo{user: &model.User{ID: "u1"}}
	svc := NewUserService(repo)

	if _, err := svc.UpdateProfile(context.Background(), "u1", nil, nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

// 空用户名报错。
func TestUpdateProfileEmptyUsername(t *testing.T) {
	repo := &fakeUserProfileRepo{user: &model.User{ID: "u1"}}
	svc := NewUserService(repo)

	if _, err := svc.UpdateProfile(context.Background(), "u1", ptr(""), nil); !errors.Is(err, ErrUsernameEmpty) {
		t.Fatalf("expected ErrUsernameEmpty, got %v", err)
	}
}
