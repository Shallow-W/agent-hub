package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/repository"
)

var (
	ErrKBNotFound     = errors.New("知识库不存在")
	ErrKBNoPermission = errors.New("无权访问该知识库")
	ErrKBNameEmpty    = errors.New("知识库名称不能为空")
	ErrKBNotPublic    = errors.New("该知识库不是公开的")
	ErrKBFileEmpty    = errors.New("上传文件不能为空")
	ErrKBFileNotFound = errors.New("文件不存在")
)

// KnowledgeService 知识库业务逻辑
type KnowledgeService struct {
	kbRepo    *repository.KnowledgeRepo
	userRepo  *repository.UserRepo
	uploadDir string
}

// NewKnowledgeService 创建知识库服务
func NewKnowledgeService(kbRepo *repository.KnowledgeRepo, userRepo *repository.UserRepo, uploadDir string) *KnowledgeService {
	if uploadDir == "" {
		uploadDir = "./uploads"
	}
	return &KnowledgeService{
		kbRepo:    kbRepo,
		userRepo:  userRepo,
		uploadDir: uploadDir,
	}
}

// Create 创建知识库
func (s *KnowledgeService) Create(ctx context.Context, userID, name, description string) (*model.KnowledgeBase, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrKBNameEmpty
	}
	if len(name) > 100 {
		name = name[:100]
	}
	return s.kbRepo.Create(ctx, userID, name, description)
}

// List 获取用户的知识库列表
func (s *KnowledgeService) List(ctx context.Context, userID string) ([]model.KnowledgeBase, error) {
	kbs, err := s.kbRepo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	// 填充文件列表
	for i := range kbs {
		files, err := s.kbRepo.ListFiles(ctx, kbs[i].ID)
		if err != nil {
			return nil, err
		}
		kbs[i].Files = files
	}
	return kbs, nil
}

// UpdateVisibility 更新知识库可见性
func (s *KnowledgeService) UpdateVisibility(ctx context.Context, userID, kbID, visibility string) error {
	kb, err := s.kbRepo.GetByID(ctx, kbID)
	if err != nil {
		return err
	}
	if kb == nil {
		return ErrKBNotFound
	}
	if kb.UserID != userID {
		return ErrKBNoPermission
	}
	if visibility != "private" && visibility != "public" {
		return errors.New("无效的可见性值")
	}
	return s.kbRepo.UpdateVisibility(ctx, kbID, visibility)
}

// Delete 删除知识库
func (s *KnowledgeService) Delete(ctx context.Context, userID, kbID string) error {
	kb, err := s.kbRepo.GetByID(ctx, kbID)
	if err != nil {
		return err
	}
	if kb == nil {
		return ErrKBNotFound
	}
	if kb.UserID != userID {
		return ErrKBNoPermission
	}
	// 删除物理文件
	files, err := s.kbRepo.ListFiles(ctx, kbID)
	if err != nil {
		return err
	}
	for _, f := range files {
		_ = os.Remove(filepath.Join(s.uploadDir, filepath.Clean(f.FilePath)))
	}
	return s.kbRepo.Delete(ctx, kbID)
}

// UploadFile 上传文件到知识库
func (s *KnowledgeService) UploadFile(ctx context.Context, userID, kbID string, fileHeader *multipart.FileHeader) error {
	kb, err := s.kbRepo.GetByID(ctx, kbID)
	if err != nil {
		return err
	}
	if kb == nil {
		return ErrKBNotFound
	}
	if kb.UserID != userID {
		return ErrKBNoPermission
	}
	if fileHeader == nil || fileHeader.Size == 0 {
		return ErrKBFileEmpty
	}

	// 读取文件并计算哈希
	src, err := fileHeader.Open()
	if err != nil {
		return fmt.Errorf("open upload: %w", err)
	}
	defer src.Close()

	fileContent, err := io.ReadAll(src)
	if err != nil {
		return fmt.Errorf("read upload: %w", err)
	}

	hash := sha256.Sum256(fileContent)
	hashHex := hex.EncodeToString(hash[:])

	// 保存文件到 uploads/knowledge/ 目录
	ext := strings.ToLower(filepath.Ext(filepath.Base(fileHeader.Filename)))
	safeName := filepath.Base(fileHeader.Filename)
	kbDir := filepath.Join(s.uploadDir, "knowledge", kbID)
	if err := os.MkdirAll(kbDir, 0o755); err != nil {
		return fmt.Errorf("create kb dir: %w", err)
	}

	storedName := hashHex + ext
	storedPath := filepath.Join(kbDir, storedName)
	if _, err := os.Stat(storedPath); os.IsNotExist(err) {
		if err := os.WriteFile(storedPath, fileContent, 0o644); err != nil {
			return fmt.Errorf("save file: %w", err)
		}
	}

	// 数据库路径使用正斜杠
	dbPath := path.Join("knowledge", kbID, storedName)

	// 检测MIME
	mimeType := detectFileMIME(fileHeader.Filename, fileContent)

	// 上传时预处理：提取文本内容或标记文件类型
	previewText, previewType := extractPreview(fileHeader.Filename, mimeType, fileContent)

	_, err = s.kbRepo.AddFile(ctx, kbID, safeName, dbPath, fileHeader.Size, mimeType, hashHex, previewText, previewType)
	return err
}

// GetUploadDir 返回上传目录路径
func (s *KnowledgeService) GetUploadDir() string {
	return s.uploadDir
}

// GetFile 获取知识库中的单个文件（含权限验证）
func (s *KnowledgeService) GetFile(ctx context.Context, userID, kbID, fileID string) (*model.KnowledgeFile, error) {
	kb, err := s.kbRepo.GetByID(ctx, kbID)
	if err != nil {
		return nil, err
	}
	if kb == nil {
		return nil, ErrKBNotFound
	}
	if kb.UserID != userID && kb.Visibility != "public" {
		return nil, ErrKBNoPermission
	}

	f, err := s.kbRepo.GetFileByID(ctx, kbID, fileID)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, ErrKBFileNotFound
	}
	return f, nil
}

// ListFiles 获取知识库中的文件列表（含权限验证）
func (s *KnowledgeService) ListFiles(ctx context.Context, userID, kbID string) ([]model.KnowledgeFile, error) {
	kb, err := s.kbRepo.GetByID(ctx, kbID)
	if err != nil {
		return nil, err
	}
	if kb == nil {
		return nil, ErrKBNotFound
	}
	if kb.UserID != userID && kb.Visibility != "public" {
		return nil, ErrKBNoPermission
	}
	return s.kbRepo.ListFiles(ctx, kbID)
}

// DeleteFile 删除知识库文件
func (s *KnowledgeService) DeleteFile(ctx context.Context, userID, kbID, fileID string) error {
	kb, err := s.kbRepo.GetByID(ctx, kbID)
	if err != nil {
		return err
	}
	if kb == nil {
		return ErrKBNotFound
	}
	if kb.UserID != userID {
		return ErrKBNoPermission
	}

	filePath, err := s.kbRepo.DeleteFile(ctx, kbID, fileID)
	if err != nil {
		return err
	}
	if filePath == "" {
		return ErrKBFileNotFound
	}
	// 删除物理文件
	_ = os.Remove(filepath.Join(s.uploadDir, filepath.Clean(filePath)))
	return nil
}

// ListGroupKnowledgeBases 返回群组中当前用户可用的知识库列表：
// 自己的全部 KB（含私有和公开） + 其他群成员的公开 KB。
func (s *KnowledgeService) ListGroupKnowledgeBases(ctx context.Context, currentUserID string, memberUserIDs []string) ([]model.KnowledgeBase, error) {
	// 1. 获取自己的全部 KB
	ownKBs, err := s.List(ctx, currentUserID)
	if err != nil {
		return nil, fmt.Errorf("list own knowledge bases: %w", err)
	}
	// 填充 username
	user, err := s.userRepo.GetUserByID(ctx, currentUserID)
	if err != nil || user == nil {
		return nil, fmt.Errorf("get current user: %w", err)
	}
	for i := range ownKBs {
		ownKBs[i].Username = user.Username
		ownKBs[i].Files = nil // 列表场景不需要文件内容
	}

	// 2. 获取其他成员的公开 KB
	otherIDs := make([]string, 0, len(memberUserIDs))
	for _, id := range memberUserIDs {
		if id != currentUserID {
			otherIDs = append(otherIDs, id)
		}
	}
	if len(otherIDs) == 0 {
		return ownKBs, nil
	}

	publicKBs, err := s.kbRepo.ListPublicByUsers(ctx, otherIDs, currentUserID)
	if err != nil {
		return nil, fmt.Errorf("list public knowledge bases: %w", err)
	}

	result := make([]model.KnowledgeBase, 0, len(ownKBs)+len(publicKBs))
	result = append(result, ownKBs...)
	result = append(result, publicKBs...)
	return result, nil
}

// ResolveKnowledgeRef 解析群聊中的知识库引用 "用户名/知识库名"
// 当前用户可以引用自己的（私有/公开）或他人的（仅公开）知识库
func (s *KnowledgeService) ResolveKnowledgeRef(ctx context.Context, currentUserID, username, kbName string) (*model.KnowledgeBase, []model.KnowledgeFile, error) {
	// 先查是否是自己的知识库
	user, err := s.userRepo.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, nil, fmt.Errorf("查找用户失败: %w", err)
	}
	if user == nil {
		return nil, nil, fmt.Errorf("用户 %s 不存在", username)
	}

	// 如果是自己，查看所有（私有+公开）
	if user.ID == currentUserID {
		kb, err := s.kbRepo.FindByUserAndName(ctx, currentUserID, kbName)
		if err != nil {
			return nil, nil, err
		}
		if kb == nil {
			return nil, nil, fmt.Errorf("知识库 %s 不存在", kbName)
		}
		files, err := s.kbRepo.ListFiles(ctx, kb.ID)
		if err != nil {
			return nil, nil, err
		}
		return kb, files, nil
	}

	// 如果是他人，只查看公开的
	kb, err := s.kbRepo.FindPublicByName(ctx, username, kbName)
	if err != nil {
		return nil, nil, err
	}
	if kb == nil {
		return nil, nil, fmt.Errorf("知识库 %s/%s 不存在或不是公开的", username, kbName)
	}
	files, err := s.kbRepo.ListFiles(ctx, kb.ID)
	if err != nil {
		return nil, nil, err
	}
	return kb, files, nil
}

// detectFileMIME 根据文件扩展名和内容检测MIME类型
func detectFileMIME(filename string, content []byte) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".pdf":
		return "application/pdf"
	case ".txt":
		return "text/plain"
	case ".md":
		return "text/markdown"
	case ".json":
		return "application/json"
	case ".csv":
		return "text/csv"
	case ".html", ".htm":
		return "text/html"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	default:
		return "application/octet-stream"
	}
}

// previewTextMaxSize 预览文本的最大字节数（200KB）
const previewTextMaxSize = 200 * 1024

// previewableTextExts 可以提取文本内容的文件扩展名
var previewableTextExts = map[string]bool{
	".txt": true, ".md": true, ".markdown": true, ".json": true, ".csv": true,
	".html": true, ".htm": true, ".xml": true, ".yaml": true, ".yml": true,
	".toml": true, ".ini": true, ".cfg": true, ".conf": true, ".log": true,
	".sh": true, ".bat": true, ".ps1": true, ".py": true, ".js": true,
	".ts": true, ".tsx": true, ".jsx": true, ".go": true, ".java": true,
	".c": true, ".cpp": true, ".h": true, ".hpp": true, ".cs": true,
	".rs": true, ".rb": true, ".php": true, ".sql": true, ".env": true,
	".dockerfile": true, ".makefile": true, ".properties": true, ".gradle": true,
	".swift": true, ".kt": true, ".scala": true, ".r": true, ".lua": true,
}

// imageExts 图片文件扩展名
var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
	".bmp": true, ".svg": true, ".ico": true,
}

// extractPreview 上传时预处理文件内容，返回 (previewText, previewType)。
// - 文本文件（<200KB）: previewType="text", previewText=文件内容
// - 超大文本文件: previewType="too_large", previewText=""
// - 图片文件: previewType="image", previewText=文件名+尺寸描述
// - 其他二进制: previewType="binary", previewText=""
func extractPreview(filename string, mimeType string, content []byte) (string, string) {
	ext := strings.ToLower(filepath.Ext(filepath.Base(filename)))

	// 1. 文本文件
	if previewableTextExts[ext] || strings.HasPrefix(mimeType, "text/") {
		if len(content) > previewTextMaxSize {
			return "", "too_large"
		}
		return string(content), "text"
	}

	// 2. 图片文件：生成描述信息供 Agent 理解图片用途
	if imageExts[ext] || strings.HasPrefix(mimeType, "image/") {
		// SVG 是文本格式，直接提取内容
		if ext == ".svg" && len(content) <= previewTextMaxSize {
			return string(content), "text"
		}
		desc := fmt.Sprintf("[图片: %s, %s, %s]", filename, formatFileSize(int64(len(content))), mimeType)
		return desc, "image"
	}

	// 3. PDF 等文档：标记为 binary（未来可扩展文本提取）
	return "", "binary"
}
