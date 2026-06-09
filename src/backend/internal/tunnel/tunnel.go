// Package tunnel 提供「零配置内网穿透」：启动时自动拉起 cloudflared 快速隧道
// （trycloudflare.com），抓取分配到的公网域名并回调上层用于拼接部署预览/下载的
// 公网链接。cloudflared 不存在时自动下载对应平台的单文件二进制，做到换机即用。
package tunnel

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"time"
)

// quickTunnelURLRe 匹配 cloudflared 输出中的快速隧道公网域名。
var quickTunnelURLRe = regexp.MustCompile(`https://[a-z0-9-]+\.trycloudflare\.com`)

// Manager 持有 cloudflared 子进程，负责其生命周期。
type Manager struct {
	cmd    *exec.Cmd
	logger *slog.Logger

	mu  sync.Mutex
	url string
}

// Start 启动一个指向 http://localhost:<port> 的 cloudflared 快速隧道。
// 二进制缺失时下载到 cacheDir。隧道公网域名就绪后调用 onURL（仅一次）。
// 非阻塞：进程在后台运行，返回的 Manager 可用于 Stop。
func Start(ctx context.Context, port int, cacheDir string, logger *slog.Logger, onURL func(string)) (*Manager, error) {
	bin, err := ensureBinary(ctx, cacheDir, logger)
	if err != nil {
		return nil, fmt.Errorf("准备 cloudflared 失败: %w", err)
	}

	local := fmt.Sprintf("http://localhost:%d", port)
	cmd := exec.Command(bin, "tunnel", "--no-autoupdate", "--url", local)

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		return nil, fmt.Errorf("启动 cloudflared 失败: %w", err)
	}

	m := &Manager{cmd: cmd, logger: logger}

	// 扫描输出，抓取公网域名。
	var once sync.Once
	go func() {
		scanner := bufio.NewScanner(pr)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if match := quickTunnelURLRe.FindString(line); match != "" {
				once.Do(func() {
					m.mu.Lock()
					m.url = match
					m.mu.Unlock()
					logger.Info("内网穿透隧道就绪", "url", match)
					if onURL != nil {
						onURL(match)
					}
				})
			}
		}
	}()

	// 回收：进程退出时关闭管道；ctx 取消时杀进程。
	go func() {
		err := cmd.Wait()
		_ = pw.Close()
		if err != nil && ctx.Err() == nil {
			logger.Warn("cloudflared 进程退出", "error", err)
		}
	}()
	go func() {
		<-ctx.Done()
		m.Stop()
	}()

	return m, nil
}

// URL 返回已抓取到的公网域名（未就绪时为空）。
func (m *Manager) URL() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.url
}

// Stop 终止 cloudflared 子进程。可重复调用。
func (m *Manager) Stop() {
	if m == nil || m.cmd == nil || m.cmd.Process == nil {
		return
	}
	_ = m.cmd.Process.Kill()
}

// ensureBinary 返回可用的 cloudflared 路径：优先 PATH / 已知安装位置 / 缓存目录，
// 否则按当前平台下载到 cacheDir。
func ensureBinary(ctx context.Context, cacheDir string, logger *slog.Logger) (string, error) {
	// 1) PATH 中已有
	if p, err := exec.LookPath("cloudflared"); err == nil {
		return p, nil
	}
	// 2) Windows 常见安装位置
	if runtime.GOOS == "windows" {
		for _, p := range []string{
			`C:\Program Files (x86)\cloudflared\cloudflared.exe`,
			`C:\Program Files\cloudflared\cloudflared.exe`,
		} {
			if fileExists(p) {
				return p, nil
			}
		}
	}
	// 3) 缓存目录中已下载
	target := filepath.Join(cacheDir, binName())
	if fileExists(target) {
		return target, nil
	}
	// 4) 下载
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	logger.Info("未找到 cloudflared，开始自动下载", "target", target)
	if err := downloadBinary(ctx, target, logger); err != nil {
		return "", err
	}
	logger.Info("cloudflared 下载完成", "path", target)
	return target, nil
}

func binName() string {
	if runtime.GOOS == "windows" {
		return "cloudflared.exe"
	}
	return "cloudflared"
}

// assetName 返回 GitHub release 资产名（darwin 为 .tgz 需解包）。
func assetName() (name string, isTgz bool, ok bool) {
	switch runtime.GOOS {
	case "windows":
		switch runtime.GOARCH {
		case "amd64":
			return "cloudflared-windows-amd64.exe", false, true
		case "386":
			return "cloudflared-windows-386.exe", false, true
		}
	case "linux":
		switch runtime.GOARCH {
		case "amd64":
			return "cloudflared-linux-amd64", false, true
		case "arm64":
			return "cloudflared-linux-arm64", false, true
		case "arm":
			return "cloudflared-linux-arm", false, true
		case "386":
			return "cloudflared-linux-386", false, true
		}
	case "darwin":
		switch runtime.GOARCH {
		case "amd64":
			return "cloudflared-darwin-amd64.tgz", true, true
		case "arm64":
			return "cloudflared-darwin-arm64.tgz", true, true
		}
	}
	return "", false, false
}

func downloadBinary(ctx context.Context, target string, logger *slog.Logger) error {
	asset, isTgz, ok := assetName()
	if !ok {
		return fmt.Errorf("不支持的平台 %s/%s，请手动安装 cloudflared", runtime.GOOS, runtime.GOARCH)
	}
	url := "https://github.com/cloudflare/cloudflared/releases/latest/download/" + asset

	client := &http.Client{Timeout: 5 * time.Minute}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载 cloudflared 返回 HTTP %d", resp.StatusCode)
	}

	if isTgz {
		return extractTgzBinary(resp.Body, target)
	}
	return writeExecutable(resp.Body, target)
}

func writeExecutable(r io.Reader, target string) error {
	tmp := target + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, target)
}

// extractTgzBinary 从 darwin 的 .tgz 中取出 cloudflared 可执行文件写到 target。
func extractTgzBinary(r io.Reader, target string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("压缩包内未找到 cloudflared")
		}
		if err != nil {
			return err
		}
		if filepath.Base(hdr.Name) == "cloudflared" {
			return writeExecutable(tr, target)
		}
	}
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
