package scanner

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestNewUsesDefaultCandidates(t *testing.T) {
	s := New(nil)
	if len(s.candidates) != 4 {
		t.Fatalf("expected 4 default candidates, got %d", len(s.candidates))
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

func TestParseSkillFileInfersShortDescription(t *testing.T) {
	content := "---\nname: access\ndescription: ok\n---\n\n# access\nUse when an agent needs to inspect permission boundaries and explain safe access steps.\n"
	skill := parseSkillFile("fallback", "C:/tmp/SKILL.md", content)
	if !strings.Contains(skill.Description, "permission boundaries") {
		t.Fatalf("expected inferred description from body, got %q", skill.Description)
	}
}

func TestParseSkillFileKeepsUsefulDescription(t *testing.T) {
	content := "---\nname: coding\ndescription: Write code safely with tests and concise implementation notes.\n---\n\n# Body\n"
	skill := parseSkillFile("fallback", "C:/tmp/SKILL.md", content)
	if skill.Description != "Write code safely with tests and concise implementation notes." {
		t.Fatalf("expected original description, got %q", skill.Description)
	}
}

func TestParseSkillFileInfersDescriptionWithoutFrontmatter(t *testing.T) {
	content := "# deploy\nPrepare release commands, verify generated artifacts, and summarize deployment risks.\n"
	skill := parseSkillFile("deploy", "C:/tmp/SKILL.md", content)
	if !strings.Contains(skill.Description, "release commands") {
		t.Fatalf("expected inferred description without frontmatter, got %q", skill.Description)
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

func TestReadSkillsSkipsAgentHubWorkspaceSkills(t *testing.T) {
	dir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)

	if err := os.MkdirAll(filepath.Join(dir, "src", "daemon-npm"), 0o755); err != nil {
		t.Fatalf("mkdir daemon package: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src", "frontend"), 0o755); err != nil {
		t.Fatalf("mkdir frontend package: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "daemon-npm", "package.json"), []byte(`{"name":"@agenthub/daemon"}`), 0o644); err != nil {
		t.Fatalf("write package: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "frontend", "package.json"), []byte(`{"name":"frontend"}`), 0o644); err != nil {
		t.Fatalf("write frontend package: %v", err)
	}

	skillDir := filepath.Join(dir, ".agents", "skills", "trellis")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: trellis\n---\nbody"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	skills := New(nil).readSkills(Candidate{CLITool: "codex"})
	for _, skill := range skills {
		if strings.Contains(skill.SourcePath, dir) {
			t.Fatalf("expected AgentHub workspace skill to be skipped, got %#v", skills)
		}
	}
}

func TestReadSkillsFindsClaudePluginMarketplaceSkills(t *testing.T) {
	dir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	skillDir := filepath.Join(dir, ".claude", "plugins", "marketplaces", "official", "plugins", "frontend-design", "skills", "frontend-design")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	content := "---\nname: frontend-design\n---\nbody"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	skills := New(nil).readSkills(Candidate{CLITool: "claude"})
	for _, skill := range skills {
		if skill.Name == "frontend-design" && skill.Detail == content {
			return
		}
	}
	t.Fatalf("expected Claude plugin marketplace skill in %#v", skills)
}

func TestOpenClawInstallSkillRootsUsesInstallRecords(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "npm", "node_modules", "@openclaw", "qqbot")
	configDir := filepath.Join(dir, ".openclaw", "plugins")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	installs := `{"installRecords":{"qqbot":{"installPath":` + strconv.Quote(pluginDir) + `}}}`
	if err := os.WriteFile(filepath.Join(configDir, "installs.json"), []byte(installs), 0o644); err != nil {
		t.Fatalf("write installs: %v", err)
	}

	roots := openClawInstallSkillRoots(dir)
	expected := filepath.Join(pluginDir, "skills")
	for _, root := range roots {
		if root == expected {
			return
		}
	}
	t.Fatalf("expected %q in %#v", expected, roots)
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
