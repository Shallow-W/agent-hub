package scanner

import (
	"context"
	"testing"
)

func TestNewUsesDefaultCandidates(t *testing.T) {
	s := New(nil)
	if len(s.candidates) != 3 {
		t.Fatalf("expected 3 default candidates, got %d", len(s.candidates))
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
