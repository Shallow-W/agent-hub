package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/pkg/ws"
)

// buildEditService 组装一个可跑 AIEditArtifact 的 service（含 daemon hub）。
//
// P8a 后 setter 已删除，所有依赖通过 OrchestratorDeps 一次性注入。
func buildEditService(t *testing.T, convRepo *fakeOrchConvRepo, agentRepo *fakeOrchAgentRepo, msgRepo *fakeMsgRepo, artRepo *fakeArtifactRepo, machineID string) (*OrchestratorService, *ws.DaemonHub) {
	t.Helper()
	deps := OrchestratorDeps{
		ConvRepo:     convRepo,
		AgentRepo:    agentRepo,
		MsgRepo:      msgRepo,
		ArtifactRepo: artRepo,
	}
	if machineID != "" {
		hub := newTestDaemonHub(t, machineID)
		deps.DaemonHub = hub
		svc := NewOrchestratorServiceWithDeps(deps)
		return svc, hub
	}
	svc := NewOrchestratorServiceWithDeps(deps)
	return svc, nil
}

func TestAIEditArtifact_EmptyInstruction(t *testing.T) {
	svc, _ := buildEditService(t,
		&fakeOrchConvRepo{conv: &model.Conversation{ID: "c1", UserID: "u1"}},
		&fakeOrchAgentRepo{},
		&fakeMsgRepo{},
		&fakeArtifactRepo{convIDByRoot: map[string]string{"root-1": "c1"}},
		"",
	)
	_, err := svc.AIEditArtifact(context.Background(), "root-1", "u1", "   ", "")
	if !errors.Is(err, ErrArtifactEditInvalid) {
		t.Fatalf("expected ErrArtifactEditInvalid, got %v", err)
	}
}

func TestAIEditArtifact_RootNotFound(t *testing.T) {
	svc, _ := buildEditService(t,
		&fakeOrchConvRepo{},
		&fakeOrchAgentRepo{},
		&fakeMsgRepo{},
		&fakeArtifactRepo{rootNotFound: true},
		"",
	)
	_, err := svc.AIEditArtifact(context.Background(), "root-x", "u1", "改一下", "")
	if !errors.Is(err, ErrArtifactEditNotFound) {
		t.Fatalf("expected ErrArtifactEditNotFound, got %v", err)
	}
}

func TestAIEditArtifact_NotMember(t *testing.T) {
	svc, _ := buildEditService(t,
		&fakeOrchConvRepo{conv: &model.Conversation{ID: "c1", UserID: "owner"}, member: nil},
		&fakeOrchAgentRepo{},
		&fakeMsgRepo{},
		&fakeArtifactRepo{convIDByRoot: map[string]string{"root-1": "c1"}},
		"",
	)
	_, err := svc.AIEditArtifact(context.Background(), "root-1", "intruder", "改一下", "")
	if !errors.Is(err, ErrArtifactEditNoPerm) {
		t.Fatalf("expected ErrArtifactEditNoPerm, got %v", err)
	}
}

func TestAIEditArtifact_NonCodeUnsupported(t *testing.T) {
	svc, _ := buildEditService(t,
		&fakeOrchConvRepo{conv: &model.Conversation{ID: "c1", UserID: "u1"}},
		&fakeOrchAgentRepo{},
		&fakeMsgRepo{},
		&fakeArtifactRepo{
			convIDByRoot: map[string]string{"root-1": "c1"},
			latest:       &model.Artifact{Type: "webpage", URL: "http://x"},
		},
		"",
	)
	_, err := svc.AIEditArtifact(context.Background(), "root-1", "u1", "改一下", "")
	if !errors.Is(err, ErrArtifactEditUnsupported) {
		t.Fatalf("expected ErrArtifactEditUnsupported, got %v", err)
	}
}

func TestAIEditArtifact_NoConnectedAgent(t *testing.T) {
	// 对话里有一个 agent，但没有 daemon 连接（agent 无 MachineID）。
	svc, _ := buildEditService(t,
		&fakeOrchConvRepo{
			conv:       &model.Conversation{ID: "c1", UserID: "u1"},
			convAgents: []model.ConversationAgent{{AgentID: "a1", Name: "A"}},
		},
		&fakeOrchAgentRepo{agent: &model.Agent{ID: "a1", Name: "A", CLITool: "claude"}},
		&fakeMsgRepo{},
		&fakeArtifactRepo{
			convIDByRoot: map[string]string{"root-1": "c1"},
			latest:       &model.Artifact{Type: "code", Content: "old", Language: "go", MessageID: "m1"},
		},
		"machine-1",
	)
	_, err := svc.AIEditArtifact(context.Background(), "root-1", "u1", "改一下", "")
	if !errors.Is(err, ErrArtifactEditNoAgent) {
		t.Fatalf("expected ErrArtifactEditNoAgent, got %v", err)
	}
}

func TestAIEditArtifact_Success_ExtractsFencedCode(t *testing.T) {
	machineID := "machine-1"
	agent := &model.Agent{ID: "a1", Name: "A", CLITool: "claude", MachineID: &machineID}
	artRepo := &fakeArtifactRepo{
		convIDByRoot: map[string]string{"root-1": "c1"},
		latest:       &model.Artifact{Type: "code", Content: "func old() {}", Language: "go", Filename: "main.go", MessageID: "m1"},
	}
	svc, hub := buildEditService(t,
		&fakeOrchConvRepo{
			conv:       &model.Conversation{ID: "c1", UserID: "u1"},
			convAgents: []model.ConversationAgent{{AgentID: "a1", Name: "A"}},
		},
		&fakeOrchAgentRepo{agent: agent, task: &model.DaemonTask{ID: "task-1"}},
		&fakeMsgRepo{},
		artRepo,
		machineID,
	)

	go func() {
		time.Sleep(10 * time.Millisecond)
		hub.ResolveTask("task-1", &ws.TaskResult{
			TaskID: "task-1",
			Result: "这是修改后的代码：\n```go\nfunc neww() {}\n```\n完成。",
		})
	}()

	created, err := svc.AIEditArtifact(context.Background(), "root-1", "u1", "重命名函数", "func old() {}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created == nil {
		t.Fatal("expected created artifact, got nil")
	}
	if artRepo.createCalls != 1 {
		t.Fatalf("expected 1 CreateVersion call, got %d", artRepo.createCalls)
	}
	if artRepo.createVersIn.Content != "func neww() {}" {
		t.Fatalf("expected extracted fenced code, got %q", artRepo.createVersIn.Content)
	}
	if artRepo.createVersIn.Type != "code" || artRepo.createVersIn.Language != "go" || artRepo.createVersIn.Filename != "main.go" {
		t.Fatalf("expected code/go/main.go preserved, got %+v", artRepo.createVersIn)
	}
}

func TestAIEditArtifact_Success_PrefersCodeArtifact(t *testing.T) {
	machineID := "machine-1"
	agent := &model.Agent{ID: "a1", Name: "A", CLITool: "claude", MachineID: &machineID}
	artRepo := &fakeArtifactRepo{
		convIDByRoot: map[string]string{"root-1": "c1"},
		latest:       &model.Artifact{Type: "code", Content: "old", Language: "ts", MessageID: "m1"},
	}
	svc, hub := buildEditService(t,
		&fakeOrchConvRepo{
			conv:       &model.Conversation{ID: "c1", UserID: "u1"},
			convAgents: []model.ConversationAgent{{AgentID: "a1", Name: "A"}},
		},
		&fakeOrchAgentRepo{agent: agent, task: &model.DaemonTask{ID: "task-2"}},
		&fakeMsgRepo{},
		artRepo,
		machineID,
	)

	go func() {
		time.Sleep(10 * time.Millisecond)
		hub.ResolveTask("task-2", &ws.TaskResult{
			TaskID:    "task-2",
			Result:    "ignored text",
			Artifacts: []ws.ArtifactResult{{Type: "code", Content: "const x = 2"}},
		})
	}()

	_, err := svc.AIEditArtifact(context.Background(), "root-1", "u1", "改值", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if artRepo.createVersIn.Content != "const x = 2" {
		t.Fatalf("expected code artifact content, got %q", artRepo.createVersIn.Content)
	}
}
