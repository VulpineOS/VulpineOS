package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
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

func TestNewServerConfiguresHTTPTimeouts(t *testing.T) {
	server := NewServer(":0", "secret", nil)

	if server.server.ReadHeaderTimeout != 5*time.Second {
		t.Fatalf("ReadHeaderTimeout = %s, want 5s", server.server.ReadHeaderTimeout)
	}
	if server.server.ReadTimeout != 15*time.Second {
		t.Fatalf("ReadTimeout = %s, want 15s", server.server.ReadTimeout)
	}
	if server.server.WriteTimeout != 30*time.Second {
		t.Fatalf("WriteTimeout = %s, want 30s", server.server.WriteTimeout)
	}
	if server.server.IdleTimeout != 60*time.Second {
		t.Fatalf("IdleTimeout = %s, want 60s", server.server.IdleTimeout)
	}
}

func TestControlEndpointsSetSecurityHeaders(t *testing.T) {
	server := NewServer(":0", "secret", nil)

	for _, tc := range []struct {
		name   string
		path   string
		status int
	}{
		{name: "health", path: "/health", status: http.StatusOK},
		{name: "auth ok", path: "/auth/check?token=secret", status: http.StatusOK},
		{name: "auth rejected", path: "/auth/check", status: http.StatusUnauthorized},
		{name: "websocket auth rejected", path: "/ws", status: http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			server.Mux().ServeHTTP(resp, req)

			if resp.Code != tc.status {
				t.Fatalf("status = %d, want %d", resp.Code, tc.status)
			}
			for name, want := range map[string]string{
				"Cache-Control":          "no-store",
				"Referrer-Policy":        "no-referrer",
				"X-Content-Type-Options": "nosniff",
			} {
				if got := resp.Header().Get(name); got != want {
					t.Fatalf("%s = %q, want %q", name, got, want)
				}
			}
		})
	}
}

func TestBroadcastEventDoesNotHoldClientRegistryWhileWriting(t *testing.T) {
	server := NewServer(":0", "secret", nil)
	blocked := &wsClient{ctx: context.Background()}
	blocked.writeMu.Lock()
	server.clients[blocked] = struct{}{}

	done := make(chan struct{})
	go func() {
		server.BroadcastEvent("Page.frameAttached", "session-1", json.RawMessage(`{}`))
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		locked := make(chan struct{})
		go func() {
			server.clientsMu.Lock()
			server.clientsMu.Unlock()
			close(locked)
		}()
		select {
		case <-locked:
			blocked.writeMu.Unlock()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("BroadcastEvent did not return after blocked writer was released")
			}
			return
		case <-time.After(10 * time.Millisecond):
			if time.Now().After(deadline) {
				blocked.writeMu.Unlock()
				t.Fatal("client registry lock remained blocked during websocket write")
			}
		}
	}
}

func TestHandleWSJugglerWithoutKernelReturnsError(t *testing.T) {
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

	payload, err := json.Marshal(map[string]interface{}{
		"id":     1,
		"method": "Browser.enable",
		"params": map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	env, err := NewJugglerEnvelope(payload)
	if err != nil {
		t.Fatalf("NewJugglerEnvelope: %v", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, env); err != nil {
		t.Fatalf("write websocket: %v", err)
	}

	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read websocket: %v", err)
	}

	var out Envelope
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if out.Type != "juggler" {
		t.Fatalf("unexpected envelope type %q", out.Type)
	}

	var msg struct {
		ID    int `json:"id"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(out.Payload, &msg); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if msg.ID != 1 {
		t.Fatalf("id = %d, want 1", msg.ID)
	}
	if msg.Error == nil || msg.Error.Message != "browser unavailable: server started without a kernel" {
		t.Fatalf("error = %#v, want browser unavailable", msg.Error)
	}
}

func TestHandleWSAcceptsAccessSubprotocol(t *testing.T) {
	server := NewServer(":0", "secret", nil)
	httpServer := httptest.NewServer(server.Mux())
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	protocol := PanelAccessSubprotocol("secret")
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		Subprotocols: []string{protocol},
	})
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	if got := conn.Subprotocol(); got != protocol {
		t.Fatalf("subprotocol = %q, want %q", got, protocol)
	}
}

func TestHandleWSRejectsOversizedMessage(t *testing.T) {
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

	oversized := bytes.Repeat([]byte("x"), int(maxWebSocketMessageBytes)+1)
	if err := conn.Write(ctx, websocket.MessageText, oversized); err != nil {
		t.Fatalf("write oversized websocket payload: %v", err)
	}
	_, _, err = conn.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusMessageTooBig {
		t.Fatalf("close status = %v, err = %v; want StatusMessageTooBig", websocket.CloseStatus(err), err)
	}
}
