package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/agent-hub/backend/internal/ghpages"
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
	ErrGitHubNotConfigured    = errors.New("未配置 GitHub 发布（需在 config.yaml 的 github 段或环境变量填入 token/owner/pages_repo）")
)

// DeployArtifactRepo 部署服务依赖的产物访问能力。
type DeployArtifactRepo interface {
	GetLatestByRoot(ctx context.Context, rootID string) (*model.Artifact, error)
	GetConversationIDByRoot(ctx context.Context, rootID string) (string, error)
	GetLatestRootByConversation(ctx context.Context, convID string) (string, error)
	GetLatestByConversationAndName(ctx context.Context, convID, name string) (*model.Artifact, error)
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
	publicBaseURL string             // 配置了内网穿透/公网入口时的基址（如 https://xxx.trycloudflare.com）
	pages         *ghpages.Publisher // 配置了 GitHub Pages 发布时非 nil
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

// SetGitHubPublisher 注入 GitHub Pages 发布器（nil 表示未配置，PublishGitHub 将返回 ErrGitHubNotConfigured）。
func (s *DeploymentService) SetGitHubPublisher(p *ghpages.Publisher) {
	s.mu.Lock()
	s.pages = p
	s.mu.Unlock()
}

// GitHubEnabled 返回是否已配置 GitHub Pages 发布（供前端决定是否展示该入口）。
func (s *DeploymentService) GitHubEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pages != nil
}

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
	// GitHub Pages 等已是绝对公网地址，原样输出，不要再拼内网穿透基址。
	if strings.HasPrefix(rel, "http://") || strings.HasPrefix(rel, "https://") {
		d.URL = rel
	} else {
		d.URL = s.publicURL(rel)
	}
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

// DeployByConversation 按对话 ID 和可选名称查找产物并部署预览。
func (s *DeploymentService) DeployByConversation(ctx context.Context, convID, userID, artifactName string) (*model.Deployment, error) {
	art, err := s.artRepo.GetLatestByConversationAndName(ctx, convID, artifactName)
	if err != nil {
		return nil, fmt.Errorf("find artifact by conversation: %w", err)
	}
	if art == nil {
		return nil, ErrDeployNoArtifact
	}
	return s.Deploy(ctx, art.RootID, userID)
}

// PublishGitHubByConversation 按对话 ID 和可选名称查找产物并发布到 GitHub Pages。
func (s *DeploymentService) PublishGitHubByConversation(ctx context.Context, convID, userID, artifactName string) (*model.Deployment, error) {
	art, err := s.artRepo.GetLatestByConversationAndName(ctx, convID, artifactName)
	if err != nil {
		return nil, fmt.Errorf("find artifact by conversation: %w", err)
	}
	if art == nil {
		return nil, ErrDeployNoArtifact
	}
	return s.PublishGitHub(ctx, art.RootID, userID)
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

// renderSiteFiles 生成一次部署的站点文件集合（文件名 → 内容）：可预览 index.html +
// 非网页产物的原始源码文件。本地落盘与 GitHub Pages 推送共用，保证两条发布路径一致。
func renderSiteFiles(art *model.Artifact) map[string][]byte {
	files := map[string][]byte{
		"index.html": []byte(renderIndexHTML(art)),
	}
	// 非网页产物额外保留原始源码文件，文件名带正确扩展名，便于打包下载拿到真源码。
	if art.Type != "webpage" && art.Content != "" {
		name := artifactSourceName(art)
		if strings.EqualFold(name, "index.html") {
			name = "source-" + name // 避免覆盖预览首页
		}
		content := art.Content
		if isMarkdownArtifact(art) {
			content = markdownDocumentContent(content)
		}
		files[name] = []byte(content)
	}
	return files
}

// writeSite 把产物写入 {baseDir}/{id}/：生成可预览 index.html + 非网页产物的原始源码文件。
func (s *DeploymentService) writeSite(id string, art *model.Artifact) error {
	dir := filepath.Join(s.baseDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir site: %w", err)
	}
	for name, content := range renderSiteFiles(art) {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}

// PublishGitHub 把某血缘根的最新产物发布到 GitHub Pages（永久公网地址），同时在本地落盘
// 一份用于「源码 zip 下载」。返回的部署记录 mode=github、url 为 Pages 公网地址。
func (s *DeploymentService) PublishGitHub(ctx context.Context, rootID, userID string) (*model.Deployment, error) {
	s.mu.RLock()
	pub := s.pages
	s.mu.RUnlock()
	if pub == nil {
		return nil, ErrGitHubNotConfigured
	}

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
		Mode:           "github",
		Status:         "pending",
	}); err != nil {
		return nil, err
	}

	fail := func(reason string) (*model.Deployment, error) {
		updated, uerr := s.repo.UpdateStatus(ctx, id, "failed", "", reason)
		if uerr != nil {
			return nil, fmt.Errorf("%s; update status: %w", reason, uerr)
		}
		return s.decorate(updated), nil
	}

	// 本地落盘一份，供「源码 zip 下载」复用既有 /api/deployments/:id/download 端点。
	if werr := s.writeSite(id, art); werr != nil {
		return fail(werr.Error())
	}

	pagesURL, perr := pub.Publish(ctx, "deploy-"+id, renderSiteFiles(art))
	if perr != nil {
		return fail(perr.Error())
	}

	updated, err := s.repo.UpdateStatus(ctx, id, "success", pagesURL, "")
	if err != nil {
		return nil, err
	}
	return s.decorate(updated), nil
}

// fetchURLContent 尝试从 URL 获取 HTML 内容，超时 5 秒。成功返回 HTML 字符串，失败返回空串。
func fetchURLContent(rawURL string) string {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(rawURL)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	// 限制读取大小为 10MB，防止读取过大内容
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return ""
	}
	return string(body)
}

// renderIndexHTML 生成站点首页：
//   - webpage：内容本身即 HTML，直接作为站点首页（或从 URL 拉取内容）
//   - document / markdown：服务端用 goldmark 渲染成干净的浅色文档页（自包含，离线可看）
//   - code：深色等宽代码页
//   - 其它 file：浅色纯文本页
func renderIndexHTML(art *model.Artifact) string {
	if art.Type == "webpage" {
		if art.Content != "" {
			return art.Content
		}
		if art.URL != "" {
			// 尝试从 URL 拉取实际 HTML 内容，避免将隧道用户重定向到不可达的 localhost
			if fetched := fetchURLContent(art.URL); fetched != "" {
				return fetched
			}
			// 拉取失败：生成友好的错误页面，而不是 meta refresh 到不可达地址
			return unavailablePreviewPage(art.URL)
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
	src = markdownDocumentContent(src)
	if err := markdownRenderer.Convert([]byte(src), &buf); err != nil {
		return `<pre class="plain">` + html.EscapeString(src) + `</pre>`
	}
	return buf.String()
}

func markdownDocumentContent(src string) string {
	unwrapped := unwrapMarkdownDocumentFence(src)
	if unwrapped != src {
		return unwrapped
	}

	normalized := strings.ReplaceAll(src, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	for i, line := range lines {
		if !isMarkdownFenceOpen(line) {
			continue
		}
		for j := len(lines) - 1; j > i; j-- {
			if !isFenceClose(lines[j]) {
				continue
			}
			candidate := strings.TrimRight(strings.Join(lines[i+1:j], "\n"), " \t\r\n")
			if looksLikeMarkdownDocument(candidate) {
				return candidate
			}
			break
		}
	}

	offset := 0
	for _, line := range strings.SplitAfter(normalized, "\n") {
		if isMarkdownHeading(strings.TrimRight(line, "\r\n")) && offset > 0 {
			candidate := strings.TrimRight(normalized[offset:], " \t\r\n")
			if looksLikeMarkdownDocument(candidate) {
				return candidate
			}
		}
		offset += len(line)
	}
	return src
}

func unwrapMarkdownDocumentFence(src string) string {
	normalized := strings.ReplaceAll(src, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	first := -1
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			first = i
			break
		}
	}
	last := len(lines) - 1
	for last >= 0 && strings.TrimSpace(lines[last]) == "" {
		last--
	}
	if first < 0 || last <= first {
		return src
	}
	if !isMarkdownFenceOpen(lines[first]) {
		return src
	}
	if !isFenceClose(lines[last]) {
		return src
	}
	return strings.TrimRight(strings.Join(lines[first+1:last], "\n"), " \t\r\n")
}

func isMarkdownFenceOpen(line string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(line))
	ticks := 0
	for ticks < len(trimmed) && trimmed[ticks] == '`' {
		ticks++
	}
	if ticks < 3 {
		return false
	}
	lang := strings.TrimSpace(trimmed[ticks:])
	return lang == "markdown" || lang == "md"
}

func isFenceClose(line string) bool {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 3 {
		return false
	}
	for _, r := range trimmed {
		if r != '`' {
			return false
		}
	}
	return true
}

func looksLikeMarkdownDocument(src string) bool {
	trimmed := strings.TrimSpace(src)
	if len(trimmed) < 40 {
		return false
	}
	headingCount := 0
	for _, line := range strings.Split(trimmed, "\n") {
		if isMarkdownHeading(line) {
			headingCount++
		}
	}
	if headingCount == 0 {
		return false
	}
	if headingCount >= 2 {
		return true
	}
	for _, line := range strings.Split(trimmed, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") || strings.HasPrefix(t, "```") || strings.Contains(t, "|") {
			return true
		}
	}
	return false
}

func isMarkdownHeading(line string) bool {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 {
		return false
	}
	count := 0
	for count < len(trimmed) && count < 3 && trimmed[count] == '#' {
		count++
	}
	if count == 0 || len(trimmed) <= count || trimmed[count] != ' ' {
		return false
	}
	return strings.TrimSpace(trimmed[count+1:]) != ""
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
	`.doc pre{margin:.6em 0 1em;color:#1f2328;white-space:pre-wrap;font:inherit}` +
	`.doc pre code{background:none;color:inherit;padding:0;font:inherit}` +
	`.doc pre.plain{background:#fff;color:#1f2328;border:1px solid #d0d7de;white-space:pre-wrap;word-break:break-word;font:14px/1.6 ui-monospace,Consolas,monospace}` +
	`.doc table{border-collapse:collapse}.doc th,.doc td{border:1px solid #d0d7de;padding:6px 12px}` +
	`.doc a{color:#0969da}`

const codeCSS = `body{margin:0;background:#1e1e1e;color:#d4d4d4;font:14px/1.6 ui-monospace,Menlo,Consolas,monospace}` +
	`.bar{position:sticky;top:0;background:#252526;color:#9da5b4;padding:10px 16px;border-bottom:1px solid #333;font-size:13px}` +
	`.code{margin:0;padding:16px;white-space:pre;overflow:auto}`

// unavailablePreviewPage 生成一个友好的错误页面，告知用户预览内容不可用。
func unavailablePreviewPage(url string) string {
	return `<!DOCTYPE html><html lang="zh"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width, initial-scale=1">` +
		`<title>预览不可用</title><style>` +
		`body{margin:0;background:#f6f8fa;color:#1f2328;display:flex;align-items:center;justify-content:center;min-height:100vh;` +
		`font:16px/1.6 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"PingFang SC","Microsoft YaHei",sans-serif}` +
		`.card{max-width:520px;padding:32px;background:#fff;border-radius:12px;box-shadow:0 1px 3px rgba(0,0,0,.1)}` +
		`h1{margin:0 0 .5em;font-size:1.4em;color:#cf222e}` +
		`p{margin:.4em 0;color:#656d76}` +
		`code{background:#eff1f3;padding:.15em .4em;border-radius:4px;font-size:14px}` +
		`</style></head><body><div class="card">` +
		`<h1>预览内容不可用</h1>` +
		`<p>该 artifact 仅包含一个本地 URL（<code>` + html.EscapeString(url) + `</code>），无法通过公网访问。</p>` +
		`<p>请确保 artifact 包含完整的 HTML 内容（content 字段），而非仅引用本地服务地址。</p>` +
		`</div></body></html>`
}

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
