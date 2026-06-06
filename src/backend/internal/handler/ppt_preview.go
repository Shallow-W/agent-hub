package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/gin-gonic/gin"
)

const pptPreviewTimeout = 45 * time.Second

var errPptPreviewToolMissing = errors.New("ppt preview tool missing")

// PptPreviewHandler renders uploaded PowerPoint files through LibreOffice when available.
type PptPreviewHandler struct {
	uploadDir string
}

func NewPptPreviewHandler(uploadDir string) *PptPreviewHandler {
	if uploadDir == "" {
		uploadDir = "./uploads"
	}
	return &PptPreviewHandler{uploadDir: uploadDir}
}

func (h *PptPreviewHandler) Preview(c *gin.Context) {
	absPath, err := h.safeUploadPath(c.Param("filepath"))
	if err != nil {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	ext := strings.ToLower(filepath.Ext(absPath))
	if ext != ".ppt" && ext != ".pptx" {
		c.AbortWithStatus(http.StatusUnsupportedMediaType)
		return
	}
	if _, err := os.Stat(absPath); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	pdfPath, err := h.ensurePDFPreview(c.Request.Context(), absPath)
	if err != nil {
		if errors.Is(err, errPptPreviewToolMissing) {
			middleware.ErrorResponse(c, http.StatusNotImplemented, 50101, "PPT preview converter is not installed")
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50031, "PPT preview failed")
		return
	}

	c.Header("Content-Disposition", "inline; filename=\""+strings.TrimSuffix(filepath.Base(absPath), ext)+".pdf\"")
	c.Header("Content-Type", "application/pdf")
	c.Header("X-Content-Type-Options", "nosniff")
	c.File(pdfPath)
}

func (h *PptPreviewHandler) safeUploadPath(filePath string) (string, error) {
	cleaned := filepath.Clean(strings.TrimPrefix(filePath, "/"))
	if cleaned == "." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) || cleaned == ".." {
		return "", fmt.Errorf("invalid path")
	}
	absPath, err := filepath.Abs(filepath.Join(h.uploadDir, cleaned))
	if err != nil {
		return "", err
	}
	uploadDirAbs, err := filepath.Abs(h.uploadDir)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(absPath, uploadDirAbs+string(os.PathSeparator)) && absPath != uploadDirAbs {
		return "", fmt.Errorf("path escapes upload dir")
	}
	return absPath, nil
}

func (h *PptPreviewHandler) ensurePDFPreview(ctx context.Context, sourcePath string) (string, error) {
	outDir := filepath.Join(h.uploadDir, "previews", "ppt")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("create preview dir: %w", err)
	}
	base := strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
	pdfPath := filepath.Join(outDir, base+".pdf")
	if previewIsFresh(pdfPath, sourcePath) {
		return pdfPath, nil
	}
	if err := convertWithLibreOffice(ctx, sourcePath, outDir); err != nil {
		if runtime.GOOS != "windows" {
			return "", err
		}
		if ppErr := convertWithPowerPoint(ctx, sourcePath, pdfPath); ppErr != nil {
			if errors.Is(err, errPptPreviewToolMissing) {
				return "", ppErr
			}
			return "", fmt.Errorf("convert ppt preview: libreoffice=%v; powerpoint=%w", err, ppErr)
		}
	}
	if _, err := os.Stat(pdfPath); err != nil {
		return "", fmt.Errorf("converted pdf missing: %w", err)
	}
	return pdfPath, nil
}

func convertWithLibreOffice(ctx context.Context, sourcePath, outDir string) error {
	soffice, err := findSoffice()
	if err != nil {
		return err
	}
	convertCtx, cancel := context.WithTimeout(ctx, pptPreviewTimeout)
	defer cancel()
	cmd := exec.CommandContext(
		convertCtx,
		soffice,
		"--headless",
		"--convert-to",
		"pdf",
		"--outdir",
		outDir,
		sourcePath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("libreoffice convert: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func convertWithPowerPoint(ctx context.Context, sourcePath, pdfPath string) error {
	powershell, err := exec.LookPath("powershell")
	if err != nil {
		return errPptPreviewToolMissing
	}
	script := `param(
  [string]$src,
  [string]$out
)
$pp = $null
$presentation = $null
try {
  $pp = New-Object -ComObject PowerPoint.Application
  $presentation = $pp.Presentations.Open($src, $true, $false, $false)
  $presentation.SaveAs($out, 32)
} finally {
  if ($presentation -ne $null) { $presentation.Close() }
  if ($pp -ne $null) { $pp.Quit() }
}
`
	scriptFile, err := os.CreateTemp("", "agenthub-ppt-preview-*.ps1")
	if err != nil {
		return fmt.Errorf("create powerpoint script: %w", err)
	}
	scriptPath := scriptFile.Name()
	defer os.Remove(scriptPath)
	if _, err := scriptFile.WriteString(script); err != nil {
		scriptFile.Close()
		return fmt.Errorf("write powerpoint script: %w", err)
	}
	if err := scriptFile.Close(); err != nil {
		return fmt.Errorf("close powerpoint script: %w", err)
	}

	convertCtx, cancel := context.WithTimeout(ctx, pptPreviewTimeout)
	defer cancel()
	cmd := exec.CommandContext(
		convertCtx,
		powershell,
		"-NoProfile",
		"-ExecutionPolicy",
		"Bypass",
		"-File",
		scriptPath,
		sourcePath,
		pdfPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("powerpoint convert: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func previewIsFresh(pdfPath, sourcePath string) bool {
	pdfInfo, err := os.Stat(pdfPath)
	if err != nil {
		return false
	}
	srcInfo, err := os.Stat(sourcePath)
	if err != nil {
		return false
	}
	return !pdfInfo.ModTime().Before(srcInfo.ModTime())
}

func findSoffice() (string, error) {
	for _, name := range []string{"soffice", "libreoffice"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	if runtime.GOOS == "windows" {
		for _, p := range []string{
			`C:\Program Files\LibreOffice\program\soffice.exe`,
			`C:\Program Files (x86)\LibreOffice\program\soffice.exe`,
		} {
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
	}
	return "", errPptPreviewToolMissing
}
