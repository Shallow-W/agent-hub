package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-hub/backend/internal/model"
)

type fakeAgentRepo struct {
	updateResult *model.Agent
	currentAgent *model.Agent
	daemonTask   *model.DaemonTask
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
	return r.currentAgent, nil
}

func (r *fakeAgentRepo) GetDaemonTask(ctx context.Context, id string) (*model.DaemonTask, error) {
	if r.daemonTask != nil {
		return r.daemonTask, nil
	}
	return nil, nil
}

func (r *fakeAgentRepo) CreateDaemonTask(ctx context.Context, userID, conversationID, agentID, machineID, cliTool, prompt, contextMessages string) (*model.DaemonTask, error) {
	r.daemonTask = &model.DaemonTask{
		ID:        "task-1",
		UserID:    userID,
		AgentID:   agentID,
		MachineID: machineID,
		CLITool:   cliTool,
		Prompt:    prompt,
		Status:    "completed",
		Result:    "ok",
	}
	return r.daemonTask, nil
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

func (r *fakeAgentRepo) CreateCustom(ctx context.Context, userID, name, cliTool, systemPrompt, toolsConfig, avatar, capabilitiesJSON string, enableManagementTools bool) (*model.Agent, error) {
	return &model.Agent{ID: "agent-1", UserID: &userID, Name: name, CLITool: cliTool, Type: "custom"}, nil
}

func (r *fakeAgentRepo) UpdateCustom(ctx context.Context, id, userID, name, cliTool, systemPrompt, toolsConfig, avatar, capabilitiesJSON string, enableManagementTools bool) (*model.Agent, error) {
	return r.updateResult, nil
}

func (r *fakeAgentRepo) DeleteOwned(ctx context.Context, id, userID string) (bool, error) {
	return r.deleted, nil
}

func (r *fakeAgentRepo) CreateDaemonTask(ctx context.Context, userID, conversationID, agentID, machineID, cliTool, prompt, contextMessages string) (*model.DaemonTask, error) {
	return &model.DaemonTask{ID: "task-1", AgentID: agentID, MachineID: machineID, Status: "pending"}, nil
}

func (r *fakeAgentRepo) GetDaemonMachineByID(ctx context.Context, id string) (*model.DaemonMachine, error) {
	for i := range r.machines {
		if r.machines[i].ID == id {
			return &r.machines[i], nil
		}
	}
	return nil, nil
}

func (r *fakeAgentRepo) UpdateAgentStatus(ctx context.Context, id, status string) error {
	return nil
}

func (r *fakeAgentRepo) ClearAgentMachine(ctx context.Context, id string) error {
	return nil
}

func (r *fakeAgentRepo) UpdateMachineAPIKey(ctx context.Context, id, apiKeyHash string) error {
	return nil
}

func TestCreateCustomRejectsEmptyName(t *testing.T) {
	svc := NewAgentService(&fakeAgentRepo{}, nil)
	_, err := svc.CreateCustom(context.Background(), "user-1", "", "claude", "", "", "", "", false)
	if !errors.Is(err, ErrAgentInvalidInput) {
		t.Fatalf("expected ErrAgentInvalidInput, got %v", err)
	}
}

func TestRegisterSystemAgentsSkipsInvalidItems(t *testing.T) {
	repo := &fakeAgentRepo{}
	svc := NewAgentService(repo, nil)
	err := svc.RegisterSystemAgents(context.Background(), []DiscoveredAgent{
		{Name: "Claude Code", CLITool: "claude", Capabilities: []DiscoveredSkill{{Name: "coding"}}},
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
	svc := NewAgentService(repo, nil)
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
	svc := NewAgentService(repo, nil)
	machine, _, err := svc.CreateDaemonMachine(context.Background(), "user-1", "remote-pc")
	if err != nil {
		t.Fatalf("create daemon machine failed: %v", err)
	}

	err = svc.RegisterMachineAgents(context.Background(), machine, "DESKTOP-1", []DiscoveredAgent{
		{Name: "Codex", CLITool: "codex", Capabilities: []DiscoveredSkill{{Name: "coding"}}},
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

func TestDiscoveredSkillAcceptsLegacyString(t *testing.T) {
	var skill DiscoveredSkill
	if err := skill.UnmarshalJSON([]byte(`"coding"`)); err != nil {
		t.Fatalf("unmarshal legacy skill: %v", err)
	}
	if skill.Name != "coding" || !skill.Auto {
		t.Fatalf("unexpected skill: %#v", skill)
	}
}

func TestSyncSkillFilesWritesExistingSkillMD(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	payload := `[{"name":"coding","detail":"new content","source_path":` + strconvQuote(path) + `}]`
	if err := syncSkillFiles(payload); err != nil {
		t.Fatalf("sync skill file: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read skill: %v", err)
	}
	if string(content) != "new content" {
		t.Fatalf("expected synced content, got %q", string(content))
	}
}

func TestSyncSkillFilesRejectsAgentHubWorkspaceSkill(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src", "daemon-npm"), 0o755); err != nil {
		t.Fatalf("mkdir daemon package: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src", "frontend"), 0o755); err != nil {
		t.Fatalf("mkdir frontend package: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "daemon-npm", "package.json"), []byte(`{"name":"@agenthub/daemon"}`), 0o644); err != nil {
		t.Fatalf("write daemon package: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "frontend", "package.json"), []byte(`{"name":"frontend"}`), 0o644); err != nil {
		t.Fatalf("write frontend package: %v", err)
	}
	skillDir := filepath.Join(dir, ".agents", "skills", "trellis")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	payload := `[{"name":"trellis","detail":"new content","source_path":` + strconvQuote(skillPath) + `}]`
	if err := syncSkillFiles(payload); err != ErrAgentInvalidInput {
		t.Fatalf("expected invalid input for AgentHub workspace skill, got %v", err)
	}
	content, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("read skill: %v", err)
	}
	if string(content) != "old" {
		t.Fatalf("expected stale workspace skill to remain unchanged, got %q", string(content))
	}
}

func TestUpdateDaemonAgentQueuesSkillSyncTask(t *testing.T) {
	userID := "user-1"
	machineID := "machine-1"
	payload := `[{"name":"coding","detail":"new content","source_path":"C:\\skills\\coding\\SKILL.md"}]`
	repo := &fakeAgentRepo{
		currentAgent: &model.Agent{
			ID: "agent-1", UserID: &userID, Name: "Agent", Type: "custom", CLITool: "claude",
			Source: "daemon", MachineID: &machineID,
			CapabilitiesJSON: `[{"name":"coding","detail":"old","source_path":"C:\\skills\\coding\\SKILL.md"}]`,
		},
		updateResult: &model.Agent{
			ID: "agent-1", UserID: &userID, Name: "Agent", Type: "custom", CLITool: "claude",
			Source: "daemon", MachineID: &machineID,
			CapabilitiesJSON: payload,
		},
	}
	svc := NewAgentService(repo)
	_, err := svc.UpdateCustom(context.Background(), "agent-1", userID, "Agent", "claude", "", "", payload)
	if err != nil {
		t.Fatalf("update daemon agent: %v", err)
	}
	if repo.daemonTask == nil || repo.daemonTask.CLITool != daemonSkillSyncTool {
		t.Fatalf("expected daemon skill sync task, got %#v", repo.daemonTask)
	}
	if !strings.Contains(repo.daemonTask.Prompt, "new content") {
		t.Fatalf("expected skill content in sync payload, got %q", repo.daemonTask.Prompt)
	}
}

func strconvQuote(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func TestUpdateCustomReturnsNotFound(t *testing.T) {
	svc := NewAgentService(&fakeAgentRepo{}, nil)
	_, err := svc.UpdateCustom(context.Background(), "agent-1", "user-1", "Agent", "claude", "", "", "", "", false)
	if !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("expected ErrAgentNotFound, got %v", err)
	}
}

func TestAddCandidateAgentStoresPrompt(t *testing.T) {
	repo := &fakeAgentRepo{}
	svc := NewAgentService(repo, nil)
	_, err := svc.AddCandidateAgent(context.Background(), "user-1", "candidate-1", "My Agent", "persona")
	if err != nil {
		t.Fatalf("add candidate agent failed: %v", err)
	}
	if repo.addedPrompt != "persona" {
		t.Fatalf("expected system prompt stored, got %q", repo.addedPrompt)
	}
}

func TestDeleteCustomReturnsNotFound(t *testing.T) {
	svc := NewAgentService(&fakeAgentRepo{}, nil)
	err := svc.DeleteOwned(context.Background(), "agent-1", "user-1")
	if !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("expected ErrAgentNotFound, got %v", err)
	}
}
