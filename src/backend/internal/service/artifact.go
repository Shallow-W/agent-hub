package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/repository"
)

// ArtifactRepoForSvc 产物服务依赖的数据访问能力。
// Deprecated: migrate to repository.ArtifactStore for canonical interface.
type ArtifactRepoForSvc interface {
	ListVersions(ctx context.Context, rootID string) ([]model.Artifact, error)
	CreateVersion(ctx context.Context, rootID string, in model.Artifact) (*model.Artifact, error)
	GetConversationIDByRoot(ctx context.Context, rootID string) (string, error)
}

// ArtifactConvRepo 产物服务用于鉴权的对话仓库能力。
// Deprecated: migrate to repository.ConvStore for canonical interface.
type ArtifactConvRepo interface {
	GetByID(ctx context.Context, id string) (*model.Conversation, error)
	GetMember(ctx context.Context, conversationID, userID string) (*model.ConversationMember, error)
}

var (
	ErrArtifactNotFound = errors.New("产物不存在")
	ErrArtifactNoPerm   = errors.New("无权访问此产物")
	ErrArtifactInvalid  = errors.New("产物参数不合法")
)

// 允许的产物类型，与 daemon 解析及前端契约一致。
var validArtifactTypes = map[string]bool{
	"code":     true,
	"webpage":  true,
	"document": true,
	"file":     true,
}

// ArtifactService 处理产物版本业务逻辑。
type ArtifactService struct {
	repo     ArtifactRepoForSvc
	convRepo ArtifactConvRepo
}

// NewArtifactService 创建产物服务。
func NewArtifactService(repo ArtifactRepoForSvc, convRepo ArtifactConvRepo) *ArtifactService {
	return &ArtifactService{repo: repo, convRepo: convRepo}
}

// ListVersions 列出某血缘根的全部版本（按 version 升序），先校验访问权限。
func (s *ArtifactService) ListVersions(ctx context.Context, rootID, userID string) ([]model.Artifact, error) {
	if err := s.checkAccess(ctx, rootID, userID); err != nil {
		return nil, err
	}
	versions, err := s.repo.ListVersions(ctx, rootID)
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	if versions == nil {
		versions = []model.Artifact{}
	}
	return versions, nil
}

// CreateVersion 为某血缘根创建新版本，先校验访问权限。
func (s *ArtifactService) CreateVersion(ctx context.Context, rootID, userID string, in model.Artifact) (*model.Artifact, error) {
	if in.Content == "" && in.URL == "" {
		return nil, ErrArtifactInvalid
	}
	if in.Type != "" && !validArtifactTypes[in.Type] {
		return nil, ErrArtifactInvalid
	}
	if err := s.checkAccess(ctx, rootID, userID); err != nil {
		return nil, err
	}
	created, err := s.repo.CreateVersion(ctx, rootID, in)
	if err != nil {
		if errors.Is(err, repository.ErrArtifactRootNotFound) {
			return nil, ErrArtifactNotFound
		}
		return nil, fmt.Errorf("create version: %w", err)
	}
	return created, nil
}

// checkAccess 校验 rootId 对应产物所属对话，且当前用户为成员（或对话创建者）。
func (s *ArtifactService) checkAccess(ctx context.Context, rootID, userID string) error {
	convID, err := s.repo.GetConversationIDByRoot(ctx, rootID)
	if err != nil {
		if errors.Is(err, repository.ErrArtifactRootNotFound) {
			return ErrArtifactNotFound
		}
		return fmt.Errorf("resolve artifact conversation: %w", err)
	}

	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return ErrArtifactNotFound
	}

	member, err := s.convRepo.GetMember(ctx, convID, userID)
	if err != nil {
		return fmt.Errorf("check member: %w", err)
	}
	if member != nil {
		return nil
	}
	// 兜底：创建者可能尚未写入成员表
	if conv.UserID == userID {
		return nil
	}
	return ErrArtifactNoPerm
}
