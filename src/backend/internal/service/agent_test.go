package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/agent-hub/backend/internal/model"
)

type fakeAgentRepo struct {
	updateResult  *model.Agent
	currentAgent  *model.Agent
	daemonTask    *model.DaemonTask
	deleted       bool
	registered    []string
	machines      []model.DaemonMachine
	machineAgent  []string
	candidates    []string
	addedPrompt   string
	addedCLITool  string
	addedTools    string
	addedSkills   string
	updatedUser   string
	updatedTools  string
	updatedSkills string
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

func (r *fakeAgentRepo) UpsertSystemAgent(ctx context.Context, name, cliTool, version, capabilitiesJSON, machineID string) error {
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

func (r *fakeAgentRepo) UpdateMachineCapabilities(ctx context.Context, id string, capabilities []string) error {
	return nil
}

func (r *fakeAgentRepo) FindMachineWithCapability(ctx context.Context, userID, capability string) (*model.DaemonMachine, error) {
	return nil, nil
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

func (r *fakeAgentRepo) AddCandidateAgent(ctx context.Context, userID, candidateID, displayName, expectedCLITool, systemPrompt, toolsConfig, customSkills string, enableManagementTools bool) (*model.Agent, error) {
	r.addedPrompt = systemPrompt
	r.addedCLITool = expectedCLITool
	r.addedTools = toolsConfig
	r.addedSkills = customSkills
	return &model.Agent{ID: "agent-1", UserID: &userID, Name: displayName, CLITool: "codex", Type: "custom", EnableManagementTools: enableManagementTools}, nil
}

func (r *fakeAgentRepo) CreateCustom(ctx context.Context, userID, name, cliTool, systemPrompt, toolsConfig, avatar, capabilitiesJSON, customSkills string, enableManagementTools bool) (*model.Agent, error) {
	return &model.Agent{ID: "agent-1", UserID: &userID, Name: name, CLITool: cliTool, Type: "custom"}, nil
}

func (r *fakeAgentRepo) UpdateCustom(ctx context.Context, id, userID, name, cliTool, systemPrompt, toolsConfig, avatar, capabilitiesJSON, customSkills string, enableManagementTools bool) (*model.Agent, error) {
	return r.updateResult, nil
}

func (r *fakeAgentRepo) UpdateToolsConfig(ctx context.Context, id, userID, toolsConfig string, enableManagementTools bool) (*model.Agent, error) {
	r.updatedUser = userID
	r.updatedTools = toolsConfig
	if r.updateResult != nil {
		r.updateResult.ToolsConfig = toolsConfig
		r.updateResult.EnableManagementTools = enableManagementTools
		return r.updateResult, nil
	}
	return nil, nil
}

func (r *fakeAgentRepo) DeleteOwned(ctx context.Context, id, userID string) (bool, error) {
	return r.deleted, nil
}

func (r *fakeAgentRepo) GetDaemonMachineByID(ctx context.Context, id string) (*model.DaemonMachine, error) {
	for i := range r.machines {
		if r.machines[i].ID == id {
			return &r.machines[i], nil
		}
	}
	return nil, nil
}

func (r *fakeAgentRepo) UpdateAvatar(_ context.Context, _, _, _ string) (*model.Agent, error) {
	return nil, nil
}

func (r *fakeAgentRepo) UpdateTags(ctx context.Context, id, tags string) (*model.Agent, error) {
	return nil, nil
}

func (r *fakeAgentRepo) UpdateCustomSkills(ctx context.Context, id, userID, customSkills string) (*model.Agent, error) {
	r.updatedUser = userID
	r.updatedSkills = customSkills
	return &model.Agent{ID: id, Name: "Agent", Type: "custom", CustomSkills: customSkills}, nil
}

func (r *fakeAgentRepo) UpdateAgentStatus(ctx context.Context, id, status string) error {
	return nil
}

func (r *fakeAgentRepo) ClearAgentMachine(ctx context.Context, id string) error {
	return nil
}

func (r *fakeAgentRepo) MarkMachineAgentsStopped(ctx context.Context, machineID string) error {
	return nil
}

func (r *fakeAgentRepo) UpdateMachineAPIKey(ctx context.Context, id, apiKeyHash string) error {
	return nil
}

func (r *fakeAgentRepo) GetAgentsByMachine(ctx context.Context, machineID string) ([]model.Agent, error) {
	return nil, nil
}

func TestCreateCustomRejectsEmptyName(t *testing.T) {
	svc := NewAgentService(&fakeAgentRepo{}, nil)
	_, err := svc.CreateCustom(context.Background(), "user-1", "", "claude", "", "", "", "", "", false)
	if !errors.Is(err, ErrAgentInvalidInput) {
		t.Fatalf("expected ErrAgentInvalidInput, got %v", err)
	}
}

func TestRegisterSystemAgentsSkipsInvalidItems(t *testing.T) {
	repo := &fakeAgentRepo{}
	svc := NewAgentService(repo, nil)
	err := svc.RegisterSystemAgents(context.Background(), "", []DiscoveredAgent{
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

func TestOpenDaemonSkillLocationQueuesOpenTask(t *testing.T) {
	userID := "user-1"
	machineID := "machine-1"
	sourcePath := `C:\skills\coding\SKILL.md`
	repo := &fakeAgentRepo{
		currentAgent: &model.Agent{
			ID: "agent-1", UserID: &userID, Name: "Agent", Type: "custom", CLITool: "claude",
			Source:           "daemon",
			MachineID:        &machineID,
			CapabilitiesJSON: `[{"name":"coding","source_path":"` + strings.ReplaceAll(sourcePath, `\`, `\\`) + `"}]`,
		},
	}
	svc := NewAgentService(repo, nil)
	// No daemonHub connected → ErrAgentOffline (validates path reaches WS push)
	err := svc.OpenDaemonSkillLocation(context.Background(), userID, "agent-1", sourcePath)
	if !errors.Is(err, ErrAgentOffline) {
		t.Fatalf("expected ErrAgentOffline without daemon hub, got %v", err)
	}
}

func TestOpenDaemonSkillLocationRejectsUnknownSourcePath(t *testing.T) {
	userID := "user-1"
	machineID := "machine-1"
	repo := &fakeAgentRepo{
		currentAgent: &model.Agent{
			ID: "agent-1", UserID: &userID, Name: "Agent", Type: "custom", CLITool: "claude",
			Source:           "daemon",
			MachineID:        &machineID,
			CapabilitiesJSON: `[{"name":"coding","source_path":"C:\\skills\\coding\\SKILL.md"}]`,
		},
	}
	svc := NewAgentService(repo, nil)
	err := svc.OpenDaemonSkillLocation(context.Background(), userID, "agent-1", `C:\other\SKILL.md`)
	if !errors.Is(err, ErrAgentInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
	if repo.daemonTask != nil {
		t.Fatalf("unexpected daemon task: %#v", repo.daemonTask)
	}
}

func TestUpdateCustomReturnsNotFound(t *testing.T) {
	svc := NewAgentService(&fakeAgentRepo{}, nil)
	_, err := svc.UpdateCustom(context.Background(), "agent-1", "user-1", "Agent", "claude", "", "", "", "", "", false)
	if !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("expected ErrAgentNotFound, got %v", err)
	}
}

func TestAddCandidateAgentStoresPrompt(t *testing.T) {
	repo := &fakeAgentRepo{}
	svc := NewAgentService(repo, nil)
	svc.SetToolRegistry(testRegistry())
	svc.SetToolsetStore(testToolsetStore())
	_, err := svc.AddCandidateAgent(
		context.Background(),
		"user-1",
		"candidate-1",
		"My Agent",
		"codex",
		"persona",
		`{"toolset":"custom","allowed_tools":["list_tasks","unknown"]}`,
		`[{"name":"审查"}]`,
		true,
	)
	if err != nil {
		t.Fatalf("add candidate agent failed: %v", err)
	}
	if repo.addedPrompt != "persona" {
		t.Fatalf("expected system prompt stored, got %q", repo.addedPrompt)
	}
	if repo.addedCLITool != "codex" {
		t.Fatalf("expected cli tool checked, got %q", repo.addedCLITool)
	}
	if repo.addedTools != `{"allowed_tools":["list_tasks"]}` {
		t.Fatalf("expected normalized tools config, got %q", repo.addedTools)
	}
	if repo.addedSkills != `[{"name":"审查"}]` {
		t.Fatalf("expected custom skills passed through, got %q", repo.addedSkills)
	}
}

func TestAddCandidateAgentRejectsInvalidCustomSkills(t *testing.T) {
	repo := &fakeAgentRepo{}
	svc := NewAgentService(repo, nil)
	_, err := svc.AddCandidateAgent(
		context.Background(),
		"user-1",
		"candidate-1",
		"My Agent",
		"codex",
		"",
		`{"toolset":"tasks"}`,
		`not json`,
		false,
	)
	if !errors.Is(err, ErrAgentInvalidInput) {
		t.Fatalf("expected ErrAgentInvalidInput, got %v", err)
	}
}

func TestUpdateToolsConfigPersistsForOwnedDaemonAgent(t *testing.T) {
	userID := "user-1"
	repo := &fakeAgentRepo{
		updateResult: &model.Agent{
			ID:      "agent-1",
			UserID:  &userID,
			Name:    "Daemon Agent",
			Type:    "custom",
			Source:  "daemon",
			CLITool: "codex",
		},
	}
	svc := NewAgentService(repo, nil)
	svc.SetToolRegistry(testRegistry())
	svc.SetToolsetStore(testToolsetStore())

	agent, err := svc.UpdateToolsConfig(
		context.Background(),
		"agent-1",
		userID,
		`{"toolset":"custom","allowed_tools":["list_tasks","unknown","create_agent"]}`,
		true,
	)
	if err != nil {
		t.Fatalf("update tools config failed: %v", err)
	}
	if agent == nil {
		t.Fatal("expected updated agent")
	}
	if repo.updatedUser != userID {
		t.Fatalf("expected scoped user id, got %q", repo.updatedUser)
	}
	if repo.updatedTools != `{"allowed_tools":["list_tasks","create_agent"]}` {
		t.Fatalf("unexpected normalized tools config: %s", repo.updatedTools)
	}
	if !agent.EnableManagementTools {
		t.Fatal("expected management flag persisted")
	}
}

func TestUpdateToolsConfigReturnsNotFound(t *testing.T) {
	svc := NewAgentService(&fakeAgentRepo{}, nil)
	_, err := svc.UpdateToolsConfig(context.Background(), "agent-1", "user-1", `{"toolset":"none"}`, false)
	if !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("expected ErrAgentNotFound, got %v", err)
	}
}

func TestUpdateToolsConfigRejectsInvalidIDs(t *testing.T) {
	svc := NewAgentService(&fakeAgentRepo{}, nil)
	_, err := svc.UpdateToolsConfig(context.Background(), "agent-1", "", `{"toolset":"none"}`, false)
	if !errors.Is(err, ErrAgentInvalidInput) {
		t.Fatalf("expected ErrAgentInvalidInput, got %v", err)
	}
}

func TestNormalizeCustomSkillsFiltersUnsafeFields(t *testing.T) {
	got, err := normalizeCustomSkills(`[{"name":" review ","description":" check ","trigger":" bug ","detail":" use checklist ","source_path":"/tmp/a"},{"name":"review"},{"name":""}]`)
	if err != nil {
		t.Fatalf("normalize custom skills: %v", err)
	}
	if got != `[{"name":"review","description":"check","trigger":"bug","detail":"use checklist"}]` {
		t.Fatalf("unexpected normalized skills: %s", got)
	}
}

func TestUpdateCustomSkillsNormalizesAndScopesUser(t *testing.T) {
	repo := &fakeAgentRepo{}
	svc := NewAgentService(repo, nil)
	_, err := svc.UpdateCustomSkills(context.Background(), "agent-1", "user-1", `[{"name":" review ","description":" check ","source_path":"/tmp/a"}]`)
	if err != nil {
		t.Fatalf("update custom skills failed: %v", err)
	}
	if repo.updatedUser != "user-1" {
		t.Fatalf("expected user id passed to repo, got %q", repo.updatedUser)
	}
	if repo.updatedSkills != `[{"name":"review","description":"check"}]` {
		t.Fatalf("unexpected normalized skills: %s", repo.updatedSkills)
	}
}

func TestBuildAgentSkillContextUsesIndexAndLookupTool(t *testing.T) {
	raw := `[{"name":"代码审查","description":"检查 bug 和测试缺口","trigger":"review, bug","detail":"逐项检查边界、权限和测试。"},{"name":"文档撰写","description":"写说明","detail":"不要命中"}]`
	got := BuildAgentSkillContext(raw)
	if !strings.Contains(got, "[平台 Skills]") {
		t.Fatal("expected skill context section")
	}
	if !strings.Contains(got, "代码审查：检查 bug 和测试缺口") {
		t.Fatalf("expected skill index, got %s", got)
	}
	if !strings.Contains(got, "get_agent_skill") {
		t.Fatalf("expected skill lookup tool instruction, got %s", got)
	}
	if strings.Contains(got, "逐项检查边界、权限和测试。") {
		t.Fatalf("expected detail to remain outside prompt, got %s", got)
	}
	if strings.Contains(got, "不要命中") {
		t.Fatalf("expected no skill detail in prompt: %s", got)
	}
}

func TestUpdateCustomSkillsRejectsInvalidJSON(t *testing.T) {
	svc := NewAgentService(&fakeAgentRepo{}, nil)
	_, err := svc.UpdateCustomSkills(context.Background(), "agent-1", "user-1", `not json`)
	if !errors.Is(err, ErrAgentInvalidInput) {
		t.Fatalf("expected ErrAgentInvalidInput, got %v", err)
	}
}

func TestDeleteCustomReturnsNotFound(t *testing.T) {
	svc := NewAgentService(&fakeAgentRepo{}, nil)
	err := svc.DeleteOwned(context.Background(), "agent-1", "user-1")
	if !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("expected ErrAgentNotFound, got %v", err)
	}
}
