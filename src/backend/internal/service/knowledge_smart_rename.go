package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/agent-hub/backend/internal/model"
)

const (
	knowledgeSmartRenameMaxContentRunes = 12000
	knowledgeSmartRenameMaxNameRunes    = 120
)

// KnowledgeFilenameSuggester 只负责通过智能体生成候选文件名，权限和落库仍由 KnowledgeService 控制。
type KnowledgeFilenameSuggester interface {
	SuggestKnowledgeFilename(ctx context.Context, userID string, file model.KnowledgeFile, text string) (string, error)
}

// SetFilenameSuggester 注入智能体命名能力，便于知识库服务保持独立可测。
func (s *KnowledgeService) SetFilenameSuggester(suggester KnowledgeFilenameSuggester) {
	s.filenameSuggester = suggester
}

// SmartRenameFile 让在线智能体扫读当前文件后生成明确文件名，并写回知识库文件记录。
func (s *KnowledgeService) SmartRenameFile(ctx context.Context, userID, kbID, fileID string) (*model.KnowledgeFile, error) {
	kb, err := s.kbRepo.GetByID(ctx, kbID)
	if err != nil {
		return nil, err
	}
	if kb == nil {
		return nil, ErrKBNotFound
	}
	if kb.UserID != userID {
		return nil, ErrKBNoPermission
	}

	file, err := s.kbRepo.GetFileByID(ctx, kbID, fileID)
	if err != nil {
		return nil, err
	}
	if file == nil {
		return nil, ErrKBFileNotFound
	}
	s.ensureFilePreview(ctx, file)

	if s.filenameSuggester == nil {
		return nil, ErrKBRenameNoAgent
	}
	rawName, err := s.filenameSuggester.SuggestKnowledgeFilename(
		ctx,
		userID,
		*file,
		buildSmartRenameFileText(*file),
	)
	if err != nil {
		if errors.Is(err, ErrAgentOffline) || errors.Is(err, ErrAgentNotFound) {
			return nil, ErrKBRenameNoAgent
		}
		return nil, err
	}

	filename, err := normalizeSmartRenameFilename(rawName, file.Filename)
	if err != nil {
		return nil, fmt.Errorf("normalize smart rename filename: %w", err)
	}
	if filename == file.Filename {
		s.enrichKnowledgeFile(file)
		return file, nil
	}

	updated, err := s.kbRepo.UpdateFileName(ctx, kbID, fileID, filename)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, ErrKBFileNotFound
	}
	s.enrichKnowledgeFile(updated)
	return updated, nil
}

func buildSmartRenameFileText(file model.KnowledgeFile) string {
	if file.PreviewType != "text" && file.PreviewType != "image" {
		return ""
	}
	return truncateString(strings.TrimSpace(file.PreviewText), knowledgeSmartRenameMaxContentRunes)
}

func normalizeSmartRenameFilename(raw, original string) (string, error) {
	name := extractFilenameCandidate(raw)
	name = stripFilenameDecorations(name)
	name = sanitizeFilenameChars(name)
	name = strings.Trim(name, " .")
	if name == "" {
		return "", errors.New("empty smart rename filename")
	}

	originalExt := strings.TrimSpace(filepath.Ext(original))
	if originalExt != "" {
		currentExt := filepath.Ext(name)
		if currentExt != "" {
			name = strings.TrimSuffix(name, currentExt)
		}
		name = strings.Trim(name, " .") + originalExt
	}

	name = truncateFilenameRunes(name, originalExt, knowledgeSmartRenameMaxNameRunes)
	name = strings.Trim(name, " .")
	if name == "" || name == originalExt {
		return "", errors.New("empty smart rename filename")
	}
	return name, nil
}

func extractFilenameCandidate(raw string) string {
	text := trimSmartRenameFence(raw)

	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err == nil {
		for _, key := range []string{"filename", "file_name", "name", "title"} {
			if value, ok := payload[key].(string); ok {
				if value = strings.TrimSpace(value); value != "" {
					return value
				}
			}
		}
	}

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, prefix := range []string{"文件名：", "新文件名：", "建议文件名：", "filename:", "file name:", "name:"} {
			if strings.HasPrefix(strings.ToLower(line), strings.ToLower(prefix)) {
				line = strings.TrimSpace(line[len(prefix):])
				break
			}
		}
		return line
	}
	return text
}

func trimSmartRenameFence(raw string) string {
	text := strings.TrimSpace(raw)
	if !strings.HasPrefix(text, "```") {
		return strings.Trim(text, "` \t\r\n")
	}
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSpace(strings.TrimSuffix(text, "```"))
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		firstLine := strings.TrimSpace(text[:idx])
		if isMarkdownFenceLanguage(firstLine) {
			text = text[idx+1:]
		}
	}
	return strings.TrimSpace(text)
}

func isMarkdownFenceLanguage(value string) bool {
	if value == "" || len([]rune(value)) > 24 {
		return false
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func stripFilenameDecorations(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Trim(name, "`*_ \t\r\n")
	name = strings.Trim(name, "\"'“”‘’")
	name = strings.TrimSpace(name)
	name = strings.Trim(name, "`*_")
	return strings.TrimSpace(name)
}

func sanitizeFilenameChars(name string) string {
	var b strings.Builder
	lastWasSpace := false
	lastWasUnderscore := false
	for _, r := range name {
		if unicode.IsControl(r) {
			continue
		}
		if strings.ContainsRune(`<>:"/\|?*`, r) {
			if !lastWasUnderscore {
				b.WriteRune('_')
				lastWasUnderscore = true
			}
			lastWasSpace = false
			continue
		}
		if unicode.IsSpace(r) {
			if !lastWasSpace {
				b.WriteRune(' ')
				lastWasSpace = true
			}
			lastWasUnderscore = false
			continue
		}
		b.WriteRune(r)
		lastWasSpace = false
		lastWasUnderscore = false
	}
	return b.String()
}

func truncateFilenameRunes(name, ext string, maxRunes int) string {
	runes := []rune(name)
	if len(runes) <= maxRunes {
		return name
	}
	if ext == "" {
		return string(runes[:maxRunes])
	}
	base := strings.TrimSuffix(name, ext)
	extRunes := []rune(ext)
	baseLimit := maxRunes - len(extRunes)
	if baseLimit <= 0 {
		return string(runes[:maxRunes])
	}
	baseRunes := []rune(base)
	if len(baseRunes) > baseLimit {
		base = string(baseRunes[:baseLimit])
	}
	return strings.Trim(base, " .") + ext
}
