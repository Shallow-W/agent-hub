package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/agent-hub/backend/internal/model"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrPlatformSkillNotFound  = errors.New("平台 Skill 不存在")
	ErrPlatformSkillInvalid   = errors.New("平台 Skill 参数无效")
	ErrPlatformSkillDuplicate = errors.New("平台 Skill 名称已存在")
)

// PlatformSkillRepo 是 PlatformSkillService 依赖的仓库接口。
type PlatformSkillRepo interface {
	ListByUser(ctx context.Context, userID string) ([]model.PlatformSkill, error)
	Create(ctx context.Context, userID, name, description, trigger, detail string) (*model.PlatformSkill, error)
	Update(ctx context.Context, id, userID, name, description, trigger, detail string) (*model.PlatformSkill, error)
	Delete(ctx context.Context, id, userID string) (bool, error)
}

type PlatformSkillService struct {
	repo PlatformSkillRepo
}

func NewPlatformSkillService(repo PlatformSkillRepo) *PlatformSkillService {
	return &PlatformSkillService{repo: repo}
}

func (s *PlatformSkillService) List(ctx context.Context, userID string) ([]model.PlatformSkill, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, ErrPlatformSkillInvalid
	}
	list, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list platform skills: %w", err)
	}
	if list == nil {
		return []model.PlatformSkill{}, nil
	}
	return list, nil
}

func (s *PlatformSkillService) Create(ctx context.Context, userID, name, description, trigger, detail string) (*model.PlatformSkill, error) {
	name, description, trigger, detail, err := normalizePlatformSkillFields(userID, name, description, trigger, detail)
	if err != nil {
		return nil, err
	}
	skill, err := s.repo.Create(ctx, userID, name, description, trigger, detail)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrPlatformSkillDuplicate
		}
		return nil, fmt.Errorf("create platform skill: %w", err)
	}
	return skill, nil
}

func (s *PlatformSkillService) Update(ctx context.Context, id, userID, name, description, trigger, detail string) (*model.PlatformSkill, error) {
	if strings.TrimSpace(id) == "" {
		return nil, ErrPlatformSkillInvalid
	}
	name, description, trigger, detail, err := normalizePlatformSkillFields(userID, name, description, trigger, detail)
	if err != nil {
		return nil, err
	}
	skill, err := s.repo.Update(ctx, id, userID, name, description, trigger, detail)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrPlatformSkillDuplicate
		}
		return nil, fmt.Errorf("update platform skill: %w", err)
	}
	if skill == nil {
		return nil, ErrPlatformSkillNotFound
	}
	return skill, nil
}

func (s *PlatformSkillService) Delete(ctx context.Context, id, userID string) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(userID) == "" {
		return ErrPlatformSkillInvalid
	}
	deleted, err := s.repo.Delete(ctx, id, userID)
	if err != nil {
		return fmt.Errorf("delete platform skill: %w", err)
	}
	if !deleted {
		return ErrPlatformSkillNotFound
	}
	return nil
}

func normalizePlatformSkillFields(userID, name, description, trigger, detail string) (string, string, string, string, error) {
	name = strings.TrimSpace(name)
	if strings.TrimSpace(userID) == "" || name == "" {
		return "", "", "", "", ErrPlatformSkillInvalid
	}
	return truncateString(name, 80),
		truncateString(strings.TrimSpace(description), 200),
		truncateString(strings.TrimSpace(trigger), 200),
		truncateString(strings.TrimSpace(detail), 2000),
		nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
