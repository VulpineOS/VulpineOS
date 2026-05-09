package remote

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"nhooyr.io/websocket"

	"vulpineos/internal/juggler"
)

func TestClientConcurrentSendSerializesWebsocketWrites(t *testing.T) {
	const messageCount = 25

	received := make(chan []int, 1)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		ids := make([]int, 0, messageCount)
		for len(ids) < messageCount {
			_, data, err := conn.Read(ctx)
			if err != nil {
				t.Errorf("read websocket: %v", err)
				return
			}

			var env Envelope
			if err := json.Unmarshal(data, &env); err != nil {
				t.Errorf("unmarshal envelope: %v", err)
				return
			}
			if env.Type != "juggler" {
				t.Errorf("unexpected envelope type %q", env.Type)
				return
			}

			var msg juggler.Message
			if err := json.Unmarshal(env.Payload, &msg); err != nil {
				t.Errorf("unmarshal payload: %v", err)
				return
			}
			ids = append(ids, msg.ID)
		}
		received <- ids
	}))
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	client, err := Dial(ctx, wsURL, "")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	var wg sync.WaitGroup
	errs := make(chan error, messageCount)
	for i := 1; i <= messageCount; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- client.Send(&juggler.Message{ID: i, Method: "Browser.enable"})
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("send: %v", err)
		}
	}

	select {
	case ids := <-received:
		if len(ids) != messageCount {
			t.Fatalf("received %d messages, want %d", len(ids), messageCount)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for server to receive concurrent sends")
	}
}

func TestClientControlCallUsesControlEnvelopeAndReturnsResult(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Errorf("read websocket: %v", err)
			return
		}
		var env Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			t.Errorf("unmarshal envelope: %v", err)
			return
		}
		if env.Type != "control" {
			t.Errorf("envelope type = %q, want control", env.Type)
			return
		}
		var payload struct {
			Command string          `json:"command"`
			Params  json.RawMessage `json:"params"`
			ID      int             `json:"id"`
		}
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			t.Errorf("unmarshal payload: %v", err)
			return
		}
		if payload.Command != "status.get" || payload.ID == 0 {
			t.Errorf("payload = %+v, want status.get with id", payload)
			return
		}

		response := map[string]any{
			"type": "control",
			"payload": map[string]any{
				"params": map[string]any{
					"id":     payload.ID,
					"result": map[string]any{"status": "ok"},
				},
			},
		}
		if err := conn.Write(ctx, websocket.MessageText, mustJSON(t, response)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	client, err := Dial(ctx, wsURL, "")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	var result struct {
		Status string `json:"status"`
	}
	if err := client.ControlCall(ctx, "status.get", map[string]string{"scope": "tui"}, &result); err != nil {
		t.Fatalf("ControlCall: %v", err)
	}
	if result.Status != "ok" {
		t.Fatalf("status = %q, want ok", result.Status)
	}
}

func TestClientEnqueueMessageReturnsAfterContextCancelWithFullBuffer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &Client{
		ctx:    ctx,
		cancel: cancel,
		recvCh: make(chan *juggler.Message, 1),
	}
	client.recvCh <- &juggler.Message{ID: 1}

	done := make(chan bool, 1)
	go func() {
		done <- client.enqueueReceivedMessage(&juggler.Message{ID: 2})
	}()

	cancel()

	select {
	case ok := <-done:
		if ok {
			t.Fatal("enqueue succeeded after context cancel with full buffer")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("enqueue stayed blocked after context cancel")
	}
}

func TestClientControlCallReturnsPromptlyWhenReadLoopEnds(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if _, _, err := conn.Read(ctx); err != nil {
			t.Errorf("read websocket: %v", err)
		}
		_ = conn.Close(websocket.StatusNormalClosure, "closed by test")
	}))
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	client, err := Dial(ctx, wsURL, "")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	start := time.Now()
	err = client.ControlCall(ctx, "status.get", map[string]string{"scope": "tui"}, nil)
	if err == nil {
		t.Fatal("ControlCall returned nil error after websocket close")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("ControlCall took %s after websocket close, want prompt failure", elapsed)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return data
}
