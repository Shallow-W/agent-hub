package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/repository"
	"github.com/google/uuid"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// markdownRenderer 复用一个开启 GFM（表格/任务列表/删除线等）的 goldmark 实例。
var markdownRenderer = goldmark.New(goldmark.WithExtensions(extension.GFM))

var (
	ErrDeployArtifactNotFound = errors.New("产物不存在")
	ErrDeployNoPerm           = errors.New("无权部署此产物")
	ErrDeployNotFound         = errors.New("部署记录不存在")
	ErrDeployEmpty            = errors.New("产物内容为空，无法部署")
	ErrDeployNoArtifact       = errors.New("当前对话还没有可部署的产物")
)

// DeployArtifactRepo 部署服务依赖的产物访问能力。
type DeployArtifactRepo interface {
	GetLatestByRoot(ctx context.Context, rootID string) (*model.Artifact, error)
	GetConversationIDByRoot(ctx context.Context, rootID string) (string, error)
	GetLatestRootByConversation(ctx context.Context, convID string) (string, error)
}

// DeployConvRepo 部署服务用于鉴权的对话仓库能力。
type DeployConvRepo interface {
	GetByID(ctx context.Context, id string) (*model.Conversation, error)
	GetMember(ctx context.Context, conversationID, userID string) (*model.ConversationMember, error)
}

// DeployRepo 部署记录持久化能力。
type DeployRepo interface {
	Create(ctx context.Context, d model.Deployment) (*model.Deployment, error)
	UpdateStatus(ctx context.Context, id, status, url, errMsg string) (*model.Deployment, error)
	GetByID(ctx context.Context, id string) (*model.Deployment, error)
}

// DeploymentService 处理产物部署：落盘为可访问站点 + 可打包下载。
type DeploymentService struct {
	repo          DeployRepo
	artRepo       DeployArtifactRepo
	convRepo      DeployConvRepo
	baseDir       string
	mu            sync.RWMutex
	publicBaseURL string // 配置了内网穿透/公网入口时的基址（如 https://xxx.trycloudflare.com）
}

// NewDeploymentService 创建部署服务。baseDir 为站点落盘根目录（空则默认 ./data/sites）；
// publicBaseURL 为可选的公网基址，设置后预览/下载链接拼成绝对公网地址（用于内网穿透分享、二维码）。
func NewDeploymentService(repo DeployRepo, artRepo DeployArtifactRepo, convRepo DeployConvRepo, baseDir, publicBaseURL string) *DeploymentService {
	if baseDir == "" {
		baseDir = "./data/sites"
	}
	return &DeploymentService{
		repo:          repo,
		artRepo:       artRepo,
		convRepo:      convRepo,
		baseDir:       baseDir,
		publicBaseURL: strings.TrimRight(publicBaseURL, "/"),
	}
}

// BaseDir 暴露站点根目录，供静态服务定位文件。
func (s *DeploymentService) BaseDir() string { return s.baseDir }

// SetPublicBaseURL 在运行时设置公网基址（用于内网穿透隧道异步就绪后回填）。
// 传入空串可清除（隧道断开时回落到相对路径）。线程安全。
func (s *DeploymentService) SetPublicBaseURL(url string) {
	s.mu.Lock()
	s.publicBaseURL = strings.TrimRight(url, "/")
	s.mu.Unlock()
}

// PublicBaseURL 返回当前公网基址（可能为空）。线程安全。
func (s *DeploymentService) PublicBaseURL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.publicBaseURL
}

// publicURL 把站内相对路径按公网基址拼成绝对地址；未配置公网基址时原样返回相对路径
// （前端会按 window.location.origin 兜底拼接）。
func (s *DeploymentService) publicURL(rel string) string {
	s.mu.RLock()
	base := s.publicBaseURL
	s.mu.RUnlock()
	if base == "" {
		return rel
	}
	return base + rel
}

// decorate 为返回给调用方/前端的部署记录补全预览与下载地址（含公网基址装饰）。
func (s *DeploymentService) decorate(d *model.Deployment) *model.Deployment {
	if d == nil {
		return nil
	}
	rel := d.URL
	if rel == "" {
		rel = "/api/sites/" + d.ID + "/index.html"
	}
	d.URL = s.publicURL(rel)
	// 下载地址末段带真实 .zip 文件名：即使浏览器忽略 Content-Disposition（跨域下载常见），
	// 也会按 URL 末段命名，避免存成无扩展名的裸 UUID「假文件」。
	d.DownloadURL = s.publicURL("/api/deployments/" + d.ID + "/download/deployment-" + d.ID + ".zip")
	return d
}

// SiteDir 返回某次部署的盘上目录。
func (s *DeploymentService) SiteDir(id string) string { return filepath.Join(s.baseDir, id) }

// Deploy 将某血缘根的最新产物落盘为可访问站点，返回部署记录（url 为相对路径）。
func (s *DeploymentService) Deploy(ctx context.Context, rootID, userID string) (*model.Deployment, error) {
	convID, err := s.checkAccess(ctx, rootID, userID)
	if err != nil {
		return nil, err
	}

	art, err := s.artRepo.GetLatestByRoot(ctx, rootID)
	if err != nil {
		if errors.Is(err, repository.ErrArtifactRootNotFound) {
			return nil, ErrDeployArtifactNotFound
		}
		return nil, fmt.Errorf("get latest artifact: %w", err)
	}
	if art.Content == "" && art.URL == "" {
		return nil, ErrDeployEmpty
	}

	id := uuid.NewString()
	if _, err := s.repo.Create(ctx, model.Deployment{
		ID:             id,
		ArtifactRootID: rootID,
		ConversationID: convID,
		Mode:           "preview",
		Status:         "pending",
	}); err != nil {
		return nil, err
	}

	if werr := s.writeSite(id, art); werr != nil {
		updated, uerr := s.repo.UpdateStatus(ctx, id, "failed", "", werr.Error())
		if uerr != nil {
			return nil, fmt.Errorf("write site failed: %v; update status: %w", werr, uerr)
		}
		return updated, nil
	}

	url := "/api/sites/" + id + "/index.html"
	updated, err := s.repo.UpdateStatus(ctx, id, "success", url, "")
	if err != nil {
		return nil, err
	}
	return s.decorate(updated), nil
}

// DeployLatestInConversation 部署某对话中最新的产物（聊天「部署」指令用）。
func (s *DeploymentService) DeployLatestInConversation(ctx context.Context, convID, userID string) (*model.Deployment, error) {
	rootID, err := s.artRepo.GetLatestRootByConversation(ctx, convID)
	if err != nil {
		if errors.Is(err, repository.ErrArtifactRootNotFound) {
			return nil, ErrDeployNoArtifact
		}
		return nil, fmt.Errorf("latest artifact in conversation: %w", err)
	}
	return s.Deploy(ctx, rootID, userID)
}

// Get 查询部署状态。
func (s *DeploymentService) Get(ctx context.Context, id string) (*model.Deployment, error) {
	d, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrDeploymentNotFound) {
			return nil, ErrDeployNotFound
		}
		return nil, err
	}
	return s.decorate(d), nil
}

// writeSite 把产物写入 {baseDir}/{id}/：生成可预览 index.html + 非网页产物的原始源码文件。
func (s *DeploymentService) writeSite(id string, art *model.Artifact) error {
	dir := filepath.Join(s.baseDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir site: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(renderIndexHTML(art)), 0o644); err != nil {
		return fmt.Errorf("write index.html: %w", err)
	}

	// 非网页产物额外保留原始源码文件，文件名带正确扩展名，便于打包下载拿到真源码。
	if art.Type != "webpage" && art.Content != "" {
		name := artifactSourceName(art)
		if strings.EqualFold(name, "index.html") {
			name = "source-" + name // 避免覆盖预览首页
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(art.Content), 0o644); err != nil {
			return fmt.Errorf("write source file: %w", err)
		}
	}
	return nil
}

// renderIndexHTML 生成站点首页：
//   - webpage：内容本身即 HTML，直接作为站点首页（或外链跳转）
//   - document / markdown：服务端用 goldmark 渲染成干净的浅色文档页（自包含，离线可看）
//   - code：深色等宽代码页
//   - 其它 file：浅色纯文本页
func renderIndexHTML(art *model.Artifact) string {
	if art.Type == "webpage" {
		if art.Content != "" {
			return art.Content
		}
		if art.URL != "" {
			return `<!DOCTYPE html><meta charset="utf-8"><title>preview</title>` +
				`<meta http-equiv="refresh" content="0;url=` + html.EscapeString(art.URL) + `">`
		}
	}

	title := art.Title
	if title == "" {
		title = art.Filename
	}
	if title == "" {
		title = defaultTitle(art.Type)
	}

	switch {
	case art.Type == "code":
		return codePage(title, art)
	case isMarkdownArtifact(art):
		return docPage(title, renderMarkdownToHTML(art.Content))
	default:
		return docPage(title, `<pre class="plain">`+html.EscapeString(art.Content)+`</pre>`)
	}
}

// renderMarkdownToHTML 服务端渲染 markdown；失败则降级为转义纯文本。
func renderMarkdownToHTML(src string) string {
	var buf bytes.Buffer
	if err := markdownRenderer.Convert([]byte(src), &buf); err != nil {
		return `<pre class="plain">` + html.EscapeString(src) + `</pre>`
	}
	return buf.String()
}

// isMarkdownArtifact 判断是否按 markdown 渲染（document 类型默认按 markdown，goldmark 处理纯文本也安全）。
func isMarkdownArtifact(art *model.Artifact) bool {
	switch strings.ToLower(art.Language) {
	case "markdown", "md":
		return true
	}
	fn := strings.ToLower(art.Filename)
	if strings.HasSuffix(fn, ".md") || strings.HasSuffix(fn, ".markdown") {
		return true
	}
	return art.Type == "document"
}

// artifactSourceName 为打包下载的源码文件推导一个带正确扩展名的文件名。
func artifactSourceName(art *model.Artifact) string {
	if art.Filename != "" {
		if b := filepath.Base(filepath.Clean(art.Filename)); b != "." && b != string(os.PathSeparator) {
			return b
		}
	}
	base := slugify(art.Title)
	if base == "" {
		base = "artifact"
	}
	return base + sourceExt(art)
}

// sourceExt 依据语言 / 类型推导源码扩展名。
func sourceExt(art *model.Artifact) string {
	switch strings.ToLower(art.Language) {
	case "go":
		return ".go"
	case "ts", "typescript":
		return ".ts"
	case "tsx":
		return ".tsx"
	case "js", "javascript":
		return ".js"
	case "jsx":
		return ".jsx"
	case "python", "py":
		return ".py"
	case "json":
		return ".json"
	case "html":
		return ".html"
	case "css":
		return ".css"
	case "sql":
		return ".sql"
	case "yaml", "yml":
		return ".yaml"
	case "markdown", "md":
		return ".md"
	case "rust", "rs":
		return ".rs"
	case "java":
		return ".java"
	case "sh", "bash", "shell":
		return ".sh"
	}
	switch art.Type {
	case "document":
		return ".md"
	case "webpage":
		return ".html"
	default:
		return ".txt"
	}
}

// slugify 把标题转成文件名安全片段：保留 Unicode 字母/数字（含中日韩），空白/连接符转 -，
// 丢弃路径分隔符等不安全字符（其余如标点直接去掉）。
func slugify(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func defaultTitle(t string) string {
	switch t {
	case "document":
		return "文档"
	case "file":
		return "文件"
	case "code":
		return "代码"
	default:
		return "产物预览"
	}
}

// docPage 浅色文档页外壳（用于 markdown 渲染结果或纯文本）。
func docPage(title, bodyHTML string) string {
	return `<!DOCTYPE html><html lang="zh"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width, initial-scale=1">` +
		`<title>` + html.EscapeString(title) + `</title><style>` + docCSS + `</style></head>` +
		`<body><main class="doc">` + bodyHTML + `</main></body></html>`
}

// codePage 深色等宽代码页，带文件名/语言标题栏。
func codePage(title string, art *model.Artifact) string {
	header := html.EscapeString(title)
	if art.Language != "" {
		header += `  ·  ` + html.EscapeString(art.Language)
	}
	return `<!DOCTYPE html><html lang="zh"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width, initial-scale=1">` +
		`<title>` + html.EscapeString(title) + `</title><style>` + codeCSS + `</style></head>` +
		`<body><div class="bar">` + header + `</div>` +
		`<pre class="code"><code>` + html.EscapeString(art.Content) + `</code></pre></body></html>`
}

const docCSS = `body{margin:0;background:#f6f8fa;color:#1f2328;` +
	`font:16px/1.6 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"PingFang SC","Microsoft YaHei",sans-serif}` +
	`.doc{max-width:820px;margin:0 auto;padding:40px 24px;background:#fff;min-height:100vh;box-shadow:0 0 0 1px #d0d7de}` +
	`.doc h1,.doc h2,.doc h3{margin:1.4em 0 .6em;line-height:1.25}` +
	`.doc h1{font-size:2em;border-bottom:1px solid #d8dee4;padding-bottom:.3em}` +
	`.doc h2{font-size:1.5em;border-bottom:1px solid #d8dee4;padding-bottom:.3em}` +
	`.doc p{margin:.6em 0}.doc ul,.doc ol{padding-left:1.6em}.doc li{margin:.2em 0}` +
	`.doc input[type=checkbox]{margin-right:.4em}` +
	`.doc code{background:#eff1f3;padding:.2em .4em;border-radius:6px;font:13px/1.4 ui-monospace,Consolas,monospace}` +
	`.doc pre{background:#1e1e1e;color:#d4d4d4;padding:16px;border-radius:8px;overflow:auto}` +
	`.doc pre code{background:none;color:inherit;padding:0}` +
	`.doc pre.plain{background:#fff;color:#1f2328;border:1px solid #d0d7de;white-space:pre-wrap;word-break:break-word;font:14px/1.6 ui-monospace,Consolas,monospace}` +
	`.doc table{border-collapse:collapse}.doc th,.doc td{border:1px solid #d0d7de;padding:6px 12px}` +
	`.doc a{color:#0969da}`

const codeCSS = `body{margin:0;background:#1e1e1e;color:#d4d4d4;font:14px/1.6 ui-monospace,Menlo,Consolas,monospace}` +
	`.bar{position:sticky;top:0;background:#252526;color:#9da5b4;padding:10px 16px;border-bottom:1px solid #333;font-size:13px}` +
	`.code{margin:0;padding:16px;white-space:pre;overflow:auto}`

// checkAccess 校验 rootId 对应产物所属对话，且当前用户为成员（或对话创建者），返回 convID。
func (s *DeploymentService) checkAccess(ctx context.Context, rootID, userID string) (string, error) {
	convID, err := s.artRepo.GetConversationIDByRoot(ctx, rootID)
	if err != nil {
		if errors.Is(err, repository.ErrArtifactRootNotFound) {
			return "", ErrDeployArtifactNotFound
		}
		return "", fmt.Errorf("resolve artifact conversation: %w", err)
	}

	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return "", fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return "", ErrDeployArtifactNotFound
	}

	member, err := s.convRepo.GetMember(ctx, convID, userID)
	if err != nil {
		return "", fmt.Errorf("check member: %w", err)
	}
	if member != nil {
		return convID, nil
	}
	if conv.UserID == userID {
		return convID, nil
	}
	return "", ErrDeployNoPerm
}
