package service

import (
	"context"
	"errors"
	"testing"

	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/repository"
)

// fakeArtifactRepo 实现 ArtifactRepoForSvc + OrchArtifactRepo，用于隔离 service 鉴权与版本逻辑。
type fakeArtifactRepo struct {
	convIDByRoot   map[string]string
	rootNotFound   bool
	createErr      error
	created        *model.Artifact
	createVersIn   model.Artifact
	createVersRoot string
	createCalls    int
	versions         []model.Artifact
	latest           *model.Artifact
	latestErr        error
	latestRootByConv string
}

func (f *fakeArtifactRepo) ListVersions(_ context.Context, rootID string) ([]model.Artifact, error) {
	return f.versions, nil
}

func (f *fakeArtifactRepo) CreateVersion(_ context.Context, rootID string, in model.Artifact) (*model.Artifact, error) {
	f.createCalls++
	f.createVersRoot = rootID
	f.createVersIn = in
	if f.createErr != nil {
		return nil, f.createErr
	}
	if f.created != nil {
		return f.created, nil
	}
	out := in
	out.RootID = rootID
	out.Version = 2
	return &out, nil
}

// GetLatestByRoot 实现 OrchArtifactRepo（AI 编辑取最新版本）。
func (f *fakeArtifactRepo) GetLatestByRoot(_ context.Context, _ string) (*model.Artifact, error) {
	return f.latest, f.latestErr
}

func (f *fakeArtifactRepo) GetConversationIDByRoot(_ context.Context, rootID string) (string, error) {
	if f.rootNotFound {
		return "", repository.ErrArtifactRootNotFound
	}
	id, ok := f.convIDByRoot[rootID]
	if !ok {
		return "", repository.ErrArtifactRootNotFound
	}
	return id, nil
}

// GetLatestRootByConversation 实现 DeployArtifactRepo（聊天部署取对话最新产物）。
func (f *fakeArtifactRepo) GetLatestRootByConversation(_ context.Context, convID string) (string, error) {
	if f.latestRootByConv != "" {
		return f.latestRootByConv, nil
	}
	return "", repository.ErrArtifactRootNotFound
}

// fakeArtifactConvRepo 实现 ArtifactConvRepo。
type fakeArtifactConvRepo struct {
	conv   *model.Conversation
	member *model.ConversationMember
}

func (f *fakeArtifactConvRepo) GetByID(_ context.Context, _ string) (*model.Conversation, error) {
	return f.conv, nil
}

func (f *fakeArtifactConvRepo) GetMember(_ context.Context, _, _ string) (*model.ConversationMember, error) {
	return f.member, nil
}

func TestArtifactService_ListVersions_MemberAllowed(t *testing.T) {
	repo := &fakeArtifactRepo{
		convIDByRoot: map[string]string{"root-1": "conv-1"},
		versions:     []model.Artifact{{ID: "a1", Version: 1}, {ID: "a2", Version: 2}},
	}
	conv := &fakeArtifactConvRepo{
		conv:   &model.Conversation{ID: "conv-1", UserID: "owner"},
		member: &model.ConversationMember{ConversationID: "conv-1", UserID: "member-x"},
	}
	svc := NewArtifactService(repo, conv)

	got, err := svc.ListVersions(context.Background(), "root-1", "member-x")
	if err != nil {
		t.Fatalf("expected member access allowed, got %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(got))
	}
}

func TestArtifactService_ListVersions_CreatorFallbackAllowed(t *testing.T) {
	repo := &fakeArtifactRepo{
		convIDByRoot: map[string]string{"root-1": "conv-1"},
		versions:     []model.Artifact{},
	}
	// member 为 nil（成员表尚未写入），靠创建者兜底放行。
	conv := &fakeArtifactConvRepo{
		conv:   &model.Conversation{ID: "conv-1", UserID: "owner"},
		member: nil,
	}
	svc := NewArtifactService(repo, conv)

	if _, err := svc.ListVersions(context.Background(), "root-1", "owner"); err != nil {
		t.Fatalf("expected creator fallback allowed, got %v", err)
	}
}

func TestArtifactService_ListVersions_NonMemberDenied(t *testing.T) {
	repo := &fakeArtifactRepo{convIDByRoot: map[string]string{"root-1": "conv-1"}}
	conv := &fakeArtifactConvRepo{
		conv:   &model.Conversation{ID: "conv-1", UserID: "owner"},
		member: nil,
	}
	svc := NewArtifactService(repo, conv)

	_, err := svc.ListVersions(context.Background(), "root-1", "stranger")
	if !errors.Is(err, ErrArtifactNoPerm) {
		t.Fatalf("expected ErrArtifactNoPerm, got %v", err)
	}
}

func TestArtifactService_RootNotFound(t *testing.T) {
	repo := &fakeArtifactRepo{rootNotFound: true}
	conv := &fakeArtifactConvRepo{conv: &model.Conversation{ID: "conv-1", UserID: "owner"}}
	svc := NewArtifactService(repo, conv)

	_, err := svc.ListVersions(context.Background(), "missing-root", "owner")
	if !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("expected ErrArtifactNotFound, got %v", err)
	}
}

func TestArtifactService_CreateVersion_AccessCheckedAndPassedThrough(t *testing.T) {
	repo := &fakeArtifactRepo{
		convIDByRoot: map[string]string{"root-1": "conv-1"},
		created:      &model.Artifact{ID: "v2", RootID: "root-1", Version: 2, Type: "code", Content: "x"},
	}
	conv := &fakeArtifactConvRepo{
		conv:   &model.Conversation{ID: "conv-1", UserID: "owner"},
		member: &model.ConversationMember{ConversationID: "conv-1", UserID: "owner"},
	}
	svc := NewArtifactService(repo, conv)

	in := model.Artifact{Type: "code", Content: "new code", Language: "go"}
	got, err := svc.CreateVersion(context.Background(), "root-1", "owner", in)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if got.Version != 2 {
		t.Fatalf("expected returned version 2, got %d", got.Version)
	}
	if repo.createVersRoot != "root-1" || repo.createVersIn.Content != "new code" {
		t.Fatalf("input not passed through to repo: root=%q in=%+v", repo.createVersRoot, repo.createVersIn)
	}
}

func TestArtifactService_CreateVersion_DeniedBeforeRepo(t *testing.T) {
	repo := &fakeArtifactRepo{convIDByRoot: map[string]string{"root-1": "conv-1"}}
	conv := &fakeArtifactConvRepo{
		conv:   &model.Conversation{ID: "conv-1", UserID: "owner"},
		member: nil,
	}
	svc := NewArtifactService(repo, conv)

	_, err := svc.CreateVersion(context.Background(), "root-1", "stranger", model.Artifact{Type: "code", Content: "x"})
	if !errors.Is(err, ErrArtifactNoPerm) {
		t.Fatalf("expected ErrArtifactNoPerm, got %v", err)
	}
	if repo.createVersRoot != "" {
		t.Fatalf("repo.CreateVersion must not be called when access denied")
	}
}

func TestArtifactService_CreateVersion_InvalidInput(t *testing.T) {
	repo := &fakeArtifactRepo{convIDByRoot: map[string]string{"root-1": "conv-1"}}
	conv := &fakeArtifactConvRepo{conv: &model.Conversation{ID: "conv-1", UserID: "owner"}}
	svc := NewArtifactService(repo, conv)

	// 空 content 且空 url -> 不合法
	if _, err := svc.CreateVersion(context.Background(), "root-1", "owner", model.Artifact{Type: "code"}); !errors.Is(err, ErrArtifactInvalid) {
		t.Fatalf("expected ErrArtifactInvalid for empty content/url, got %v", err)
	}
	// 非法 type -> 不合法
	if _, err := svc.CreateVersion(context.Background(), "root-1", "owner", model.Artifact{Type: "bogus", Content: "x"}); !errors.Is(err, ErrArtifactInvalid) {
		t.Fatalf("expected ErrArtifactInvalid for bad type, got %v", err)
	}
	// 不合法输入不应触达 repo
	if repo.createVersRoot != "" {
		t.Fatalf("repo.CreateVersion must not be called on invalid input")
	}
}
