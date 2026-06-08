package ws

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nhooyr.io/websocket"
)

func newTestDaemonConn(t *testing.T) (*websocket.Conn, func()) {
	t.Helper()

	connCh := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		connCh <- conn
	}))

	url := "ws" + strings.TrimPrefix(server.URL, "http")
	clientConn, _, err := websocket.Dial(context.Background(), url, nil)
	if err != nil {
		server.Close()
		t.Fatalf("dial websocket: %v", err)
	}

	select {
	case serverConn := <-connCh:
		return serverConn, func() {
			clientConn.Close(websocket.StatusNormalClosure, "test done")
			serverConn.Close(websocket.StatusNormalClosure, "test done")
			server.Close()
		}
	case <-time.After(time.Second):
		clientConn.Close(websocket.StatusInternalError, "accept timeout")
		server.Close()
		t.Fatal("timed out waiting for websocket accept")
	}
	return nil, func() {}
}

func waitForDaemonConnection(t *testing.T, hub *DaemonHub, machineID string, want bool) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if hub.IsConnected(machineID) == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("machine %s connected=%v did not become %v", machineID, hub.IsConnected(machineID), want)
}

func TestDaemonHubReconnectIgnoresOldUnregister(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewDaemonHub(slog.Default())
	go hub.Run(ctx)

	conn1, cleanup1 := newTestDaemonConn(t)
	defer cleanup1()
	conn2, cleanup2 := newTestDaemonConn(t)
	defer cleanup2()

	machineID := "machine-1"
	oldClient := NewDaemonClient(conn1, machineID)
	newClient := NewDaemonClient(conn2, machineID)

	hub.Register(oldClient)
	waitForDaemonConnection(t, hub, machineID, true)

	hub.Register(newClient)
	waitForDaemonConnection(t, hub, machineID, true)

	hub.Unregister(oldClient)
	waitForDaemonConnection(t, hub, machineID, true)

	hub.Unregister(newClient)
	waitForDaemonConnection(t, hub, machineID, false)
}
