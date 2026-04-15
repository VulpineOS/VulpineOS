package remote

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nhooyr.io/websocket"
)

func TestBroadcastEventPreservesSessionID(t *testing.T) {
	server := NewServer(":0", "secret", nil)
	httpServer := httptest.NewServer(server.Mux())
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws?token=secret"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		server.clientsMu.RLock()
		count := len(server.clients)
		server.clientsMu.RUnlock()
		if count == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	params := json.RawMessage(`{"frameId":"frame-1"}`)
	server.BroadcastEvent("Page.frameAttached", "session-1", params)

	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read websocket: %v", err)
	}

	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.Type != "juggler" {
		t.Fatalf("unexpected envelope type %q", env.Type)
	}

	var msg struct {
		Method    string          `json:"method"`
		SessionID string          `json:"sessionId"`
		Params    json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(env.Payload, &msg); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if msg.Method != "Page.frameAttached" {
		t.Fatalf("unexpected method %q", msg.Method)
	}
	if msg.SessionID != "session-1" {
		t.Fatalf("unexpected session id %q", msg.SessionID)
	}
	if string(msg.Params) != string(params) {
		t.Fatalf("unexpected params %s", string(msg.Params))
	}
}
