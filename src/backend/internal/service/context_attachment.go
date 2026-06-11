package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/agent-hub/backend/internal/docextract"
	"github.com/agent-hub/backend/internal/model"
)

// AttachmentBuilder 把消息附件抽取为文本，前置到 current。
// 无附件时原样返回 current。
type AttachmentBuilder struct {
	UploadDir string
	MaxRunes  int
}

// Build 实现 ContextBuilder。无附件时返回 current 不变。
func (b *AttachmentBuilder) Build(ctx context.Context, in ContextInput, current string) string {
	if len(in.Attachments) == 0 {
		return current
	}
	uploadDir := b.UploadDir
	maxRunes := b.MaxRunes
	if maxRunes == 0 {
		maxRunes = attachmentTextMaxRunes
	}
	text := BuildAttachmentText(ctx, in.Attachments, uploadDir, maxRunes)
	if text == "" {
		return current
	}
	return text + current
}

// BuildAttachmentText 把附件抽取为「消息附件」段文本。
// 抽出来的逻辑供 AttachmentBuilder 与 OrchestratorService 的 façade 方法复用。
// 返回空串表示无附件。
func BuildAttachmentText(ctx context.Context, attachments []model.MessageAttachment, uploadDir string, maxRunes int) string {
	if len(attachments) == 0 {
		return ""
	}
	if maxRunes <= 0 {
		maxRunes = attachmentTextMaxRunes
	}
	var sb strings.Builder
	sb.WriteString("[消息附件]\n")
	sb.WriteString("用户在本条消息中附带了以下文件，已由系统抽取为文本内联在下方，请据此回答：\n\n")
	for _, a := range attachments {
		header := fmt.Sprintf("=== 附件：%s (%s, %s) ===\n", a.FileName, a.MimeType, formatFileSize(a.FileSize))
		sb.WriteString(header)
		absPath := filepath.Join(uploadDir, filepath.FromSlash(strings.TrimLeft(a.FilePath, "/\\")))
		if text, ok := docextract.Extract(ctx, absPath, a.FileName, maxRunes); ok {
			sb.WriteString(text)
			sb.WriteString("\n\n")
		} else {
			sb.WriteString("（该格式无法在服务端自动解析，请提示用户转换为 pptx/docx/xlsx/pdf 或纯文本后重发）\n\n")
		}
	}
	return sb.String()
}
