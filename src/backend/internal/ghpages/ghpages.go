// Package ghpages 提供「基于 GitHub Pages 的永久发布」：用 Personal Access Token 调用
// GitHub REST API，把部署产物的静态文件推送到一个专用公开仓库，并启用 Pages，得到一个
// 重启不变、可长期分享的公网地址（https://<owner>.github.io/<repo>/deploy-<id>/）。
//
// 与零配置内网穿透（cloudflared 临时隧道）互补：隧道适合即时预览、URL 每次重启都变；
// Pages 适合正式发布、URL 永久稳定。
package ghpages

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const apiBase = "https://api.github.com"

const (
	pagesReadyTimeout  = 5 * time.Minute
	pagesReadyInterval = 5 * time.Second
)

// Publisher 持有 GitHub 凭据与目标仓库，负责把站点文件推送到 GitHub Pages。
// 通过 NewPublisher 构造；凭据不全时返回 nil，调用方据此判断功能是否启用。
type Publisher struct {
	token  string
	owner  string
	repo   string
	client *http.Client

	mu      sync.Mutex
	ensured bool // 仓库存在 + Pages 已启用（首次发布时惰性确保，避免每次发布重复探测）
}

// NewPublisher 创建发布器。token/owner/repo 任一为空时返回 nil（表示未配置 GitHub 发布）。
func NewPublisher(token, owner, repo string) *Publisher {
	token = strings.TrimSpace(token)
	owner = strings.TrimSpace(owner)
	repo = strings.TrimSpace(repo)
	if token == "" || owner == "" || repo == "" {
		return nil
	}
	return &Publisher{
		token:  token,
		owner:  owner,
		repo:   repo,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// PagesBaseURL 返回该仓库 Pages 的根地址（不含末尾斜杠）。
func (p *Publisher) PagesBaseURL() string {
	return fmt.Sprintf("https://%s.github.io/%s", strings.ToLower(p.owner), p.repo)
}

// Publish 把 files（文件名 → 内容）推送到仓库的 dir/ 目录，返回 dir/index.html 的公网地址。
// 首次调用会惰性确保仓库存在并启用 Pages。
func (p *Publisher) Publish(ctx context.Context, dir string, files map[string][]byte) (string, error) {
	if err := p.ensure(ctx); err != nil {
		return "", err
	}
	for name, content := range files {
		path := dir + "/" + name
		if err := p.putFile(ctx, path, content, "deploy "+dir); err != nil {
			return "", fmt.Errorf("推送 %s 失败: %w", path, err)
		}
	}
	url := p.PagesBaseURL() + "/" + dir + "/index.html"
	if err := p.putFile(ctx, "index.html", latestIndexPage(url), "update latest deployment index"); err != nil {
		return "", fmt.Errorf("更新 Pages 首页失败: %w", err)
	}
	if err := p.waitUntilAvailable(ctx, url); err != nil {
		return "", err
	}
	return url, nil
}

func (p *Publisher) waitUntilAvailable(ctx context.Context, url string) error {
	ctx, cancel := context.WithTimeout(ctx, pagesReadyTimeout)
	defer cancel()

	var lastStatus int
	var lastErr error
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "AgentHub-GitHub-Pages-Publisher")
		resp, err := p.client.Do(req)
		if err == nil {
			lastStatus = resp.StatusCode
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		} else {
			lastErr = err
		}

		timer := time.NewTimer(pagesReadyInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			if lastErr != nil {
				return fmt.Errorf("GitHub Pages 已推送但预览暂不可访问: %w", lastErr)
			}
			return fmt.Errorf("GitHub Pages 已推送但预览暂不可访问，最后状态 HTTP %d", lastStatus)
		case <-timer.C:
		}
	}
}

func latestIndexPage(url string) []byte {
	return []byte(`<!DOCTYPE html><html lang="zh"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width, initial-scale=1">` +
		`<meta http-equiv="refresh" content="0;url=` + url + `">` +
		`<title>AgentHub Latest Deployment</title></head>` +
		`<body><p>正在打开最新部署：<a href="` + url + `">` + url + `</a></p></body></html>`)
}

// ensure 确保目标仓库存在且已启用 Pages（main 分支根目录）。线程安全、只成功执行一次。
func (p *Publisher) ensure(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ensured {
		return nil
	}
	if err := p.ensureRepo(ctx); err != nil {
		return err
	}
	if err := p.enablePages(ctx); err != nil {
		return err
	}
	// 站点是纯静态 HTML，放置 .nojekyll 让 Pages 跳过 Jekyll 构建，直接按原样服务文件，
	// 否则默认的 legacy(Jekyll) 构建会对 deploy-*/ 目录处理出错（status=errored / 404）。
	if err := p.putFile(ctx, ".nojekyll", []byte(""), "disable jekyll"); err != nil {
		return fmt.Errorf("写入 .nojekyll 失败: %w", err)
	}
	p.ensured = true
	return nil
}

// ensureRepo 探测仓库；不存在则创建为公开仓库（auto_init 生成初始 commit 与 main 分支，
// 以便后续 Contents API 写文件）。
func (p *Publisher) ensureRepo(ctx context.Context) error {
	status, _, err := p.do(ctx, http.MethodGet, "/repos/"+p.owner+"/"+p.repo, nil)
	if err != nil {
		return err
	}
	if status == http.StatusOK {
		return nil
	}
	if status != http.StatusNotFound {
		return fmt.Errorf("探测仓库返回 HTTP %d", status)
	}
	// 创建公开仓库（在当前 token 用户名下）。
	body := map[string]any{
		"name":        p.repo,
		"private":     false,
		"auto_init":   true,
		"description": "agent-hub 部署产物（GitHub Pages 永久发布）",
	}
	status, respBody, err := p.do(ctx, http.MethodPost, "/user/repos", body)
	if err != nil {
		return err
	}
	if status != http.StatusCreated {
		return fmt.Errorf("创建仓库失败 HTTP %d: %s", status, truncate(respBody, 300))
	}
	return nil
}

// enablePages 启用 GitHub Pages（源：main 分支根目录）。已启用时忽略 409。
func (p *Publisher) enablePages(ctx context.Context) error {
	body := map[string]any{
		"source": map[string]string{"branch": "main", "path": "/"},
	}
	status, respBody, err := p.do(ctx, http.MethodPost, "/repos/"+p.owner+"/"+p.repo+"/pages", body)
	if err != nil {
		return err
	}
	// 201 创建成功；409 已存在；422 有时表示已启用/正在处理 —— 均视为可用。
	switch status {
	case http.StatusCreated, http.StatusConflict, http.StatusUnprocessableEntity:
		return nil
	default:
		return fmt.Errorf("启用 Pages 失败 HTTP %d: %s", status, truncate(respBody, 300))
	}
}

// putFile 用 Contents API 写入单个文件；文件已存在时取其 sha 后更新（保证幂等可重发）。
func (p *Publisher) putFile(ctx context.Context, path string, content []byte, msg string) error {
	endpoint := "/repos/" + p.owner + "/" + p.repo + "/contents/" + path
	body := map[string]any{
		"message": msg,
		"content": base64.StdEncoding.EncodeToString(content),
		"branch":  "main",
	}
	if sha := p.fileSHA(ctx, endpoint); sha != "" {
		body["sha"] = sha
	}
	status, respBody, err := p.do(ctx, http.MethodPut, endpoint, body)
	if err != nil {
		return err
	}
	if status != http.StatusCreated && status != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", status, truncate(respBody, 300))
	}
	return nil
}

// fileSHA 返回已存在文件的 blob sha；不存在或出错时返回空串。
func (p *Publisher) fileSHA(ctx context.Context, endpoint string) string {
	status, respBody, err := p.do(ctx, http.MethodGet, endpoint+"?ref=main", nil)
	if err != nil || status != http.StatusOK {
		return ""
	}
	var meta struct {
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal(respBody, &meta); err != nil {
		return ""
	}
	return meta.SHA
}

// do 执行一次 GitHub API 请求，返回状态码与响应体。
func (p *Publisher) do(ctx context.Context, method, path string, body any) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, apiBase+path, reader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody, nil
}

func truncate(b []byte, n int) string {
	s := string(b)
	if len(s) > n {
		return s[:n]
	}
	return s
}
