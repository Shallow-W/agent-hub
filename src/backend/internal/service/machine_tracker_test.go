package service

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

type fakeMachineStatusRepo struct {
	online  []string
	offline []string
}

func (r *fakeMachineStatusRepo) SetMachineAndAgentsOnline(_ context.Context, machineID string) error {
	r.online = append(r.online, machineID)
	return nil
}

func (r *fakeMachineStatusRepo) SetMachineAndAgentsOffline(_ context.Context, machineID string) error {
	r.offline = append(r.offline, machineID)
	return nil
}

func newTestMachineTracker(repo *fakeMachineStatusRepo) *MachineTracker {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewMachineTracker(repo, logger)
}

func TestMachineOfflineThresholdExceedsPingCadence(t *testing.T) {
	if machineOfflineThreshold <= 30*time.Second {
		t.Fatalf("offline threshold %s must exceed 30s ping cadence", machineOfflineThreshold)
	}
}

func TestMachineTrackerDoesNotSweepBeforeFirstPingWindow(t *testing.T) {
	repo := &fakeMachineStatusRepo{}
	tracker := newTestMachineTracker(repo)

	tracker.MarkOnline("machine-1")

	tracker.mu.Lock()
	tracker.machines["machine-1"].lastHB = time.Now().Add(-20 * time.Second)
	tracker.mu.Unlock()

	tracker.sweep()

	if !tracker.IsOnline("machine-1") {
		t.Fatal("machine should remain online before the first 30s ping window")
	}
	if len(repo.offline) != 0 {
		t.Fatalf("expected no offline writes, got %v", repo.offline)
	}
}

func TestMachineTrackerSweepsAfterOfflineThreshold(t *testing.T) {
	repo := &fakeMachineStatusRepo{}
	tracker := newTestMachineTracker(repo)

	tracker.MarkOnline("machine-1")

	tracker.mu.Lock()
	tracker.machines["machine-1"].lastHB = time.Now().Add(-(machineOfflineThreshold + time.Second))
	tracker.mu.Unlock()

	tracker.sweep()

	if tracker.IsOnline("machine-1") {
		t.Fatal("machine should be offline after threshold expires")
	}
	if len(repo.offline) != 1 || repo.offline[0] != "machine-1" {
		t.Fatalf("expected one offline write for machine-1, got %v", repo.offline)
	}
}
