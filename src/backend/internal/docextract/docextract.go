// Package docextract 尽力把上传的文档（文本/Office/PDF 等）抽取为纯文本，
// 供后端在派发任务前内联到 Agent 上下文，避免让 Agent 自行下载并解析二进制文件。
//
// 抽取策略：
//   - 纯文本类（txt/md/csv/json/...）：直接读取。
//   - 现代 Office（pptx/docx/xlsx，zip+xml 容器）：纯 Go 解析，无外部依赖。
//   - 旧格式 / PDF（ppt/doc/xls/pdf/odp/...）：调用 LibreOffice `--convert-to txt`（若已安装）。
//
// 任一步失败或格式不支持时返回 ok=false，调用方据此给出降级提示。
package docextract

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const sofficeTimeout = 45 * time.Second

var plainTextExts = map[string]bool{
	".txt": true, ".md": true, ".markdown": true, ".csv": true, ".tsv": true,
	".json": true, ".log": true, ".xml": true, ".yaml": true, ".yml": true,
	".html": true, ".htm": true, ".svg": true, ".ini": true, ".conf": true,
}

// xmlTextRe 抓取 OOXML 文本节点（<a:t>…</a:t> / <w:t>…</w:t> / <t>…</t>）的内容。
var xmlTextRe = regexp.MustCompile(`<(?:a:t|w:t|t)(?:\s[^>]*)?>([^<]*)</(?:a:t|w:t|t)>`)

// Extract 尽力把 filePath 指向的文档抽取为纯文本。maxRunes>0 时按字符数截断。
// 返回 ok=false 表示该格式无法在当前环境解析。
func Extract(ctx context.Context, filePath, fileName string, maxRunes int) (text string, ok bool) {
	// soffice 工作目录不确定，统一用绝对路径，避免「source file could not be loaded」。
	if abs, err := filepath.Abs(filePath); err == nil {
		filePath = abs
	}
	ext := strings.ToLower(filepath.Ext(fileName))
	if ext == "" {
		ext = strings.ToLower(filepath.Ext(filePath))
	}

	switch ext {
	case ".pptx", ".docx", ".xlsx":
		if t, err := extractOOXML(filePath); err == nil && strings.TrimSpace(t) != "" {
			return cap(t, maxRunes), true
		}
	case ".ppt", ".odp":
		// 演示文稿：先转 pptx 再纯 Go 抽取（txt 过滤器不适用于 Impress）。
		if t, ok := extractViaConvert(ctx, filePath, "pptx:Impress MS PowerPoint 2007 XML", "pptx"); ok {
			return cap(t, maxRunes), true
		}
	case ".doc", ".odt", ".rtf":
		if t, ok := extractViaConvert(ctx, filePath, "docx:MS Word 2007 XML", "docx"); ok {
			return cap(t, maxRunes), true
		}
	case ".xls", ".ods":
		if t, ok := extractViaConvert(ctx, filePath, "xlsx:Calc MS Excel 2007 XML", "xlsx"); ok {
			return cap(t, maxRunes), true
		}
	default:
		if plainTextExts[ext] {
			if b, err := os.ReadFile(filePath); err == nil {
				return cap(strings.TrimSpace(string(b)), maxRunes), true
			}
		}
		// 其它（含 pdf）：尝试用 Writer 文本过滤器兜底
		if t, ok := extractWithSofficeTxt(ctx, filePath); ok {
			return cap(t, maxRunes), true
		}
	}
	return "", false
}

// SofficeAvailable 报告当前环境是否能用 LibreOffice 做文档转换。
func SofficeAvailable() bool { return findSoffice() != "" }

func cap(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if maxRunes <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "\n...[内容过长已截断]"
}

// extractOOXML 从 pptx/docx/xlsx（zip 容器）中抽取所有文本节点。
func extractOOXML(filePath string) (string, error) {
	zr, err := zip.OpenReader(filePath)
	if err != nil {
		return "", err
	}
	defer zr.Close()

	var sb strings.Builder
	for _, f := range zr.File {
		name := f.Name
		if !strings.HasSuffix(name, ".xml") {
			continue
		}
		// 只看正文相关部件，跳过主题/样式/关系等噪声
		if !(strings.HasPrefix(name, "ppt/slides/slide") ||
			strings.HasPrefix(name, "ppt/notesSlides/") ||
			name == "word/document.xml" ||
			strings.HasPrefix(name, "word/") && strings.Contains(name, "document") ||
			strings.HasPrefix(name, "xl/sharedStrings") ||
			strings.HasPrefix(name, "xl/worksheets/")) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(io.LimitReader(rc, 8*1024*1024))
		rc.Close()
		for _, m := range xmlTextRe.FindAllSubmatch(data, -1) {
			seg := strings.TrimSpace(unescapeXML(string(m[1])))
			if seg != "" {
				sb.WriteString(seg)
				sb.WriteString(" ")
			}
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

func unescapeXML(s string) string {
	r := strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", "\"", "&apos;", "'")
	return r.Replace(s)
}

// sofficeConvert 用 LibreOffice 把 src 转成指定 filter/targetExt，返回输出文件路径与临时目录。
// 调用方负责删除 cleanupDir。
func sofficeConvert(ctx context.Context, src, filter, targetExt string) (outPath, cleanupDir string, ok bool) {
	soffice := findSoffice()
	if soffice == "" {
		return "", "", false
	}
	outDir, err := os.MkdirTemp("", "agenthub-extract-*")
	if err != nil {
		return "", "", false
	}
	cctx, cancel := context.WithTimeout(ctx, sofficeTimeout)
	defer cancel()
	// 独立 user profile，避免与正在运行的 LibreOffice 实例冲突导致转换失败。
	profile := "-env:UserInstallation=file:///" + filepath.ToSlash(filepath.Join(outDir, "lo-profile"))
	cmd := exec.CommandContext(cctx, soffice, profile, "--headless", "--convert-to", filter, "--outdir", outDir, src)
	if err := cmd.Run(); err != nil {
		os.RemoveAll(outDir)
		return "", "", false
	}
	out := filepath.Join(outDir, strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))+"."+targetExt)
	if info, err := os.Stat(out); err != nil || info.IsDir() {
		os.RemoveAll(outDir)
		return "", "", false
	}
	return out, outDir, true
}

// extractViaConvert 先用 LibreOffice 转成 OOXML（pptx/docx/xlsx），再用纯 Go 抽取文本。
func extractViaConvert(ctx context.Context, src, filter, targetExt string) (string, bool) {
	out, cleanup, ok := sofficeConvert(ctx, src, filter, targetExt)
	if !ok {
		return "", false
	}
	defer os.RemoveAll(cleanup)
	t, err := extractOOXML(out)
	if err != nil || strings.TrimSpace(t) == "" {
		return "", false
	}
	return t, true
}

// extractWithSofficeTxt 用 Writer 文本过滤器把文档转成 txt（适用于 doc/odt/pdf 等）。
func extractWithSofficeTxt(ctx context.Context, src string) (string, bool) {
	out, cleanup, ok := sofficeConvert(ctx, src, "txt:Text", "txt")
	if !ok {
		return "", false
	}
	defer os.RemoveAll(cleanup)
	b, err := os.ReadFile(out)
	if err != nil || strings.TrimSpace(string(b)) == "" {
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}

func findSoffice() string {
	// Windows 上优先用 soffice.com（控制台版，--convert-to 会同步等待完成）。
	if runtime.GOOS == "windows" {
		for _, p := range []string{
			`C:\Program Files\LibreOffice\program\soffice.com`,
			`C:\Program Files (x86)\LibreOffice\program\soffice.com`,
		} {
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				return p
			}
		}
	}
	for _, name := range []string{"soffice", "libreoffice"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	var candidates []string
	switch runtime.GOOS {
	case "windows":
		candidates = []string{
			`C:\Program Files\LibreOffice\program\soffice.com`,
			`C:\Program Files\LibreOffice\program\soffice.exe`,
			`C:\Program Files (x86)\LibreOffice\program\soffice.exe`,
		}
	case "darwin":
		candidates = []string{"/Applications/LibreOffice.app/Contents/MacOS/soffice"}
	default:
		candidates = []string{"/usr/bin/soffice", "/usr/local/bin/soffice", "/snap/bin/libreoffice"}
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}
