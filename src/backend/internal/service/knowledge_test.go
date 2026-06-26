package service

import (
	"archive/zip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-hub/backend/internal/docextract"
	"github.com/agent-hub/backend/internal/model"
)

func TestParseKnowledgeRefs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []KnowledgeRef
	}{
		{
			name:     "无引用",
			input:    "这是一条普通消息",
			expected: nil,
		},
		{
			name:  "单个引用",
			input: "请使用 {{alice/项目文档}} 来帮助完成任务",
			expected: []KnowledgeRef{
				{Username: "alice", KBName: "项目文档", Raw: "{{alice/项目文档}}"},
			},
		},
		{
			name:  "多个引用",
			input: "参考 {{bob/API文档}} 和 {{alice/设计稿}} 来完成",
			expected: []KnowledgeRef{
				{Username: "bob", KBName: "API文档"},
				{Username: "alice", KBName: "设计稿"},
			},
		},
		{
			name:  "去重",
			input: "{{alice/共享}} 和 {{alice/共享}}",
			expected: []KnowledgeRef{
				{Username: "alice", KBName: "共享"},
			},
		},
		{
			name:     "空括号不匹配",
			input:    "{{}} 和 {{ / }} 不应匹配",
			expected: nil,
		},
		{
			name:  "带@mention混合",
			input: "@Agent1 请查阅 {{alice/技术文档}} 后回答",
			expected: []KnowledgeRef{
				{Username: "alice", KBName: "技术文档"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseKnowledgeRefs(tt.input)
			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d refs, got %d", len(tt.expected), len(result))
			}
			for i, exp := range tt.expected {
				if result[i].Username != exp.Username {
					t.Errorf("ref[%d].Username = %q, want %q", i, result[i].Username, exp.Username)
				}
				if result[i].KBName != exp.KBName {
					t.Errorf("ref[%d].KBName = %q, want %q", i, result[i].KBName, exp.KBName)
				}
			}
		})
	}
}

func TestExtractKnowledgePreviewUsesDocExtractForTextAndOOXML(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name     string
		filename string
		content  []byte
		want     string
	}{
		{
			name:     "markdown",
			filename: "guide.md",
			content:  []byte("# AgentHub\n\n知识库可以帮助 Agent。"),
			want:     "知识库可以帮助 Agent",
		},
		{
			name:     "json",
			filename: "config.json",
			content:  []byte(`{"goal":"让群聊 agent 使用知识库"}`),
			want:     "群聊 agent",
		},
		{
			name:     "docx",
			filename: "notes.docx",
			content:  minimalOOXML(t, "word/document.xml", "Word 文档知识"),
			want:     "Word 文档知识",
		},
		{
			name:     "pptx",
			filename: "slides.pptx",
			content:  minimalOOXML(t, "ppt/slides/slide1.xml", "PPT 幻灯片知识"),
			want:     "PPT 幻灯片知识",
		},
		{
			name:     "xlsx",
			filename: "sheet.xlsx",
			content:  minimalOOXML(t, "xl/sharedStrings.xml", "Excel 表格知识"),
			want:     "Excel 表格知识",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, tt.filename)
			if err := os.WriteFile(path, tt.content, 0o644); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			previewText, previewType := extractKnowledgePreview(context.Background(), path, tt.filename, "application/octet-stream", int64(len(tt.content)))

			if previewType != "text" {
				t.Fatalf("previewType = %q, want text", previewType)
			}
			if !strings.Contains(previewText, tt.want) {
				t.Fatalf("previewText missing %q: %q", tt.want, previewText)
			}
		})
	}
}

func TestExtractKnowledgePreviewPDFUsesSofficeWhenAvailable(t *testing.T) {
	if !docextract.SofficeAvailable() {
		t.Skip("LibreOffice not available")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "note.pdf")
	if err := os.WriteFile(path, minimalTextPDF("Knowledge PDF Preview"), 0o644); err != nil {
		t.Fatalf("write pdf: %v", err)
	}

	previewText, previewType := extractKnowledgePreview(context.Background(), path, "note.pdf", "application/pdf", 128)

	if previewType != "text" {
		t.Fatalf("previewType = %q, want text", previewType)
	}
	if !strings.Contains(previewText, "Knowledge PDF Preview") {
		t.Fatalf("previewText missing PDF content: %q", previewText)
	}
}

func minimalTextPDF(text string) []byte {
	escaped := strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`).Replace(text)
	content := fmt.Sprintf("BT /F1 24 Tf 72 720 Td (%s) Tj ET", escaped)
	objects := []string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>\nendobj\n",
		"4 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n",
		fmt.Sprintf("5 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(content), content),
	}

	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, 0, len(objects))
	for _, object := range objects {
		offsets = append(offsets, b.Len())
		b.WriteString(object)
	}

	xrefOffset := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n", len(objects)+1)
	b.WriteString("0000000000 65535 f \n")
	for _, offset := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xrefOffset)
	return []byte(b.String())
}

func TestPreloadKBContextIncludesFileIDsAndInlineDocumentText(t *testing.T) {
	resolver := fakeKBResolver{
		kb: &model.KnowledgeBase{ID: "kb-1", Visibility: "public"},
		files: []model.KnowledgeFile{
			{
				ID:              "file-1",
				KnowledgeBaseID: "kb-1",
				Filename:        "guide.docx",
				FileSize:        2048,
				PreviewType:     "text",
				PreviewText:     "这里是已经抽取的 Office 文档内容",
			},
		},
	}
	// 直接调 KBBuilder.resolveKB（原 OrchestratorService.PreloadKBContext 的 façade 已删除，
	// façade 委托的就是这个纯函数；与 chain 内 KBBuilder 共享同一实现）
	b := &KBBuilder{KBResolver: resolver}
	got := b.resolveKB(context.Background(), "请参考 {{alice/项目知识}}", "u1")

	if !strings.Contains(got, "[知识库: alice/项目知识") {
		t.Fatalf("missing knowledge section: %s", got)
	}
	if !strings.Contains(got, "file_id=file-1") {
		t.Fatalf("missing file id for read_knowledge_file: %s", got)
	}
	if !strings.Contains(got, "Office 文档内容") {
		t.Fatalf("missing inline extracted text: %s", got)
	}
}

func TestBuildSummaryPromptIncludesKBPreload(t *testing.T) {
	prompt := BuildSummaryPrompt(&model.OrchTask{
		OriginalMessage: "请汇总",
		WorkerResults:   `{"Codex":"完成"}`,
		KBPreload:       "[引用的知识库]\n- guide.docx: 文档内容\n",
	})

	if !strings.Contains(prompt, "[引用的知识库]") {
		t.Fatalf("summary prompt missing KB preload: %s", prompt)
	}
	if !strings.Contains(prompt, "guide.docx") {
		t.Fatalf("summary prompt missing KB file context: %s", prompt)
	}
}

type fakeKBResolver struct {
	kb    *model.KnowledgeBase
	files []model.KnowledgeFile
	err   error
}

func (f fakeKBResolver) ResolveKnowledgeRef(context.Context, string, string, string) (*model.KnowledgeBase, []model.KnowledgeFile, error) {
	return f.kb, f.files, f.err
}

func minimalOOXML(t *testing.T, name, text string) []byte {
	t.Helper()
	tmp := filepath.Join(t.TempDir(), "doc.zip")
	file, err := os.Create(tmp)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	zw := zip.NewWriter(file)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := w.Write([]byte(`<root><w:t>` + text + `</w:t><a:t>` + text + `</a:t><t>` + text + `</t></root>`)); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}
	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}
	return data
}
