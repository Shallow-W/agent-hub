package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"

	"github.com/agent-hub/backend/internal/model"
)

const (
	thumbMaxW = 200
	thumbMaxH = 200
)

var allowedMIMETypes = map[string]bool{
	"image/jpeg":      true,
	"image/png":       true,
	"image/gif":       true,
	"image/webp":      true,
	"application/pdf": true,
}

var allowedExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".webp": true,
	".pdf":  true,
}

var (
	ErrUploadEmpty       = errors.New("上传文件不能为空")
	ErrUploadTypeInvalid = errors.New("不支持的文件类型")
	ErrUploadTooBig      = errors.New("文件过大")
)

// UploadConfig 上传配置
type UploadConfig struct {
	Dir        string
	MaxImageMB int
	MaxPDFMB   int
}

// UploadService 文件上传服务
type UploadService struct {
	cfg UploadConfig
}

// NewUploadService 创建上传服务
func NewUploadService(cfg UploadConfig) *UploadService {
	if cfg.Dir == "" {
		cfg.Dir = "./uploads"
	}
	if cfg.MaxImageMB <= 0 {
		cfg.MaxImageMB = 20
	}
	if cfg.MaxPDFMB <= 0 {
		cfg.MaxPDFMB = 50
	}
	return &UploadService{cfg: cfg}
}

// UploadResult 上传结果
type UploadResult struct {
	FileName      string `json:"file_name"`
	MimeType      string `json:"mime_type"`
	FileSize      int64  `json:"file_size"`
	FilePath      string `json:"file_path"`
	ThumbnailPath string `json:"thumbnail_path,omitempty"`
	Width         int    `json:"width,omitempty"`
	Height        int    `json:"height,omitempty"`
}

// ProcessUpload 处理上传文件：验证 → 去重检查 → 保存 → 生成缩略图
// 基于文件内容 SHA256 去重，相同内容的文件复用已有物理文件，避免重复存储
func (s *UploadService) ProcessUpload(ctx context.Context, fileHeader *multipart.FileHeader) (*UploadResult, error) {
	if fileHeader == nil || fileHeader.Size == 0 {
		return nil, ErrUploadEmpty
	}

	// 预检：根据扩展名判断类型并检查客户端声明的大小
	ext := strings.ToLower(filepath.Ext(filepath.Base(fileHeader.Filename)))
	if !allowedExtensions[ext] {
		return nil, ErrUploadTypeInvalid
	}
	preMaxSize := int64(s.cfg.MaxPDFMB) << 20
	if isImageExt(ext) {
		preMaxSize = int64(s.cfg.MaxImageMB) << 20
	}
	if fileHeader.Size > preMaxSize {
		return nil, ErrUploadTooBig
	}

	// 文件名净化：只取 base name，防止路径穿越；去除 HTML 特殊字符防止 XSS
	safeName := filepath.Base(fileHeader.Filename)
	safeName = strings.Map(func(r rune) rune {
		switch r {
		case '<', '>', '&', '"', '\'', '{', '}', '|', ';':
			return '-'
		}
		return r
	}, safeName)

	// 确保目录存在
	origDir := filepath.Join(s.cfg.Dir, "originals")
	thumbDir := filepath.Join(s.cfg.Dir, "thumbnails")
	if err := os.MkdirAll(origDir, 0o755); err != nil {
		return nil, fmt.Errorf("create originals dir: %w", err)
	}

	// 读取文件内容并计算 SHA256 哈希用于去重
	src, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("open upload: %w", err)
	}
	defer src.Close()

	fileContent, err := io.ReadAll(src)
	if err != nil {
		return nil, fmt.Errorf("read upload: %w", err)
	}

	hash := sha256.Sum256(fileContent)
	hashHex := hex.EncodeToString(hash[:])
	contentBasedName := hashHex + ext

	// 检查去重：如果同哈希文件已存在，直接复用
	existingPath := filepath.Join(origDir, contentBasedName)
	if fi, err := os.Stat(existingPath); err == nil {
		// 文件已存在，复用
		mimeType, _ := detectMIME(existingPath)
		result := &UploadResult{
			FileName: safeName,
			MimeType: mimeType,
			FileSize: fi.Size(),
			FilePath: path.Join("uploads", "originals", contentBasedName),
		}

		// 检查缩略图是否也已存在
		if isImageMIME(mimeType) {
			thumbName := hashHex + ".jpg"
			existingThumb := filepath.Join(thumbDir, thumbName)
			if _, err := os.Stat(existingThumb); err == nil {
					result.ThumbnailPath = path.Join("uploads", "thumbnails", thumbName)
				// 从已有图片读取宽高
				if img, err := imaging.Open(existingPath); err == nil {
					bounds := img.Bounds()
					result.Width = bounds.Dx()
					result.Height = bounds.Dy()
				}
			} else {
				// 缩略图不存在，生成
				thumbPath, w, h, thumbErr := s.generateImageThumbnail(existingPath, hashHex, thumbDir)
				if thumbErr == nil {
					result.ThumbnailPath = thumbPath
					result.Width = w
					result.Height = h
				}
			}
		}

		return result, nil
	}

	// 新文件：写入磁盘
	if err := os.WriteFile(existingPath, fileContent, 0o644); err != nil {
		return nil, fmt.Errorf("save file: %w", err)
	}

	// 用实际文件内容检测 MIME 类型（不信任客户端）
	mimeType, err := detectMIME(existingPath)
	if err != nil || !allowedMIMETypes[mimeType] {
		os.Remove(existingPath)
		return nil, ErrUploadTypeInvalid
	}

	fi, err := os.Stat(existingPath)
	if err != nil {
		os.Remove(existingPath)
		return nil, fmt.Errorf("stat file: %w", err)
	}
	maxSize := int64(s.cfg.MaxPDFMB) << 20
	if isImageMIME(mimeType) {
		maxSize = int64(s.cfg.MaxImageMB) << 20
	}
	if fi.Size() > maxSize {
		os.Remove(existingPath)
		return nil, ErrUploadTooBig
	}

	result := &UploadResult{
		FileName: safeName,
		MimeType: mimeType,
		FileSize: fi.Size(),
		FilePath: path.Join("uploads", "originals", contentBasedName),
	}

	// 图片缩略图
	if isImageMIME(mimeType) {
		thumbPath, w, h, thumbErr := s.generateImageThumbnail(existingPath, hashHex, thumbDir)
		if thumbErr == nil {
			result.ThumbnailPath = thumbPath
			result.Width = w
			result.Height = h
		}
	}

	return result, nil
}

// generateImageThumbnail 生成图片缩略图
func (s *UploadService) generateImageThumbnail(srcPath, id, thumbDir string) (string, int, int, error) {
	img, err := imaging.Open(srcPath)
	if err != nil {
		return "", 0, 0, fmt.Errorf("open image: %w", err)
	}

	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	thumb := imaging.Thumbnail(img, thumbMaxW, thumbMaxH, imaging.Lanczos)
	if err := os.MkdirAll(thumbDir, 0o755); err != nil {
		return "", w, h, fmt.Errorf("create thumb dir: %w", err)
	}

	// 使用 path.Join（总是正斜杠），确保 URL 路径在所有操作系统上正确
	thumbRelPath := path.Join("uploads", "thumbnails", id+".jpg")
	thumbFullPath := filepath.Join(s.cfg.Dir, "thumbnails", id+".jpg")
	if err := imaging.Save(thumb, thumbFullPath); err != nil {
		return "", w, h, fmt.Errorf("save thumbnail: %w", err)
	}

	return thumbRelPath, w, h, nil
}

// ToMessageAttachment 将上传结果转换为消息附件模型
func (r *UploadResult) ToMessageAttachment() model.MessageAttachment {
	return model.MessageAttachment{
		FileName:      r.FileName,
		MimeType:      r.MimeType,
		FileSize:      r.FileSize,
		FilePath:      r.FilePath,
		ThumbnailPath: r.ThumbnailPath,
		Width:         r.Width,
		Height:        r.Height,
	}
}

// generateFileID 用 crypto/rand 生成唯一文件名
func generateFileID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// detectMIME 读取文件头部检测实际 MIME 类型
func detectMIME(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}
	return http.DetectContentType(buf[:n]), nil
}

func isImageMIME(mime string) bool {
	return strings.HasPrefix(mime, "image/")
}

func isImageExt(ext string) bool {
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" || ext == ".webp"
}

// ImageDimensions 从图片读取宽高（用于已上传但未生成缩略图的场景）
func ImageDimensions(path string) (int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	img, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, err
	}
	return img.Width, img.Height, nil
}
