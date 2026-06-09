package service

import (
	"errors"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var ErrInvalidStoragePath = errors.New("invalid storage path")

// FileURLBuilder builds API URLs for files stored under the configured upload dir.
// Database rows keep relative storage paths; this builder decides whether clients
// receive relative API paths or absolute public URLs.
type FileURLBuilder struct {
	publicBaseURL string
}

func NewFileURLBuilder(publicBaseURL string) *FileURLBuilder {
	return &FileURLBuilder{publicBaseURL: strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")}
}

func (b *FileURLBuilder) APIURL(apiPath string) string {
	cleaned := cleanURLPath(apiPath)
	if cleaned == "" {
		return ""
	}
	if b == nil || b.publicBaseURL == "" {
		return "/" + cleaned
	}
	return b.publicBaseURL + "/" + cleaned
}

func (b *FileURLBuilder) UploadURL(filePath string) string {
	cleaned := cleanURLPath(filePath)
	if cleaned == "" {
		return ""
	}
	if strings.HasPrefix(cleaned, "uploads/") {
		return b.APIURL("/api/" + cleaned)
	}
	return b.APIURL("/api/uploads/" + cleaned)
}

func (b *FileURLBuilder) KnowledgeFileURL(kbID, fileID string) string {
	kbSegment := cleanURLSegment(kbID)
	fileSegment := cleanURLSegment(fileID)
	if kbSegment == "" || fileSegment == "" {
		return ""
	}
	return b.APIURL("/api/knowledge-bases/" + kbSegment + "/files/" + fileSegment + "/content")
}

// SafeJoinUploadPath resolves a persisted upload storage path under uploadDir.
// It accepts paths with or without the public "uploads/" prefix, but rejects
// traversal instead of cleaning it into a different file.
func SafeJoinUploadPath(uploadDir, storagePath string) (string, error) {
	if strings.TrimSpace(uploadDir) == "" {
		uploadDir = "./uploads"
	}
	cleaned := cleanUploadStoragePath(storagePath)
	if cleaned == "" {
		return "", ErrInvalidStoragePath
	}
	absPath, err := filepath.Abs(filepath.Join(uploadDir, filepath.FromSlash(cleaned)))
	if err != nil {
		return "", err
	}
	uploadDirAbs, err := filepath.Abs(uploadDir)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(absPath, uploadDirAbs+string(os.PathSeparator)) && absPath != uploadDirAbs {
		return "", ErrInvalidStoragePath
	}
	return absPath, nil
}

func cleanURLPath(value string) string {
	return cleanRelativeSlashPath(value, false)
}

func cleanUploadStoragePath(value string) string {
	trimmed := strings.TrimSpace(value)
	slashed := strings.ReplaceAll(trimmed, "\\", "/")
	if strings.HasPrefix(slashed, "/") || path.IsAbs(slashed) || filepath.IsAbs(trimmed) || strings.Contains(slashed, ":") {
		return ""
	}
	return cleanRelativeSlashPath(value, true)
}

func cleanRelativeSlashPath(value string, trimUploadsPrefix bool) string {
	cleaned := strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	cleaned = strings.TrimLeft(cleaned, "/")
	if hasDotDotPathSegment(cleaned) {
		return ""
	}
	if trimUploadsPrefix {
		cleaned = strings.TrimPrefix(cleaned, "uploads/")
	}
	cleaned = path.Clean("/" + cleaned)
	cleaned = strings.TrimLeft(cleaned, "/")
	if cleaned == "." || cleaned == "" || hasDotDotPathSegment(cleaned) {
		return ""
	}
	return cleaned
}

func cleanURLSegment(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "." || trimmed == ".." || strings.ContainsAny(trimmed, `/\`) {
		return ""
	}
	return url.PathEscape(trimmed)
}

func hasDotDotPathSegment(value string) bool {
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}
