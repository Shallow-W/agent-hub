package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/repository"
)

// fakeDeployRepo 实现 DeployRepo，记录最后一次创建/更新的部署。
type fakeDeployRepo struct {
	created *model.Deployment
	updated *model.Deployment
}

func (f *fakeDeployRepo) Create(_ context.Context, d model.Deployment) (*model.Deployment, error) {
	cp := d
	f.created = &cp
	return &cp, nil
}

func (f *fakeDeployRepo) UpdateStatus(_ context.Context, id, status, url, errMsg string) (*model.Deployment, error) {
	d := model.Deployment{ID: id, Status: status, URL: url, Error: errMsg}
	f.updated = &d
	return &d, nil
}

func (f *fakeDeployRepo) GetByID(_ context.Context, id string) (*model.Deployment, error) {
	if f.created != nil && f.created.ID == id {
		return f.created, nil
	}
	return nil, repository.ErrDeploymentNotFound
}

func newDeploySvc(t *testing.T, art *model.Artifact, member *model.ConversationMember, ownerID string) (*DeploymentService, *fakeDeployRepo, string) {
	t.Helper()
	dir := t.TempDir()
	aRepo := &fakeArtifactRepo{convIDByRoot: map[string]string{"root-1": "conv-1"}, latest: art}
	cRepo := &fakeArtifactConvRepo{conv: &model.Conversation{UserID: ownerID}, member: member}
	dRepo := &fakeDeployRepo{}
	return NewDeploymentService(dRepo, aRepo, cRepo, dir, ""), dRepo, dir
}

func TestDeploy_WebpageSuccess(t *testing.T) {
	art := &model.Artifact{Type: "webpage", Content: "<h1>hi from artifact</h1>"}
	svc, _, dir := newDeploySvc(t, art, &model.ConversationMember{}, "owner")

	dep, err := svc.Deploy(context.Background(), "root-1", "member-x")
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if dep.Status != "success" {
		t.Fatalf("status = %q, want success", dep.Status)
	}
	wantURL := "/api/sites/" + dep.ID + "/index.html"
	if dep.URL != wantURL {
		t.Fatalf("url = %q, want %q", dep.URL, wantURL)
	}
	b, err := os.ReadFile(filepath.Join(dir, dep.ID, "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	if !strings.Contains(string(b), "<h1>hi from artifact</h1>") {
		t.Fatalf("index.html missing webpage content: %s", b)
	}
}

func TestDeploy_CodeWritesSourceAndEscapesHTML(t *testing.T) {
	art := &model.Artifact{Type: "code", Language: "go", Filename: "main.go", Content: "fmt.Println(\"<b>\")"}
	svc, _, dir := newDeploySvc(t, art, &model.ConversationMember{}, "owner")

	dep, err := svc.Deploy(context.Background(), "root-1", "member-x")
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	// 原始源码文件保留，便于打包下载拿到真源码
	src, err := os.ReadFile(filepath.Join(dir, dep.ID, "main.go"))
	if err != nil {
		t.Fatalf("read source file: %v", err)
	}
	if string(src) != art.Content {
		t.Fatalf("source = %q, want %q", src, art.Content)
	}
	// index.html 对源码做 HTML 转义，避免被当作标签渲染
	idx, err := os.ReadFile(filepath.Join(dir, dep.ID, "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	if strings.Contains(string(idx), "<b>") {
		t.Fatalf("index.html should escape source HTML, got: %s", idx)
	}
	if !strings.Contains(string(idx), "&lt;b&gt;") {
		t.Fatalf("index.html missing escaped content: %s", idx)
	}
}

func TestDeploy_DocumentRendersMarkdownAndNamesSource(t *testing.T) {
	md := "# 今日待办\n\n- [ ] 修复 PPT\n- [x] 已完成项"
	art := &model.Artifact{Type: "document", Title: "今日待办清单", Content: md}
	svc, _, dir := newDeploySvc(t, art, &model.ConversationMember{}, "owner")

	dep, err := svc.Deploy(context.Background(), "root-1", "member-x")
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}

	idx, err := os.ReadFile(filepath.Join(dir, dep.ID, "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	// markdown 被服务端渲染：标题成 <h1>，任务列表成 checkbox，而不是裸 # 文本
	if !strings.Contains(string(idx), "<h1") || !strings.Contains(string(idx), "今日待办") {
		t.Fatalf("markdown heading not rendered: %s", idx)
	}
	if !strings.Contains(string(idx), "type=\"checkbox\"") {
		t.Fatalf("GFM task list not rendered to checkbox: %s", idx)
	}
	if strings.Contains(string(idx), "# 今日待办") {
		t.Fatalf("raw markdown leaked into page (not rendered): %s", idx)
	}

	// 源码文件用带 .md 扩展名的真实文件名，而非通用 source.txt
	if _, err := os.Stat(filepath.Join(dir, dep.ID, "source.txt")); err == nil {
		t.Fatalf("should not fall back to generic source.txt")
	}
	matches, _ := filepath.Glob(filepath.Join(dir, dep.ID, "*.md"))
	if len(matches) != 1 {
		t.Fatalf("expected exactly one .md source file, got %v", matches)
	}
	srcBytes, _ := os.ReadFile(matches[0])
	if string(srcBytes) != md {
		t.Fatalf("source .md content mismatch")
	}
}

func TestDeploy_NonMemberDenied(t *testing.T) {
	art := &model.Artifact{Type: "webpage", Content: "<h1>x</h1>"}
	// member 为 nil 且 conv.UserID != 调用者 → 拒绝
	svc, dRepo, _ := newDeploySvc(t, art, nil, "owner")

	_, err := svc.Deploy(context.Background(), "root-1", "stranger")
	if !errors.Is(err, ErrDeployNoPerm) {
		t.Fatalf("err = %v, want ErrDeployNoPerm", err)
	}
	if dRepo.created != nil {
		t.Fatalf("must not create deployment when access denied")
	}
}

func TestDeploy_EmptyArtifact(t *testing.T) {
	art := &model.Artifact{Type: "webpage"} // 无 content 无 url
	svc, _, _ := newDeploySvc(t, art, &model.ConversationMember{}, "owner")

	_, err := svc.Deploy(context.Background(), "root-1", "member-x")
	if !errors.Is(err, ErrDeployEmpty) {
		t.Fatalf("err = %v, want ErrDeployEmpty", err)
	}
}

func TestDeploy_ArtifactRootNotFound(t *testing.T) {
	svc, _, _ := newDeploySvc(t, nil, &model.ConversationMember{}, "owner")
	_, err := svc.Deploy(context.Background(), "missing-root", "member-x")
	if !errors.Is(err, ErrDeployArtifactNotFound) {
		t.Fatalf("err = %v, want ErrDeployArtifactNotFound", err)
	}
}
