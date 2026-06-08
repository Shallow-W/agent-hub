package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestMigrationsAreIdempotentForRepeatedStartup(t *testing.T) {
	files, err := filepath.Glob("../../migrations/*.sql")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected migration files")
	}

	createIndex := regexp.MustCompile(`(?i)^\s*CREATE\s+(UNIQUE\s+)?INDEX\s+`)
	createIndexIfNotExists := regexp.MustCompile(`(?i)^\s*CREATE\s+(UNIQUE\s+)?INDEX\s+IF\s+NOT\s+EXISTS\s+`)
	addColumn := regexp.MustCompile(`(?i)^\s*ALTER\s+TABLE\s+\S+\s+ADD\s+COLUMN\s+`)
	addColumnIfNotExists := regexp.MustCompile(`(?i)^\s*ALTER\s+TABLE\s+\S+\s+ADD\s+COLUMN\s+IF\s+NOT\s+EXISTS\s+`)
	addConstraint := regexp.MustCompile(`(?i)^\s*ALTER\s+TABLE\s+\S+\s+ADD\s+CONSTRAINT\s+(\S+)`)

	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", filepath.Base(path), err)
		}
		upSQL := string(content)
		if idx := strings.Index(upSQL, "---- DOWN"); idx != -1 {
			upSQL = upSQL[:idx]
		}
		lines := strings.Split(upSQL, "\n")
		for lineNo, line := range lines {
			if createIndex.MatchString(line) && !createIndexIfNotExists.MatchString(line) {
				t.Fatalf("%s:%d uses non-idempotent CREATE INDEX: %s", filepath.Base(path), lineNo+1, strings.TrimSpace(line))
			}
			if addColumn.MatchString(line) && !addColumnIfNotExists.MatchString(line) {
				t.Fatalf("%s:%d uses non-idempotent ADD COLUMN: %s", filepath.Base(path), lineNo+1, strings.TrimSpace(line))
			}
			if matches := addConstraint.FindStringSubmatch(line); matches != nil {
				constraintName := matches[1]
				previousLine := ""
				if lineNo > 0 {
					previousLine = strings.TrimSpace(lines[lineNo-1])
				}
				wantDrop := regexp.MustCompile(`(?i)^ALTER\s+TABLE\s+\S+\s+DROP\s+CONSTRAINT\s+IF\s+EXISTS\s+` + regexp.QuoteMeta(constraintName) + `\s*;?$`)
				if !wantDrop.MatchString(previousLine) {
					t.Fatalf("%s:%d adds constraint %s without prior DROP CONSTRAINT IF EXISTS", filepath.Base(path), lineNo+1, constraintName)
				}
			}
		}
	}
}
