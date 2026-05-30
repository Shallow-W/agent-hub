package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewUsesDefaultCandidates(t *testing.T) {
	s := New(nil)
	if len(s.candidates) != 3 {
		t.Fatalf("expected 3 default candidates, got %d", len(s.candidates))
	}
}

func TestParseSkillFileReadsFrontmatterAndContent(t *testing.T) {
	content := "---\nname: coding\ndescription: Write code safely\n---\n\n# Body\n"
	skill := parseSkillFile("fallback", "C:/tmp/SKILL.md", content)
	if skill.Name != "coding" {
		t.Fatalf("expected frontmatter name, got %q", skill.Name)
	}
	if skill.Description != "Write code safely" {
		t.Fatalf("expected description, got %q", skill.Description)
	}
	if skill.Detail != content {
		t.Fatalf("expected original content preserved")
	}
}

func TestReadSkillsReturnsSkillFiles(t *testing.T) {
	dir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)

	skillDir := filepath.Join(dir, ".agents", "skills", "coding")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	content := "---\nname: coding\n---\nbody"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	skills := New(nil).readSkills(Candidate{CLITool: "codex"})
	var found bool
	for _, skill := range skills {
		if skill.Name == "coding" && skill.Detail == content {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected project skill in %#v", skills)
	}
}

func TestScanSkipsMissingCommand(t *testing.T) {
	s := New([]Candidate{
		{
			Name:    "Missing",
			CLITool: "missing",
			Command: "agenthub-command-that-should-not-exist",
		},
	})

	agents, err := s.Scan(context.Background())
	if err != nil {
		t.Fatalf("scan should not fail for missing command: %v", err)
	}
	if len(agents) != 0 {
		t.Fatalf("expected no agents, got %d", len(agents))
	}
}
