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
	ErrKBNotFound    = errors.New("知识库不存在")
	ErrKBNoPermission = errors.New("无权访问该知识库")
	ErrKBNameEmpty   = errors.New("知识库名称不能为空")
	ErrKBNotPublic   = errors.New("该知识库不是公开的")
	ErrKBFileEmpty   = errors.New("上传文件不能为空")
	ErrKBFileNotFound = errors.New("文件不存在")
)

// KnowledgeService 知识库业务逻辑
type KnowledgeService struct {
	kbRepo   *repository.KnowledgeRepo
	userRepo *repository.UserRepo
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

	_, err = s.kbRepo.AddFile(ctx, kbID, safeName, dbPath, fileHeader.Size, mimeType, hashHex)
	return err
}

// GetUploadDir 返回上传目录路径
func (s *KnowledgeService) GetUploadDir() string {
	return s.uploadDir
}

// ReadTextFileContent 读取知识库文件的文本内容（用于注入 Agent 上下文）。
// filePath 是数据库中存储的相对路径（如 "knowledge/{kb_id}/{hash}.ext"）。
func (s *KnowledgeService) ReadTextFileContent(_ context.Context, filePath string) (string, error) {
	absPath := filepath.Join(s.uploadDir, filepath.Clean(filePath))
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("read kb file: %w", err)
	}
	return string(data), nil
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
