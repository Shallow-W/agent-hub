package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/service"
	"github.com/agent-hub/backend/pkg/ws"
	"nhooyr.io/websocket"
)

// ---------------------------------------------------------------------------
// Fake repo for daemon handler tests
// ---------------------------------------------------------------------------

// fakeDaemonAgentRepo satisfies service.AgentRepo for daemon handler tests.
// It records calls to key methods so tests can assert side effects.
type fakeDaemonAgentRepo struct {
	mu sync.Mutex

	// statusCalls records (agentID, status) pairs from UpdateAgentStatus
	statusCalls []agentStatusCall

	// markMachineStoppedCalls records machineIDs passed to MarkMachineAgentsStopped
	markMachineStoppedCalls []string
}

type agentStatusCall struct {
	AgentID string
	Status  string
}

// Ensure fakeDaemonAgentRepo satisfies service.AgentRepo at compile time.
var _ service.AgentRepo = (*fakeDaemonAgentRepo)(nil)

func (r *fakeDaemonAgentRepo) ListAvailable(_ context.Context, _ string) ([]model.Agent, error) {
	return nil, nil
}
func (r *fakeDaemonAgentRepo) GetByID(_ context.Context, _ string) (*model.Agent, error) {
	return nil, nil
}
func (r *fakeDaemonAgentRepo) GetDaemonTask(_ context.Context, _ string) (*model.DaemonTask, error) {
	return nil, nil
}
func (r *fakeDaemonAgentRepo) CreateDaemonTask(_ context.Context, _, _, _, _, _, _, _ string) (*model.DaemonTask, error) {
	return &model.DaemonTask{ID: "task-1", Status: "pending"}, nil
}
func (r *fakeDaemonAgentRepo) ClaimDaemonTask(_ context.Context, _ string) (*model.DaemonTask, error) {
	return nil, nil
}
func (r *fakeDaemonAgentRepo) CompleteDaemonTask(_ context.Context, _, _, _, _ string) (bool, error) {
	return true, nil
}
func (r *fakeDaemonAgentRepo) UpsertSystemAgent(_ context.Context, _, _, _, _ string) error {
	return nil
}
func (r *fakeDaemonAgentRepo) CreateDaemonMachine(_ context.Context, _, _, _ string) (*model.DaemonMachine, error) {
	return nil, nil
}
func (r *fakeDaemonAgentRepo) ListDaemonMachines(_ context.Context, _ string) ([]model.DaemonMachine, error) {
	return nil, nil
}
func (r *fakeDaemonAgentRepo) DeleteDaemonMachine(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
func (r *fakeDaemonAgentRepo) GetDaemonMachineByAPIKeyHash(_ context.Context, _ string) (*model.DaemonMachine, error) {
	return nil, nil
}
func (r *fakeDaemonAgentRepo) GetDaemonMachineByID(_ context.Context, _ string) (*model.DaemonMachine, error) {
	return nil, nil
}
func (r *fakeDaemonAgentRepo) MarkDaemonMachineConnected(_ context.Context, _, _ string) error {
	return nil
}
func (r *fakeDaemonAgentRepo) UpsertMachineAgentCandidate(_ context.Context, _, _, _, _, _ string) error {
	return nil
}
func (r *fakeDaemonAgentRepo) ListAgentCandidates(_ context.Context, _ string) ([]model.AgentCandidate, error) {
	return nil, nil
}
func (r *fakeDaemonAgentRepo) AddCandidateAgent(_ context.Context, _, _, _, _ string) (*model.Agent, error) {
	return nil, nil
}
func (r *fakeDaemonAgentRepo) CreateCustom(_ context.Context, _, _, _, _, _, _, _ string, _ bool) (*model.Agent, error) {
	return nil, nil
}
func (r *fakeDaemonAgentRepo) UpdateCustom(_ context.Context, _, _, _, _, _, _, _, _ string, _ bool) (*model.Agent, error) {
	return nil, nil
}
func (r *fakeDaemonAgentRepo) UpdateAgentStatus(_ context.Context, id, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statusCalls = append(r.statusCalls, agentStatusCall{AgentID: id, Status: status})
	return nil
}
func (r *fakeDaemonAgentRepo) ClearAgentMachine(_ context.Context, _ string) error { return nil }
func (r *fakeDaemonAgentRepo) MarkMachineAgentsStopped(_ context.Context, machineID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.markMachineStoppedCalls = append(r.markMachineStoppedCalls, machineID)
	return nil
}
func (r *fakeDaemonAgentRepo) UpdateMachineAPIKey(_ context.Context, _, _ string) error {
	return nil
}
func (r *fakeDaemonAgentRepo) DeleteOwned(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}

// helpers to read recorded calls
func (r *fakeDaemonAgentRepo) getStatusCalls() []agentStatusCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]agentStatusCall, len(r.statusCalls))
	copy(out, r.statusCalls)
	return out
}

func (r *fakeDaemonAgentRepo) getMarkMachineStoppedCalls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.markMachineStoppedCalls))
	copy(out, r.markMachineStoppedCalls)
	return out
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTestDaemonHandler creates a DaemonHandler wired to a real DaemonHub and a
// fake AgentService backed by fakeDaemonAgentRepo. Returns the handler, hub,
// and fake repo so tests can make assertions.
func newTestDaemonHandler(t *testing.T) (*DaemonHandler, *ws.DaemonHub, *fakeDaemonAgentRepo) {
	t.Helper()

	fakeRepo := &fakeDaemonAgentRepo{}
	agentSvc := service.NewAgentService(fakeRepo, nil) // nil tracker: not needed for these tests
	agentSvc.SetJWTSecret("test-secret")

	daemonHub := ws.NewDaemonHub(slog.Default())
	hubCtx, hubCancel := context.WithCancel(context.Background())
	t.Cleanup(hubCancel)
	go daemonHub.Run(hubCtx)

	agentSvc.SetDaemonHub(daemonHub)

	handler := NewDaemonHandler(agentSvc, "test-daemon-token", slog.Default(), []string{"*"}, daemonHub)
	return handler, daemonHub, fakeRepo
}

// dialDaemonWS creates an in-memory WebSocket connection pair.
// The server side accepts the WS upgrade and runs the handler in a goroutine.
// Returns the client-side connection and a cancel function to shut down.
func dialDaemonWS(t *testing.T, handlerFn http.HandlerFunc) (*websocket.Conn, context.CancelFunc) {
	t.Helper()

	server := httptest.NewServer(handlerFn)
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	t.Cleanup(func() { conn.Close(websocket.StatusNormalClosure, "test done") })

	return conn, cancel
}

// writeWSJSON marshals v and writes it as a text message on conn.
func writeWSJSON(t *testing.T, ctx context.Context, conn *websocket.Conn, v interface{}) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("ws write: %v", err)
	}
}

// readWSJSON reads a text message from conn and unmarshals into v.
func readWSJSON(t *testing.T, ctx context.Context, conn *websocket.Conn, v interface{}) {
	t.Helper()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("ws read: %v", err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("unmarshal: %v, data: %s", err, string(data))
	}
}

// ---------------------------------------------------------------------------
// Test 1: Daemon WS connect + task dispatch flow
// ---------------------------------------------------------------------------

func TestDaemonWS_TaskDispatch_DaemonReceivesAndResolves(t *testing.T) {
	_, hub, _ := newTestDaemonHandler(t)

	// The handler reads from the WS in a loop. When it receives a message it
	// dispatches to the appropriate sub-handler. We need to:
	// 1. Connect a daemon client via real WS
	// 2. Register a task promise on the hub
	// 3. Send a task message to the daemon via SendToMachine
	// 4. Verify the daemon receives the message on its WS
	// 5. Have the daemon send back a task.complete
	// 6. Verify the promise channel resolves

	machineID := "machine-test-1"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: []string{"*"},
		})
		if err != nil {
			t.Logf("websocket accept failed: %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")

		// Create a DaemonClient and register it
		client := ws.NewDaemonClient(conn, machineID)
		hub.Register(client)

		clientCtx, clientCancel := context.WithCancel(r.Context())
		defer clientCancel()

		go client.WritePump(clientCtx)

		// Read loop: just handle task.complete by calling hub.ResolveTask
		for {
			_, data, err := conn.Read(clientCtx)
			if err != nil {
				return
			}
			var envelope struct {
				Type string          `json:"type"`
				Data json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(data, &envelope); err != nil {
				continue
			}
			if envelope.Type == "task.complete" {
				var req struct {
					TaskID string `json:"task_id"`
					Result string `json:"result"`
					Error  string `json:"error"`
				}
				if err := json.Unmarshal(envelope.Data, &req); err != nil {
					continue
				}
				hub.ResolveTask(req.TaskID, &ws.TaskResult{
					TaskID: req.TaskID,
					Result: req.Result,
					Error:  req.Error,
				})
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	daemonConn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("daemon dial failed: %v", err)
	}
	defer daemonConn.Close(websocket.StatusNormalClosure, "test done")

	// Wait briefly for the Register to propagate through the hub event loop
	time.Sleep(50 * time.Millisecond)

	if !hub.IsConnected(machineID) {
		t.Fatal("expected daemon to be connected")
	}

	// Register a task promise
	taskID := "task-ws-1"
	promiseCh := hub.RegisterTaskPromise(taskID)

	// Send a task message to the daemon via the hub
	err = hub.SendToMachine(machineID, ws.WSMessage{
		Type: "task.dispatch",
		Data: map[string]interface{}{
			"task_id": taskID,
			"prompt":  "echo hello",
		},
	})
	if err != nil {
		t.Fatalf("SendToMachine failed: %v", err)
	}

	// Daemon reads the dispatched task
	_, data, err := daemonConn.Read(ctx)
	if err != nil {
		t.Fatalf("daemon read failed: %v", err)
	}
	var receivedMsg ws.WSMessage
	if err := json.Unmarshal(data, &receivedMsg); err != nil {
		t.Fatalf("unmarshal received: %v", err)
	}
	if receivedMsg.Type != "task.dispatch" {
		t.Fatalf("expected type task.dispatch, got %q", receivedMsg.Type)
	}

	// Daemon sends back task.complete
	taskCompleteMsg := struct {
		Type string `json:"type"`
		Data struct {
			TaskID string `json:"task_id"`
			Result string `json:"result"`
			Error  string `json:"error"`
		} `json:"data"`
	}{
		Type: "task.complete",
	}
	taskCompleteMsg.Data.TaskID = taskID
	taskCompleteMsg.Data.Result = "hello world"
	writeWSJSON(t, ctx, daemonConn, taskCompleteMsg)

	// Verify the promise channel resolves
	select {
	case result := <-promiseCh:
		if result.TaskID != taskID {
			t.Errorf("result.TaskID = %q, want %q", result.TaskID, taskID)
		}
		if result.Result != "hello world" {
			t.Errorf("result.Result = %q, want %q", result.Result, "hello world")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for task promise to resolve")
	}
}

// ---------------------------------------------------------------------------
// Test 2: Agent started/stopped status updates via WS
// ---------------------------------------------------------------------------

func TestDaemonWS_AgentStarted_UpdatesStatus(t *testing.T) {
	fakeRepo := &fakeDaemonAgentRepo{}
	agentSvc := service.NewAgentService(fakeRepo, nil)
	agentSvc.SetJWTSecret("test-secret")

	daemonHub := ws.NewDaemonHub(slog.Default())
	hubCtx, hubCancel := context.WithCancel(context.Background())
	t.Cleanup(hubCancel)
	go daemonHub.Run(hubCtx)
	agentSvc.SetDaemonHub(daemonHub)

	handler := NewDaemonHandler(agentSvc, "test-daemon-token", slog.Default(), []string{"*"}, daemonHub)

	machineID := "machine-status-1"

	// Build an HTTP server that runs the handler's readLoop equivalent
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: []string{"*"},
		})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")

		client := ws.NewDaemonClient(conn, machineID)
		daemonHub.Register(client)

		clientCtx, clientCancel := context.WithCancel(r.Context())
		defer clientCancel()
		go client.WritePump(clientCtx)

		// Reuse the real handler's readLoop by calling it directly.
		// We pass nil machine because with the global token, authenticateMachine
		// returns nil,nil (system daemon). The handler still processes messages.
		machine := &model.DaemonMachine{ID: machineID, UserID: "user-1"}
		handler.readLoop(clientCtx, client, machine)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	daemonConn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer daemonConn.Close(websocket.StatusNormalClosure, "test done")

	// Wait for registration
	time.Sleep(50 * time.Millisecond)

	// Send agent.started message
	agentStarted := struct {
		Type string `json:"type"`
		Data struct {
			AgentID string `json:"agent_id"`
		} `json:"data"`
	}{
		Type: "agent.started",
	}
	agentStarted.Data.AgentID = "agent-abc"
	writeWSJSON(t, ctx, daemonConn, agentStarted)

	// Wait for handler to process
	time.Sleep(100 * time.Millisecond)

	calls := fakeRepo.getStatusCalls()
	found := false
	for _, c := range calls {
		if c.AgentID == "agent-abc" && c.Status == "online" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected UpdateAgentStatus(agent-abc, online), got calls: %+v", calls)
	}

	// Send agent.stopped message
	agentStopped := struct {
		Type string `json:"type"`
		Data struct {
			AgentID string `json:"agent_id"`
		} `json:"data"`
	}{
		Type: "agent.stopped",
	}
	agentStopped.Data.AgentID = "agent-abc"
	writeWSJSON(t, ctx, daemonConn, agentStopped)

	// Wait for handler to process
	time.Sleep(100 * time.Millisecond)

	calls = fakeRepo.getStatusCalls()
	foundStopped := false
	for _, c := range calls {
		if c.AgentID == "agent-abc" && c.Status == "stopped" {
			foundStopped = true
			break
		}
	}
	if !foundStopped {
		t.Errorf("expected UpdateAgentStatus(agent-abc, stopped), got calls: %+v", calls)
	}
}

// ---------------------------------------------------------------------------
// Test 3: Machine disconnect -> agents stopped
// ---------------------------------------------------------------------------

func TestDaemonWS_MachineDisconnect_MarksAgentsStopped(t *testing.T) {
	fakeRepo := &fakeDaemonAgentRepo{}
	agentSvc := service.NewAgentService(fakeRepo, nil)
	agentSvc.SetJWTSecret("test-secret")

	daemonHub := ws.NewDaemonHub(slog.Default())
	hubCtx, hubCancel := context.WithCancel(context.Background())
	t.Cleanup(hubCancel)
	go daemonHub.Run(hubCtx)
	agentSvc.SetDaemonHub(daemonHub)

	_ = NewDaemonHandler(agentSvc, "test-daemon-token", slog.Default(), []string{"*"}, daemonHub)

	// Simulate a machine disconnect by calling MarkMachineOffline directly
	// (this is what the handler does in its defer after readLoop returns)
	machineID := "machine-disconnect-1"
	agentSvc.MarkMachineOffline(machineID)

	// Verify MarkMachineAgentsStopped was called
	time.Sleep(100 * time.Millisecond)

	stoppedCalls := fakeRepo.getMarkMachineStoppedCalls()
	if len(stoppedCalls) == 0 {
		t.Fatal("expected MarkMachineAgentsStopped to be called, got 0 calls")
	}
	if stoppedCalls[0] != machineID {
		t.Errorf("MarkMachineAgentsStopped called with %q, want %q", stoppedCalls[0], machineID)
	}
}

// ---------------------------------------------------------------------------
// Test 4: Agent started with error sets status to "error"
// ---------------------------------------------------------------------------

func TestDaemonWS_AgentStartedWithError_SetsErrorStatus(t *testing.T) {
	fakeRepo := &fakeDaemonAgentRepo{}
	agentSvc := service.NewAgentService(fakeRepo, nil)
	agentSvc.SetJWTSecret("test-secret")

	daemonHub := ws.NewDaemonHub(slog.Default())
	hubCtx, hubCancel := context.WithCancel(context.Background())
	t.Cleanup(hubCancel)
	go daemonHub.Run(hubCtx)
	agentSvc.SetDaemonHub(daemonHub)

	handler := NewDaemonHandler(agentSvc, "test-daemon-token", slog.Default(), []string{"*"}, daemonHub)

	machineID := "machine-err-1"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: []string{"*"},
		})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")

		client := ws.NewDaemonClient(conn, machineID)
		daemonHub.Register(client)

		clientCtx, clientCancel := context.WithCancel(r.Context())
		defer clientCancel()
		go client.WritePump(clientCtx)

		machine := &model.DaemonMachine{ID: machineID, UserID: "user-1"}
		handler.readLoop(clientCtx, client, machine)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	daemonConn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer daemonConn.Close(websocket.StatusNormalClosure, "test done")

	time.Sleep(50 * time.Millisecond)

	// Send agent.started with error field
	agentStartedErr := struct {
		Type string `json:"type"`
		Data struct {
			AgentID string `json:"agent_id"`
			Error   string `json:"error"`
		} `json:"data"`
	}{
		Type: "agent.started",
	}
	agentStartedErr.Data.AgentID = "agent-err-1"
	agentStartedErr.Data.Error = "port conflict"
	writeWSJSON(t, ctx, daemonConn, agentStartedErr)

	time.Sleep(100 * time.Millisecond)

	calls := fakeRepo.getStatusCalls()
	found := false
	for _, c := range calls {
		if c.AgentID == "agent-err-1" && c.Status == "error" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected UpdateAgentStatus(agent-err-1, error), got calls: %+v", calls)
	}
}

// ---------------------------------------------------------------------------
// Test 5: Agent started with empty agent_id is ignored
// ---------------------------------------------------------------------------

func TestDaemonWS_AgentStarted_EmptyAgentID_Ignored(t *testing.T) {
	fakeRepo := &fakeDaemonAgentRepo{}
	agentSvc := service.NewAgentService(fakeRepo, nil)
	agentSvc.SetJWTSecret("test-secret")

	daemonHub := ws.NewDaemonHub(slog.Default())
	hubCtx, hubCancel := context.WithCancel(context.Background())
	t.Cleanup(hubCancel)
	go daemonHub.Run(hubCtx)
	agentSvc.SetDaemonHub(daemonHub)

	handler := NewDaemonHandler(agentSvc, "test-daemon-token", slog.Default(), []string{"*"}, daemonHub)

	machineID := "machine-empty-1"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: []string{"*"},
		})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")

		client := ws.NewDaemonClient(conn, machineID)
		daemonHub.Register(client)

		clientCtx, clientCancel := context.WithCancel(r.Context())
		defer clientCancel()
		go client.WritePump(clientCtx)

		machine := &model.DaemonMachine{ID: machineID, UserID: "user-1"}
		handler.readLoop(clientCtx, client, machine)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	daemonConn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer daemonConn.Close(websocket.StatusNormalClosure, "test done")

	time.Sleep(50 * time.Millisecond)

	// Send agent.started with empty agent_id
	agentStartedEmpty := struct {
		Type string `json:"type"`
		Data struct {
			AgentID string `json:"agent_id"`
		} `json:"data"`
	}{
		Type: "agent.started",
	}
	agentStartedEmpty.Data.AgentID = ""
	writeWSJSON(t, ctx, daemonConn, agentStartedEmpty)

	time.Sleep(100 * time.Millisecond)

	calls := fakeRepo.getStatusCalls()
	if len(calls) != 0 {
		t.Errorf("expected no UpdateAgentStatus calls for empty agent_id, got: %+v", calls)
	}
}

// ---------------------------------------------------------------------------
// Test 6: Ping/pong via WS
// ---------------------------------------------------------------------------

func TestDaemonWS_Ping_RespondsPong(t *testing.T) {
	fakeRepo := &fakeDaemonAgentRepo{}
	agentSvc := service.NewAgentService(fakeRepo, nil)
	agentSvc.SetJWTSecret("test-secret")

	daemonHub := ws.NewDaemonHub(slog.Default())
	hubCtx, hubCancel := context.WithCancel(context.Background())
	t.Cleanup(hubCancel)
	go daemonHub.Run(hubCtx)
	agentSvc.SetDaemonHub(daemonHub)

	handler := NewDaemonHandler(agentSvc, "test-daemon-token", slog.Default(), []string{"*"}, daemonHub)

	machineID := "machine-ping-1"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: []string{"*"},
		})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")

		client := ws.NewDaemonClient(conn, machineID)
		daemonHub.Register(client)

		clientCtx, clientCancel := context.WithCancel(r.Context())
		defer clientCancel()
		go client.WritePump(clientCtx)

		machine := &model.DaemonMachine{ID: machineID, UserID: "user-1"}
		handler.readLoop(clientCtx, client, machine)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	daemonConn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer daemonConn.Close(websocket.StatusNormalClosure, "test done")

	time.Sleep(50 * time.Millisecond)

	// Send ping
	pingMsg := struct {
		Type string `json:"type"`
	}{
		Type: "ping",
	}
	writeWSJSON(t, ctx, daemonConn, pingMsg)

	// Read pong response
	var pong ws.WSMessage
	readWSJSON(t, ctx, daemonConn, &pong)

	if pong.Type != "pong" {
		t.Errorf("expected pong, got type %q", pong.Type)
	}
}
