package service

import "testing"

func TestNormalizeSmartRenameFilename(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		original string
		want     string
	}{
		{
			name:     "json fence without extension",
			raw:      "```json\n{\"filename\":\"AgentHub 架构说明\"}\n```",
			original: "untitled.pdf",
			want:     "AgentHub 架构说明.pdf",
		},
		{
			name:     "label and path separators",
			raw:      "新文件名：2026/06 知识库更新需求.txt",
			original: "old.md",
			want:     "2026_06 知识库更新需求.md",
		},
		{
			name:     "text fence without json",
			raw:      "```text\n项目验收清单\n```",
			original: "checklist.xlsx",
			want:     "项目验收清单.xlsx",
		},
		{
			name:     "wrong extension is replaced",
			raw:      "客户调研纪要.docx",
			original: "random-name.pdf",
			want:     "客户调研纪要.pdf",
		},
		{
			name:     "quotes and markdown emphasis are stripped",
			raw:      "**\"多智能体协作方案\"**",
			original: "draft",
			want:     "多智能体协作方案",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeSmartRenameFilename(tt.raw, tt.original)
			if err != nil {
				t.Fatalf("normalizeSmartRenameFilename() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("normalizeSmartRenameFilename() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeSmartRenameFilenameRejectsEmpty(t *testing.T) {
	if _, err := normalizeSmartRenameFilename("``` \n \n```", "old.txt"); err == nil {
		t.Fatal("expected empty filename error")
	}
}
