package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/agent-hub/backend/internal/model"
)

type fakeAgentRepo struct {
	updateResult *model.Agent
	deleted      bool
	registered   []string
	machines     []model.DaemonMachine
	machineAgent []string
	candidates   []string
	addedPrompt  string
}

func (r *fakeAgentRepo) ListAvailable(ctx context.Context, userID string) ([]model.Agent, error) {
	return nil, nil
}

func (r *fakeAgentRepo) GetByID(ctx context.Context, id string) (*model.Agent, error) {
	return nil, nil
}

func (r *fakeAgentRepo) GetDaemonTask(ctx context.Context, id string) (*model.DaemonTask, error) {
	return nil, nil
}

func (r *fakeAgentRepo) ClaimDaemonTask(ctx context.Context, machineID string) (*model.DaemonTask, error) {
	return nil, nil
}

func (r *fakeAgentRepo) CompleteDaemonTask(ctx context.Context, id, machineID, result, taskError string) (bool, error) {
	return true, nil
}

func (r *fakeAgentRepo) UpsertSystemAgent(ctx context.Context, name, cliTool, version, capabilitiesJSON string) error {
	r.registered = append(r.registered, cliTool)
	return nil
}

func (r *fakeAgentRepo) CreateDaemonMachine(ctx context.Context, userID, name, apiKeyHash string) (*model.DaemonMachine, error) {
	machine := model.DaemonMachine{ID: "machine-1", UserID: userID, Name: name, APIKeyHash: apiKeyHash, Status: "pending"}
	r.machines = append(r.machines, machine)
	return &machine, nil
}

func (r *fakeAgentRepo) ListDaemonMachines(ctx context.Context, userID string) ([]model.DaemonMachine, error) {
	return r.machines, nil
}

func (r *fakeAgentRepo) DeleteDaemonMachine(ctx context.Context, id, userID string) (bool, error) {
	return r.deleted, nil
}

func (r *fakeAgentRepo) GetDaemonMachineByAPIKeyHash(ctx context.Context, apiKeyHash string) (*model.DaemonMachine, error) {
	for i := range r.machines {
		if r.machines[i].APIKeyHash == apiKeyHash {
			return &r.machines[i], nil
		}
	}
	return nil, nil
}

func (r *fakeAgentRepo) MarkDaemonMachineConnected(ctx context.Context, id, machineID string) error {
	for i := range r.machines {
		if r.machines[i].ID == id {
			r.machines[i].MachineID = machineID
			r.machines[i].Status = "connected"
		}
	}
	return nil
}

func (r *fakeAgentRepo) UpsertMachineAgent(ctx context.Context, userID, machineID, machineName, name, cliTool, version, capabilitiesJSON string) error {
	r.machineAgent = append(r.machineAgent, machineID+":"+cliTool)
	return nil
}

func (r *fakeAgentRepo) UpsertMachineAgentCandidate(ctx context.Context, machineID, name, cliTool, version, capabilitiesJSON string) error {
	r.candidates = append(r.candidates, machineID+":"+cliTool)
	return nil
}

func (r *fakeAgentRepo) ListAgentCandidates(ctx context.Context, userID string) ([]model.AgentCandidate, error) {
	return nil, nil
}

func (r *fakeAgentRepo) AddCandidateAgent(ctx context.Context, userID, candidateID, displayName, systemPrompt string) (*model.Agent, error) {
	r.addedPrompt = systemPrompt
	return &model.Agent{ID: "agent-1", UserID: &userID, Name: displayName, CLITool: "codex", Type: "custom"}, nil
}

func (r *fakeAgentRepo) CreateCustom(ctx context.Context, userID, name, cliTool, systemPrompt, avatar, capabilitiesJSON string) (*model.Agent, error) {
	return &model.Agent{ID: "agent-1", UserID: &userID, Name: name, CLITool: cliTool, Type: "custom"}, nil
}

func (r *fakeAgentRepo) UpdateCustom(ctx context.Context, id, userID, name, cliTool, systemPrompt, avatar, capabilitiesJSON string) (*model.Agent, error) {
	return r.updateResult, nil
}

func (r *fakeAgentRepo) DeleteOwned(ctx context.Context, id, userID string) (bool, error) {
	return r.deleted, nil
}

func TestCreateCustomRejectsEmptyName(t *testing.T) {
	svc := NewAgentService(&fakeAgentRepo{})
	_, err := svc.CreateCustom(context.Background(), "user-1", "", "claude", "", "", "")
	if !errors.Is(err, ErrAgentInvalidInput) {
		t.Fatalf("expected ErrAgentInvalidInput, got %v", err)
	}
}

func TestRegisterSystemAgentsSkipsInvalidItems(t *testing.T) {
	repo := &fakeAgentRepo{}
	svc := NewAgentService(repo)
	err := svc.RegisterSystemAgents(context.Background(), []DiscoveredAgent{
		{Name: "Claude Code", CLITool: "claude", Capabilities: []string{"coding"}},
		{Name: "", CLITool: "codex"},
		{Name: "OpenCode", CLITool: ""},
	})
	if err != nil {
		t.Fatalf("register system agents failed: %v", err)
	}
	if len(repo.registered) != 1 || repo.registered[0] != "claude" {
		t.Fatalf("expected only claude registered, got %#v", repo.registered)
	}
}

func TestCreateDaemonMachineReturnsMachineKey(t *testing.T) {
	repo := &fakeAgentRepo{}
	svc := NewAgentService(repo)
	machine, apiKey, err := svc.CreateDaemonMachine(context.Background(), "user-1", "my-computer")
	if err != nil {
		t.Fatalf("create daemon machine failed: %v", err)
	}
	if machine == nil || machine.Name != "my-computer" {
		t.Fatalf("expected machine named my-computer, got %#v", machine)
	}
	if !strings.HasPrefix(apiKey, machineAPIKeyPrefix) {
		t.Fatalf("expected machine api key prefix, got %s", apiKey)
	}
	if repo.machines[0].APIKeyHash == "" || repo.machines[0].APIKeyHash == apiKey {
		t.Fatalf("expected hashed api key, got %#v", repo.machines[0].APIKeyHash)
	}
	found, err := svc.GetDaemonMachineByAPIKey(context.Background(), apiKey)
	if err != nil {
		t.Fatalf("get daemon machine by api key failed: %v", err)
	}
	if found == nil || found.ID != machine.ID {
		t.Fatalf("expected machine lookup by api key, got %#v", found)
	}
}

func TestRegisterMachineAgentsMarksMachineConnected(t *testing.T) {
	repo := &fakeAgentRepo{}
	svc := NewAgentService(repo)
	machine, _, err := svc.CreateDaemonMachine(context.Background(), "user-1", "remote-pc")
	if err != nil {
		t.Fatalf("create daemon machine failed: %v", err)
	}

	err = svc.RegisterMachineAgents(context.Background(), machine, "DESKTOP-1", []DiscoveredAgent{
		{Name: "Codex", CLITool: "codex", Capabilities: []string{"coding"}},
		{Name: "", CLITool: "broken"},
	})
	if err != nil {
		t.Fatalf("register machine agents failed: %v", err)
	}
	if len(repo.candidates) != 1 || repo.candidates[0] != "machine-1:codex" {
		t.Fatalf("expected codex saved as candidate, got %#v", repo.candidates)
	}
	if repo.machines[0].Status != "connected" || repo.machines[0].MachineID != "DESKTOP-1" {
		t.Fatalf("expected connected machine, got %#v", repo.machines[0])
	}
}

func TestUpdateCustomReturnsNotFound(t *testing.T) {
	svc := NewAgentService(&fakeAgentRepo{})
	_, err := svc.UpdateCustom(context.Background(), "agent-1", "user-1", "Agent", "claude", "", "", "")
	if !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("expected ErrAgentNotFound, got %v", err)
	}
}

func TestAddCandidateAgentStoresPrompt(t *testing.T) {
	repo := &fakeAgentRepo{}
	svc := NewAgentService(repo)
	_, err := svc.AddCandidateAgent(context.Background(), "user-1", "candidate-1", "My Agent", "persona")
	if err != nil {
		t.Fatalf("add candidate agent failed: %v", err)
	}
	if repo.addedPrompt != "persona" {
		t.Fatalf("expected system prompt stored, got %q", repo.addedPrompt)
	}
}

func TestDeleteCustomReturnsNotFound(t *testing.T) {
	svc := NewAgentService(&fakeAgentRepo{})
	err := svc.DeleteOwned(context.Background(), "agent-1", "user-1")
	if !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("expected ErrAgentNotFound, got %v", err)
	}
}
